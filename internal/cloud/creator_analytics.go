package cloud

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"time"
)

const analyticsPrivacyMinimum = 5

type CreatorVideoAnalytics struct {
	ID                  string   `json:"id"`
	Title               string   `json:"title"`
	Format              string   `json:"format"`
	QualifiedViews      int64    `json:"qualified_views"`
	WatchSeconds        int64    `json:"watch_seconds"`
	Sessions            int64    `json:"sessions"`
	UniqueViewers       *int64   `json:"unique_viewers"`
	CompletionPercent   *float64 `json:"completion_percent"`
	AverageWatchSeconds *float64 `json:"average_watch_seconds"`
	Reach25             *float64 `json:"watch_reach_25_percent"`
	Reach50             *float64 `json:"watch_reach_50_percent"`
	Reach75             *float64 `json:"watch_reach_75_percent"`
	Reach100            *float64 `json:"watch_reach_100_percent"`
}
type CreatorAnalyticsDetail struct {
	Days           int                     `json:"days"`
	Since          time.Time               `json:"since"`
	GeneratedAt    time.Time               `json:"generated_at"`
	PrivacyMinimum int                     `json:"privacy_minimum"`
	QualifiedViews int64                   `json:"qualified_views"`
	WatchSeconds   int64                   `json:"watch_seconds"`
	Sessions       int64                   `json:"sessions"`
	UniqueViewers  *int64                  `json:"unique_viewers"`
	Videos         []CreatorVideoAnalytics `json:"videos"`
	Daily          []map[string]any        `json:"daily"`
}

func (s *Server) creatorAnalyticsDetail(w http.ResponseWriter, r *http.Request) {
	days := 28
	if raw := r.URL.Query().Get("days"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || (value != 7 && value != 28 && value != 90) {
			s.fail(w, ErrValidation)
			return
		}
		days = value
	}
	detail, err := s.Store.CreatorAnalyticsDetail(r.Context(), currentUser(r), days)
	s.result(w, detail, err)
}
func (s *Store) CreatorAnalyticsDetail(ctx context.Context, u User, days int) (CreatorAnalyticsDetail, error) {
	out := CreatorAnalyticsDetail{Days: days, GeneratedAt: time.Now().UTC(), PrivacyMinimum: analyticsPrivacyMinimum, Videos: []CreatorVideoAnalytics{}, Daily: []map[string]any{}}
	if days != 7 && days != 28 && days != 90 {
		return out, ErrValidation
	}
	if u.ID == "" || u.ChannelID == "" {
		return out, ErrUnauthorized
	}
	now := out.GeneratedAt
	out.Since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -days+1)
	var unique int64
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM cloud_daily_viewers d JOIN cloud_videos v ON v.id=d.video_id WHERE v.channel_id=$1 AND d.day>=($2::timestamptz AT TIME ZONE 'UTC')::date),(SELECT coalesce(sum(e.seconds),0) FROM cloud_watch_events e JOIN cloud_videos v ON v.id=e.video_id WHERE v.channel_id=$1 AND e.created_at>=$2::timestamptz),(SELECT count(DISTINCT e.session_id) FROM cloud_watch_events e JOIN cloud_videos v ON v.id=e.video_id WHERE v.channel_id=$1 AND e.created_at>=$2::timestamptz AND e.seconds>0),(SELECT count(DISTINCT e.user_id) FROM cloud_watch_events e JOIN cloud_videos v ON v.id=e.video_id WHERE v.channel_id=$1 AND e.created_at>=$2::timestamptz AND e.seconds>0)`, u.ChannelID, out.Since).Scan(&out.QualifiedViews, &out.WatchSeconds, &out.Sessions, &unique)
	if err != nil {
		return out, err
	}
	if unique >= analyticsPrivacyMinimum {
		out.UniqueViewers = &unique
	}
	rows, err := tx.QueryContext(ctx, `WITH owned AS (SELECT id,title,format,source_asset_id,published_at,created_at FROM cloud_videos WHERE channel_id=$1), sessions AS (SELECT e.video_id,e.session_id,e.user_id,sum(e.seconds)::bigint AS seconds FROM cloud_watch_events e JOIN owned v ON v.id=e.video_id WHERE e.created_at>=$2::timestamptz AND e.seconds>0 AND e.session_id IS NOT NULL GROUP BY e.video_id,e.session_id,e.user_id), stats AS (SELECT ss.video_id,sum(ss.seconds)::bigint AS seconds,count(*) sessions,count(DISTINCT ss.user_id) unique_viewers,count(*) FILTER(WHERE a.measured AND a.duration_ms>0) measured_sessions,count(*) FILTER(WHERE a.measured AND a.duration_ms>0 AND ss.seconds*1000>=a.duration_ms*.9) complete,count(*) FILTER(WHERE a.measured AND a.duration_ms>0 AND ss.seconds*1000>=a.duration_ms*.25) reach25,count(*) FILTER(WHERE a.measured AND a.duration_ms>0 AND ss.seconds*1000>=a.duration_ms*.5) reach50,count(*) FILTER(WHERE a.measured AND a.duration_ms>0 AND ss.seconds*1000>=a.duration_ms*.75) reach75,count(*) FILTER(WHERE a.measured AND a.duration_ms>0 AND ss.seconds*1000>=a.duration_ms) reach100 FROM sessions ss JOIN owned v ON v.id=ss.video_id LEFT JOIN cloud_media_assets a ON a.id=v.source_asset_id GROUP BY ss.video_id), qualified AS (SELECT d.video_id,count(*) n FROM cloud_daily_viewers d JOIN owned v ON v.id=d.video_id WHERE d.day>=($2::timestamptz AT TIME ZONE 'UTC')::date GROUP BY d.video_id) SELECT v.id,v.title,v.format,coalesce(q.n,0),coalesce(st.seconds,0),coalesce(st.sessions,0),coalesce(st.unique_viewers,0),coalesce(st.measured_sessions,0),coalesce(st.complete,0),coalesce(st.reach25,0),coalesce(st.reach50,0),coalesce(st.reach75,0),coalesce(st.reach100,0) FROM owned v LEFT JOIN stats st ON st.video_id=v.id LEFT JOIN qualified q ON q.video_id=v.id ORDER BY coalesce(q.n,0) DESC,coalesce(v.published_at,v.created_at) DESC,v.id LIMIT 50`, u.ChannelID, out.Since)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v CreatorVideoAnalytics
		var unique, measured, complete, r25, r50, r75, r100 int64
		if err = rows.Scan(&v.ID, &v.Title, &v.Format, &v.QualifiedViews, &v.WatchSeconds, &v.Sessions, &unique, &measured, &complete, &r25, &r50, &r75, &r100); err != nil {
			return out, err
		}
		if unique >= analyticsPrivacyMinimum {
			v.UniqueViewers = &unique
			if v.Sessions > 0 {
				value := float64(v.WatchSeconds) / float64(v.Sessions)
				v.AverageWatchSeconds = &value
			}
			if measured >= analyticsPrivacyMinimum {
				percent := func(n int64) *float64 { value := 100 * float64(n) / float64(measured); return &value }
				v.CompletionPercent = percent(complete)
				v.Reach25 = percent(r25)
				v.Reach50 = percent(r50)
				v.Reach75 = percent(r75)
				v.Reach100 = percent(r100)
			}
		}
		out.Videos = append(out.Videos, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	rows.Close()
	daily, err := tx.QueryContext(ctx, `WITH days AS (SELECT generate_series(($2::timestamptz AT TIME ZONE 'UTC')::date,(now() AT TIME ZONE 'UTC')::date,interval '1 day')::date AS day), watch AS (SELECT (e.created_at AT TIME ZONE 'UTC')::date AS day,sum(e.seconds)::bigint AS seconds FROM cloud_watch_events e JOIN cloud_videos v ON v.id=e.video_id WHERE v.channel_id=$1 AND e.created_at>=$2::timestamptz GROUP BY 1), views AS (SELECT d.day,count(*) n FROM cloud_daily_viewers d JOIN cloud_videos v ON v.id=d.video_id WHERE v.channel_id=$1 AND d.day>=($2::timestamptz AT TIME ZONE 'UTC')::date GROUP BY 1) SELECT to_char(d.day,'YYYY-MM-DD'),coalesce(v.n,0),coalesce(w.seconds,0) FROM days d LEFT JOIN watch w USING(day) LEFT JOIN views v USING(day) ORDER BY d.day`, u.ChannelID, out.Since)
	if err != nil {
		return out, err
	}
	defer daily.Close()
	for daily.Next() {
		var day string
		var views, seconds int64
		if err = daily.Scan(&day, &views, &seconds); err != nil {
			return out, err
		}
		out.Daily = append(out.Daily, map[string]any{"date": day, "qualified_views": views, "watch_seconds": seconds})
	}
	if err = daily.Err(); err != nil {
		return out, err
	}
	daily.Close()
	return out, tx.Commit()
}
