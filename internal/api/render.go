package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"6eek-go/internal/models"

	"github.com/go-chi/chi/v5"
)

var defaultCaptionStyles = []string{
	"kids", "cinematic", "neon", "minimal", "bold", "clean", "classic", "comic",
}

func (s *Server) handleStartRender(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}

	var body map[string]any
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		http.Error(w, "invalid request body", 400)
		return
	}
	provider, _ := body["provider"].(string)
	if provider == "" {
		provider = "openrouter"
	}
	if provider != "openrouter" {
		http.Error(w, "Video generation uses OpenRouter", 400)
		return
	}
	model, _ := body["model"].(string)
	if model == "" {
		model = s.routes().MediaModel("video")
	}
	body["model"] = model
	var sceneID, shotID *string
	if value, ok := body["scene_id"].(string); ok {
		sceneID = &value
	}
	if value, ok := body["shot_id"].(string); ok {
		shotID = &value
	}
	delete(body, "provider") // Provider is a local routing field; OpenRouter routing options use provider_options.
	if value, ok := body["provider_options"]; ok {
		body["provider"] = value
		delete(body, "provider_options")
	}

	stage := "submitted"
	progress := "Queued for rendering"
	reqBytes, _ := json.Marshal(body)

	job := &models.RenderJob{
		ProjectID:   &projectID,
		SceneID:     sceneID,
		ShotID:      shotID,
		Provider:    provider,
		Model:       model,
		Status:      "pending",
		Stage:       &stage,
		Progress:    &progress,
		RequestJSON: reqBytes,
	}

	if err := s.database.CreateRenderJob(job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.workers.Enqueue(job.ID)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"job_id": job.ID,
		"status": job.Status,
	})
}

func (s *Server) handleRenderFromShot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SceneID string `json:"scene_id"`
		ShotID  string `json:"shot_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	shot, err := s.database.GetShot(body.ShotID)
	if err != nil {
		http.Error(w, "shot not found", http.StatusNotFound)
		return
	}

	projectID, _ := s.database.GetActiveProjectID()
	stage := "submitted"
	progress := "Queued from shot"
	reqBytes, _ := json.Marshal(map[string]any{"prompt": shot.Prompt})

	job := &models.RenderJob{
		ProjectID:   &projectID,
		SceneID:     &body.SceneID,
		ShotID:      &body.ShotID,
		Provider:    "openrouter",
		Model:       s.routes().MediaModel("video"),
		Status:      "pending",
		Stage:       &stage,
		Progress:    &progress,
		RequestJSON: reqBytes,
	}

	if err := s.database.CreateRenderJob(job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.workers.Enqueue(job.ID)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"job_id": job.ID,
		"status": job.Status,
	})
}

func (s *Server) handleListRenderJobs(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	jobs, err := s.database.ListRenderJobs(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if jobs == nil {
		jobs = []models.RenderJob{}
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) handleGetRenderJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := s.database.GetRenderJob(id)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	outputs, _ := s.database.ListOutputsForJob(id)
	if outputs == nil {
		outputs = []models.RenderOutput{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"job":     job,
		"outputs": outputs,
	})
}

func (s *Server) handleResubmitJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.database.GetRenderJob(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "job not found", 404)
		return
	}
	// Explicit retry starts a new generation; only restart reconciliation reuses the upstream ID.
	stage, progress := "submitted", "Retry queued"
	if job.Provider != "openrouter" {
		job.Model = s.routes().MediaModel("video")
	}
	fresh := &models.RenderJob{ProjectID: job.ProjectID, SceneID: job.SceneID, ShotID: job.ShotID, Provider: "openrouter", Model: job.Model, Status: "pending", Stage: &stage, Progress: &progress, RequestJSON: job.RequestJSON}
	if err = s.database.CreateRenderJob(fresh); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.workers.Enqueue(fresh.ID)
	writeJSON(w, 202, map[string]any{"job_id": fresh.ID, "status": fresh.Status})
}

func (s *Server) handleGetEditorData(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	output, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		http.Error(w, "output not found", 404)
		return
	}

	// 1. Output block
	qaIssues := []string{}
	if len(output.QAJSON) > 0 {
		var qaData struct {
			Issues   []string `json:"issues"`
			QAIssues []string `json:"qa_issues"`
		}
		if err := json.Unmarshal(output.QAJSON, &qaData); err == nil {
			if len(qaData.Issues) > 0 {
				qaIssues = qaData.Issues
			} else if len(qaData.QAIssues) > 0 {
				qaIssues = qaData.QAIssues
			}
		}
	}

	outputBlock := map[string]interface{}{
		"id":             output.ID,
		"video_path":     output.VideoPath,
		"captioned_path": output.CaptionedPath,
		"thumbnail_path": output.ThumbnailPath,
		"score":          output.Score,
		"qa_issues":      qaIssues,
	}

	// 2. Captions block
	capSegments := []models.CaptionSegment{}
	capStyle := "clean"

	if len(output.CaptionsJSON) > 0 {
		var storedCaps struct {
			Segments []models.CaptionSegment `json:"segments"`
			Style    string                  `json:"style"`
		}
		if err := json.Unmarshal(output.CaptionsJSON, &storedCaps); err == nil && (storedCaps.Segments != nil || storedCaps.Style != "") {
			if storedCaps.Segments != nil {
				capSegments = storedCaps.Segments
			}
			if storedCaps.Style != "" {
				capStyle = storedCaps.Style
			}
		} else {
			var arrSegs []models.CaptionSegment
			if err := json.Unmarshal(output.CaptionsJSON, &arrSegs); err == nil && arrSegs != nil {
				capSegments = arrSegs
			}
		}
	}

	captionsBlock := map[string]interface{}{
		"segments":  capSegments,
		"style":     capStyle,
		"available": len(capSegments) > 0,
	}

	// 3. Scene + Shots blocks
	var sceneBlock map[string]interface{}
	shotsBlock := []map[string]interface{}{}
	totalDuration := 5.0

	var job *models.RenderJob
	if output.RenderJobID != "" {
		job, _ = s.database.GetRenderJob(output.RenderJobID)
	}

	var spec struct {
		SceneID     *string `json:"scene_id"`
		Duration    int     `json:"duration"`
		Prompt      string  `json:"prompt"`
		MultiPrompt []struct {
			Index    int     `json:"index"`
			Duration float64 `json:"duration"`
			Prompt   string  `json:"prompt"`
		} `json:"multi_prompt"`
	}

	if job != nil && len(job.RequestJSON) > 0 {
		var reqData struct {
			Spec struct {
				SceneID     *string `json:"scene_id"`
				Duration    int     `json:"duration"`
				Prompt      string  `json:"prompt"`
				MultiPrompt []struct {
					Index    int     `json:"index"`
					Duration float64 `json:"duration"`
					Prompt   string  `json:"prompt"`
				} `json:"multi_prompt"`
			} `json:"spec"`
		}
		_ = json.Unmarshal(job.RequestJSON, &reqData)
		spec = reqData.Spec
		if spec.Duration == 0 {
			_ = json.Unmarshal(job.RequestJSON, &spec)
		}
	}

	var sceneID *string
	if spec.SceneID != nil {
		sceneID = spec.SceneID
	} else if job != nil && job.SceneID != nil {
		sceneID = job.SceneID
	}

	var liveShots []models.Shot
	if sceneID != nil {
		if scene, err := s.database.GetScene(*sceneID); err == nil && scene != nil {
			sceneBlock = map[string]interface{}{
				"id":    scene.ID,
				"title": scene.Title,
			}
			liveShots, _ = s.database.ListShots(scene.ID)
		}
	}

	if len(spec.MultiPrompt) > 0 {
		cursor := 0.0
		for i, entry := range spec.MultiPrompt {
			dur := entry.Duration
			if dur <= 0 {
				dur = 3.0
			}
			start := cursor
			end := cursor + dur
			idx := entry.Index
			if idx == 0 {
				idx = i + 1
			}
			var shotID, camera, movement *string
			if i < len(liveShots) {
				shotID = &liveShots[i].ID
				camera = liveShots[i].Camera
				movement = liveShots[i].Movement
			}
			shotsBlock = append(shotsBlock, map[string]interface{}{
				"index":    idx,
				"start":    start,
				"end":      end,
				"duration": dur,
				"prompt":   entry.Prompt,
				"shot_id":  shotID,
				"camera":   camera,
				"movement": movement,
			})
			cursor = end
		}
		totalDuration = cursor
	} else if len(liveShots) > 0 {
		cursor := 0.0
		for i, ls := range liveShots {
			dur := float64(ls.Duration)
			if dur <= 0 {
				dur = 3.0
			}
			start := cursor
			end := cursor + dur
			shotsBlock = append(shotsBlock, map[string]interface{}{
				"index":    i + 1,
				"start":    start,
				"end":      end,
				"duration": dur,
				"prompt":   ls.Prompt,
				"shot_id":  &ls.ID,
				"camera":   ls.Camera,
				"movement": ls.Movement,
			})
			cursor = end
		}
		totalDuration = cursor
	} else {
		specDur := float64(spec.Duration)
		if specDur <= 0 {
			specDur = 5.0
		}
		totalDuration = specDur
		shotsBlock = append(shotsBlock, map[string]interface{}{
			"index":    1,
			"start":    0.0,
			"end":      totalDuration,
			"duration": totalDuration,
			"prompt":   spec.Prompt,
			"shot_id":  nil,
			"camera":   nil,
			"movement": nil,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"output":         outputBlock,
		"captions":       captionsBlock,
		"shots":          shotsBlock,
		"scene":          sceneBlock,
		"total_duration": totalDuration,
	})
}

func (s *Server) handleRetryOutput(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	output, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		http.Error(w, "output not found", http.StatusNotFound)
		return
	}

	var reqJSON json.RawMessage
	var provider = "openrouter"
	var model = s.routes().MediaModel("video")
	var sceneID *string

	if origJob, err := s.database.GetRenderJob(output.RenderJobID); err == nil && origJob != nil {
		reqJSON = origJob.RequestJSON
		if origJob.Provider == "openrouter" {
			model = origJob.Model
		}
		sceneID = origJob.SceneID
	}
	if len(reqJSON) == 0 {
		reqJSON = []byte(fmt.Sprintf(`{"output_id":"%s"}`, outputID))
	}

	projectID, _ := s.database.GetActiveProjectID()
	stage := "submitted"
	progress := "Queued for corrective re-render"
	retryJob := &models.RenderJob{
		ProjectID:   &projectID,
		SceneID:     sceneID,
		Provider:    provider,
		Model:       model,
		Status:      "pending",
		Stage:       &stage,
		Progress:    &progress,
		RequestJSON: reqJSON,
	}
	if err := s.database.CreateRenderJob(retryJob); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.workers.Enqueue(retryJob.ID)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"job_id": retryJob.ID,
		"status": retryJob.Status,
	})
}

func (s *Server) handleSelectOutput(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	var body struct {
		Selected bool `json:"selected"`
	}
	body.Selected = true
	_ = json.NewDecoder(r.Body).Decode(&body)

	_ = s.database.SelectRenderOutput(outputID, body.Selected)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":       outputID,
		"selected": body.Selected,
	})
}

func (s *Server) handleRunQA(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	output, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		http.Error(w, "output not found", 404)
		return
	}
	s.operation(w, r, &models.Op{Kind: "qa", OutputID: &outputID}, func(ctx context.Context) (any, error) {
		path, err := s.storagePath(output.VideoPath)
		if err != nil {
			return nil, err
		}
		directory, err := os.MkdirTemp(s.cfg.StorageRoot, "qa-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(directory)
		frames := []string{}
		for index, second := range []float64{0, 1, 2} {
			frame := filepath.Join(directory, fmt.Sprintf("frame-%d.jpg", index))
			if err = s.media.MakeThumbnail(ctx, path, frame, second); err != nil {
				return nil, fmt.Errorf("extract QA frame: %w", err)
			}
			image, err := s.imageURL(&models.Asset{FilePath: frame})
			if err != nil {
				return nil, err
			}
			frames = append(frames, image)
		}
		var result struct {
			Score          *float64 `json:"score"`
			Issues         []string `json:"qa_issues"`
			Recommendation string   `json:"recommendation"`
		}
		prompt := "Evaluate these sampled video frames for visual consistency, artifacts, composition and adherence to the requested scene. Score from 0 to 10; do not claim to assess audio or motion from still frames."
		if job, err := s.database.GetRenderJob(output.RenderJobID); err == nil {
			prompt += "\nRender request: " + string(job.RequestJSON)
		}
		if err = s.agentJSON(ctx, "qa_agent", "You are a film quality reviewer. Return score, qa_issues, recommendation.", prompt, frames, &result); err != nil {
			return nil, err
		}
		if result.Score == nil || *result.Score < 0 || *result.Score > 10 {
			return nil, fmt.Errorf("OpenRouter returned an invalid QA score")
		}
		if result.Issues == nil {
			result.Issues = []string{}
		}
		output.Score = result.Score
		output.QAJSON, _ = json.Marshal(result)
		if err = s.database.UpdateRenderOutput(output); err != nil {
			return nil, err
		}
		return map[string]any{"id": output.ID, "output_id": output.ID, "score": result.Score, "qa_issues": result.Issues, "recommendation": result.Recommendation, "assessment": "sampled_frames"}, nil
	})
}

func (s *Server) handleCaptionStyles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, defaultCaptionStyles)
}

func (s *Server) handleCaptionConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"styles":           defaultCaptionStyles,
		"models":           []string{"openai/whisper-1", "openai/whisper-large-v3", "openai/whisper-large-v3-turbo"},
		"default_model":    s.routes().Setting("whisper_model", "openai/whisper-1"),
		"default_language": "auto",
		"default_style":    "clean",
	})
}

func (s *Server) handleGetCaptions(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	output, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"segments":  []models.CaptionSegment{},
			"style":     "clean",
			"available": false,
		})
		return
	}

	capSegments := []models.CaptionSegment{}
	capStyle := "clean"

	if len(output.CaptionsJSON) > 0 {
		var storedCaps struct {
			Segments []models.CaptionSegment `json:"segments"`
			Style    string                  `json:"style"`
		}
		if err := json.Unmarshal(output.CaptionsJSON, &storedCaps); err == nil && (storedCaps.Segments != nil || storedCaps.Style != "") {
			if storedCaps.Segments != nil {
				capSegments = storedCaps.Segments
			}
			if storedCaps.Style != "" {
				capStyle = storedCaps.Style
			}
		} else {
			var arrSegs []models.CaptionSegment
			if err := json.Unmarshal(output.CaptionsJSON, &arrSegs); err == nil && arrSegs != nil {
				capSegments = arrSegs
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"segments":  capSegments,
		"style":     capStyle,
		"available": len(capSegments) > 0,
	})
}

func (s *Server) handleUpdateCaptions(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	output, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		http.Error(w, "output not found", 404)
		return
	}
	var body struct {
		Segments []models.CaptionSegment `json:"segments"`
		Style    string                  `json:"style"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		http.Error(w, "invalid captions", 400)
		return
	}
	for _, segment := range body.Segments {
		if segment.Start < 0 || segment.End <= segment.Start {
			http.Error(w, "invalid caption timestamps", 400)
			return
		}
	}
	if body.Style == "" {
		body.Style = "clean"
	}
	s.operation(w, r, &models.Op{Kind: "caption", OutputID: &outputID}, func(ctx context.Context) (any, error) {
		output.CaptionsJSON, _ = json.Marshal(body)
		output.CaptionedPath = nil
		if err := s.database.UpdateRenderOutput(output); err != nil {
			return nil, err
		}
		return map[string]string{"output_id": outputID, "status": "saved"}, nil
	})
}
func (s *Server) handleTranscribeCaptions(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	output, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		http.Error(w, "output not found", 404)
		return
	}
	var body struct {
		Model    string `json:"model"`
		Language string `json:"language"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Model == "" {
		body.Model = s.routes().Setting("whisper_model", "openai/whisper-1")
	}
	if body.Language == "" {
		body.Language = s.routes().Setting("caption_language", "auto")
	}
	s.operation(w, r, &models.Op{Kind: "caption", OutputID: &outputID}, func(ctx context.Context) (any, error) {
		client, err := s.routes().Client("openrouter")
		if err != nil {
			return nil, err
		}
		path, err := s.storagePath(output.VideoPath)
		if err != nil {
			return nil, err
		}
		dir, err := os.MkdirTemp(s.cfg.StorageRoot, "transcribe-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		audioPath := filepath.Join(dir, "audio.wav")
		if err = s.media.ExtractAudio(ctx, path, audioPath); err != nil {
			return nil, err
		}
		audio, err := os.ReadFile(audioPath)
		if err != nil {
			return nil, err
		}
		if len(audio) > 25<<20 {
			return nil, fmt.Errorf("audio exceeds 25 MB; transcribe a shorter video")
		}
		transcript, err := client.Transcribe(ctx, body.Model, audio, "wav", body.Language)
		if err != nil {
			return nil, err
		}
		segments := []models.CaptionSegment{}
		for _, segment := range transcript.Segments {
			segments = append(segments, models.CaptionSegment{Start: segment.Start, End: segment.End, Text: segment.Text})
		}
		output.CaptionsJSON, _ = json.Marshal(map[string]any{"segments": segments, "style": s.routes().Setting("caption_style", "clean")})
		output.CaptionedPath = nil
		if err = s.database.UpdateRenderOutput(output); err != nil {
			return nil, err
		}
		return map[string]any{"output_id": output.ID, "segments": segments, "text": transcript.Text}, nil
	})
}
func (s *Server) handleCaptionOutput(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	output, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		http.Error(w, "output not found", 404)
		return
	}
	s.operation(w, r, &models.Op{Kind: "caption", OutputID: &outputID}, func(ctx context.Context) (any, error) {
		var captions struct {
			Segments []models.CaptionSegment `json:"segments"`
			Style    string                  `json:"style"`
		}
		if err := json.Unmarshal(output.CaptionsJSON, &captions); err != nil {
			_ = json.Unmarshal(output.CaptionsJSON, &captions.Segments)
		}
		path, err := s.storagePath(output.VideoPath)
		if err != nil {
			return nil, err
		}
		name := output.ID + "-captioned.mp4"
		dest := filepath.Join(s.cfg.StorageRoot, "outputs", name)
		if err = s.media.BurnCaptions(ctx, path, dest, captions.Segments, captions.Style); err != nil {
			return nil, err
		}
		public := filepath.ToSlash(filepath.Join("storage", "outputs", name))
		output.CaptionedPath = &public
		if err = s.database.UpdateRenderOutput(output); err != nil {
			return nil, err
		}
		return map[string]string{"output_id": output.ID, "status": "burned", "captioned_path": public}, nil
	})
}

func (s *Server) handleDownloadOutput(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	output, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		http.Error(w, "output not found", 404)
		return
	}
	path := output.VideoPath
	if r.URL.Query().Get("variant") == "captioned" {
		if output.CaptionedPath == nil {
			http.Error(w, "captioned video has not been generated", 404)
			return
		}
		path = *output.CaptionedPath
	}
	full, err := s.storagePath(path)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if _, err = os.Stat(full); err != nil {
		http.Error(w, "video file not found", 404)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="output-%s.mp4"`, outputID))
	http.ServeFile(w, r, full)
}
