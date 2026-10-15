package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	st "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"math"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
)

const videoPartBytes int64 = 5 << 20

type VideoUpload struct {
	ID          string    `json:"id"`
	AssetID     string    `json:"asset_id"`
	WorkspaceID string    `json:"-"`
	Key         string    `json:"-"`
	StorageID   string    `json:"-"`
	Bytes       int64     `json:"bytes"`
	Checksum    string    `json:"checksum"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	State       string    `json:"state"`
	Error       string    `json:"error,omitempty"`
	Expires     time.Time `json:"expires_at"`
}

const uploadCols = `id,asset_id,workspace_id,object_key,storage_upload_id,bytes,checksum,filename,content_type,state,error,expires_at`

func scanUpload(row interface{ Scan(...any) error }) (VideoUpload, error) {
	var u VideoUpload
	e := row.Scan(&u.ID, &u.AssetID, &u.WorkspaceID, &u.Key, &u.StorageID, &u.Bytes, &u.Checksum, &u.Filename, &u.ContentType, &u.State, &u.Error, &u.Expires)
	if e == sql.ErrNoRows {
		e = ErrNotFound
	}
	return u, e
}
func (s *Store) ownedUpload(ctx context.Context, u User, id string) (VideoUpload, error) {
	return scanUpload(s.DB.QueryRowContext(ctx, `SELECT `+uploadCols+` FROM cloud_video_uploads WHERE id=$1 AND workspace_id=$2`, id, u.WorkspaceID))
}
func (s *Server) uploadOptions(w http.ResponseWriter, r *http.Request) {
	s.json(w, 200, map[string]any{"enabled": s.Config.VideoUploadsEnabled && s.Media != nil && s.Config.UploadsBucket != "" && s.Config.OriginalsBucket != "", "max_bytes": MaxUploadBytes, "workspace_quota_bytes": WorkspaceUploadQuotaBytes, "part_bytes": videoPartBytes, "max_duration_seconds": 600})
}
func (s *Server) beginVideoUpload(w http.ResponseWriter, r *http.Request) {
	if !s.Config.VideoUploadsEnabled || s.Media == nil || s.Config.UploadsBucket == "" || s.Config.OriginalsBucket == "" {
		s.fail(w, ErrUnavailable)
		return
	}
	u := currentUser(r)
	if !u.Verified {
		s.fail(w, ErrUnauthorized)
		return
	}
	if !s.rate(w, r, "video-upload:"+u.ID, 10, time.Hour, true) {
		return
	}
	var in struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		Bytes       int64  `json:"bytes"`
		Checksum    string `json:"checksum"`
		Rights      bool   `json:"rights_confirmed"`
		Key         string `json:"idempotency_key"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	if !in.Rights || len(in.Filename) > 180 || in.Filename == "" || path.Base(in.Filename) != in.Filename || strings.ContainsAny(in.Filename, "\\\r\n\x00") || (in.ContentType != "video/mp4" && in.ContentType != "video/webm") || in.Bytes < 1 || in.Bytes > MaxUploadBytes || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(in.Checksum) || len(in.Key) < 8 || len(in.Key) > 128 {
		s.fail(w, ErrValidation)
		return
	}
	b, _ := json.Marshal(in)
	fingerprint := tokenHash(string(b))
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	var locked string
	e = tx.QueryRowContext(r.Context(), `SELECT workspace_id FROM cloud_credit_accounts WHERE workspace_id=$1 FOR UPDATE`, u.WorkspaceID).Scan(&locked)
	if e != nil {
		s.fail(w, e)
		return
	}
	var existingID, old string
	e = tx.QueryRowContext(r.Context(), `SELECT id,fingerprint FROM cloud_video_uploads WHERE workspace_id=$1 AND idempotency_key=$2`, u.WorkspaceID, in.Key).Scan(&existingID, &old)
	if e == nil {
		if old != fingerprint {
			s.fail(w, ErrConflict)
			return
		}
		tx.Rollback()
		v, e := s.Store.ownedUpload(r.Context(), u, existingID)
		s.result(w, v, e)
		return
	}
	if e != sql.ErrNoRows {
		s.fail(w, e)
		return
	}
	var used int64
	var active int
	e = workspaceUploadBytes(r.Context(), tx, u.WorkspaceID, &used)
	if e == nil {
		e = tx.QueryRowContext(r.Context(), `SELECT count(*) FROM cloud_video_uploads WHERE workspace_id=$1 AND state IN ('creating','uploading','completing','validating','aborting')`, u.WorkspaceID).Scan(&active)
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	if used > WorkspaceUploadQuotaBytes-in.Bytes || active > 0 {
		s.fail(w, ErrConflict)
		return
	}
	id, asset := uuid.NewString(), uuid.NewString()
	key := "workspaces/" + u.WorkspaceID + "/video-uploads/" + id + "/source" + safeVideoExtension(in.ContentType)
	original := "workspaces/" + u.WorkspaceID + "/videos/" + id + "/source" + safeVideoExtension(in.ContentType)
	_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_media_assets(id,workspace_id,origin,object_key,status,filename,content_type,bytes,checksum)VALUES($1,$2,'upload',$3,'validating',$4,$5,$6,$7)`, asset, u.WorkspaceID, original, in.Filename, in.ContentType, in.Bytes, in.Checksum)
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_video_uploads(id,asset_id,workspace_id,object_key,bytes,checksum,filename,content_type,state,idempotency_key,fingerprint)VALUES($1,$2,$3,$4,$5,$6,$7,$8,'creating',$9,$10)`, id, asset, u.WorkspaceID, key, in.Bytes, in.Checksum, in.Filename, in.ContentType, in.Key, fingerprint)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	// Serialize the remote creation and its durable identifier against cancellation.
	createTx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer createTx.Rollback()
	var state string
	e = createTx.QueryRowContext(r.Context(), `SELECT state FROM cloud_video_uploads WHERE id=$1 FOR UPDATE`, id).Scan(&state)
	if e != nil {
		s.fail(w, e)
		return
	}
	if state != "creating" {
		s.fail(w, ErrConflict)
		return
	}
	out, e := s.Media.s3.CreateMultipartUpload(r.Context(), &s3.CreateMultipartUploadInput{Bucket: aws.String(s.Config.UploadsBucket), Key: aws.String(key), ContentType: aws.String(in.ContentType)})
	if e != nil || out.UploadId == nil {
		s.fail(w, ErrUnavailable)
		return
	}
	_, e = createTx.ExecContext(r.Context(), `UPDATE cloud_video_uploads SET storage_upload_id=$2,state='uploading' WHERE id=$1`, id, *out.UploadId)
	if e == nil {
		e = createTx.Commit()
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	v, e := s.Store.ownedUpload(r.Context(), u, id)
	s.result(w, v, e)
}
func (s *Server) videoUploadStatus(w http.ResponseWriter, r *http.Request) {
	v, e := s.Store.ownedUpload(r.Context(), currentUser(r), chi.URLParam(r, "id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	parts := []int32{}
	if v.State == "uploading" && s.Media != nil {
		out, err := s.Media.s3.ListParts(r.Context(), &s3.ListPartsInput{Bucket: aws.String(s.Config.UploadsBucket), Key: aws.String(v.Key), UploadId: aws.String(v.StorageID), MaxParts: aws.Int32(21)})
		if err != nil {
			s.fail(w, ErrUnavailable)
			return
		}
		for _, part := range out.Parts {
			number := aws.ToInt32(part.PartNumber)
			if number > 0 && number <= 20 && aws.ToInt64(part.Size) == min(videoPartBytes, v.Bytes-int64(number-1)*videoPartBytes) {
				parts = append(parts, number)
			}
		}
	}
	b, _ := json.Marshal(v)
	var result map[string]any
	json.Unmarshal(b, &result)
	result["uploaded_parts"] = parts
	result["part_bytes"] = videoPartBytes
	s.json(w, 200, result)
}
func (s *Server) uploadPart(w http.ResponseWriter, r *http.Request) {
	v, e := s.Store.ownedUpload(r.Context(), currentUser(r), chi.URLParam(r, "id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	if s.Media == nil || !s.Config.VideoUploadsEnabled {
		s.fail(w, ErrUnavailable)
		return
	}
	if v.State != "uploading" || time.Now().After(v.Expires) {
		s.fail(w, ErrConflict)
		return
	}
	var in struct {
		Part int32 `json:"part_number"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	count := int32(math.Ceil(float64(v.Bytes) / float64(videoPartBytes)))
	if in.Part < 1 || in.Part > count {
		s.fail(w, ErrValidation)
		return
	}
	if !s.rate(w, r, "video-part:"+currentUser(r).ID, 100, time.Minute, true) {
		return
	}
	size := min(videoPartBytes, v.Bytes-int64(in.Part-1)*videoPartBytes)
	signed, e := s.Media.presign.PresignUploadPart(r.Context(), &s3.UploadPartInput{Bucket: aws.String(s.Config.UploadsBucket), Key: aws.String(v.Key), UploadId: aws.String(v.StorageID), PartNumber: aws.Int32(in.Part), ContentLength: aws.Int64(size)}, s3.WithPresignExpires(10*time.Minute))
	if e != nil {
		s.fail(w, e)
		return
	}
	s.json(w, 200, map[string]any{"url": signed.URL, "part_number": in.Part, "bytes": size, "expires_in": 600})
}
func (s *Server) completeVideoUpload(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	v, e := s.Store.ownedUpload(r.Context(), u, chi.URLParam(r, "id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	if s.Media == nil || !s.Config.VideoUploadsEnabled {
		s.fail(w, ErrUnavailable)
		return
	}
	if v.State == "ready" || v.State == "validating" {
		s.json(w, 200, v)
		return
	}
	if (v.State != "uploading" && v.State != "completing") || time.Now().After(v.Expires) {
		s.fail(w, ErrConflict)
		return
	}
	// Serialize remote completion against abort; ambiguous S3 completion is reconciled by HEAD on retry.
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	v, e = scanUpload(tx.QueryRowContext(r.Context(), `SELECT `+uploadCols+` FROM cloud_video_uploads WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, v.ID, u.WorkspaceID))
	if e != nil {
		s.fail(w, e)
		return
	}
	if v.State != "uploading" && v.State != "completing" {
		s.result(w, v, nil)
		return
	}
	parts, e := s.Media.s3.ListParts(r.Context(), &s3.ListPartsInput{Bucket: aws.String(s.Config.UploadsBucket), Key: aws.String(v.Key), UploadId: aws.String(v.StorageID), MaxParts: aws.Int32(21)})
	if e == nil {
		var total int64
		completed := []st.CompletedPart{}
		for i, part := range parts.Parts {
			expected := min(videoPartBytes, v.Bytes-int64(i)*videoPartBytes)
			if aws.ToInt32(part.PartNumber) != int32(i+1) || aws.ToInt64(part.Size) != expected || expected <= 0 {
				s.fail(w, ErrValidation)
				return
			}
			total += aws.ToInt64(part.Size)
			completed = append(completed, st.CompletedPart{ETag: part.ETag, PartNumber: part.PartNumber})
		}
		if total != v.Bytes || aws.ToBool(parts.IsTruncated) {
			s.fail(w, ErrValidation)
			return
		}
		_, e = s.Media.s3.CompleteMultipartUpload(r.Context(), &s3.CompleteMultipartUploadInput{Bucket: aws.String(s.Config.UploadsBucket), Key: aws.String(v.Key), UploadId: aws.String(v.StorageID), MultipartUpload: &st.CompletedMultipartUpload{Parts: completed}})
	}
	head, headErr := s.Media.s3.HeadObject(r.Context(), &s3.HeadObjectInput{Bucket: aws.String(s.Config.UploadsBucket), Key: aws.String(v.Key)})
	if headErr != nil || aws.ToInt64(head.ContentLength) != v.Bytes {
		if e != nil {
			s.fail(w, ErrUnavailable)
		} else {
			s.fail(w, ErrValidation)
		}
		return
	}
	_, e = tx.ExecContext(r.Context(), `UPDATE cloud_video_uploads SET state='validating' WHERE id=$1`, v.ID)
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_outbox(id,kind,aggregate_id)VALUES($1,'media_validation',$2)`, uuid.NewString(), v.ID)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	v.State = "validating"
	s.json(w, 200, v)
}
func (s *Server) abortVideoUpload(w http.ResponseWriter, r *http.Request) {
	v, e := s.Store.ownedUpload(r.Context(), currentUser(r), chi.URLParam(r, "id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	result, e := s.Store.DB.ExecContext(r.Context(), `UPDATE cloud_video_uploads SET state='aborting' WHERE id=$1 AND state IN ('creating','uploading','completing','validating','rejected')`, v.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 && v.State != "aborted" && v.State != "aborting" {
		s.fail(w, ErrConflict)
		return
	}
	s.json(w, 200, map[string]string{"state": "aborting"})
}
