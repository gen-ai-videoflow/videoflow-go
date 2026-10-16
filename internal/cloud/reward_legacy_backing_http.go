package cloud

import (
	"github.com/go-chi/chi/v5"
	"net/http"
)

// Register in the existing finance/owner administrator router behind recent MFA.
func (s *Server) rewardLegacyBackingRoutes(r chi.Router) {
	r.Get("/rewards/legacy-backing", func(w http.ResponseWriter, r *http.Request) {
		s.rewardEngineList(w, r, `SELECT jsonb_build_object('type','proposal','record',to_jsonb(p)) FROM (SELECT * FROM cloud_reward_legacy_backing_proposals ORDER BY created_at DESC LIMIT 100)p
 UNION ALL SELECT jsonb_build_object('type','unbacked','record',jsonb_build_object('id',e.id,'user_id',e.user_id,'reference',e.reference,'amount_cents',e.amount_cents,'status',e.status,'created_at',e.created_at)) FROM (SELECT * FROM cloud_earnings WHERE amount_cents>0 AND currency='usd' AND status<>'paid' AND NOT cloud_reward_earning_backed(id) ORDER BY created_at,id LIMIT 100)e
 UNION ALL SELECT jsonb_build_object('type','source','record',jsonb_build_object('id',s.id,'reference',s.reference,'available_cents',s.cents-s.reserved_cents,'evidence_reference',s.evidence_reference)) FROM (SELECT * FROM cloud_reward_sources s WHERE kind<>'support' AND cents>reserved_cents AND NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_source_holds h WHERE h.source_id=s.id) AND NOT EXISTS(SELECT 1 FROM cloud_sponsor_campaigns c WHERE c.source_id=s.id) ORDER BY reconciled_at DESC LIMIT 100)s
 UNION ALL SELECT jsonb_build_object('type','historical_transfer_exception','record',to_jsonb(x)||jsonb_build_object('user_id',e.user_id,'amount_cents',e.amount_cents,'reference',e.reference)) FROM (SELECT * FROM cloud_reward_legacy_transfer_exceptions ORDER BY created_at DESC LIMIT 100)x JOIN cloud_earnings e ON e.id=x.earning_id`)
	})
	r.Post("/rewards/legacy-backing", func(w http.ResponseWriter, r *http.Request) {
		var p RewardLegacyBackingProposal
		if !s.decode(w, r, &p) {
			return
		}
		id, e := s.Store.ProposeRewardLegacyBacking(r.Context(), p, currentUser(r).ID)
		s.result(w, map[string]string{"id": id, "status": "proposed"}, e)
	})
	r.Post("/rewards/legacy-backing/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Reason string `json:"reason"`
		}
		if !s.decode(w, r, &b) {
			return
		}
		e := s.Store.ApproveRewardLegacyBacking(r.Context(), chi.URLParam(r, "id"), currentUser(r).ID, b.Reason)
		s.result(w, map[string]bool{"approved": e == nil}, e)
	})
}
