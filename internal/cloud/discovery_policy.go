package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
	"strings"
	"time"
	"unicode"
)

type DiscoveryPolicy struct {
	PageLimit         int64             `json:"page_limit"`
	SignalLimit       int64             `json:"signal_limit"`
	HistoryDays       int64             `json:"history_days"`
	FreshnessSeconds  int64             `json:"freshness_seconds"`
	FreshnessWeight   int64             `json:"freshness_weight"`
	Synonyms          map[string]string `json:"synonyms,omitempty"`
	Revision          int64             `json:"revision"`
	Enabled           bool              `json:"enabled"`
	WatchCap          int64             `json:"watch_cap"`
	ViewCap           int64             `json:"view_cap"`
	ViewWeight        int64             `json:"view_weight"`
	FollowWeight      int64             `json:"follow_weight"`
	LikeWeight        int64             `json:"like_weight"`
	HistoryWeight     int64             `json:"history_weight"`
	CandidateLimit    int64             `json:"candidate_limit"`
	TitleWeight       int64             `json:"title_weight"`
	DescriptionWeight int64             `json:"description_weight"`
}

func defaultDiscoveryPolicy() DiscoveryPolicy {
	return DiscoveryPolicy{PageLimit: 20, SignalLimit: 300, HistoryDays: 30, FreshnessSeconds: 604800, Enabled: true, WatchCap: 100000, ViewCap: 10000, ViewWeight: 10, FollowWeight: 1000000, LikeWeight: 500000, HistoryWeight: 250000, CandidateLimit: 2000, TitleWeight: 1000, DescriptionWeight: 100}
}
func (p DiscoveryPolicy) validate() error {
	if p.PageLimit < 5 || p.PageLimit > 50 || p.SignalLimit < 10 || p.SignalLimit > 1000 || p.HistoryDays < 1 || p.HistoryDays > 90 || p.FreshnessSeconds < 3600 || p.FreshnessSeconds > 7776000 || p.FreshnessWeight < 0 || p.FreshnessWeight > 1000000 {
		return ErrValidation
	}
	if len(p.Synonyms) > 100 {
		return ErrValidation
	}
	for k, v := range p.Synonyms {
		if strings.TrimSpace(k) != k || strings.ToLower(k) != k || k == "" || len(k) > 100 || strings.TrimSpace(v) != v || v == "" || len(v) > 100 {
			return ErrValidation
		}
	}

	if p.CandidateLimit < 100 || p.CandidateLimit > 5000 || p.TitleWeight < 1 || p.TitleWeight > 10000 || p.DescriptionWeight < 0 || p.DescriptionWeight > p.TitleWeight {
		return ErrValidation
	}
	for _, v := range []int64{p.WatchCap, p.ViewCap, p.ViewWeight, p.FollowWeight, p.LikeWeight, p.HistoryWeight} {
		if v < 0 || v > 1000000 {
			return ErrValidation
		}
	}
	return nil
}
func (s *Store) discoveryPolicy(ctx context.Context) (DiscoveryPolicy, error) {
	p := defaultDiscoveryPolicy()
	var b []byte
	var rev int64
	e := s.DB.QueryRowContext(ctx, `SELECT revision,policy FROM cloud_discovery_policies ORDER BY revision DESC LIMIT 1`).Scan(&rev, &b)
	if e == sql.ErrNoRows {
		return p, nil
	}
	if e != nil {
		return p, e
	}
	e = json.Unmarshal(b, &p)
	p.Revision = rev
	return p, e
}

// CJK queries use literal substring matching: PostgreSQL simple FTS does not segment Han words.
func discoveryCJK(q string) bool {
	for _, r := range q {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}
func (s *Server) discoveryAdminRoutes(r chi.Router) {
	r.Get("/discovery/policy", func(w http.ResponseWriter, r *http.Request) {
		p, e := s.Store.discoveryPolicy(r.Context())
		s.result(w, p, e)
	})
	r.Put("/discovery/policy", s.updateDiscoveryPolicy)
	r.Get("/discovery/history", s.discoveryPolicyHistory)
	r.Get("/discovery/overrides", s.discoveryOverrideHistory)
	r.Post("/discovery/overrides", s.createDiscoveryOverride)
	r.Get("/discovery/preview", s.viewingFeed)
	r.Get("/discovery/index", s.discoveryIndexHealth)
	r.Get("/discovery/audience", s.discoveryAudience)
	r.Post("/discovery/exclusions", s.excludeWatchEvent)
}
func (s *Server) updateDiscoveryPolicy(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Policy DiscoveryPolicy `json:"policy"`
		Reason string          `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if b.Policy.validate() != nil || len(strings.TrimSpace(b.Reason)) < 3 || len(b.Reason) > 1000 {
		s.fail(w, ErrValidation)
		return
	}
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(601060)`)
	var revision int64
	if e == nil {
		e = tx.QueryRowContext(r.Context(), `SELECT coalesce(max(revision),0) FROM cloud_discovery_policies`).Scan(&revision)
	}
	if e == nil && revision != b.Policy.Revision {
		e = ErrConflict
	}
	b.Policy.Revision = revision + 1
	data, _ := json.Marshal(b.Policy)
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_discovery_policies(revision,policy,actor,reason)VALUES($1,$2,$3,$4)`, b.Policy.Revision, data, currentUser(r).ID, b.Reason)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'discovery_policy',$2,$3)`, currentUser(r).ID, fmt.Sprint(b.Policy.Revision), b.Reason)
	}
	if e == nil {
		e = tx.Commit()
	}
	s.result(w, b.Policy, e)
}
func (s *Server) createDiscoveryOverride(w http.ResponseWriter, r *http.Request) {
	var b struct {
		SearchQuery string    `json:"search_query"`
		VideoID     *string   `json:"video_id"`
		ChannelID   *string   `json:"channel_id"`
		Weight      int64     `json:"weight"`
		ExpiresAt   time.Time `json:"expires_at"`
		Reason      string    `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if len(b.SearchQuery) > 200 || b.VideoID != nil && strings.TrimSpace(*b.VideoID) == "" || b.ChannelID != nil && strings.TrimSpace(*b.ChannelID) == "" || (b.VideoID == nil) == (b.ChannelID == nil) || b.Weight == 0 || b.Weight < -10000000 || b.Weight > 10000000 || !b.ExpiresAt.After(time.Now()) || b.ExpiresAt.After(time.Now().Add(30*24*time.Hour)) || len(strings.TrimSpace(b.Reason)) < 3 || len(b.Reason) > 1000 {
		s.fail(w, ErrValidation)
		return
	}
	id := uuid.NewString()
	_, e := s.Store.DB.ExecContext(r.Context(), `WITH added AS(INSERT INTO cloud_discovery_overrides(id,video_id,channel_id,weight,expires_at,reason,actor,search_query)VALUES($1,$2,$3,$4,$5,$6,$7,$8)RETURNING id)INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)SELECT $7,'discovery_override',id,$6 FROM added`, id, b.VideoID, b.ChannelID, b.Weight, b.ExpiresAt, b.Reason, currentUser(r).ID, strings.ToLower(strings.TrimSpace(b.SearchQuery)))
	s.result(w, map[string]string{"id": id}, e)
}

func (s *Server) discoveryIndexHealth(w http.ResponseWriter, r *http.Request) {
	var ready bool
	var estimate int64
	e := s.Store.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid WHERE c.relname='cloud_discovery_search_fts' AND i.indisvalid),coalesce((SELECT reltuples::bigint FROM pg_class WHERE oid='cloud_videos'::regclass),0)`).Scan(&ready, &estimate)
	s.result(w, map[string]any{"fts_index_ready": ready, "estimated_video_rows": estimate, "index_version": 1, "maintenance": "Transactional index updates are automatic. Reindex is deployment managed; no unsafe unbounded admin SQL job is exposed.", "cjk_analysis": "literal substring", "cache": "No result cache; policy revision binds pagination cursors."}, e)
}

func (s *Server) discoveryPolicyHistory(w http.ResponseWriter, r *http.Request) {
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT revision,policy,actor,reason,created_at FROM cloud_discovery_policies ORDER BY revision DESC LIMIT 20`)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var rev int64
		var policy json.RawMessage
		var actor, reason string
		var created time.Time
		if e = rows.Scan(&rev, &policy, &actor, &reason, &created); e != nil {
			s.fail(w, e)
			return
		}
		out = append(out, map[string]any{"revision": rev, "policy": policy, "actor": actor, "reason": reason, "created_at": created})
	}
	s.result(w, map[string]any{"items": out}, rows.Err())
}
func (s *Server) discoveryOverrideHistory(w http.ResponseWriter, r *http.Request) {
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT id,coalesce(video_id,''),coalesce(channel_id,''),weight,search_query,expires_at,reason,actor FROM cloud_discovery_overrides ORDER BY created_at DESC,id LIMIT 100`)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, video, channel, query, reason, actor string
		var weight int64
		var expires time.Time
		if e = rows.Scan(&id, &video, &channel, &weight, &query, &expires, &reason, &actor); e != nil {
			s.fail(w, e)
			return
		}
		out = append(out, map[string]any{"id": id, "video_id": video, "channel_id": channel, "weight": weight, "search_query": query, "expires_at": expires, "reason": reason, "actor": actor})
	}
	s.result(w, map[string]any{"items": out}, rows.Err())
}
