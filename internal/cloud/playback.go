package cloud

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
	"time"
)

func (s *Server) issuePlayback(w http.ResponseWriter, r *http.Request) {
	u := s.optionalUser(r)
	v, err := s.Store.Video(r.Context(), chi.URLParam(r, "id"), u.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !s.Store.CanWatch(r.Context(), u, v) {
		s.fail(w, ErrUnauthorized)
		return
	}
	if s.Media == nil || v.ManifestKey == "" || s.Config.CloudFrontDomain == "" {
		s.fail(w, ErrUnavailable)
		return
	}
	if !s.rate(w, r, "playback:"+s.peer(r)+":"+u.ID, 60, time.Minute, false) {
		return
	}
	random := make([]byte, 32)
	if _, err = rand.Read(random); err != nil {
		s.fail(w, err)
		return
	}
	token := hex.EncodeToString(random)
	var userID any
	if u.ID != "" {
		userID = u.ID
	}
	_, err = s.Store.DB.ExecContext(r.Context(), `INSERT INTO cloud_playback_grants(token_hash,video_id,user_id,expires_at)VALUES($1,$2,$3,now()+interval '60 seconds')`, tokenHash(token), v.ID, userID)
	if err != nil {
		s.fail(w, err)
		return
	}
	base := "https://" + strings.TrimSuffix(strings.TrimPrefix(s.Config.CloudFrontDomain, "https://"), "/")
	grant := map[string]any{"url": base + "/" + v.ManifestKey, "authorize_url": base + "/authorize?token=" + token, "expires_in": 300}
	var captioned bool
	if s.Store.DB.QueryRowContext(r.Context(), `SELECT jsonb_array_length(caption_segments)>0 FROM cloud_videos WHERE id=$1`, v.ID).Scan(&captioned) == nil && captioned {
		grant["captions_url"] = base + "/videos/" + v.ID + "/hls/captions.vtt"
	}
	s.json(w, 200, grant)
}
func (s *Server) authorizeMedia(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if len(token) != 64 || s.Media == nil {
		s.fail(w, ErrUnauthorized)
		return
	}
	var videoID, userID string
	err := s.Store.DB.QueryRowContext(r.Context(), `DELETE FROM cloud_playback_grants WHERE token_hash=$1 AND expires_at>now() RETURNING video_id,coalesce(user_id,'')`, tokenHash(token)).Scan(&videoID, &userID)
	if err != nil {
		s.fail(w, ErrUnauthorized)
		return
	}
	v, err := s.Store.Video(r.Context(), videoID, userID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !s.Store.CanWatch(r.Context(), User{ID: userID}, v) {
		s.fail(w, ErrUnauthorized)
		return
	}
	grant, err := s.Media.PlaybackGrant(r.Context(), v.ManifestKey)
	if err != nil {
		s.fail(w, ErrUnavailable)
		return
	}
	// Marshal only server-generated cookies. Never accept client-supplied policy/cookie values.
	data, _ := json.Marshal(grant["cookies"])
	var cookies []http.Cookie
	if json.Unmarshal(data, &cookies) != nil {
		s.fail(w, ErrUnavailable)
		return
	}
	for _, cookie := range cookies {
		cookie.SameSite = http.SameSiteNoneMode
		cookie.MaxAge = 300
		cookie.Domain = ""
		http.SetCookie(w, &cookie)
	}
	s.json(w, 200, map[string]bool{"ok": true})
}
