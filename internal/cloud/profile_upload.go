package cloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// normalizeProfileImage strips metadata and rejects decompression bombs before decoding.
func normalizeProfileImage(data []byte) ([]byte, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || cfg.Width < 32 || cfg.Height < 32 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 12000000 {
		return nil, ErrValidation
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrValidation
	}
	var out bytes.Buffer
	if err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 88}); err != nil {
		return nil, err
	}
	if out.Len() > 3<<20 {
		return nil, ErrValidation
	}
	return out.Bytes(), nil
}
func (s *Server) uploadProfileImage(w http.ResponseWriter, r *http.Request) {
	if s.Media == nil || s.Config.OriginalsBucket == "" {
		s.json(w, http.StatusServiceUnavailable, map[string]string{"message": "Profile image storage is not configured yet. You can still use an HTTPS image link."})
		return
	}
	var in struct {
		Kind string `json:"kind"`
		Data string `json:"data"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		s.fail(w, ErrValidation)
		return
	}
	if in.Kind != "avatar" && in.Kind != "banner" {
		s.fail(w, ErrValidation)
		return
	}
	data, err := base64.StdEncoding.DecodeString(in.Data)
	if err != nil || len(data) > 3<<20 {
		s.fail(w, ErrValidation)
		return
	}
	normalized, err := normalizeProfileImage(data)
	if err != nil {
		s.fail(w, err)
		return
	}
	u := currentUser(r)
	tx, err := s.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback()
	var channel string
	if err = tx.QueryRowContext(r.Context(), `SELECT id FROM cloud_channels WHERE id=$1 FOR UPDATE`, u.ChannelID).Scan(&channel); err != nil {
		s.fail(w, err)
		return
	}
	var count int
	if err = tx.QueryRowContext(r.Context(), `SELECT count(*) FROM cloud_profile_images WHERE channel_id=$1 AND created_at>now()-interval '1 hour'`, u.ChannelID).Scan(&count); err != nil {
		s.fail(w, err)
		return
	}
	if count >= 20 {
		s.json(w, http.StatusTooManyRequests, map[string]string{"message": "You have uploaded several profile images recently. Please try again in an hour."})
		return
	}
	id := uuid.NewString()
	key := "profile-images/" + u.ChannelID + "/" + id + ".jpg"
	_, err = tx.ExecContext(r.Context(), `INSERT INTO cloud_profile_images(id,channel_id,object_key,kind,bytes)VALUES($1,$2,$3,$4,$5)`, id, u.ChannelID, key, in.Kind, len(normalized))
	if err != nil {
		s.fail(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		s.fail(w, err)
		return
	}
	_, err = s.Media.s3.PutObject(r.Context(), &s3.PutObjectInput{Bucket: aws.String(s.Config.OriginalsBucket), Key: aws.String(key), Body: bytes.NewReader(normalized), ContentLength: aws.Int64(int64(len(normalized))), ContentType: aws.String("image/jpeg")})
	if err != nil {
		s.Store.DB.ExecContext(r.Context(), `DELETE FROM cloud_profile_images WHERE id=$1 AND channel_id=$2`, id, u.ChannelID)
		s.fail(w, err)
		return
	}
	s.json(w, 201, map[string]string{"url": strings.TrimSuffix(s.Config.PublicOrigin, "/") + "/api/v1/profile-images/" + id})
}
func (s *Server) profileImage(w http.ResponseWriter, r *http.Request) {
	if s.Media == nil || s.Config.OriginalsBucket == "" {
		http.NotFound(w, r)
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		http.NotFound(w, r)
		return
	}
	var key string
	// Only an image currently published on its owner's profile can be fetched.
	err := s.Store.DB.QueryRowContext(r.Context(), `SELECT p.object_key FROM cloud_profile_images p JOIN cloud_channels c ON c.id=p.channel_id WHERE p.id=$1 AND (c.avatar_url LIKE '%/profile-images/'||p.id OR c.banner_url LIKE '%/profile-images/'||p.id)`, id).Scan(&key)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	obj, err := s.Media.s3.GetObject(r.Context(), &s3.GetObjectInput{Bucket: aws.String(s.Config.OriginalsBucket), Key: aws.String(key)})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer obj.Body.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=300")
	io.Copy(w, io.LimitReader(obj.Body, 3<<20))
}

// Only application-generated local images may use HTTP in development. Other links stay HTTPS-only.
func (s *Server) validateProfilePatch(p profilePatch) bool {
	for _, field := range []**string{&p.Avatar, &p.Banner} {
		if *field == nil || validProfileURL(**field, true) {
			continue
		}
		if !s.Config.DevMode {
			return false
		}
		candidate, err := url.Parse(**field)
		origin, originErr := url.Parse(s.Config.PublicOrigin)
		if err != nil || originErr != nil || origin.Scheme != "http" || candidate.Scheme != origin.Scheme || candidate.Host != origin.Host || candidate.User != nil || candidate.RawQuery != "" || candidate.Fragment != "" {
			return false
		}
		id := strings.TrimPrefix(candidate.Path, "/api/v1/profile-images/")
		if candidate.Path != "/api/v1/profile-images/"+id {
			return false
		}
		if _, err = uuid.Parse(id); err != nil {
			return false
		}
		replacement := "https://validated-local-profile-image.invalid/image.jpg"
		*field = &replacement
	}
	return p.validate()
}

// CleanupProfileImages is an explicit maintenance hook; call from an operator/admin job.
// Each bounded pass deletes at most 100 unreferenced images older than 24 hours.
func (s *Server) CleanupProfileImages(ctx context.Context) (int, error) {
	if s.Media == nil || s.Config.OriginalsBucket == "" {
		return 0, ErrUnavailable
	}
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT p.id,p.object_key FROM cloud_profile_images p JOIN cloud_channels c ON c.id=p.channel_id WHERE p.created_at<now()-interval '24 hours' AND c.avatar_url NOT LIKE '%/profile-images/'||p.id AND c.banner_url NOT LIKE '%/profile-images/'||p.id ORDER BY p.created_at LIMIT 100`)
	if err != nil {
		return 0, err
	}
	type object struct{ id, key string }
	items := []object{}
	for rows.Next() {
		var v object
		if err = rows.Scan(&v.id, &v.key); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, v := range items {
		// Lock the profile while checking references so a concurrent profile save cannot republish an object being deleted.
		tx, err := s.Store.DB.BeginTx(ctx, nil)
		if err != nil {
			return count, err
		}
		var id string
		err = tx.QueryRowContext(ctx, `SELECT c.id FROM cloud_channels c JOIN cloud_profile_images p ON p.channel_id=c.id WHERE p.id=$1 AND c.avatar_url NOT LIKE '%/profile-images/'||p.id AND c.banner_url NOT LIKE '%/profile-images/'||p.id FOR UPDATE OF c`, v.id).Scan(&id)
		if err != nil {
			tx.Rollback()
			continue
		}
		if _, err = s.Media.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.Config.OriginalsBucket), Key: aws.String(v.key)}); err != nil {
			tx.Rollback()
			return count, err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM cloud_profile_images WHERE id=$1`, v.id); err != nil {
			tx.Rollback()
			return count, err
		}
		if err = tx.Commit(); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
