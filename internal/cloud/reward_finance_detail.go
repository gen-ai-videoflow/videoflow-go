package cloud

import (
	"database/sql"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (s *Server) rewardFinanceDetailRoutes(r chi.Router) {
	r.Get("/rewards/users/{id}", s.adminRewardCreatorDetail)
	r.Get("/rewards/quality/{id}", func(w http.ResponseWriter, r *http.Request) {
		var item json.RawMessage
		e := s.Store.DB.QueryRowContext(r.Context(), `SELECT jsonb_build_object('video_id',video_id,'originality_bps',originality_bps,'trust_bps',trust_bps,'eligible',eligible,'reason',reason,'actor',actor,'revision',revision) FROM cloud_reward_quality_reviews WHERE video_id=$1`, chi.URLParam(r, "id")).Scan(&item)
		if e == nil {
			s.json(w, 200, map[string]any{"item": item})
			return
		}
		if e == sql.ErrNoRows {
			s.json(w, 200, map[string]any{"item": nil})
			return
		}
		s.fail(w, e)
	})
	r.Get("/rewards/payout-runs", func(w http.ResponseWriter, r *http.Request) {
		s.rewardEngineList(w, r, `SELECT jsonb_build_object('id',id,'period',period,'actor',actor,'reason',reason,'status',status,'batches',batches,'failures',failures,'created_at',created_at) FROM cloud_reward_payout_runs ORDER BY created_at DESC LIMIT 100`)
	})
	r.Post("/rewards/payouts/{id}/reconcile", s.adminRewardReconcileTransfer)
}
func (s *Server) adminRewardCreatorDetail(w http.ResponseWriter, r *http.Request) {
	user := chi.URLParam(r, "id")
	out := map[string]any{"user_id": user}
	queries := map[string]string{
		"allocations": `SELECT jsonb_build_object('id',a.id,'video_id',a.video_id,'batch_id',a.batch_id,'cents',a.cents,'confirmed_at',b.confirmed_at,'vesting_seconds',p.vesting_seconds,'released_cents',coalesce(r.released_cents,0)) FROM cloud_reward_allocations a JOIN cloud_reward_batches b ON b.id=a.batch_id JOIN cloud_reward_policies p ON p.version=b.policy_version LEFT JOIN cloud_reward_releases r ON r.allocation_id=a.id WHERE a.user_id=$1 AND b.status='confirmed' ORDER BY b.day DESC,a.id LIMIT 100`,
		"earnings":    `SELECT jsonb_build_object('id',id,'reference',reference,'amount_cents',amount_cents,'status',status,'created_at',created_at) FROM cloud_earnings WHERE user_id=$1 ORDER BY created_at DESC,id LIMIT 100`,
		"payouts":     `SELECT jsonb_build_object('id',id,'amount_cents',amount_cents,'currency',currency,'status',status,'provider_transfer',provider_transfer,'created_at',created_at,'paid_at',paid_at) FROM cloud_payout_batches WHERE user_id=$1 ORDER BY created_at DESC,id LIMIT 100`,
		"holds":       `SELECT jsonb_build_object('reason',reason,'actor',actor,'updated_at',updated_at) FROM cloud_reward_holds WHERE user_id=$1`,
	}
	for key, q := range queries {
		rows, e := s.Store.DB.QueryContext(r.Context(), q, user)
		if e != nil {
			s.fail(w, e)
			return
		}
		items := []json.RawMessage{}
		for rows.Next() {
			var raw json.RawMessage
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				s.fail(w, e)
				return
			}
			items = append(items, raw)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			s.fail(w, e)
			return
		}
		out[key] = items
	}
	s.json(w, 200, out)
}
