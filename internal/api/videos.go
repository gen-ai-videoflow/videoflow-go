package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"6eek-go/internal/agents"
	"6eek-go/internal/models"
)

func (s *Server) handleVideoPreflight(w http.ResponseWriter, r *http.Request) {
	aspectRatio := r.URL.Query().Get("aspect_ratio")
	if aspectRatio == "" {
		aspectRatio = s.routes().Setting("default_aspect_ratio", "9:16")
	}
	style := r.URL.Query().Get("style")
	if style == "" {
		style = "2d-picture-book"
	}

	missing := []map[string]string{}
	if _, err := s.routes().Client("openrouter"); err != nil {
		missing = append(missing, map[string]string{
			"route":   "media",
			"setting": "/settings",
			"reason":  "Configure an OpenRouter API key in settings.",
		})
	}
	elseCheck := len(missing) == 0
	if elseCheck {
		client, _ := s.routes().Client("openrouter")
		if err := client.CheckKey(r.Context()); err != nil {
			missing = append(missing, map[string]string{"route": "authentication", "setting": "/settings", "reason": err.Error()})
		} else {
			for kind, model := range map[string]string{"image": s.routes().MediaModel("image"), "video": s.routes().MediaModel("video")} {
				metadata, err := client.Model(r.Context(), model, kind)
				if err != nil {
					missing = append(missing, map[string]string{"route": kind, "setting": "/settings#engines", "reason": err.Error()})
					continue
				}
				if kind == "video" {
					supported := false
					for _, aspect := range metadata.AspectRatios {
						if aspect == aspectRatio {
							supported = true
						}
					}
					if !supported {
						missing = append(missing, map[string]string{"route": "video", "setting": "/settings#engines", "reason": "Selected video model does not support aspect ratio " + aspectRatio})
					}
				}
			}
			if _, _, err := s.routes().Agent(r.Context(), "script_agent"); err != nil {
				missing = append(missing, map[string]string{"route": "text", "setting": "/settings", "reason": err.Error()})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ready":   len(missing) == 0,
		"missing": missing,
		"effective_routes": map[string]string{
			"video_provider": "openrouter",
			"video_model":    s.routes().MediaModel("video"),
			"image_model":    s.routes().MediaModel("image"),
		},
		"style": map[string]string{
			"selected": style,
			"aspect":   aspectRatio,
		},
	})
}

func (s *Server) handleGenerateVideo(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}

	var body struct {
		Idea             string `json:"idea"`
		TargetDuration   int    `json:"target_duration"`
		SceneCount       int    `json:"scene_count"`
		Style            string `json:"style"`
		AspectRatio      string `json:"aspect_ratio"`
		Language         string `json:"language"`
		ConversationMode string `json:"conversation_mode"`
		Instruction      string `json:"instruction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Idea == "" {
		http.Error(w, "idea is required", http.StatusBadRequest)
		return
	}

	if body.SceneCount <= 0 {
		body.SceneCount = 3
	}
	if body.TargetDuration <= 0 {
		body.TargetDuration = body.SceneCount * int(s.routes().NumberSetting("default_scene_duration", 5))
	}
	if body.AspectRatio == "" {
		body.AspectRatio = s.routes().Setting("default_aspect_ratio", "9:16")
	}

	op := &models.Op{
		Kind:      "video_generation",
		Status:    "running",
		ProjectID: &projectID,
	}
	if err := s.database.CreateOp(op); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Run guided video pipeline asynchronously
	s.tasks.Add(1)
	go func(opID, pid string) {
		defer s.tasks.Done()
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
		defer cancel()

		stages := []string{}
		recordStage := func(stage string) {
			stages = append(stages, stage)
			s.broker.Publish(map[string]interface{}{
				"type":       "pipeline_stage",
				"op_id":      opID,
				"stage":      stage,
				"stages":     stages,
				"project_id": pid,
			})
		}

		// Stage 1: Story / Script
		draft, err := s.agents.GenerateScript(agents.WithDialogueLanguage(ctx, body.Language), body.Idea, body.SceneCount, body.TargetDuration, body.AspectRatio)
		if err != nil {
			errStr := err.Error()
			_ = s.database.UpdateOp(opID, "failed", nil, &errStr)
			return
		}
		recordStage("story")

		draftBytes, _ := json.Marshal(draft)
		script := &models.Script{
			ProjectID: &pid,
			Idea:      body.Idea,
			Title:     draft.Title,
			Summary:   draft.Summary,
			DraftJSON: draftBytes,
		}
		if err := s.database.CreateScript(script); err != nil {
			message := err.Error()
			_ = s.database.UpdateOp(opID, "failed", nil, &message)
			return
		}

		// Stage 2: Scenes & Shots
		createdScenes := []models.Scene{}
		for i, sc := range draft.Scenes {
			scene := models.Scene{
				ProjectID:   &pid,
				ScriptID:    &script.ID,
				SceneOrder:  i + 1,
				Title:       sc.Title,
				Summary:     sc.Summary,
				Duration:    sc.Duration,
				AspectRatio: sc.AspectRatio,
			}
			if err := s.database.CreateScene(&scene); err != nil {
				message := err.Error()
				_ = s.database.UpdateOp(opID, "failed", nil, &message)
				return
			}
			createdScenes = append(createdScenes, scene)

			// Generate shots for scene
			shots, err := s.agents.GenerateShots(ctx, scene.Title, scene.Summary, scene.Duration)
			if err != nil {
				message := err.Error()
				_ = s.database.UpdateOp(opID, "failed", nil, &message)
				return
			}
			for _, sh := range shots {
				shot := models.Shot{
					SceneID:   scene.ID,
					ShotOrder: sh.Order,
					Duration:  sh.Duration,
					Prompt:    sh.Prompt,
					Camera:    &sh.Camera,
					Movement:  &sh.Movement,
				}
				if err := s.database.CreateShot(&shot); err != nil {
					message := err.Error()
					_ = s.database.UpdateOp(opID, "failed", nil, &message)
					return
				}
			}
		}
		recordStage("scenes")
		recordStage("shots")

		// Stage 3: Dialogue
		// Dialogue is included in the script/video prompt; no separate speech stage is claimed.

		// Stage 4: Visuals
		for _, scene := range createdScenes {
			_, err := s.generateAssets(ctx, scene.ProjectID, nil, scene.ID, scene.Title+" keyframe", "storyboard", scene.Title+". "+scene.Summary, scene.AspectRatio, nil)
			if err != nil {
				message := err.Error()
				_ = s.database.UpdateOp(opID, "failed", nil, &message)
				return
			}
		}
		recordStage("visuals")

		// Stage 5: Render Jobs
		renderJobIDs := []string{}
		for _, sc := range createdScenes {
			stage := "submitted"
			progress := "Queued for render"
			job := models.RenderJob{
				ProjectID: &pid,
				SceneID:   &sc.ID,
				Provider:  "openrouter",
				Model:     s.routes().MediaModel("video"),
				Status:    "pending",
				Stage:     &stage,
				Progress:  &progress,
			}
			if err := s.database.CreateRenderJob(&job); err != nil {
				message := err.Error()
				_ = s.database.UpdateOp(opID, "failed", nil, &message)
				return
			}
			renderJobIDs = append(renderJobIDs, job.ID)
			s.workers.Enqueue(job.ID)
		}
		recordStage("render")

		sceneIDs := make([]string, len(createdScenes))
		for i, sc := range createdScenes {
			sceneIDs[i] = sc.ID
		}

		result := map[string]interface{}{
			"project_id":     pid,
			"script_id":      script.ID,
			"scene_ids":      sceneIDs,
			"render_job_ids": renderJobIDs,
			"stages":         stages,
			"status":         "rendering",
		}
		resultBytes, _ := json.Marshal(result)
		_ = s.database.UpdateOp(opID, "succeeded", resultBytes, nil)

		log.Printf("Guided video pipeline completed for op %s (%d scenes, %d render jobs)", opID, len(sceneIDs), len(renderJobIDs))
	}(op.ID, projectID)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"op_id":  op.ID,
		"status": op.Status,
	})
}
