package cloud

import (
	"database/sql"
	"net/http"
	"strings"
	"time"
)

// Audience facts expose aggregates only, never event users, viewer hashes or session identifiers.
func (s *Server) discoveryAudience(w http.ResponseWriter, r *http.Request) {
	video, channel := r.URL.Query().Get("video_id"), r.URL.Query().Get("channel_id")
	if (video == "") == (channel == "") {
		s.fail(w, ErrValidation)
		return
	}
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT to_char((e.created_at AT TIME ZONE 'UTC')::date,'YYYY-MM-DD'),sum(e.seconds),coalesce(sum(e.seconds) FILTER(WHERE x.event_id IS NULL),0),count(*) FILTER(WHERE x.event_id IS NOT NULL),count(DISTINCT e.user_id), CASE WHEN count(DISTINCT e.user_id) FILTER(WHERE ws.traffic_source='organic')>=5 THEN coalesce(sum(e.seconds) FILTER(WHERE ws.traffic_source='organic' AND x.event_id IS NULL),0) END, CASE WHEN count(DISTINCT e.user_id) FILTER(WHERE ws.traffic_source='paid')>=5 THEN coalesce(sum(e.seconds) FILTER(WHERE ws.traffic_source='paid' AND x.event_id IS NULL),0) END FROM cloud_watch_events e LEFT JOIN cloud_watch_sessions ws ON ws.id=e.session_id JOIN cloud_videos v ON v.id=e.video_id LEFT JOIN cloud_watch_exclusions x ON x.event_id=e.id WHERE ($1='' OR v.id=$1) AND ($2='' OR v.channel_id=$2) AND e.created_at>=now()-interval '90 days' GROUP BY 1 ORDER BY 1`, video, channel)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var day string
		var raw, valid, invalid, unique int64
		var organic, paid sql.NullInt64
		if e = rows.Scan(&day, &raw, &valid, &invalid, &unique, &organic, &paid); e != nil {
			s.fail(w, e)
			return
		}
		if unique >= analyticsPrivacyMinimum {
			out = append(out, map[string]any{"date": day, "measured_seconds": raw, "accepted_seconds": valid, "excluded_events": invalid, "unique_viewers": unique, "organic_seconds": nullableAudienceSeconds(organic), "paid_seconds": nullableAudienceSeconds(paid)})
		}
	}
	s.result(w, map[string]any{"daily": out, "privacy_minimum": analyticsPrivacyMinimum, "metric_version": 2, "traffic_provenance": "New sessions record organic or campaign-paid provenance. Legacy sessions are unclassified; each traffic cohort requires five viewers. Distribution boosts never create reward evidence."}, rows.Err())
}
func (s *Server) excludeWatchEvent(w http.ResponseWriter, r *http.Request) {
	var b struct {
		EventID string `json:"event_id"`
		Reason  string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if b.EventID == "" || len(strings.TrimSpace(b.Reason)) < 3 || len(b.Reason) > 1000 {
		s.fail(w, ErrValidation)
		return
	}
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	var video string
	e = tx.QueryRowContext(r.Context(), `SELECT video_id FROM cloud_watch_events WHERE id=$1`, b.EventID).Scan(&video)
	var beforeViews, beforeSeconds int64
	if e == nil {
		e = tx.QueryRowContext(r.Context(), `SELECT views,watch_seconds FROM cloud_videos WHERE id=$1 FOR UPDATE`, video).Scan(&beforeViews, &beforeSeconds)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_watch_exclusions(event_id,actor,reason)VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, b.EventID, currentUser(r).ID, b.Reason)
	}
	// Rebuild accepted seconds and newly provenance-backed qualifications.
	// Preserve legacy view counts whose qualification evidence was never captured.
	var delta int64
	if e == nil {
		e = tx.QueryRowContext(r.Context(), `WITH revised AS(SELECT q.video_id,q.viewer_hash,q.day,q.valid old_valid,coalesce((SELECT sum(ev.seconds) FROM cloud_watch_events ev WHERE ev.session_id=q.session_id AND NOT EXISTS(SELECT 1 FROM cloud_watch_exclusions x WHERE x.event_id=ev.id)),0)>=q.threshold_seconds new_valid FROM cloud_view_qualifications q WHERE q.video_id=$1), changed AS(UPDATE cloud_view_qualifications q SET valid=r.new_valid FROM revised r WHERE q.video_id=r.video_id AND q.viewer_hash=r.viewer_hash AND q.day=r.day RETURNING r.old_valid,r.new_valid) SELECT coalesce(sum(new_valid::integer-old_valid::integer),0) FROM changed`, video).Scan(&delta)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `UPDATE cloud_videos SET views=greatest(0,views+$2) WHERE id=$1`, video, delta)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `DELETE FROM cloud_daily_viewers d USING cloud_view_qualifications q WHERE q.video_id=$1 AND NOT q.valid AND d.video_id=q.video_id AND d.viewer_hash=q.viewer_hash AND d.day=q.day`, video)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_daily_viewers(video_id,viewer_hash,day)SELECT video_id,viewer_hash,day FROM cloud_view_qualifications WHERE video_id=$1 AND valid ON CONFLICT DO NOTHING`, video)
	}
	var after int64
	if e == nil {
		e = tx.QueryRowContext(r.Context(), `UPDATE cloud_videos SET watch_seconds=(SELECT coalesce(sum(e.seconds),0) FROM cloud_watch_events e WHERE e.video_id=$1 AND NOT EXISTS(SELECT 1 FROM cloud_watch_exclusions x WHERE x.event_id=e.id)) WHERE id=$1 RETURNING watch_seconds`, video).Scan(&after)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_audience_rebuilds(video_id,actor,reason,before_views,before_seconds,after_views,after_seconds)VALUES($1,$2,$3,$4,$5,$4::bigint+$7::bigint,$6)`, video, currentUser(r).ID, b.Reason, beforeViews, beforeSeconds, after, delta)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'exclude_watch_event',$2,$3)`, currentUser(r).ID, b.EventID, b.Reason)
	}
	if e == nil {
		e = tx.Commit()
	}
	s.result(w, map[string]any{"video_id": video, "views": beforeViews + delta, "watch_seconds": after, "metric_version": 2, "rebuilt_at": time.Now().UTC(), "view_correction_status": "Historical daily views preserved; event provenance does not support safe replay."}, e)
}

func nullableAudienceSeconds(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}
