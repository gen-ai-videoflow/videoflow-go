package cloud

import (
	"encoding/base64"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type legacyDiscoveryContext struct{}

type feedCursor struct {
	At      time.Time `json:"at"`
	ID      string    `json:"id"`
	Context string    `json:"context"`
	Rank    int64     `json:"rank,omitempty"`
}

func (s *Server) viewingFeed(w http.ResponseWriter, r *http.Request) {
	u := s.optionalUser(r)
	legacy, _ := r.Context().Value(legacyDiscoveryContext{}).(bool)
	pageSize := 20
	if legacy {
		pageSize = 60
	}
	q := r.URL.Query()
	format := q.Get("format")
	if format == "" {
		format = "standard"
	}
	query := strings.TrimSpace(q.Get("q"))
	filter := q.Get("filter")
	if (format != "standard" && format != "short" && !(legacy && format == "all")) || len(query) > 200 || (filter != "" && filter != "following") {
		s.fail(w, ErrValidation)
		return
	}
	ranking := q.Get("rank")
	if ranking == "" {
		ranking = "recent"
	}
	if ranking != "recent" && ranking != "popular" && ranking != "for-you" && ranking != "following" {
		s.fail(w, ErrValidation)
		return
	}
	if ranking == "following" {
		filter = "following"
	}
	history := false
	if u.ID != "" && ranking == "for-you" {
		preferences, err := s.Store.PlaybackPreferences(r.Context(), u.ID)
		if err != nil {
			s.fail(w, err)
			return
		}
		history = preferences.HistoryEnabled
	}
	policy, err := s.Store.discoveryPolicy(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	if canonical, ok := policy.Synonyms[strings.ToLower(query)]; ok {
		query = canonical
	}
	if !legacy {
		pageSize = int(policy.PageLimit)
	}
	context := tokenHash(strconv.FormatInt(policy.Revision, 10) + "|" + format + "|" + query + "|" + filter + "|" + ranking + "|" + u.ID + "|" + strconv.FormatBool(history))
	cursor := feedCursor{Rank: math.MaxInt64, At: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), ID: "~"}
	if raw := q.Get("cursor"); raw != "" {
		cursor = feedCursor{}
		b, e := base64.RawURLEncoding.DecodeString(raw)
		if e != nil || len(raw) > 1024 || json.Unmarshal(b, &cursor) != nil || cursor.ID == "" || cursor.At.IsZero() || cursor.Context != context {
			s.fail(w, ErrValidation)
			return
		}
	}
	rows, e := s.Store.DB.QueryContext(r.Context(), `WITH liked_channels AS (SELECT DISTINCT lv.channel_id FROM (SELECT video_id FROM cloud_video_likes WHERE user_id=$1 ORDER BY created_at DESC LIMIT $22) l JOIN cloud_videos lv ON lv.id=l.video_id), history_channels AS (SELECT DISTINCT hv.channel_id FROM (SELECT video_id FROM cloud_video_progress WHERE user_id=$1 AND $8 AND updated_at>now()-make_interval(days=>$23::int) ORDER BY updated_at DESC LIMIT $22) h JOIN cloud_videos hv ON hv.id=h.video_id), candidates AS (SELECT v.* FROM cloud_videos v JOIN cloud_channels cc ON cc.id=v.channel_id WHERE v.status='published' AND v.visibility='public' AND ($2='all' OR v.format=$2) AND (v.audience='free' OR cc.user_id=$1 OR EXISTS(SELECT 1 FROM cloud_crypto_premium WHERE user_id=$1 AND paid_through>now()) OR EXISTS(SELECT 1 FROM cloud_premium_subscriptions m WHERE m.user_id=$1 AND m.status='active' AND m.paid_through>now() AND m.paid_through>m.revoked_through AND NOT EXISTS(SELECT 1 FROM cloud_premium_disputes d WHERE d.user_id=$1 AND d.status NOT IN ('won','warning_closed')))) AND NOT ($18 AND EXISTS(SELECT 1 FROM cloud_discovery_overrides o WHERE o.expires_at>now() AND o.weight=-10000000 AND (o.search_query='' OR o.search_query=lower($3)) AND (o.video_id=v.id OR o.channel_id=v.channel_id))) AND ($3='' OR ($18 AND EXISTS(SELECT 1 FROM cloud_discovery_overrides o WHERE o.expires_at>now() AND o.weight>0 AND o.search_query<>'' AND o.search_query=lower($3) AND (o.video_id=v.id OR o.channel_id=v.channel_id))) OR to_tsvector('simple',v.title||' '||v.description)@@plainto_tsquery('simple',$3) OR ($17 AND (strpos(lower(v.title),lower($3))>0 OR strpos(lower(v.description),lower($3))>0))) AND ($4<>'following' OR EXISTS(SELECT 1 FROM cloud_follows f WHERE f.user_id=$1 AND f.channel_id=v.channel_id)) AND NOT EXISTS(SELECT 1 FROM cloud_viewer_preferences p WHERE p.user_id=$1 AND p.channel_id=v.channel_id AND p.hidden) AND NOT EXISTS(SELECT 1 FROM cloud_user_restrictions ur WHERE ur.user_id=cc.user_id AND ur.suspended) ORDER BY coalesce(v.published_at,v.created_at) DESC,v.id DESC LIMIT $16), ranked AS (SELECT v.*,CASE WHEN $7 IN ('popular','for-you') THEN least(v.watch_seconds,$10)+least(v.views,$11)*$12 ELSE 0 END + CASE WHEN $7='for-you' AND $1<>'' THEN CASE WHEN EXISTS(SELECT 1 FROM cloud_follows f WHERE f.user_id=$1 AND f.channel_id=v.channel_id) THEN $13 ELSE 0 END + CASE WHEN EXISTS(SELECT 1 FROM liked_channels l WHERE l.channel_id=v.channel_id) THEN $14 ELSE 0 END + CASE WHEN EXISTS(SELECT 1 FROM history_channels h WHERE h.channel_id=v.channel_id) THEN $15 ELSE 0 END ELSE 0 END + CASE WHEN $18 THEN coalesce((SELECT sum(o.weight) FROM cloud_discovery_overrides o WHERE o.expires_at>now() AND (o.search_query='' OR o.search_query=lower($3)) AND (o.video_id=v.id OR o.channel_id=v.channel_id)),0)::bigint ELSE 0 END + CASE WHEN $3<>'' THEN CASE WHEN strpos(lower(v.title),lower($3))>0 THEN $19 ELSE 0 END + CASE WHEN strpos(lower(v.description),lower($3))>0 THEN $20 ELSE 0 END ELSE 0 END + CASE WHEN $7 IN ('for-you','popular') THEN (least(1,greatest(0,1-extract(epoch FROM (now()-coalesce(v.published_at,v.created_at)))/$24::numeric))*$25)::bigint ELSE 0 END rank FROM candidates v) SELECT `+videoCols+`,v.published_at,v.rank FROM ranked v JOIN cloud_channels c ON c.id=v.channel_id WHERE (v.rank,coalesce(v.published_at,v.created_at),v.id)<($9,$5,$6) ORDER BY v.rank DESC,coalesce(v.published_at,v.created_at) DESC,v.id DESC LIMIT $21`, u.ID, format, query, filter, cursor.At, cursor.ID, ranking, history, cursor.Rank, policy.WatchCap, policy.ViewCap, policy.ViewWeight, policy.FollowWeight, policy.LikeWeight, policy.HistoryWeight, policy.CandidateLimit, discoveryCJK(query), policy.Enabled, policy.TitleWeight, policy.DescriptionWeight, pageSize+1, policy.SignalLimit, policy.HistoryDays, policy.FreshnessSeconds, policy.FreshnessWeight)

	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	items := []Video{}
	var dates []time.Time
	var ranks []int64
	for rows.Next() {
		var v Video
		var at *time.Time
		var rank int64
		e = rows.Scan(&v.ID, &v.ChannelID, &v.JobID, &v.Title, &v.Description, &v.Visibility, &v.Audience, &v.Status, &v.ManifestKey, &v.ThumbnailKey, &v.Views, &v.WatchSeconds, &v.CreatedAt, &v.Handle, &v.ChannelName, &v.SourceAssetID, &v.Format, &v.Origin, &v.AIGenerated, &v.Width, &v.Height, &v.DurationMS, &at, &rank)
		if e != nil {
			s.fail(w, e)
			return
		}
		if at == nil {
			at = &v.CreatedAt
		}
		if v.ThumbnailKey != "" {
			v.ThumbnailURL = "/api/v1/videos/" + v.ID + "/poster"
		}
		items = append(items, v)
		dates = append(dates, *at)
		ranks = append(ranks, rank)
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	next := ""
	if len(items) > pageSize {
		items = items[:pageSize]
		b, _ := json.Marshal(feedCursor{At: dates[pageSize-1], ID: items[pageSize-1].ID, Context: context, Rank: ranks[pageSize-1]})
		next = base64.RawURLEncoding.EncodeToString(b)
	}
	if legacy {
		s.json(w, 200, items)
		return
	}
	s.json(w, 200, map[string]any{"items": items, "next_cursor": next, "ranking": ranking, "history_used": history, "candidate_limit": policy.CandidateLimit, "policy_revision": policy.Revision})
}
func (s *Server) relatedVideos(w http.ResponseWriter, r *http.Request) {
	u := s.optionalUser(r)
	v, e := s.Store.Video(r.Context(), chi.URLParam(r, "id"), u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	items, e := s.Store.Discovery(r.Context(), u, "", "")
	out := []Video{}
	for _, item := range items {
		if item.ID != v.ID && item.Format == "standard" && len(out) < 12 {
			out = append(out, item)
		}
	}
	s.result(w, out, e)
}
func (s *Server) videoActions(w http.ResponseWriter, r *http.Request) {
	u := s.optionalUser(r)
	v, e := s.Store.Video(r.Context(), chi.URLParam(r, "id"), u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	var liked, saved bool
	var likes, position int64
	e = s.Store.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_video_likes WHERE user_id=$1 AND video_id=$2),EXISTS(SELECT 1 FROM cloud_saved_videos WHERE user_id=$1 AND video_id=$2),(SELECT count(*) FROM cloud_video_likes WHERE video_id=$2),coalesce((SELECT position_ms FROM cloud_video_progress WHERE user_id=$1 AND video_id=$2),0)`, u.ID, v.ID).Scan(&liked, &saved, &likes, &position)
	access := "allowed"
	if !s.Store.CanWatch(r.Context(), u, v) {
		if v.Audience == "members" {
			if u.ID == "" {
				access = "sign_in"
			} else {
				access = "premium"
			}
		} else {
			access = "unavailable"
		}
	}
	s.result(w, map[string]any{"liked": liked, "saved": saved, "likes": likes, "position_ms": position, "access": access}, e)
}
func (s *Server) changeVideoAction(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id := chi.URLParam(r, "id")
	kind := chi.URLParam(r, "action")
	table := "cloud_saved_videos"
	if kind == "like" {
		table = "cloud_video_likes"
	}
	if r.Method == "PUT" {
		v, e := s.Store.Video(r.Context(), id, u.ID)
		if e != nil {
			s.fail(w, e)
			return
		}
		if v.Status != "published" {
			s.fail(w, ErrConflict)
			return
		}
		_, e = s.Store.DB.ExecContext(r.Context(), `INSERT INTO `+table+`(user_id,video_id)VALUES($1,$2)ON CONFLICT DO NOTHING`, u.ID, id)
		s.result(w, map[string]bool{"ok": true}, e)
	} else {
		_, e := s.Store.DB.ExecContext(r.Context(), `DELETE FROM `+table+` WHERE user_id=$1 AND video_id=$2`, u.ID, id)
		s.result(w, map[string]bool{"ok": true}, e)
	}
}
func (s *Server) saveVideoProgress(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	v, e := s.Store.Video(r.Context(), chi.URLParam(r, "id"), u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	if !s.Store.CanWatch(r.Context(), u, v) {
		s.fail(w, ErrUnauthorized)
		return
	}
	var in struct {
		Position int64 `json:"position_ms"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	if in.Position < 0 || in.Position > 86400000 || (v.DurationMS > 0 && in.Position > v.DurationMS) {
		s.fail(w, ErrValidation)
		return
	}
	p, e := s.Store.PlaybackPreferences(r.Context(), u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	if p.HistoryEnabled {
		_, e = s.Store.DB.ExecContext(r.Context(), `INSERT INTO cloud_video_progress(user_id,video_id,position_ms)VALUES($1,$2,$3)ON CONFLICT(user_id,video_id)DO UPDATE SET position_ms=EXCLUDED.position_ms,updated_at=now()`, u.ID, v.ID, in.Position)
	}
	s.result(w, map[string]bool{"ok": true}, e)
}
func (s *Server) viewerLibrary(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	table := "cloud_saved_videos"
	sort := "x.created_at"
	kind := chi.URLParam(r, "kind")
	if kind == "history" {
		table = "cloud_video_progress"
		sort = "x.updated_at"
	}
	cursor := feedCursor{At: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), ID: "~"}
	key := tokenHash(u.ID + ":" + kind)
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		b, e := base64.RawURLEncoding.DecodeString(raw)
		if e != nil || len(raw) > 1024 || json.Unmarshal(b, &cursor) != nil || cursor.Context != key || cursor.ID == "" || cursor.At.IsZero() {
			s.fail(w, ErrValidation)
			return
		}
	}
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT `+videoCols+`,`+sort+` FROM `+table+` x JOIN cloud_videos v ON v.id=x.video_id JOIN cloud_channels c ON c.id=v.channel_id WHERE x.user_id=$1 AND v.status='published' AND (v.visibility IN ('public','unlisted') OR c.user_id=$1) AND (`+sort+`,v.id)<($2,$3) ORDER BY `+sort+` DESC,v.id DESC LIMIT 21`, u.ID, cursor.At, cursor.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	out := []Video{}
	dates := []time.Time{}
	for rows.Next() {
		var v Video
		var at time.Time
		e = rows.Scan(&v.ID, &v.ChannelID, &v.JobID, &v.Title, &v.Description, &v.Visibility, &v.Audience, &v.Status, &v.ManifestKey, &v.ThumbnailKey, &v.Views, &v.WatchSeconds, &v.CreatedAt, &v.Handle, &v.ChannelName, &v.SourceAssetID, &v.Format, &v.Origin, &v.AIGenerated, &v.Width, &v.Height, &v.DurationMS, &at)
		if e != nil {
			s.fail(w, e)
			return
		}
		if v.ThumbnailKey != "" {
			v.ThumbnailURL = "/api/v1/videos/" + v.ID + "/poster"
		}
		out = append(out, v)
		dates = append(dates, at)
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	next := ""
	if len(out) > 20 {
		out = out[:20]
		b, _ := json.Marshal(feedCursor{At: dates[19], ID: out[19].ID, Context: key})
		next = base64.RawURLEncoding.EncodeToString(b)
	}
	s.json(w, 200, map[string]any{"items": out, "next_cursor": next})
}
