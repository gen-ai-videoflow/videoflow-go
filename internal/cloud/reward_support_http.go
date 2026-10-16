package cloud

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
)

// Register inside the existing authenticated creator router.
func (s *Server) rewardSupportCreatorRoutes(r chi.Router) {
	r.Get("/rewards/support", s.rewardSupportOverview)
	r.Post("/rewards/support/{id}/claims", s.rewardSupportClaim)
	r.Get("/rewards/appeals", s.rewardAppealList)
	r.Get("/rewards/corrections", s.rewardCorrectionList)
	r.Post("/rewards/appeals", s.rewardAppealSubmit)
	r.Get("/rewards/notifications", s.rewardNotificationList)
	r.Post("/rewards/notifications/{id}/read", s.rewardNotificationRead)
}

// Register inside the existing administrator router, behind role + recent MFA.
// The central permission map must explicitly allow these paths for finance.
func (s *Server) rewardSupportAdminRoutes(r chi.Router) {
	r.Get("/rewards/support", s.adminRewardSupportOverview)
	r.Post("/rewards/support", s.adminRewardSupportPropose)
	r.Post("/rewards/support/{id}/approve", s.adminRewardSupportApprove)
	r.Post("/rewards/support/claims/{id}/review", s.adminRewardSupportReview)
	r.Get("/rewards/appeals", s.adminRewardAppeals)
	r.Post("/rewards/appeals/{id}/resolve", s.adminRewardAppealResolve)
}
func (s *Server) supportJSONRows(w http.ResponseWriter, r *http.Request, query string, args ...any) {
	rows, e := s.Store.DB.QueryContext(r.Context(), query, args...)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var raw json.RawMessage
		if e = rows.Scan(&raw); e != nil {
			s.fail(w, e)
			return
		}
		out = append(out, raw)
	}
	s.result(w, map[string]any{"items": out, "limit": 100}, rows.Err())
}
func (s *Server) rewardSupportOverview(w http.ResponseWriter, r *http.Request) {
	// No private financial source/evidence fields in the creator projection.
	s.supportJSONRows(w, r, `SELECT jsonb_build_object('type','program','id',id,'title',title,'criteria',criteria,'enabled',enabled,'grant_credits',grant_credits,'expiry_days',expiry_days,'creator_cap',creator_cap,'budget_available',enabled AND committed_cents+service_cost_cents<=budget_cents AND granted<grant_cap AND EXISTS(SELECT 1 FROM cloud_reward_sources s WHERE s.id=source_id AND s.kind='support' AND s.cents-s.reserved_cents>=service_cost_cents)) FROM cloud_reward_support_programs WHERE approved_by IS NOT NULL UNION ALL SELECT jsonb_build_object('type','claim','id',id,'program_id',program_id,'status',status,'reason',reason,'created_at',created_at) FROM (SELECT * FROM cloud_reward_support_claims WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100) c UNION ALL SELECT jsonb_build_object('type','credit','id',id,'credits',credits,'available',CASE WHEN expires_at>now() THEN credits-consumed-reserved ELSE 0 END,'reserved',reserved,'consumed',consumed,'expires_at',expires_at,'withdrawable',false) FROM (SELECT * FROM cloud_promotional_credits WHERE workspace_id=$2 ORDER BY created_at DESC LIMIT 100) p LIMIT 100`, currentUser(r).ID, currentUser(r).WorkspaceID)
}
func (s *Server) rewardSupportClaim(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Evidence string `json:"evidence"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	id, e := s.Store.ClaimRewardSupport(r.Context(), currentUser(r), chi.URLParam(r, "id"), b.Evidence)
	s.result(w, map[string]string{"id": id}, e)
}
func (s *Server) adminRewardSupportOverview(w http.ResponseWriter, r *http.Request) {
	s.supportJSONRows(w, r, `SELECT jsonb_build_object('type','program','record',to_jsonb(p)) FROM (SELECT * FROM cloud_reward_support_programs ORDER BY created_at DESC LIMIT 50)p UNION ALL SELECT jsonb_build_object('type','claim','record',to_jsonb(c)) FROM (SELECT * FROM cloud_reward_support_claims ORDER BY created_at DESC LIMIT 50)c`)
}
func (s *Server) adminRewardSupportPropose(w http.ResponseWriter, r *http.Request) {
	var p RewardSupportProgram
	if !s.decode(w, r, &p) {
		return
	}
	e := s.Store.ProposeRewardSupport(r.Context(), p, currentUser(r).ID)
	s.result(w, map[string]bool{"created": e == nil}, e)
}
func (s *Server) adminRewardSupportApprove(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Reason string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	e := s.Store.ApproveRewardSupport(r.Context(), chi.URLParam(r, "id"), currentUser(r).ID, b.Reason)
	s.result(w, map[string]bool{"approved": e == nil}, e)
}
func (s *Server) adminRewardSupportReview(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Reason  string `json:"reason"`
		Approve bool   `json:"approve"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	e := s.Store.ReviewRewardSupport(r.Context(), chi.URLParam(r, "id"), currentUser(r).ID, b.Reason, b.Approve)
	s.result(w, map[string]bool{"reviewed": e == nil}, e)
}
func (s *Server) rewardAppealList(w http.ResponseWriter, r *http.Request) {
	s.supportJSONRows(w, r, `SELECT jsonb_build_object('id',id,'subject',subject,'allocation_id',allocation_id,'evidence',evidence,'status',status,'resolution',resolution,'created_at',created_at,'resolved_at',resolved_at) FROM cloud_reward_appeals WHERE user_id=$1 ORDER BY created_at DESC,id LIMIT 100`, currentUser(r).ID)
}
func (s *Server) adminRewardAppeals(w http.ResponseWriter, r *http.Request) {
	s.supportJSONRows(w, r, `SELECT to_jsonb(a) FROM cloud_reward_appeals a ORDER BY created_at DESC,id LIMIT 100`)
}
func (s *Server) rewardAppealSubmit(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Subject      string `json:"subject"`
		AllocationID string `json:"allocation_id"`
		Evidence     string `json:"evidence"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	id, e := s.Store.SubmitRewardAppeal(r.Context(), currentUser(r).ID, b.AllocationID, b.Subject, b.Evidence)
	s.result(w, map[string]string{"id": id}, e)
}
func (s *Server) adminRewardAppealResolve(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Resolution string `json:"resolution"`
		Upheld     bool   `json:"upheld"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	e := s.Store.ResolveRewardAppeal(r.Context(), chi.URLParam(r, "id"), currentUser(r).ID, b.Resolution, b.Upheld)
	s.result(w, map[string]bool{"resolved": e == nil}, e)
}
func (s *Server) rewardNotificationList(w http.ResponseWriter, r *http.Request) {
	s.supportJSONRows(w, r, `SELECT jsonb_build_object('id',id,'kind',kind,'message',message,'read_at',read_at,'created_at',created_at) FROM cloud_reward_notifications WHERE user_id=$1 ORDER BY created_at DESC,id LIMIT 100`, currentUser(r).ID)
}
func (s *Server) rewardNotificationRead(w http.ResponseWriter, r *http.Request) {
	res, e := s.Store.DB.ExecContext(r.Context(), `UPDATE cloud_reward_notifications SET read_at=coalesce(read_at,now()) WHERE id=$1 AND user_id=$2`, chi.URLParam(r, "id"), currentUser(r).ID)
	if e == nil {
		n, _ := res.RowsAffected()
		if n != 1 {
			e = ErrNotFound
		}
	}
	s.result(w, map[string]bool{"read": e == nil}, e)
}
