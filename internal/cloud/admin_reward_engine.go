package cloud

import (
	"database/sql"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
)

func (s *Server) rewardEngineAdminRoutes(r chi.Router) {
	s.rewardFinanceDetailRoutes(r)
	r.Get("/rewards/calculations/{id}", s.adminRewardCalculationDetail)
	r.Get("/rewards/policy/history", func(w http.ResponseWriter, r *http.Request) {
		s.rewardEngineList(w, r, `SELECT to_jsonb(h) FROM cloud_reward_engine_policy_history h ORDER BY revision DESC LIMIT 100`)
	})
	r.Get("/rewards/policy", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Store.RewardEnginePolicy(r.Context())
		s.result(w, out, e)
	})
	r.Put("/rewards/policy", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Policy   RewardEnginePolicy `json:"policy"`
			Revision int64              `json:"revision"`
			Reason   string             `json:"reason"`
		}
		if !s.decode(w, r, &b) {
			return
		}
		id, e := s.Store.ProposeRewardEnginePolicy(r.Context(), b.Policy, b.Revision, b.Reason, currentUser(r).ID)
		s.result(w, map[string]any{"id": id, "status": "proposed"}, e)
	})
	r.Get("/rewards/policy/proposals", func(w http.ResponseWriter, r *http.Request) {
		s.rewardEngineList(w, r, `SELECT jsonb_build_object('id',id,'policy',policy,'base_revision',base_revision,'reason',reason,'proposed_by',proposed_by,'status',status,'created_at',created_at) FROM cloud_reward_engine_proposals ORDER BY created_at DESC LIMIT 100`)
	})
	r.Post("/rewards/policy/proposals/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
		e := s.Store.ApproveRewardEnginePolicy(r.Context(), chi.URLParam(r, "id"), currentUser(r).ID)
		s.result(w, map[string]bool{"approved": e == nil}, e)
	})
	r.Get("/rewards/health", s.adminRewardHealth)
	r.Get("/rewards/calculations", func(w http.ResponseWriter, r *http.Request) {
		s.rewardEngineList(w, r, `SELECT jsonb_build_object('id',id,'day',day,'shadow',shadow,'status',status,'attempt',attempt,'videos',videos,'estimated_cents',estimated_cents,'draft_batches',draft_batches,'error',error,'updated_at',updated_at) FROM cloud_reward_calculation_runs ORDER BY created_at DESC LIMIT 100`)
	})
	r.Post("/rewards/calculate", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Day    string `json:"day"`
			Shadow bool   `json:"shadow"`
			Reason string `json:"reason"`
		}
		if !s.decode(w, r, &b) {
			return
		}
		if !b.Shadow && !s.Config.RewardsEnabled {
			s.fail(w, ErrUnavailable)
			return
		}
		out, e := s.Store.QueueRewardCalculation(r.Context(), b.Day, b.Shadow, currentUser(r).ID, b.Reason)
		s.result(w, out, e)
	})
	r.Get("/rewards/reviews", func(w http.ResponseWriter, r *http.Request) {
		s.rewardEngineList(w, r, `SELECT jsonb_build_object('user_id',user_id,'reason',reason,'updated_at',updated_at) FROM cloud_reward_holds ORDER BY updated_at DESC LIMIT 100`)
	})
	r.Get("/rewards/quality", func(w http.ResponseWriter, r *http.Request) {
		s.rewardEngineList(w, r, `SELECT jsonb_build_object('video_id',video_id,'originality_bps',originality_bps,'trust_bps',trust_bps,'eligible',eligible,'reason',reason,'actor',actor,'revision',revision) FROM cloud_reward_quality_reviews ORDER BY updated_at DESC LIMIT 100`)
	})
	r.Put("/rewards/quality/{id}", s.adminRewardQuality)
	r.Post("/rewards/payout-runs", s.adminRewardPayoutRun)
}
func (s *Server) rewardEngineList(w http.ResponseWriter, r *http.Request, query string) {
	rows, e := s.Store.DB.QueryContext(r.Context(), query)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	items := []json.RawMessage{}
	for rows.Next() {
		var raw json.RawMessage
		if e = rows.Scan(&raw); e != nil {
			s.fail(w, e)
			return
		}
		items = append(items, raw)
	}
	s.result(w, map[string]any{"items": items}, rows.Err())
}
func (s *Server) adminRewardQuality(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Originality int64  `json:"originality_bps"`
		Trust       int64  `json:"trust_bps"`
		Eligible    bool   `json:"eligible"`
		Revision    int64  `json:"revision"`
		Reason      string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if b.Originality < 0 || b.Originality > 10000 || b.Trust < 0 || b.Trust > 10000 || strings.TrimSpace(b.Reason) == "" || len(b.Reason) > 1000 || b.Revision < 0 {
		s.fail(w, ErrValidation)
		return
	}
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	result, e := tx.ExecContext(r.Context(), `INSERT INTO cloud_reward_quality_reviews(video_id,originality_bps,trust_bps,eligible,reason,actor) SELECT $1,$2,$3,$4,$5,$6 WHERE $7=0 OR EXISTS(SELECT 1 FROM cloud_reward_quality_reviews existing WHERE existing.video_id=$1) ON CONFLICT(video_id) DO UPDATE SET originality_bps=$2,trust_bps=$3,eligible=$4,reason=$5,actor=$6,revision=cloud_reward_quality_reviews.revision+1,updated_at=now() WHERE cloud_reward_quality_reviews.revision=$7`, chi.URLParam(r, "id"), b.Originality, b.Trust, b.Eligible, b.Reason, currentUser(r).ID, b.Revision)
	if e == nil {
		n, _ := result.RowsAffected()
		if n != 1 {
			e = ErrConflict
		}
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'review_reward_quality',$2,$3)`, currentUser(r).ID, chi.URLParam(r, "id"), b.Reason)
	}
	if e == nil {
		e = tx.Commit()
	}
	s.result(w, map[string]bool{"reviewed": e == nil}, e)
}
func (s *Server) adminRewardHealth(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{}
	var funded, reserved, confirmed, unvested, held, failed, legacy int64
	e := s.Store.DB.QueryRowContext(r.Context(), `SELECT coalesce(sum(cents),0),coalesce(sum(reserved_cents),0) FROM cloud_reward_sources`).Scan(&funded, &reserved)
	if e == nil {
		e = s.Store.DB.QueryRowContext(r.Context(), `SELECT coalesce(sum(a.cents),0),coalesce(sum(a.cents-floor(a.cents::numeric*least(1::numeric,greatest(0::numeric,extract(epoch FROM (now()-b.confirmed_at))/p.vesting_seconds)))),0) FROM cloud_reward_allocations a JOIN cloud_reward_batches b ON b.id=a.batch_id JOIN cloud_reward_releases r ON r.allocation_id=a.id JOIN cloud_reward_policies p ON p.version=b.policy_version WHERE b.status='confirmed'`).Scan(&confirmed, &unvested)
	}
	if e == nil {
		e = s.Store.DB.QueryRowContext(r.Context(), `SELECT (SELECT count(*) FROM cloud_reward_holds),(SELECT count(*) FROM cloud_reward_calculation_runs WHERE status='failed'),(SELECT coalesce(sum(amount_cents),0) FROM cloud_earnings WHERE status='pending' AND amount_cents>0)`).Scan(&held, &failed, &legacy)
	}
	var corrections, correctionUnvested, available int64
	if e == nil {
		e = s.Store.DB.QueryRowContext(r.Context(), `SELECT coalesce(sum(cents),0),coalesce(sum(cents-floor(cents::numeric*least(1::numeric,greatest(0::numeric,extract(epoch FROM (now()-approved_at))/vesting_seconds)))),0) FROM cloud_reward_corrections WHERE approved_at IS NOT NULL AND cents>0`).Scan(&corrections, &correctionUnvested)
	}
	if e == nil {
		e = s.Store.DB.QueryRowContext(r.Context(), `SELECT coalesce(sum(greatest(0,s.cents-s.reserved_cents-coalesce((SELECT sum(b.total_cents) FROM cloud_reward_batches b JOIN cloud_reward_policies p ON p.version=b.policy_version WHERE b.source_id=s.id AND b.status='draft' AND p.active),0))),0) FROM cloud_reward_sources s WHERE kind<>'support' AND NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_source_holds h WHERE h.source_id=s.id) AND NOT EXISTS(SELECT 1 FROM cloud_sponsor_campaigns c WHERE c.source_id=s.id)`).Scan(&available)
	}
	confirmed += corrections
	unvested += correctionUnvested
	out["funded_cents"] = funded
	out["reserved_cents"] = reserved
	out["available_cents"] = available
	out["confirmed_cents"] = confirmed
	out["unvested_cents"] = unvested
	out["held_accounts"] = held
	out["failed_runs"] = failed
	out["legacy_pending_cents"] = legacy
	var latest json.RawMessage
	if e == nil {
		rowErr := s.Store.DB.QueryRowContext(r.Context(), `SELECT jsonb_build_object('id',id,'status',status,'day',day,'updated_at',updated_at) FROM cloud_reward_calculation_runs ORDER BY created_at DESC LIMIT 1`).Scan(&latest)
		if rowErr != nil && rowErr != sql.ErrNoRows {
			e = rowErr
		}
	}
	out["latest_run"] = latest
	var recovery int64
	if e == nil {
		e = s.Store.DB.QueryRowContext(r.Context(), `SELECT coalesce(sum(cents),0) FROM cloud_reward_revenue_recoveries`).Scan(&recovery)
	}
	out["recovery_reserved_cents"] = recovery
	s.result(w, out, e)
}
