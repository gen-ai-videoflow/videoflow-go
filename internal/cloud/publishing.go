package cloud

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"strings"
	"time"
)

type Video struct {
	ID           string    `json:"id"`
	ChannelID    string    `json:"channel_id"`
	JobID        string    `json:"job_id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Visibility   string    `json:"visibility"`
	Audience     string    `json:"audience"`
	Status       string    `json:"status"`
	ManifestKey  string    `json:"-"`
	ThumbnailKey string    `json:"-"`
	Views        int64     `json:"views"`
	WatchSeconds int64     `json:"watch_seconds"`
	CreatedAt    time.Time `json:"created_at"`
	Handle       string    `json:"handle"`
	ChannelName  string    `json:"channel_name"`
	ThumbnailURL string    `json:"thumbnail_url,omitempty"`
}

const videoCols = `v.id,v.channel_id,v.job_id,v.title,v.description,v.visibility,v.audience,v.status,v.manifest_key,v.thumbnail_key,v.views,v.watch_seconds,v.created_at,c.handle,c.name`

func scanVideo(row interface{ Scan(...any) error }) (Video, error) {
	var v Video
	err := row.Scan(&v.ID, &v.ChannelID, &v.JobID, &v.Title, &v.Description, &v.Visibility, &v.Audience, &v.Status, &v.ManifestKey, &v.ThumbnailKey, &v.Views, &v.WatchSeconds, &v.CreatedAt, &v.Handle, &v.ChannelName)
	return v, err
}
func (s *Store) Videos(ctx context.Context, channel string, owner bool) ([]Video, error) {
	filter := `v.status='published' AND v.visibility='public'`
	args := []any{}
	if channel != "" {
		args = append(args, channel)
		filter += " AND c.id=$1"
	}
	if owner {
		filter = "c.id=$1"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+videoCols+` FROM cloud_videos v JOIN cloud_channels c ON c.id=v.channel_id WHERE `+filter+` ORDER BY v.created_at DESC LIMIT 60`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Video{}
	for rows.Next() {
		v, e := scanVideo(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) Video(ctx context.Context, id, userID string) (Video, error) {
	v, err := scanVideo(s.DB.QueryRowContext(ctx, `SELECT `+videoCols+` FROM cloud_videos v JOIN cloud_channels c ON c.id=v.channel_id WHERE v.id=$1 AND (c.user_id=$2 OR (v.status='published' AND v.visibility IN ('public','unlisted')))`, id, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}
func (s *Store) CreateVideo(ctx context.Context, u User, v Video) (Video, error) {
	v.Title = strings.TrimSpace(v.Title)
	if v.Title == "" || len(v.Title) > 180 || len(v.Description) > 5000 || (v.Visibility != "public" && v.Visibility != "private" && v.Visibility != "unlisted") || (v.Audience != "free" && v.Audience != "members") {
		return Video{}, ErrValidation
	}
	j, err := s.Job(ctx, u, v.JobID)
	if err != nil {
		return Video{}, err
	}
	if j.Status != "succeeded" {
		return Video{}, ErrConflict
	}
	v.ID = uuid.NewString()
	_, err = s.DB.ExecContext(ctx, `INSERT INTO cloud_videos(id,channel_id,job_id,title,description,visibility,audience,caption_segments)VALUES($1,$2,$3,$4,$5,$6,$7,coalesce((SELECT segments FROM cloud_job_captions WHERE job_id=$3),'[]'::jsonb))`, v.ID, u.ChannelID, v.JobID, v.Title, v.Description, v.Visibility, v.Audience)
	if err != nil {
		return Video{}, dbError(err)
	}
	return s.Video(ctx, v.ID, u.ID)
}
func (s *Store) PublishVideo(ctx context.Context, u User, id string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status, manifest string
	err = tx.QueryRowContext(ctx, `SELECT status,manifest_key FROM cloud_videos WHERE id=$1 AND channel_id=$2 FOR UPDATE`, id, u.ChannelID).Scan(&status, &manifest)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status == "blocked" || status == "deleted" {
		return ErrUnauthorized
	}
	if status == "published" || status == "processing" {
		return tx.Commit()
	}
	if status != "draft" && status != "ready" {
		return ErrConflict
	}
	if manifest != "" && status != "ready" {
		return ErrConflict
	}
	if manifest != "" {
		_, err = tx.ExecContext(ctx, `UPDATE cloud_videos SET status='published',published_at=now() WHERE id=$1`, id)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE cloud_videos SET status='processing' WHERE id=$1`, id)
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO cloud_outbox(id,kind,aggregate_id)VALUES($1,'encoding',$2)`, uuid.NewString(), id)
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) CanWatch(ctx context.Context, u User, v Video) bool {
	if v.Status == "blocked" || v.Status == "deleted" {
		return false
	}
	var owner string
	if s.DB.QueryRowContext(ctx, `SELECT user_id FROM cloud_channels WHERE id=$1`, v.ChannelID).Scan(&owner) != nil {
		return false
	}
	if u.ID == owner {
		return true
	}
	if v.Status != "published" || v.Visibility == "private" {
		return false
	}
	if v.Audience == "free" {
		return true
	}
	if u.ID == "" {
		return false
	}
	var allowed bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_memberships WHERE user_id=$1 AND channel_id=$2 AND status='active' AND paid_through>now())`, u.ID, v.ChannelID).Scan(&allowed)
	return err == nil && allowed
}
