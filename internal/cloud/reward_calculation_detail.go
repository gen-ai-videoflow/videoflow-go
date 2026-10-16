package cloud

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (s *Server) adminRewardCalculationDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var run json.RawMessage
	var policyRaw []byte
	e := s.Store.DB.QueryRowContext(r.Context(), `SELECT to_jsonb(r),policy_snapshot FROM cloud_reward_calculation_runs r WHERE id=$1`, id).Scan(&run, &policyRaw)
	if e != nil {
		s.fail(w, e)
		return
	}
	var policy RewardEnginePolicy
	if e = json.Unmarshal(policyRaw, &policy); e != nil {
		s.fail(w, e)
		return
	}
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT to_jsonb(e),video_id,weight FROM cloud_reward_evidence_snapshots e WHERE run_id=$1 ORDER BY video_id LIMIT 1000`, id)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	items := []json.RawMessage{}
	weights := map[string]int64{}
	for rows.Next() {
		var raw json.RawMessage
		var video string
		var weight int64
		if e = rows.Scan(&raw, &video, &weight); e != nil {
			s.fail(w, e)
			return
		}
		items = append(items, raw)
		if weight > 0 {
			weights[video] = weight
		}
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	rows.Close()
	var available int64
	e = s.Store.DB.QueryRowContext(r.Context(), `SELECT coalesce(sum(greatest(0,s.cents-s.reserved_cents-coalesce((SELECT sum(b.total_cents) FROM cloud_reward_batches b JOIN cloud_reward_policies p ON p.version=b.policy_version WHERE b.source_id=s.id AND b.status='draft' AND p.active),0))),0) FROM cloud_reward_sources s WHERE kind<>'support' AND NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_source_holds h WHERE h.source_id=s.id) AND NOT EXISTS(SELECT 1 FROM cloud_sponsor_campaigns c WHERE c.source_id=s.id)`).Scan(&available)
	if e != nil {
		s.fail(w, e)
		return
	}
	budget := min(available, policy.DailyCapCents)
	allocation := map[string]int64{}
	if budget > 0 && len(weights) > 0 {
		allocation, e = AllocateRewards(budget, 10000, budget, weights)
		if e != nil {
			s.fail(w, e)
			return
		}
	}
	s.json(w, 200, map[string]any{"run": run, "evidence": items, "preview_allocations": allocation, "preview_budget_cents": budget, "preview_note": "Current available-funding simulation only; not a promise, reservation or confirmed allocation."})
}
