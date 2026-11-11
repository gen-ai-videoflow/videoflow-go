package api

import (
	"6eek-go/internal/models"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLostResponseReplayDoesNotRepeatPaidCall(t *testing.T) {
	calls := 0
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
			return
		}
		calls++
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"reply\":\"Hello\",\"action\":\"none\"}"}}]}`))
	})
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("POST", "/chat", strings.NewReader(`{"message":"Hello"}`))
		r.Header.Set("Idempotency-Key", "same-request")
		result := httptest.NewRecorder()
		s.Router().ServeHTTP(result, r)
		if result.Code != 200 {
			t.Fatalf("%d %s", result.Code, result.Body.String())
		}
	}
	if calls != 1 {
		t.Fatalf("provider called %d times for one request", calls)
	}
	r := httptest.NewRequest("POST", "/chat", strings.NewReader(`{"message":"Different"}`))
	r.Header.Set("Idempotency-Key", "same-request")
	result := httptest.NewRecorder()
	s.Router().ServeHTTP(result, r)
	if result.Code != 409 {
		t.Fatal("changed request key accepted")
	}
}
func TestUncertainRenderCannotBeResubmitted(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("provider should not be called") })
	project, _ := s.database.GetActiveProjectID()
	_ = project
	var jobID string
	// Create through the ordinary API without starting workers, then mark its durable submission intent.
	r := call(t, s, "POST", "/render", `{"prompt":"A clip"}`)
	var body map[string]any
	json.Unmarshal(r.Body.Bytes(), &body)
	jobID, _ = body["job_id"].(string)
	job, _ := s.database.GetRenderJob(jobID)
	job.Status = "reconciliation_required"
	s.database.UpdateRenderJob(job)
	if result := call(t, s, "POST", "/render-jobs/"+jobID+"/resubmit", `{}`); result.Code != 409 {
		t.Fatalf("uncertain paid request was duplicated: %d", result.Code)
	}
}

func TestResubmitLineageReturnsTheActiveReplacement(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("provider must not run") })
	job := &models.RenderJob{Provider: "openrouter", Model: "fixture", Status: "failed", RequestJSON: []byte(`{"prompt":"A clip"}`)}
	s.database.CreateRenderJob(job)
	first := call(t, s, "POST", "/render-jobs/"+job.ID+"/resubmit", `{}`)
	second := call(t, s, "POST", "/render-jobs/"+job.ID+"/resubmit", `{}`)
	var a, b map[string]any
	json.Unmarshal(first.Body.Bytes(), &a)
	json.Unmarshal(second.Body.Bytes(), &b)
	if a["job_id"] != b["job_id"] {
		t.Fatal("retrying the original job created multiple paid replacements")
	}
}
