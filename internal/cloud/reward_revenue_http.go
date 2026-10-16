package cloud

import (
	"github.com/go-chi/chi/v5"
	"net/http"
)

// Mount inside protected admin router; central RBAC must grant finance paths.
func (s *Server) rewardRevenueAdminRoutes(r chi.Router) {
	r.Get("/rewards/revenue", s.adminRewardRevenueOverview)
	r.Get("/rewards/revenue/stripe/{id}/statement", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.RewardStripeStatement(r.Context(), chi.URLParam(r, "id"))
		s.result(w, v, e)
	})
	r.Post("/rewards/revenue/evidence", s.adminRewardRevenueEvidence)
	r.Post("/rewards/revenue/evidence/{id}/approve", s.adminRewardRevenueApprove)
	r.Post("/rewards/revenue/reconcile", s.adminRewardRevenueReconcile)
}
func (s *Server) adminRewardRevenueOverview(w http.ResponseWriter, r *http.Request) {
	s.supportJSONRows(w, r, `SELECT jsonb_build_object('type','evidence','record',to_jsonb(e)) FROM (SELECT * FROM cloud_reward_revenue_evidence ORDER BY created_at DESC,id LIMIT 100)e
 UNION ALL SELECT jsonb_build_object('type','lot','record',to_jsonb(l)||jsonb_build_object('consumed_credits',coalesce((SELECT sum(c.credits) FROM cloud_reward_revenue_consumption c WHERE c.lot_id=l.id),0))) FROM (SELECT * FROM cloud_reward_revenue_lots ORDER BY settled_at DESC,id LIMIT 100)l
 UNION ALL SELECT jsonb_build_object('type','day','record',to_jsonb(d)) FROM (SELECT * FROM cloud_reward_revenue_days ORDER BY day DESC LIMIT 100)d
 UNION ALL SELECT jsonb_build_object('type','coverage_hold','record',jsonb_build_object('source_id',h.source_id,'reason',h.reason,'coverage_recovered',cloud_reward_revenue_covered(s.id),'reserved_shortfall_cents',CASE WHEN cloud_reward_revenue_covered(s.id) THEN 0 ELSE s.reserved_cents END,'unavailable_cents',s.cents,'created_at',h.created_at)) FROM (SELECT * FROM cloud_reward_revenue_source_holds ORDER BY created_at DESC LIMIT 100) h JOIN cloud_reward_sources s ON s.id=h.source_id
 UNION ALL SELECT jsonb_build_object('type','recovery','record',to_jsonb(c)) FROM (SELECT * FROM cloud_reward_revenue_recoveries ORDER BY created_at DESC LIMIT 100) c
 UNION ALL SELECT jsonb_build_object('type','replacement_source','record',jsonb_build_object('id',s.id,'reference',s.reference,'available_cents',s.cents-s.reserved_cents,'evidence_reference',s.evidence_reference)) FROM (SELECT * FROM cloud_reward_sources s WHERE s.kind<>'support' AND NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_source_holds h WHERE h.source_id=s.id) AND NOT EXISTS(SELECT 1 FROM cloud_sponsor_campaigns c WHERE c.source_id=s.id) ORDER BY reconciled_at DESC LIMIT 100) s
 UNION ALL SELECT jsonb_build_object('type','receipt_candidate','record',jsonb_build_object('provider','stripe','receipt_id',p.id,'credits',p.credits,'currency',p.currency,'gross_cents',p.price_cents,'refunded_cents',p.refunded_cents,'created_at',p.created_at)) FROM (SELECT * FROM cloud_purchases p WHERE status='paid' AND NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_lots l WHERE l.provider='stripe' AND l.receipt_id=p.id) ORDER BY created_at LIMIT 100)p
 UNION ALL SELECT jsonb_build_object('type','receipt_candidate','record',jsonb_build_object('provider','cryptorails','receipt_id',i.id,'credits',i.credits,'token',i.token,'gross_token_amount',s.gross_amount,'provider_token_fee',s.provider_fee,'net_token_amount',s.net_amount,'tx_hash',s.tx_hash,'created_at',s.created_at,'usd_conversion_required',true)) FROM (SELECT * FROM cloud_crypto_invoices i WHERE status='settled' AND purpose='credits' AND NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_lots l WHERE l.provider='cryptorails' AND l.receipt_id=i.id) ORDER BY created_at LIMIT 100)i JOIN cloud_crypto_settlements s ON s.reference=i.reference
 UNION ALL SELECT jsonb_build_object('type','cost_candidate','record',jsonb_build_object('job_id',j.id,'purchased_credits',j.credits-j.promotional_credits,'promotional_credits',j.promotional_credits,'completed_at',j.updated_at)) FROM (SELECT * FROM cloud_jobs j WHERE status='succeeded' AND credits>promotional_credits AND NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_evidence e WHERE e.kind='cost' AND e.resource_id=j.id AND e.approved_by IS NOT NULL) ORDER BY updated_at,id LIMIT 100)j`)
}
func (s *Server) adminRewardRevenueEvidence(w http.ResponseWriter, r *http.Request) {
	var b RewardRevenueEvidence
	if !s.decode(w, r, &b) {
		return
	}
	id, err := s.Store.ProposeRewardRevenueEvidence(r.Context(), b, currentUser(r).ID)
	s.result(w, map[string]string{"id": id, "status": "proposed"}, err)
}
func (s *Server) adminRewardRevenueApprove(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Reason string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	err := s.Store.ApproveRewardRevenueEvidence(r.Context(), chi.URLParam(r, "id"), currentUser(r).ID, b.Reason)
	s.result(w, map[string]bool{"approved": err == nil}, err)
}
func (s *Server) adminRewardRevenueReconcile(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Day    string `json:"day"`
		Reason string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if len(b.Reason) == 0 || len(b.Reason) > 1000 {
		s.fail(w, ErrValidation)
		return
	}
	result, err := s.Store.reconcileRewardRevenue(r.Context(), b.Day, currentUser(r).ID, b.Reason)
	s.result(w, result, err)
}
