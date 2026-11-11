package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
)

// Guard only generation requests, never uploads, static assets or event streams.
func (s *Server) requestReplay(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		paid := r.Method == "POST" && (r.URL.Path == "/chat" || r.URL.Path == "/ideas/develop" || r.URL.Path == "/scripts/generate" || r.URL.Path == "/videos/generate" || r.URL.Path == "/render" || r.URL.Path == "/render/from-shot" || (strings.HasPrefix(r.URL.Path, "/render-jobs/") && strings.HasSuffix(r.URL.Path, "/resubmit")) || strings.HasPrefix(r.URL.Path, "/scenes/") || strings.HasPrefix(r.URL.Path, "/characters/") || strings.HasPrefix(r.URL.Path, "/outputs/"))
		if paid {
			select {
			case s.paidRequests <- struct{}{}:
				defer func() { <-s.paidRequests }()
			default:
				http.Error(w, "Three generation requests are already active. Wait for Activity before starting another.", 429)
				return
			}
		}
		if !paid || key == "" {
			next.ServeHTTP(w, r)
			return
		}
		if len(key) > 128 {
			http.Error(w, "invalid request key", 400)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		if err != nil || len(data) > 1<<20 {
			http.Error(w, "generation request too large", 413)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(data))
		project, err := s.database.GetActiveProjectID()
		if err != nil {
			http.Error(w, "no active project", 400)
			return
		}
		digest := sha256.Sum256(append([]byte(project+":"+r.Method+":"+r.URL.RequestURI()+":"), data...))
		s.recoveryMu.Lock()
		replay, err := s.database.BeginRequest(key, hex.EncodeToString(digest[:]))
		if err != nil {
			s.recoveryMu.Unlock()
			http.Error(w, err.Error(), 409)
			return
		}
		if replay != nil {
			s.recoveryMu.Unlock()
			if replay.Status == 0 {
				http.Error(w, "This request is running or its response was interrupted. Review Activity and saved work before starting another request.", 409)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(replay.Status)
			w.Write(replay.Body)
			return
		}
		s.activeRequests.Store(key, true)
		s.recoveryMu.Unlock()
		defer s.activeRequests.Delete(key)
		capture := &capturedResponse{header: make(http.Header), status: 200}
		next.ServeHTTP(capture, r)
		if err = s.database.FinishRequest(key, capture.status, capture.body.Bytes()); err != nil {
			http.Error(w, "Could not record operation response. Check Activity before retrying.", 503)
			return
		}
		for k, values := range capture.header {
			w.Header()[k] = values
		}
		w.WriteHeader(capture.status)
		w.Write(capture.body.Bytes())
	})
}

type capturedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *capturedResponse) Header() http.Header         { return w.header }
func (w *capturedResponse) WriteHeader(status int)      { w.status = status }
func (w *capturedResponse) Write(b []byte) (int, error) { return w.body.Write(b) }
