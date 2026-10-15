package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"net/http"
	"time"
)

func progressTopic(workspace, job string) string { return "vf:v1:progress:" + workspace + ":" + job }
func (w *Worker) notifyProgress(ctx context.Context, id string) {
	if w.redis == nil {
		return
	}
	var workspace string
	if w.store.DB.QueryRowContext(ctx, `SELECT workspace_id FROM cloud_jobs WHERE id=$1`, id).Scan(&workspace) == nil {
		w.redis.Publish(ctx, progressTopic(workspace, id), id)
	}
}
func (s *Server) streamJob(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	j, e := s.Store.Job(r.Context(), u, chi.URLParam(r, "id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, ErrUnavailable)
		return
	}
	var subscription *redis.PubSub
	var hints <-chan *redis.Message
	interval := 2 * time.Second
	if s.Redis != nil {
		subscription = s.Redis.Subscribe(r.Context(), progressTopic(u.WorkspaceID, j.ID))
		defer subscription.Close()
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		_, err := subscription.Receive(ctx)
		cancel()
		if err == nil {
			hints = subscription.Channel()
			interval = 30 * time.Second
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lifetime := time.NewTimer(5 * time.Minute)
	defer lifetime.Stop()
	for {
		var eventID int64
		s.Store.DB.QueryRowContext(r.Context(), `SELECT coalesce(max(id),0)FROM cloud_job_events WHERE job_id=$1`, j.ID).Scan(&eventID)
		data, _ := json.Marshal(j)
		fmt.Fprintf(w, "id: %d\nevent: job\ndata: %s\n\n", eventID, data)
		f.Flush()
		if j.Status == "succeeded" || j.Status == "failed" || j.Status == "cancelled" {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-lifetime.C:
			return
		case <-ticker.C:
		case <-hints:
		}
		if token, e := r.Cookie("videoflow_session"); e != nil {
			return
		} else if _, e = s.Store.Authenticate(r.Context(), token.Value); e != nil {
			return
		}
		j, e = s.Store.Job(r.Context(), u, j.ID)
		if e != nil {
			return
		}
	}
}
