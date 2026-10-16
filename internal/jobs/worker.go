package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"6eek-go/internal/routing"

	"6eek-go/internal/config"
	"6eek-go/internal/db"
	"6eek-go/internal/events"
	"6eek-go/internal/media"
	"6eek-go/internal/models"
)

type WorkerPool struct {
	cfg          *config.Config
	database     *db.DB
	broker       *events.Broker
	media        *media.MediaEngine
	queue        chan string
	semaphore    chan struct{}
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	httpClient   *http.Client
	pollInterval time.Duration
	activeMu     sync.Mutex
	active       map[string]bool
}

func NewWorkerPool(cfg *config.Config, database *db.DB, broker *events.Broker, mediaEngine *media.MediaEngine) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		cfg:          cfg,
		database:     database,
		broker:       broker,
		media:        mediaEngine,
		queue:        make(chan string, 1000),
		semaphore:    make(chan struct{}, cfg.MaxConcurrentPolls),
		ctx:          ctx,
		cancel:       cancel,
		httpClient:   &http.Client{Timeout: 120 * time.Second},
		pollInterval: 5 * time.Second,
		active:       make(map[string]bool),
	}
}

func (w *WorkerPool) Start() {
	for i := 0; i < w.cfg.WorkerConcurrency; i++ {
		w.wg.Add(1)
		go w.workerLoop(i)
	}
	log.Printf("Started %d render worker goroutines (max %d concurrent polls)", w.cfg.WorkerConcurrency, w.cfg.MaxConcurrentPolls)
}

func (w *WorkerPool) Stop() {
	w.cancel()
	w.wg.Wait()
}

func (w *WorkerPool) Enqueue(jobID string) {
	select {
	case w.queue <- jobID:
	case <-w.ctx.Done():
	}
}

func (w *WorkerPool) ReconcilePending() int {
	jobs, err := w.database.ListPendingRenderJobs()
	if err != nil {
		log.Printf("Error reconciling pending jobs: %v", err)
		return 0
	}
	for _, j := range jobs {
		w.Enqueue(j.ID)
	}
	if len(jobs) > 0 {
		log.Printf("Reconciled %d in-flight render jobs on startup", len(jobs))
	}
	return len(jobs)
}

func (w *WorkerPool) workerLoop(workerID int) {
	defer w.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			return
		case jobID := <-w.queue:
			select {
			case w.semaphore <- struct{}{}:
			case <-w.ctx.Done():
				return
			}
			err := w.processJob(jobID)
			<-w.semaphore
			if err != nil {
				log.Printf("[worker %d] job %s failed: %v", workerID, jobID, err)
			}
		}
	}
}

func (w *WorkerPool) processJob(jobID string) (resultErr error) {
	w.activeMu.Lock()
	if w.active[jobID] {
		w.activeMu.Unlock()
		return nil
	}
	w.active[jobID] = true
	w.activeMu.Unlock()
	defer func() { w.activeMu.Lock(); delete(w.active, jobID); w.activeMu.Unlock() }()
	job, err := w.database.GetRenderJob(jobID)
	if err != nil {
		return err
	}
	if job.Status == "succeeded" || job.Status == "failed" {
		return nil
	}
	ctx, cancel := context.WithTimeout(w.ctx, 30*time.Minute)
	defer cancel()
	update := func(status, stage, progress string) error {
		job.Status = status
		job.Stage = &stage
		job.Progress = &progress
		if err := w.database.UpdateRenderJob(job); err != nil {
			return err
		}
		w.broker.Publish(map[string]any{"type": "job_updated", "job_id": job.ID, "status": status, "stage": stage, "progress": progress, "project_id": job.ProjectID})
		return nil
	}
	defer func() {
		if resultErr != nil && !errors.Is(resultErr, context.Canceled) && w.ctx.Err() == nil {
			message := resultErr.Error()
			job.Error = &message
			if err := update("failed", "failed", message); err != nil {
				log.Printf("persist render failure: %v", err)
			}
		}
	}()
	resolver := routing.Resolver{Config: w.cfg, DB: w.database}
	provider := job.Provider
	if provider == "" {
		provider = "openrouter"
	}
	job.Provider = provider
	client, err := resolver.Client(provider)
	if err != nil {
		return err
	}
	if job.ProviderJobID == nil || *job.ProviderJobID == "" {
		payload := map[string]any{}
		if len(job.RequestJSON) > 0 {
			if err = json.Unmarshal(job.RequestJSON, &payload); err != nil {
				return fmt.Errorf("invalid render request: %w", err)
			}
		}
		// Only fields in the provider contract are forwarded. Local IDs never leave the app.
		allowed := map[string]bool{"prompt": true, "duration": true, "aspect_ratio": true, "resolution": true, "generate_audio": true, "seed": true, "frame_images": true, "input_references": true, "provider": true}
		for key := range payload {
			if !allowed[key] {
				delete(payload, key)
			}
		}
		prompt, _ := payload["prompt"].(string)
		if strings.TrimSpace(prompt) == "" && job.ShotID != nil {
			shot, e := w.database.GetShot(*job.ShotID)
			if e != nil {
				return e
			}
			prompt = shot.Prompt
		}
		if strings.TrimSpace(prompt) == "" && job.SceneID != nil {
			scene, e := w.database.GetScene(*job.SceneID)
			if e != nil {
				return e
			}
			prompt = scene.Title + ". " + scene.Summary
			shots, e := w.database.ListShots(scene.ID)
			if e != nil {
				return e
			}
			for _, shot := range shots {
				prompt += fmt.Sprintf("\nShot %d (%ds): %s", shot.ShotOrder, shot.Duration, shot.Prompt)
			}
		}
		if strings.TrimSpace(prompt) == "" {
			return fmt.Errorf("a video prompt or scene/shot is required")
		}
		negatives := resolver.StringListSetting("render_negatives", []string{"blurry", "low quality", "distorted", "watermark"})
		if len(negatives) > 0 {
			prompt += "\nAvoid: " + strings.Join(negatives, ", ")
		}
		prompt += "\nSpoken dialogue language: " + resolver.Setting("dialogue_language", "English")
		payload["prompt"] = prompt
		if job.Model == "" {
			job.Model = resolver.MediaModel("video")
		}
		payload["model"] = job.Model
		// Local durations may be storyboard timings. Only explicitly supplied generation durations are sent unchanged.
		if _, ok := payload["duration"]; !ok {
			model, e := client.Model(ctx, job.Model, "video")
			if e != nil {
				return e
			}
			target := 4
			if job.SceneID != nil {
				if scene, e := w.database.GetScene(*job.SceneID); e == nil {
					target = scene.Duration
				}
			}
			chosen := 0
			for _, duration := range model.Durations {
				if duration >= target && (chosen == 0 || duration < chosen) {
					chosen = duration
				}
			}
			if chosen == 0 {
				for _, duration := range model.Durations {
					if duration > chosen {
						chosen = duration
					}
				}
			}
			if chosen > 0 {
				payload["duration"] = chosen
			}
		}
		if _, ok := payload["aspect_ratio"]; !ok {
			aspect := resolver.Setting("default_aspect_ratio", "9:16")
			if job.SceneID != nil {
				if scene, e := w.database.GetScene(*job.SceneID); e == nil && scene.AspectRatio != "" {
					aspect = scene.AspectRatio
				}
			}
			payload["aspect_ratio"] = aspect
		}
		// Use a generated storyboard as the first frame only when the selected model supports it.
		if _, explicit := payload["frame_images"]; !explicit && payload["input_references"] == nil && job.SceneID != nil && job.ProjectID != nil {
			model, e := client.Model(ctx, job.Model, "video")
			if e != nil {
				return e
			}
			supports := false
			for _, frame := range model.FrameImages {
				if frame == "first_frame" {
					supports = true
				}
			}
			if supports {
				assets, e := w.database.ListAssets(*job.ProjectID)
				if e != nil {
					return e
				}
				for _, asset := range assets {
					var meta struct {
						SceneID string `json:"scene_id"`
					}
					_ = json.Unmarshal(asset.MetadataJSON, &meta)
					if asset.Type == "storyboard" && meta.SceneID == *job.SceneID {
						image, e := media.ImageDataURL(w.cfg.StorageRoot, asset.FilePath)
						if e != nil {
							return e
						}
						payload["frame_images"] = []map[string]any{{"type": "image_url", "image_url": map[string]string{"url": image}, "frame_type": "first_frame"}}
						break
					}
				}
			}
		}
		for _, key := range []string{"frame_images", "input_references"} {
			if raw, ok := payload[key]; ok {
				data, _ := json.Marshal(raw)
				var refs []map[string]any
				if json.Unmarshal(data, &refs) != nil {
					return fmt.Errorf("invalid %s", key)
				}
				for _, ref := range refs {
					image, ok := ref["image_url"].(map[string]any)
					if !ok || ref["type"] != "image_url" {
						return fmt.Errorf("only image_url references are supported")
					}
					path, _ := image["url"].(string)
					if path == "" {
						return fmt.Errorf("reference URL is required")
					}
					if !strings.HasPrefix(path, "https://") && !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "data:image/") {
						encoded, e := media.ImageDataURL(w.cfg.StorageRoot, path)
						if e != nil {
							return e
						}
						image["url"] = encoded
					}
				}
				payload[key] = refs
			}
		}
		job.RequestJSON, err = json.Marshal(payload)
		if err != nil {
			return err
		}
		if err = update("running", "submitting", "Submitting to OpenRouter"); err != nil {
			return err
		}
		id, e := client.CreateVideo(ctx, payload)
		if e != nil {
			return e
		}
		job.ProviderJobID = &id
		// Save the upstream ID before polling, so restart resumes rather than submits again.
		if err = update("running", "polling", "Waiting for OpenRouter video"); err != nil {
			return err
		}
	}
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		status, urls, e := client.GetVideo(ctx, *job.ProviderJobID)
		if e != nil {
			return e
		}
		switch status {
		case "completed":
			count := len(urls)
			if count == 0 {
				count = 1
			}
			existing, e := w.database.ListOutputsForJob(job.ID)
			if e != nil {
				return e
			}
			if len(existing) >= count {
				return update("succeeded", "done", "Video downloaded")
			}
			if err = update("running", "downloading", "Downloading OpenRouter video"); err != nil {
				return err
			}
			completed := map[string]bool{}
			for _, output := range existing {
				completed[output.VideoPath] = true
			}
			for index := 0; index < count; index++ {
				public := filepath.ToSlash(filepath.Join("storage", "outputs", fmt.Sprintf("%s-%d.mp4", job.ID, index)))
				if completed[public] {
					continue
				}
				data, e := client.VideoContent(ctx, *job.ProviderJobID, index)
				if e != nil {
					return e
				}
				filename := fmt.Sprintf("%s-%d.mp4", job.ID, index)
				path := filepath.Join(w.cfg.StorageRoot, "outputs", filename)
				if e = os.MkdirAll(filepath.Dir(path), 0755); e != nil {
					return e
				}
				if e = os.WriteFile(path+".part", data, 0644); e != nil {
					return e
				}
				if e = os.Rename(path+".part", path); e != nil {
					return e
				}
				output := &models.RenderOutput{RenderJobID: job.ID, VideoPath: filepath.ToSlash(filepath.Join("storage", "outputs", filename)), CaptionsJSON: json.RawMessage(`[]`), QAJSON: json.RawMessage(`{"status":"not_run"}`), Selected: index == 0}
				if e = w.database.CreateRenderOutput(output); e != nil {
					return e
				}
				w.broker.Publish(map[string]any{"type": "job_updated", "job_id": job.ID, "status": "running", "output_id": output.ID, "project_id": job.ProjectID})
			}
			return update("succeeded", "done", "Video downloaded")
		case "failed", "cancelled", "canceled", "expired":
			return fmt.Errorf("OpenRouter video job %s", status)
		case "pending", "queued", "in_progress", "processing", "running":
		default:
			return fmt.Errorf("unknown OpenRouter video status %q", status)
		}
		timer := time.NewTimer(w.pollInterval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
}

func (w *WorkerPool) DownloadFile(url, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	resp, err := w.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
