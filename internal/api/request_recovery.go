package api

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
)

func (s *Server) handlePendingRequests(w http.ResponseWriter, r *http.Request) {
	requests, err := s.database.PendingRequests()
	if err != nil {
		http.Error(w, "cannot load pending requests", 500)
		return
	}
	inactive := []map[string]string{}
	for _, request := range requests {
		if _, active := s.activeRequests.Load(request["key"]); !active {
			inactive = append(inactive, request)
		}
	}
	writeJSON(w, 200, inactive)
}
func (s *Server) handleAcknowledgeRequest(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	if _, active := s.activeRequests.Load(key); active {
		http.Error(w, "request is still running", 409)
		return
	}
	var body struct {
		Reason   string `json:"reason"`
		Reviewed bool   `json:"reviewed_provider_and_saved_work"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || !body.Reviewed || len(strings.TrimSpace(body.Reason)) < 10 || len(body.Reason) > 1000 {
		http.Error(w, "Review provider activity and saved outputs, then record the evidence before unlocking this request", 400)
		return
	}
	if err := s.database.AcknowledgeRequest(key, body.Reason); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "acknowledged"})
}
