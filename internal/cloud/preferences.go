package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
)

type PlaybackPreferences struct {
	SchemaVersion   int    `json:"schema_version"`
	CaptionsEnabled bool   `json:"captions_enabled"`
	CaptionLanguage string `json:"caption_language"`
	AutoplayNext    bool   `json:"autoplay_next"`
	ShortsAutoplay  bool   `json:"shorts_autoplay"`
	ReduceMotion    string `json:"reduce_motion"`
	DataSaving      bool   `json:"data_saving"`
	HistoryEnabled  bool   `json:"history_enabled"`
}

func defaultPlaybackPreferences() PlaybackPreferences {
	return PlaybackPreferences{SchemaVersion: 1, ShortsAutoplay: true, ReduceMotion: "system", HistoryEnabled: true}
}
func (s *Store) PlaybackPreferences(ctx context.Context, user string) (PlaybackPreferences, error) {
	p := defaultPlaybackPreferences()
	var b []byte
	e := s.DB.QueryRowContext(ctx, `SELECT settings FROM cloud_video_preferences WHERE user_id=$1`, user).Scan(&b)
	if e == sql.ErrNoRows {
		return p, nil
	}
	if e == nil {
		e = json.Unmarshal(b, &p)
	}
	return p, e
}
func (s *Server) getPreferences(w http.ResponseWriter, r *http.Request) {
	p, e := s.Store.PlaybackPreferences(r.Context(), currentUser(r).ID)
	s.result(w, p, e)
}
func (s *Server) patchPreferences(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if !s.decode(w, r, &patch) {
		return
	}
	if patch == nil {
		s.fail(w, ErrValidation)
		return
	}
	p, e := s.Store.PlaybackPreferences(r.Context(), currentUser(r).ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	b, _ := json.Marshal(p)
	var merged map[string]json.RawMessage
	json.Unmarshal(b, &merged)
	for k, v := range patch {
		if k == "schema_version" || merged[k] == nil || string(v) == "null" {
			s.fail(w, ErrValidation)
			return
		}
		merged[k] = v
	}
	b, _ = json.Marshal(merged)
	if json.Unmarshal(b, &p) != nil || len(p.CaptionLanguage) > 35 || !regexp.MustCompile(`^([a-zA-Z]{2,3}(-[a-zA-Z0-9]{2,8})*)?$`).MatchString(p.CaptionLanguage) || (p.ReduceMotion != "system" && p.ReduceMotion != "reduce" && p.ReduceMotion != "no-preference") {
		s.fail(w, ErrValidation)
		return
	}
	patchJSON, _ := json.Marshal(patch)
	_, e = s.Store.DB.ExecContext(r.Context(), `INSERT INTO cloud_video_preferences(user_id,settings)VALUES($1,$2)ON CONFLICT(user_id)DO UPDATE SET settings=cloud_video_preferences.settings||$3::jsonb,updated_at=now()`, currentUser(r).ID, string(b), string(patchJSON))
	if e == nil {
		p, e = s.Store.PlaybackPreferences(r.Context(), currentUser(r).ID)
	}
	s.result(w, p, e)
}
