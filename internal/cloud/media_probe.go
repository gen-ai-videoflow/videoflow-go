package cloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type mediaMetadata struct {
	Width, Height int
	DurationMS    int64
	HasAudio      bool
}
type cappedProbeOutput struct{ bytes.Buffer }

func (b *cappedProbeOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 128<<10 {
		return 0, ErrValidation
	}
	return b.Buffer.Write(p)
}
func probeMedia(ctx context.Context, binary, file string) (mediaMetadata, error) {
	var result mediaMetadata
	if binary == "" {
		binary = "ffprobe"
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-v", "error", "-max_alloc", "67108864", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm", "-show_entries", "format=duration,format_name:stream=codec_type,codec_name,width,height:stream_tags=rotate:stream_side_data=rotation", "-of", "json", file)
	cmd.Dir = filepath.Dir(file)
	out := &cappedProbeOutput{}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if e := cmd.Run(); e != nil {
		return result, ErrValidation
	}
	var data struct {
		Format struct {
			Duration string `json:"duration"`
			Name     string `json:"format_name"`
		} `json:"format"`
		Streams []struct {
			Type   string `json:"codec_type"`
			Codec  string `json:"codec_name"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
			Tags   struct {
				Rotate string `json:"rotate"`
			} `json:"tags"`
			Side []struct {
				Rotation int `json:"rotation"`
			} `json:"side_data_list"`
		} `json:"streams"`
	}
	if json.Unmarshal(out.Bytes(), &data) != nil || len(data.Streams) > 16 {
		return result, ErrValidation
	}
	duration, e := strconv.ParseFloat(data.Format.Duration, 64)
	if e != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 || duration > 600 {
		return result, ErrValidation
	}
	result.DurationMS = int64(math.Round(duration * 1000))
	for _, stream := range data.Streams {
		if stream.Type == "audio" {
			if stream.Codec != "aac" && stream.Codec != "opus" && stream.Codec != "vorbis" && stream.Codec != "mp3" {
				return result, ErrValidation
			}
			result.HasAudio = true
		}
		if stream.Type == "video" && result.Width == 0 {
			if stream.Codec != "h264" && stream.Codec != "hevc" && stream.Codec != "vp8" && stream.Codec != "vp9" && stream.Codec != "av1" {
				return result, ErrValidation
			}
			result.Width, result.Height = stream.Width, stream.Height
			rotation, _ := strconv.Atoi(stream.Tags.Rotate)
			for _, side := range stream.Side {
				rotation = side.Rotation
			}
			if rotation%90 != 0 {
				return result, ErrValidation
			}
			if rotation%180 != 0 {
				result.Width, result.Height = result.Height, result.Width
			}
		}
	}
	if result.Width < 16 || result.Height < 16 || result.Width > 4096 || result.Height > 4096 || int64(result.Width)*int64(result.Height) > 16777216 {
		return result, ErrValidation
	}
	return result, nil
}
func (w *Worker) inspectStoredMedia(ctx context.Context, bucket, key, checksum string, expected int64, assetID string) (mediaMetadata, string, error) {
	var m mediaMetadata
	if w.media == nil {
		return m, "", ErrUnavailable
	}
	source, e := w.media.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if e != nil {
		return m, "", e
	}
	defer source.Body.Close()
	dir, e := os.MkdirTemp("", "videoflow-media-probe-")
	if e != nil {
		return m, "", e
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "source"+filepath.Ext(key))
	f, e := os.Create(file)
	if e != nil {
		return m, "", e
	}
	hash := sha256.New()
	n, e := io.Copy(io.MultiWriter(f, hash), io.LimitReader(source.Body, MaxUploadBytes+1))
	closeErr := f.Close()
	if e != nil || closeErr != nil {
		return m, "", errors.Join(e, closeErr)
	}
	if n > MaxUploadBytes || n == 0 || (expected > 0 && n != expected) || (checksum != "" && hex.EncodeToString(hash.Sum(nil)) != checksum) {
		return m, "", ErrValidation
	}
	m, e = probeMedia(ctx, w.cfg.MediaProbePath, file)
	if e != nil {
		return m, "", e
	}
	poster := ""
	if w.cfg.PlaybackBucket != "" {
		jpg := filepath.Join(dir, "poster.jpg")
		probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		e = runFFmpeg(probeCtx, w.cfg.FFmpegPath, dir, []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm", "-i", file, "-frames:v", "1", "-vf", "scale=640:-2:force_original_aspect_ratio=decrease", "-threads", "1", jpg})
		if e != nil {
			return m, "", ErrValidation
		}
		image, e := os.Open(jpg)
		if e != nil {
			return m, "", e
		}
		defer image.Close()
		poster = "posters/assets/" + assetID + ".jpg"
		if _, e = w.media.s3.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(w.cfg.PlaybackBucket), Key: aws.String(poster), Body: image, ContentType: aws.String("image/jpeg")}); e != nil {
			return m, "", e
		}
	}
	return m, poster, nil
}
func (w *Worker) validateVideoUpload(ctx context.Context, id string) error {
	tx, e := w.store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	u, e := scanUpload(tx.QueryRowContext(ctx, `SELECT `+uploadCols+` FROM cloud_video_uploads WHERE id=$1 FOR UPDATE`, id))
	if e != nil {
		return e
	}
	if u.State != "validating" {
		return nil
	}
	var original string
	e = tx.QueryRowContext(ctx, `SELECT object_key FROM cloud_media_assets WHERE id=$1`, u.AssetID).Scan(&original)
	if e != nil {
		return e
	}
	sourceBucket, sourceKey := w.cfg.UploadsBucket, u.Key
	if _, headErr := w.media.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(sourceBucket), Key: aws.String(sourceKey)}); objectAbsent(headErr) {
		sourceBucket, sourceKey = w.cfg.OriginalsBucket, original
	} else if headErr != nil {
		return headErr
	}
	m, poster, e := w.inspectStoredMedia(ctx, sourceBucket, sourceKey, u.Checksum, u.Bytes, u.AssetID)
	if e != nil {
		if !errors.Is(e, ErrValidation) {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE cloud_media_assets SET status='rejected',error='The file could not be validated. Use a supported video under 10 minutes.' WHERE id=$1`, u.AssetID)
		if e == nil {
			_, e = tx.ExecContext(ctx, `UPDATE cloud_video_uploads SET state='rejected',error='The video failed file, checksum or format validation.' WHERE id=$1`, u.ID)
		}
		if e == nil {
			e = tx.Commit()
		}
		return e
	}
	var key string
	e = tx.QueryRowContext(ctx, `SELECT object_key FROM cloud_media_assets WHERE id=$1`, u.AssetID).Scan(&key)
	if e != nil {
		return e
	}
	if sourceBucket == w.cfg.UploadsBucket {
		_, e = w.media.s3.CopyObject(ctx, &s3.CopyObjectInput{Bucket: aws.String(w.cfg.OriginalsBucket), Key: aws.String(key), CopySource: aws.String(w.cfg.UploadsBucket + "/" + u.Key)})
		if e != nil {
			return e
		}
	}
	// The quarantined copy is removed before readiness; on transient errors retry validation safely.
	if _, e = w.media.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(w.cfg.UploadsBucket), Key: aws.String(u.Key)}); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE cloud_media_assets SET status='ready',width=$2,height=$3,duration_ms=$4,has_audio=$5,measured=true,poster_key=$6,error='' WHERE id=$1`, u.AssetID, m.Width, m.Height, m.DurationMS, m.HasAudio, poster)
	if e == nil {
		_, e = tx.ExecContext(ctx, `UPDATE cloud_video_uploads SET state='ready',error='' WHERE id=$1`, id)
	}
	if e == nil {
		e = tx.Commit()
	}
	return e
}
func (w *Worker) probeGenerated(ctx context.Context, id string) error {
	var workspace string
	e := w.store.DB.QueryRowContext(ctx, `SELECT workspace_id FROM cloud_media_assets WHERE id=$1 AND origin='generated' AND NOT measured`, id).Scan(&workspace)
	if e != nil {
		return nil
	}
	a, e := w.store.ownedAsset(ctx, User{WorkspaceID: workspace}, id)
	if e != nil {
		return e
	}
	m, poster, e := w.inspectStoredMedia(ctx, w.cfg.OriginalsBucket, a.Key, "", 0, id)
	if e != nil {
		return e
	}
	_, e = w.store.DB.ExecContext(ctx, `UPDATE cloud_media_assets SET width=$2,height=$3,duration_ms=$4,has_audio=$5,measured=true,poster_key=$6 WHERE id=$1`, id, m.Width, m.Height, m.DurationMS, m.HasAudio, poster)
	if e == nil && poster != "" {
		_, e = w.store.DB.ExecContext(ctx, `UPDATE cloud_videos SET thumbnail_key=$2 WHERE source_asset_id=$1`, id, poster)
	}
	return e
}
func (s *Server) videoPoster(w http.ResponseWriter, r *http.Request) {
	v, e := s.Store.Video(r.Context(), chi.URLParam(r, "id"), s.optionalUser(r).ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	if v.ThumbnailKey == "" || !strings.HasPrefix(v.ThumbnailKey, "posters/") || s.Media == nil {
		s.fail(w, ErrNotFound)
		return
	}
	signed, e := s.Media.presign.PresignGetObject(r.Context(), &s3.GetObjectInput{Bucket: aws.String(s.Config.PlaybackBucket), Key: aws.String(v.ThumbnailKey)}, s3.WithPresignExpires(5*time.Minute))
	if e != nil {
		s.fail(w, e)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=60")
	http.Redirect(w, r, signed.URL, http.StatusTemporaryRedirect)
}
