package cloud

import (
	"net/http"
	"sync/atomic"
	"time"
)

type capacityMetrics struct{ admitted, rejected, completed, active, totalMicros atomic.Int64 }

func (s *Server) capacitySnapshot() map[string]any {
	db := s.Store.DB.Stats()
	return map[string]any{"scope": "this API process since startup", "admitted_requests": s.metrics.admitted.Load(), "overload_rejections": s.metrics.rejected.Load(), "completed_requests": s.metrics.completed.Load(), "active_requests": s.metrics.active.Load(), "total_duration_micros": s.metrics.totalMicros.Load(), "admission_capacity": cap(s.permits), "database": map[string]any{"open_connections": db.OpenConnections, "in_use": db.InUse, "idle": db.Idle, "max_open_connections": db.MaxOpenConnections, "pool_wait_count": db.WaitCount, "pool_wait_millis": db.WaitDuration.Milliseconds()}, "note": "Request counts include polling; these are not viewer or revenue metrics."}
}
func (s *Server) adminCapacity(w http.ResponseWriter, r *http.Request) {
	s.json(w, 200, s.capacitySnapshot())
}
func (s *Server) recordRequest() func() {
	s.metrics.admitted.Add(1)
	s.metrics.active.Add(1)
	start := time.Now()
	return func() {
		s.metrics.active.Add(-1)
		s.metrics.completed.Add(1)
		s.metrics.totalMicros.Add(time.Since(start).Microseconds())
	}
}
