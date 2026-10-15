package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
	"time"
)

type MediaAsset struct {
	ID                string `json:"asset_id"`
	WorkspaceID       string `json:"-"`
	JobID             string `json:"job_id,omitempty"`
	Origin            string `json:"origin"`
	Key               string `json:"-"`
	Status            string `json:"status"`
	Filename          string `json:"filename"`
	ContentType       string `json:"content_type"`
	Bytes             int64  `json:"bytes"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	DurationMS        int64  `json:"duration_ms"`
	Measured          bool   `json:"measured"`
	HasAudio          bool   `json:"has_audio"`
	Error             string `json:"error,omitempty"`
	PosterAvailable   bool   `json:"poster_available"`
	CaptionsAvailable bool   `json:"captions_available"`
	JobStatus         string `json:"job_status,omitempty"`
}

const assetCols = `id,workspace_id,coalesce(source_job_id,''),origin,object_key,status,filename,content_type,bytes,width,height,duration_ms,measured,has_audio,error,poster_key<>'',coalesce((SELECT jsonb_array_length(segments)>0 FROM cloud_job_captions WHERE job_id=source_job_id),false),coalesce((SELECT status FROM cloud_jobs WHERE id=source_job_id),'')`

func scanAsset(row interface{ Scan(...any) error }) (MediaAsset, error) {
	var a MediaAsset
	e := row.Scan(&a.ID, &a.WorkspaceID, &a.JobID, &a.Origin, &a.Key, &a.Status, &a.Filename, &a.ContentType, &a.Bytes, &a.Width, &a.Height, &a.DurationMS, &a.Measured, &a.HasAudio, &a.Error, &a.PosterAvailable, &a.CaptionsAvailable, &a.JobStatus)
	if e == sql.ErrNoRows {
		e = ErrNotFound
	}
	return a, e
}
func (s *Store) ownedAsset(ctx context.Context, u User, id string) (MediaAsset, error) {
	return scanAsset(s.DB.QueryRowContext(ctx, `SELECT `+assetCols+` FROM cloud_media_assets WHERE id=$1 AND workspace_id=$2`, id, u.WorkspaceID))
}
func (s *Store) generatedAsset(ctx context.Context, u User, job string) (MediaAsset, error) {
	j, e := s.Job(ctx, u, job)
	if e != nil {
		return MediaAsset{}, e
	}
	if j.Status != "succeeded" || !validMediaKey(j.OutputKey) {
		return MediaAsset{}, ErrConflict
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return MediaAsset{}, e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_media_assets(id,workspace_id,source_job_id,origin,object_key,status)VALUES($1,$2,$3,'generated',$4,'ready') ON CONFLICT(source_job_id)DO NOTHING`, uuid.NewString(), u.WorkspaceID, job, j.OutputKey)
	if e != nil {
		return MediaAsset{}, e
	}
	a, e := scanAsset(tx.QueryRowContext(ctx, `SELECT `+assetCols+` FROM cloud_media_assets WHERE source_job_id=$1 AND workspace_id=$2`, job, u.WorkspaceID))
	if e != nil {
		return a, e
	}
	if !a.Measured {
		_, e = tx.ExecContext(ctx, `INSERT INTO cloud_outbox(id,kind,aggregate_id)VALUES($1,'media_probe',$2)ON CONFLICT DO NOTHING`, uuid.NewString(), a.ID)
		if e != nil {
			return a, e
		}
	}
	// Legacy generated videos acquire an owned source lazily without changing URLs.
	_, e = tx.ExecContext(ctx, `UPDATE cloud_videos SET source_asset_id=$2 WHERE job_id=$1 AND source_asset_id IS NULL`, job, a.ID)
	if e == nil {
		e = tx.Commit()
	}
	return a, e
}
func (s *Server) outputDetail(w http.ResponseWriter, r *http.Request) {
	a, e := s.Store.generatedAsset(r.Context(), currentUser(r), chi.URLParam(r, "id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	b, _ := json.Marshal(a)
	var out map[string]any
	json.Unmarshal(b, &out)
	var publication string
	s.Store.DB.QueryRowContext(r.Context(), `SELECT id FROM cloud_videos WHERE source_asset_id=$1 AND channel_id=$2`, a.ID, currentUser(r).ChannelID).Scan(&publication)
	out["publication_id"] = publication
	s.json(w, 200, out)
}
func (s *Server) assetDownload(w http.ResponseWriter, r *http.Request) {
	a, e := s.Store.ownedAsset(r.Context(), currentUser(r), chi.URLParam(r, "id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	if a.Status != "ready" || s.Media == nil {
		s.fail(w, ErrUnavailable)
		return
	}
	if !s.rate(w, r, "download:"+currentUser(r).ID, 30, time.Minute, true) {
		return
	}
	disposition := `attachment; filename="video` + safeVideoExtension(a.ContentType) + `"`
	if chi.URLParam(r, "action") == "preview" {
		disposition = "inline"
	}
	req, e := s.Media.presign.PresignGetObject(r.Context(), &s3.GetObjectInput{Bucket: aws.String(s.Config.OriginalsBucket), Key: aws.String(a.Key), ResponseContentDisposition: aws.String(disposition)}, s3.WithPresignExpires(5*time.Minute))
	if e != nil {
		s.fail(w, e)
		return
	}
	s.json(w, 200, map[string]any{"url": req.URL, "expires_in": 300, "filename": a.Filename})
}
func safeVideoExtension(contentType string) string {
	if contentType == "video/webm" {
		return ".webm"
	}
	return ".mp4"
}
func (s *Server) createPublication(w http.ResponseWriter, r *http.Request) {
	var v Video
	if !s.decode(w, r, &v) {
		return
	}
	out, e := s.Store.CreateVideo(r.Context(), currentUser(r), v)
	s.result(w, out, e)
}
