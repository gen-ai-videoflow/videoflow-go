package cloud

import (
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (s *Server) rewardCorrectionAdminRoutes(r chi.Router) {
	r.Get("/rewards/corrections", s.adminRewardCorrections)
	r.Post("/rewards/corrections", s.adminRewardCorrectionPropose)
	r.Post("/rewards/corrections/{id}/approve", s.adminRewardCorrectionApprove)
}
func (s *Server) adminRewardCorrections(w http.ResponseWriter, r *http.Request) {
	s.supportJSONRows(w, r, `SELECT to_jsonb(c) FROM cloud_reward_corrections c ORDER BY created_at DESC,id LIMIT 100`)
}
func (s *Server) adminRewardCorrectionPropose(w http.ResponseWriter, r *http.Request) {
	var c RewardCorrection
	if !s.decode(w, r, &c) {
		return
	}
	e := s.Store.ProposeRewardCorrection(r.Context(), c, currentUser(r).ID)
	s.result(w, map[string]bool{"proposed": e == nil}, e)
}
func (s *Server) adminRewardCorrectionApprove(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Reason string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	var cents int64
	if e := s.Store.DB.QueryRowContext(r.Context(), `SELECT cents FROM cloud_reward_corrections WHERE id=$1`, chi.URLParam(r, "id")).Scan(&cents); e != nil {
		s.fail(w, e)
		return
	}
	if cents > 0 && !s.Config.RewardsEnabled {
		s.fail(w, ErrUnavailable)
		return
	}
	e := s.Store.ApproveRewardCorrection(r.Context(), chi.URLParam(r, "id"), currentUser(r).ID, b.Reason)
	s.result(w, map[string]bool{"approved": e == nil}, e)
}
func (s *Server) rewardCorrectionList(w http.ResponseWriter, r *http.Request) {
	s.supportJSONRows(w, r, `SELECT jsonb_build_object('id',id,'original_batch_id',original_batch_id,'cents',cents,'reason',reason,'approved_at',approved_at,'vesting_seconds',vesting_seconds) FROM cloud_reward_corrections WHERE user_id=$1 AND approved_by IS NOT NULL ORDER BY approved_at DESC,id LIMIT 100`, currentUser(r).ID)
}
