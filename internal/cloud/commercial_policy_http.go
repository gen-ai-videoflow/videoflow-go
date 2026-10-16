package cloud

import (
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (s *Server) commercialAdminRoutes(r chi.Router) {
	r.Get("/commercial-policy", s.adminCommercialPolicy)
	r.Put("/commercial-policy", s.adminCommercialPropose)
	r.Get("/commercial-policy/history", s.adminCommercialHistory)
	r.Get("/commercial-policy/proposals", s.adminCommercialProposals)
	r.Post("/commercial-policy/preview", s.adminCommercialPreview)
	r.Post("/commercial-policy/proposals/{id}/approve", s.adminCommercialApprove)
}
func (s *Server) adminCommercialPolicy(w http.ResponseWriter, r *http.Request) {
	c, e := s.Store.CommercialPolicy(r.Context())
	if e != nil {
		s.fail(w, e)
		return
	}
	runtime, runtimeErr := commercialRuntime(r.Context(), s.Store.DB, s.Config)
	var preview *CommercialPreview
	previewStatus := "generation_policy_required"
	if runtimeErr == nil {
		value, previewErr := commercialPreview(c.Policy, runtime)
		if previewErr == nil {
			preview = &value
			previewStatus = "ready"
		}
	}
	s.json(w, 200, map[string]any{"policy": c.Policy, "revision": c.Revision, "updated_at": c.UpdatedAt, "source": c.Source, "preview": preview, "preview_status": previewStatus})
}
func (s *Server) adminCommercialPropose(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Policy   CommercialPolicy `json:"policy"`
		Revision int64            `json:"revision"`
		Reason   string           `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	id, e := s.Store.ProposeCommercialPolicy(r.Context(), b.Policy, b.Revision, b.Reason, currentUser(r).ID)
	s.result(w, map[string]string{"id": id}, e)
}
func (s *Server) adminCommercialPreview(w http.ResponseWriter, r *http.Request) {
	var p CommercialPolicy
	if !s.decode(w, r, &p) {
		return
	}
	runtime, e := commercialRuntime(r.Context(), s.Store.DB, s.Config)
	if e != nil {
		s.fail(w, e)
		return
	}
	preview, e := commercialPreview(p, runtime)
	s.result(w, preview, e)
}
func (s *Server) adminCommercialApprove(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Reason string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	e := s.Store.ApproveCommercialPolicy(r.Context(), chi.URLParam(r, "id"), currentUser(r).ID, b.Reason, s.Config)
	s.result(w, map[string]bool{"approved": e == nil}, e)
}
func (s *Server) adminCommercialHistory(w http.ResponseWriter, r *http.Request) {
	s.supportJSONRows(w, r, `SELECT to_jsonb(h) FROM cloud_commercial_history h ORDER BY revision DESC LIMIT 100`)
}
func (s *Server) adminCommercialProposals(w http.ResponseWriter, r *http.Request) {
	s.supportJSONRows(w, r, `SELECT to_jsonb(p) FROM cloud_commercial_proposals p ORDER BY created_at DESC LIMIT 100`)
}
