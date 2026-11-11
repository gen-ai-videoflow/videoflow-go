package api

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
)

// Reconciliation adopts an existing provider job; it never creates another video.
func (s *Server) handleReconcileRender(w http.ResponseWriter, r *http.Request) {
	job, err := s.database.GetRenderJob(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "job not found", 404)
		return
	}
	if job.Status != "reconciliation_required" {
		http.Error(w, "job does not require reconciliation", 409)
		return
	}
	var body struct {
		ProviderJobID string `json:"provider_job_id"`
		Reason        string `json:"reason"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.ProviderJobID) == "" || len(body.ProviderJobID) > 200 || len(strings.TrimSpace(body.Reason)) < 10 {
		http.Error(w, "Existing provider job ID and verification evidence are required", 400)
		return
	}
	client, err := s.routes().Client("openrouter")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if _, _, err = client.GetVideo(r.Context(), body.ProviderJobID); err != nil {
		http.Error(w, "Cannot verify that provider job with this account", 502)
		return
	}
	if err = s.database.ResolveRenderSubmission(job.ID, body.ProviderJobID, body.Reason); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	s.workers.Enqueue(job.ID)
	writeJSON(w, 202, map[string]any{"job_id": job.ID, "status": "running"})
}
