package cloud

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/cloudfront/sign"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"path"
	"strings"
	"time"
)

type Media struct {
	cfg     Config
	s3      *s3.Client
	presign *s3.PresignClient
	signer  *sign.CookieSigner
}

func NewMedia(ctx context.Context, cfg Config) (*Media, error) {
	ac, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(ac, func(o *s3.Options) { o.UsePathStyle = cfg.DevMode })
	m := &Media{cfg: cfg, s3: client, presign: s3.NewPresignClient(client)}
	if cfg.DevMode && cfg.S3PublicEndpoint != "" {
		publicClient := s3.NewFromConfig(ac, func(o *s3.Options) { o.UsePathStyle = true; o.BaseEndpoint = aws.String(cfg.S3PublicEndpoint) })
		m.presign = s3.NewPresignClient(publicClient)
	}
	if cfg.CloudFrontPrivateKey != "" {
		key, err := sign.LoadPEMPrivKey(strings.NewReader(cfg.CloudFrontPrivateKey))
		if err != nil {
			return nil, fmt.Errorf("CloudFront signing key: %w", err)
		}
		m.signer = sign.NewCookieSigner(cfg.CloudFrontKeyPairID, key)
	}
	return m, nil
}
func validMediaKey(key string) bool {
	return key != "" && !strings.HasPrefix(key, "/") && !strings.ContainsAny(key, "\\:\x00") && path.Clean(key) == key && key != ".." && !strings.HasPrefix(key, "../")
}

// UploadGrant rejects legacy unbounded PUT grants.
func (m *Media) UploadGrant(ctx context.Context, workspace, filename, contentType string) (map[string]any, error) {
	return nil, ErrValidation
}
func (m *Media) UploadPostGrant(ctx context.Context, workspace, filename, contentType string, size int64) (map[string]any, error) {
	if size < 1 || size > MaxUploadBytes || workspace == "" || !validMediaKey(workspace) || strings.Contains(workspace, "/") || path.Base(filename) != filename || !validMediaKey(filename) {
		return nil, ErrValidation
	}
	if m.cfg.UploadsBucket == "" {
		return nil, errors.New("quarantined uploads bucket is not configured")
	}
	switch contentType {
	case "image/jpeg", "image/png", "image/webp", "video/mp4", "audio/mpeg", "audio/wav":
	default:
		return nil, ErrValidation
	}
	key := "workspaces/" + workspace + "/uploads/" + uuid.NewString() + "/" + filename
	request, err := m.presign.PresignPostObject(ctx, &s3.PutObjectInput{Bucket: aws.String(m.cfg.UploadsBucket), Key: aws.String(key)}, func(o *s3.PresignPostOptions) {
		o.Expires = 10 * time.Minute
		o.Conditions = []any{[]any{"content-length-range", 1, size}, map[string]string{"Content-Type": contentType}}
	})
	if err != nil {
		return nil, err
	}
	request.Values["Content-Type"] = contentType
	return map[string]any{"url": request.URL, "fields": request.Values, "key": key, "method": "POST", "expires_in": 600, "max_bytes": size}, nil
}
func (m *Media) DownloadGrant(ctx context.Context, key string) (string, error) {
	if !validMediaKey(key) {
		return "", ErrValidation
	}
	if m.cfg.OriginalsBucket == "" {
		return "", errors.New("originals bucket is not configured")
	}
	req, err := m.presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(m.cfg.OriginalsBucket), Key: aws.String(key)}, s3.WithPresignExpires(5*time.Minute))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
func (m *Media) PlaybackGrant(ctx context.Context, manifest string) (map[string]any, error) {
	if !validMediaKey(manifest) || !strings.HasSuffix(manifest, ".m3u8") {
		return nil, ErrValidation
	}
	if m.signer == nil || m.cfg.CloudFrontDomain == "" || m.cfg.CloudFrontKeyPairID == "" {
		return nil, errors.New("private playback signing is not configured")
	}
	base := "https://" + strings.TrimSuffix(strings.TrimPrefix(m.cfg.CloudFrontDomain, "https://"), "/") + "/"
	expiry := time.Now().Add(5 * time.Minute)
	cookies, err := m.signer.Sign(base+path.Dir(manifest)+"/*", expiry)
	if err != nil {
		return nil, err
	}
	for _, cookie := range cookies {
		cookie.Secure = true
		cookie.HttpOnly = true
		cookie.Path = "/" + path.Dir(manifest) + "/"
	}
	return map[string]any{"url": base + manifest, "cookies": cookies, "expires_at": expiry}, nil
}
