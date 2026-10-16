package cloud

import (
	"context"
	"net/http"
	"strings"
	"time"
)

const localLimitCapacity = 10000

// Admission never queues unbounded HTTP work. Health probes retain independent access.
func (s *Server) admission(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/live" || r.URL.Path == "/ready" {
			next.ServeHTTP(w, r)
			return
		}
		select {
		case s.permits <- struct{}{}:
			defer func() { <-s.permits }()
			defer s.recordRequest()()
		default:
			s.metrics.rejected.Add(1)
			w.Header().Set("Retry-After", "2")
			s.json(w, 503, map[string]string{"message": "The service is busy. Please try again shortly."})
			return
		}
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && strings.HasPrefix(r.URL.Path, "/v1/") && !s.rate(w, r, "reads:"+s.peer(r), 600, time.Minute, false) {
			return
		}
		// Job event streams intentionally outlive ordinary JSON requests.
		if !(strings.HasPrefix(r.URL.Path, "/v1/jobs/") && strings.HasSuffix(r.URL.Path, "/events")) {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if s.Store == nil || s.Store.DB.PingContext(ctx) != nil {
		s.json(w, 503, map[string]string{"status": "unavailable"})
		return
	}
	if !s.Config.DevMode && s.Redis == nil || s.Redis != nil && s.Redis.Ping(ctx).Err() != nil {
		s.json(w, 503, map[string]string{"status": "unavailable"})
		return
	}
	s.json(w, 200, map[string]string{"status": "ready"})
}
