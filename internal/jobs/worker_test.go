package jobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"6eek-go/internal/config"
	"6eek-go/internal/db"
	"6eek-go/internal/events"
	"6eek-go/internal/media"
	"6eek-go/internal/models"
)

func workerFixture(t *testing.T, h http.HandlerFunc) (*WorkerPool, *models.RenderJob) {
	t.Helper()
	remote := httptest.NewServer(h)
	t.Cleanup(remote.Close)
	cfg := config.Load()
	cfg.OpenRouterBaseURL = remote.URL
	cfg.StorageRoot = t.TempDir()
	database, err := db.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err = database.SetProviderSecret("openrouter", "saved-key", nil); err != nil {
		t.Fatal(err)
	}
	worker := NewWorkerPool(cfg, database, events.NewBroker(), media.NewMediaEngine())
	worker.pollInterval = time.Millisecond
	t.Cleanup(worker.Stop)
	job := &models.RenderJob{Provider: "openrouter", Model: "test/video", Status: "pending", RequestJSON: json.RawMessage(`{"prompt":"A real requested scene","duration":4,"aspect_ratio":"16:9"}`)}
	if err = database.CreateRenderJob(job); err != nil {
		t.Fatal(err)
	}
	return worker, job
}
func TestWorkerUsesProviderAndDownloads(t *testing.T) {
	submissions, downloads := 0, 0
	worker, job := workerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer saved-key" {
			t.Error("missing DB credential")
		}
		switch r.URL.Path {
		case "/videos/models":
			w.Write([]byte(`{"data":[{"id":"test/video","supported_durations":[4],"supported_aspect_ratios":["16:9"]}]}`))
		case "/videos":
			submissions++
			w.WriteHeader(202)
			w.Write([]byte(`{"id":"upstream-job"}`))
		case "/videos/upstream-job":
			w.Write([]byte(`{"status":"completed","unsigned_urls":["/videos/upstream-job/content?index=0"]}`))
		case "/videos/upstream-job/content":
			downloads++
			w.Write([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"))
		default:
			t.Error(r.URL.Path)
			w.WriteHeader(404)
		}
	})
	if err := worker.processJob(job.ID); err != nil {
		t.Fatal(err)
	}
	saved, _ := worker.database.GetRenderJob(job.ID)
	outputs, _ := worker.database.ListOutputsForJob(job.ID)
	if saved.Status != "succeeded" || saved.ProviderJobID == nil || *saved.ProviderJobID != "upstream-job" || len(outputs) != 1 {
		t.Fatal(saved, outputs)
	}
	data, err := os.ReadFile(filepath.Join(worker.cfg.StorageRoot, "outputs", job.ID+"-0.mp4"))
	if err != nil || string(data) != "\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom" {
		t.Fatalf("file %q %v", data, err)
	}
	if outputs[0].Score != nil {
		t.Fatal("fabricated QA score")
	}
	if submissions != 1 || downloads != 1 {
		t.Fatal(submissions, downloads)
	}
}
func TestWorkerFailureProducesNoOutput(t *testing.T) {
	worker, job := workerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"invalid key"}`))
	})
	if err := worker.processJob(job.ID); err == nil {
		t.Fatal("provider failure swallowed")
	}
	saved, _ := worker.database.GetRenderJob(job.ID)
	outputs, _ := worker.database.ListOutputsForJob(job.ID)
	if saved.Status != "failed" || saved.Error == nil || len(outputs) != 0 {
		t.Fatal(saved, outputs)
	}
}
func TestWorkerRestartDoesNotResubmit(t *testing.T) {
	worker, job := workerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			t.Error("resubmitted existing job")
		}
		if r.URL.Path == "/videos/upstream-job" {
			w.Write([]byte(`{"status":"completed"}`))
			return
		}
		w.Write([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"))
	})
	id := "upstream-job"
	job.ProviderJobID = &id
	worker.database.UpdateRenderJob(job)
	if err := worker.processJob(job.ID); err != nil {
		t.Fatal(err)
	}
}
func TestWorkerCancellationLeavesResumableJob(t *testing.T) {
	worker, job := workerFixture(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"status":"in_progress"}`)) })
	id := "upstream-job"
	job.ProviderJobID = &id
	worker.database.UpdateRenderJob(job)
	worker.ctx, worker.cancel = context.WithCancel(context.Background())
	worker.cancel()
	if err := worker.processJob(job.ID); err == nil {
		t.Fatal("expected cancellation")
	}
	saved, _ := worker.database.GetRenderJob(job.ID)
	if saved.Status == "failed" || saved.Status == "succeeded" {
		t.Fatal("shutdown destroyed resumable job", saved.Status)
	}
}

func TestWorkerUsesStoryboardFrameAndNegatives(t *testing.T) {
	submitted := false
	worker, job := workerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/videos/models" {
			w.Write([]byte(`{"data":[{"id":"test/video","supported_durations":[4],"supported_aspect_ratios":["16:9"],"supported_frame_images":["first_frame"]}]}`))
			return
		}
		if r.URL.Path == "/videos" {
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			if !strings.Contains(payload["prompt"].(string), "watermark") {
				t.Error("saved negatives ignored", payload)
			}
			if payload["frame_images"] == nil {
				t.Error("storyboard did not reach video model")
			}
			submitted = true
			w.Write([]byte(`{"id":"upstream"}`))
			return
		}
		w.Write([]byte(`{"status":"failed","error":"mock stop after submission"}`))
	})
	pid, _ := worker.database.GetActiveProjectID()
	scene := &models.Scene{ProjectID: &pid, Title: "Ocean", Summary: "Wave", Duration: 4, AspectRatio: "16:9"}
	worker.database.CreateScene(scene)
	path := filepath.Join(worker.cfg.StorageRoot, "story.png")
	os.WriteFile(path, []byte("\x89PNG\r\n\x1a\n"), 0600)
	asset := &models.Asset{ProjectID: &pid, Type: "storyboard", Name: "Storyboard", FilePath: path, MetadataJSON: json.RawMessage(`{"scene_id":"` + scene.ID + `"}`)}
	worker.database.CreateAsset(asset)
	job = &models.RenderJob{ProjectID: &pid, SceneID: &scene.ID, Provider: "openrouter", Model: "test/video", Status: "pending", RequestJSON: job.RequestJSON}
	if err := worker.database.CreateRenderJob(job); err != nil {
		t.Fatal(err)
	}
	worker.database.SetAppSetting("render_negatives", `["watermark"]`)
	_ = worker.processJob(job.ID)
	if !submitted {
		t.Fatal("no video submission")
	}
}
