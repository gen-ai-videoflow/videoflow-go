package cloud

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"math"
	"net/http"
	"strings"
	"time"
)

func (s *Store) BeginWatch(ctx context.Context, u User, videoID, viewer string) (string, error) {
	v, e := s.Video(ctx, videoID, u.ID)
	if e != nil {
		return "", e
	}
	if !s.CanWatch(ctx, u, v) {
		return "", ErrUnauthorized
	}
	if u.ID == "" {
		return "", ErrUnauthorized
	}
	if viewer == "" {
		viewer = u.ID
	}
	if _, e = s.DB.ExecContext(ctx, `INSERT INTO cloud_watch_clocks(user_id)VALUES($1)ON CONFLICT DO NOTHING`, u.ID); e != nil {
		return "", e
	}
	id := uuid.NewString()
	_, e = s.DB.ExecContext(ctx, `INSERT INTO cloud_watch_sessions(id,video_id,user_id,viewer_hash)VALUES($1,$2,$3,$4)`, id, videoID, u.ID, tokenHash(viewer))
	return id, e
}
func (s *Store) RecordWatch(ctx context.Context, u User, session, eventID string, seconds int) (int, error) {
	if seconds < 1 || seconds > 15 || len(eventID) > 128 || len(eventID) < 8 {
		return 0, ErrValidation
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	var video, viewer, user string
	var last, started time.Time
	var total int
	var counted bool
	e = tx.QueryRowContext(ctx, `SELECT video_id,viewer_hash,user_id,last_event_at,started_at,seconds,view_counted FROM cloud_watch_sessions WHERE id=$1 AND user_id=$2 FOR UPDATE`, session, u.ID).Scan(&video, &viewer, &user, &last, &started, &total, &counted)
	if errors.Is(e, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if e != nil {
		return 0, e
	}
	if time.Since(started) > 4*time.Hour {
		return 0, ErrConflict
	}
	var viewerLast time.Time
	if e = tx.QueryRowContext(ctx, `SELECT last_at FROM cloud_watch_clocks WHERE user_id=$1 FOR UPDATE`, u.ID).Scan(&viewerLast); e != nil {
		return 0, e
	}
	if viewerLast.After(last) {
		last = viewerLast
	}
	elapsed := int(time.Since(last).Seconds())
	if elapsed < 0 {
		elapsed = 0
	}
	if seconds > elapsed {
		seconds = elapsed
	}
	if seconds > 15 {
		seconds = 15
	}
	res, e := tx.ExecContext(ctx, `INSERT INTO cloud_watch_events(id,user_id,video_id,seconds,session_id)VALUES($1,$2,$3,$4,$5)ON CONFLICT DO NOTHING`, eventID, u.ID, video, seconds, session)
	if e != nil {
		return 0, e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return 0, tx.Commit()
	}
	var allowed bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_videos v JOIN cloud_channels c ON c.id=v.channel_id WHERE v.id=$1 AND v.status='published' AND v.visibility<>'private' AND c.user_id<>$2 AND (v.audience='free' OR EXISTS(SELECT 1 FROM cloud_crypto_premium WHERE user_id=$2 AND paid_through>now()) OR EXISTS(SELECT 1 FROM cloud_premium_subscriptions m WHERE m.user_id=$2 AND m.status='active' AND m.paid_through>now() AND m.paid_through>m.revoked_through AND NOT EXISTS(SELECT 1 FROM cloud_premium_disputes d WHERE d.user_id=$2 AND d.status NOT IN ('won','warning_closed')))))`, video, u.ID).Scan(&allowed)
	if e != nil {
		return 0, e
	}
	if !allowed {
		seconds = 0
	}
	if _, e = tx.ExecContext(ctx, `UPDATE cloud_watch_clocks SET last_at=now() WHERE user_id=$1`, u.ID); e != nil {
		return 0, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE cloud_watch_events SET seconds=$2 WHERE id=$1`, eventID, seconds); e != nil {
		return 0, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE cloud_watch_sessions SET seconds=seconds+$2,last_event_at=now() WHERE id=$1`, session, seconds); e != nil {
		return 0, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE cloud_videos SET watch_seconds=watch_seconds+$2 WHERE id=$1`, video, seconds); e != nil {
		return 0, e
	}
	var format string
	var duration int64
	var measured bool
	if e = tx.QueryRowContext(ctx, `SELECT v.format,coalesce(a.duration_ms,0),coalesce(a.measured,false) FROM cloud_videos v LEFT JOIN cloud_media_assets a ON a.id=v.source_asset_id WHERE v.id=$1`, video).Scan(&format, &duration, &measured); e != nil {
		return 0, e
	}
	if !counted && total+seconds >= qualifyingViewSeconds(format, duration, measured) {
		res, e = tx.ExecContext(ctx, `INSERT INTO cloud_daily_viewers(video_id,viewer_hash,day)VALUES($1,$2,(now() AT TIME ZONE 'UTC')::date)ON CONFLICT DO NOTHING`, video, viewer)
		if e != nil {
			return 0, e
		}
		added, _ := res.RowsAffected()
		if added > 0 {
			if _, e = tx.ExecContext(ctx, `UPDATE cloud_videos SET views=views+1 WHERE id=$1`, video); e != nil {
				return 0, e
			}
		}
		if _, e = tx.ExecContext(ctx, `UPDATE cloud_watch_sessions SET view_counted=true WHERE id=$1`, session); e != nil {
			return 0, e
		}
	}
	return seconds, tx.Commit()
}
func (s *Server) beginWatch(w http.ResponseWriter, r *http.Request) {
	if !s.rate(w, r, "watch-session:"+currentUser(r).ID, 30, time.Minute, false) {
		return
	}
	var b struct {
		VideoID string `json:"video_id"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	id, e := s.Store.BeginWatch(r.Context(), currentUser(r), b.VideoID, currentUser(r).ID)
	s.result(w, map[string]string{"id": id}, e)
}
func (s *Server) watchEvent(w http.ResponseWriter, r *http.Request) {
	if !s.rate(w, r, "watch-event:"+currentUser(r).ID, 12, time.Minute, false) {
		return
	}
	var b struct {
		SessionID string `json:"session_id"`
		EventID   string `json:"event_id"`
		Seconds   int    `json:"seconds"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	n, e := s.Store.RecordWatch(r.Context(), currentUser(r), b.SessionID, b.EventID, b.Seconds)
	s.result(w, map[string]int{"accepted_seconds": n}, e)
}
func (s *Server) viewerPreferences(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var b struct {
		ChannelID string `json:"channel_id"`
		Hidden    bool   `json:"hidden"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if b.ChannelID == "" {
		s.fail(w, ErrValidation)
		return
	}
	_, e := s.Store.DB.ExecContext(r.Context(), `INSERT INTO cloud_viewer_preferences(user_id,channel_id,hidden)VALUES($1,$2,$3)ON CONFLICT(user_id,channel_id)DO UPDATE SET hidden=EXCLUDED.hidden`, u.ID, b.ChannelID, b.Hidden)
	s.result(w, map[string]bool{"ok": true}, e)
}
func (s *Store) Discovery(ctx context.Context, u User, query, category string) ([]Video, error) {
	query = strings.TrimSpace(query)
	if len(query) > 200 {
		return nil, ErrValidation
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT `+videoCols+` FROM cloud_videos v JOIN cloud_channels c ON c.id=v.channel_id WHERE v.status='published' AND v.visibility='public'
 AND ($2='' OR to_tsvector('simple',v.title||' '||v.description)@@plainto_tsquery('simple',$2))
 AND NOT EXISTS(SELECT 1 FROM cloud_viewer_preferences p WHERE p.user_id=$1 AND p.channel_id=v.channel_id AND p.hidden)
 AND ($3<>'following' OR EXISTS(SELECT 1 FROM cloud_follows f WHERE f.user_id=$1 AND f.channel_id=v.channel_id))
 ORDER BY CASE WHEN $3='trending' THEN v.watch_seconds ELSE 0 END DESC,
 CASE WHEN EXISTS(SELECT 1 FROM cloud_follows f WHERE f.user_id=$1 AND f.channel_id=v.channel_id)THEN 1 ELSE 0 END DESC,v.published_at DESC LIMIT 60`, u.ID, query, category)
	if e != nil {
		return nil, e
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

// Public qualified views are separate from approved financial reward weights.
func qualifyingViewSeconds(format string, duration int64, measured bool) int {
	if format == "short" && measured && duration > 0 {
		return int(math.Ceil(math.Min(10, math.Max(1, float64(duration)*.8/1000))))
	}
	return 10
}
