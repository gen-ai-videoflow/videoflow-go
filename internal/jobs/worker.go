package jobs

import (
	"6eek-go/internal/providers"
	"6eek-go/internal/routing"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	if job.Status == "succeeded" || job.Status == "failed" || job.Status == "reconciliation_required" {
		return nil
	}
	if (job.ProviderJobID == nil || *job.ProviderJobID == "") && job.Stage != nil && *job.Stage == "submitting" {
		job.Status = "reconciliation_required"
		message := "Provider submission outcome is unknown. Reconcile the existing job before starting another paid generation."
		job.Error = &message
		return w.database.UpdateRenderJob(job)
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
		if resultErr != nil && !errors.Is(resultErr, context.Canceled) && w.ctx.Err() == nil && job.Status != "reconciliation_required" {
			message := resultErr.Error()
			job.Error = &message
			status, stage := "failed", "failed"
			if job.ProviderJobID != nil && *job.ProviderJobID != "" && job.Status != "failed" {
				status, stage = "reconciliation_required", "polling"
			}
			if err := update(status, stage, message); err != nil {
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
		language := resolver.Setting("dialogue_language", "English")
		if job.SceneID != nil {
			if scene, e := w.database.GetScene(*job.SceneID); e == nil {
				// Explicit tags below preserve the persisted brief across later settings changes.
				var raw map[string]map[string]any
				json.Unmarshal(scene.SceneJSON, &raw)
				brief := raw["generation_brief"]
				if value, ok := brief["language"].(string); ok && value != "" {
					language = value
				}
				for _, key := range []string{"style", "instruction", "conversation_mode", "character_notes"} {
					if value, ok := brief[key].(string); ok && value != "" {
						prompt += "\n" + key + ": " + value
					}
				}
			}
		}
		prompt += "\nSpoken dialogue language: " + language
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
			if job.ShotID != nil {
				if shot, e := w.database.GetShot(*job.ShotID); e == nil {
					target = shot.Duration
				}
			}
			chosen := 0
			for _, duration := range model.Durations {
				if duration >= target && (chosen == 0 || duration < chosen) {
					chosen = duration
				}
			}
			if chosen == 0 {
				return fmt.Errorf("planned clip duration %ds exceeds or cannot be verified against this model's supported durations; shorten the scene or choose a suitable model", target)
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
			if !supports {
				if scene, e := w.database.GetScene(*job.SceneID); e == nil {
					var brief map[string]map[string]json.RawMessage
					json.Unmarshal(scene.SceneJSON, &brief)
					var refs []string
					json.Unmarshal(brief["generation_brief"]["reference_asset_ids"], &refs)
					if len(refs) > 0 {
						return fmt.Errorf("selected video model cannot use the character-conditioned keyframe; select a first-frame model")
					}
				}
			}
			if supports {
				assets, e := w.database.ListAssets(*job.ProjectID)
				if e != nil {
					return e
				}
				approved := map[string]bool{}
				if scene, e := w.database.GetScene(*job.SceneID); e == nil {
					var ids []string
					json.Unmarshal(scene.AssetIDsJSON, &ids)
					for _, id := range ids {
						approved[id] = true
					}
				}
				for _, asset := range assets {
					var meta struct {
						SceneID string `json:"scene_id"`
					}
					_ = json.Unmarshal(asset.MetadataJSON, &meta)
					if asset.Type == "storyboard" && meta.SceneID == *job.SceneID && approved[asset.ID] {
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
		if err = client.ValidateVideo(ctx, payload); err != nil {
			return err
		}
		claimed, e := w.database.ClaimRenderSubmission(job.ID, job.Model, job.RequestJSON)
		if e != nil {
			return e
		}
		if !claimed {
			return nil
		}
		job.Status = "running"
		stage := "submitting"
		job.Stage = &stage

		id, e := client.CreateVideo(ctx, payload)
		if e != nil {
			var rejected *providers.HTTPError
			if errors.As(e, &rejected) && rejected.Status >= 400 && rejected.Status < 500 && rejected.Status != 408 && rejected.Status != 409 {
				return e
			}
			job.Status = "reconciliation_required"
			message := "Submission outcome needs review: " + e.Error()
			job.Error = &message
			if saveErr := w.database.UpdateRenderJob(job); saveErr != nil {
				return saveErr
			}
			return e
		}
		job.ProviderJobID = &id
		// Save the upstream ID before polling, so restart resumes rather than submits again.
		if err = update("running", "polling", "Waiting for OpenRouter video"); err != nil {
			return err
		}
	}
	pollFailures := 0
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		status, urls, e := client.GetVideo(ctx, *job.ProviderJobID)
		if e != nil {
			if status == "failed" {
				job.Status = "failed"
				return e
			}
			pollFailures++
			if pollFailures >= 12 {
				return e
			}
			var rejected *providers.HTTPError
			if errors.As(e, &rejected) && rejected.Status >= 400 && rejected.Status < 500 && rejected.Status != 408 && rejected.Status != 429 {
				return e
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(w.pollInterval):
				continue
			}
		}
		pollFailures = 0
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
			job.Status = "failed"
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
