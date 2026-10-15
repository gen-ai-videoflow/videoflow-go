package cloud

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) creatorVideos(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	status, format := r.URL.Query().Get("status"), r.URL.Query().Get("format")
	visibility := r.URL.Query().Get("visibility")
	if (visibility != "" && visibility != "public" && visibility != "private" && visibility != "unlisted") || (status != "" && status != "draft" && status != "processing" && status != "published" && status != "failed" && status != "blocked") || (format != "" && format != "standard" && format != "short") {
		s.fail(w, ErrValidation)
		return
	}
	context := tokenHash(u.ID + "|creator|" + status + "|" + format + "|" + visibility)
	cursor := feedCursor{At: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), ID: "~"}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		b, e := base64.RawURLEncoding.DecodeString(raw)
		if e != nil || len(raw) > 1024 || json.Unmarshal(b, &cursor) != nil || cursor.At.IsZero() || cursor.ID == "" || cursor.Context != context {
			s.fail(w, ErrValidation)
			return
		}
	}
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT `+videoCols+` FROM cloud_videos v JOIN cloud_channels c ON c.id=v.channel_id WHERE c.user_id=$1 AND ($2='' OR v.status=$2) AND ($3='' OR v.format=$3) AND ($6='' OR v.visibility=$6) AND (v.created_at,v.id)<($4,$5) ORDER BY v.created_at DESC,v.id DESC LIMIT 21`, u.ID, status, format, cursor.At, cursor.ID, visibility)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	items := []Video{}
	for rows.Next() {
		v, e := scanVideo(rows)
		if e != nil {
			s.fail(w, e)
			return
		}
		items = append(items, v)
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	next := ""
	if len(items) > 20 {
		items = items[:20]
		last := items[19]
		b, _ := json.Marshal(feedCursor{At: last.CreatedAt, ID: last.ID, Context: context})
		next = base64.RawURLEncoding.EncodeToString(b)
	}
	s.json(w, 200, map[string]any{"items": items, "next_cursor": next})
}
