package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"6eek-go/internal/models"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListScenes(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	scenes, err := s.database.ListScenes(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if scenes == nil {
		scenes = []models.Scene{}
	}
	writeJSON(w, http.StatusOK, scenes)
}

func (s *Server) handleGetScene(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(id)
	if err != nil {
		http.Error(w, "scene not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, scene)
}

func (s *Server) handleCreateScene(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	var scene models.Scene
	if err := json.NewDecoder(r.Body).Decode(&scene); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	scene.ProjectID = &projectID
	if scene.Duration <= 0 {
		scene.Duration = int(s.routes().NumberSetting("default_scene_duration", 5))
	}
	if scene.AspectRatio == "" {
		scene.AspectRatio = s.routes().Setting("default_aspect_ratio", "9:16")
	}
	if err := s.database.CreateScene(&scene); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, scene)
}

func (s *Server) handleUpdateScene(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(id)
	if err != nil {
		http.Error(w, "scene not found", http.StatusNotFound)
		return
	}

	// Snapshot revision before update
	oldSnap, _ := json.Marshal(scene)
	_, _ = s.database.CreateRevision("scene", id, oldSnap)

	if err := json.NewDecoder(r.Body).Decode(scene); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	scene.ID = id
	if err := s.database.UpdateScene(scene); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, scene)
}

func (s *Server) handleDeleteScene(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.database.DeleteScene(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"deleted": id, "shots_deleted": 0})
}

func (s *Server) handleListShots(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	shots, err := s.database.ListShots(sceneID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if shots == nil {
		shots = []models.Shot{}
	}
	writeJSON(w, http.StatusOK, shots)
}

func (s *Server) handleCreateShot(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	var shot models.Shot
	if err := json.NewDecoder(r.Body).Decode(&shot); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	shot.SceneID = sceneID
	if shot.Duration <= 0 {
		shot.Duration = 5
	}
	if err := s.database.CreateShot(&shot); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, shot)
}

func (s *Server) handleUpdateShot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	shot, err := s.database.GetShot(id)
	if err != nil {
		http.Error(w, "shot not found", http.StatusNotFound)
		return
	}

	// Snapshot revision before update
	oldSnap, _ := json.Marshal(shot)
	_, _ = s.database.CreateRevision("shot", id, oldSnap)

	if err := json.NewDecoder(r.Body).Decode(shot); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	shot.ID = id
	if err := s.database.UpdateShot(shot); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, shot)
}

func (s *Server) handleDeleteShot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.database.DeleteShot(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}

func (s *Server) handleExpandScene(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		http.Error(w, "scene not found", http.StatusNotFound)
		return
	}

	shots, err := s.agents.GenerateShots(r.Context(), scene.Title, scene.Summary, scene.Duration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	createdShots := []models.Shot{}
	for _, sh := range shots {
		shot := models.Shot{
			SceneID:   scene.ID,
			ShotOrder: sh.Order,
			Duration:  sh.Duration,
			Prompt:    sh.Prompt,
			Camera:    &sh.Camera,
			Movement:  &sh.Movement,
		}
		_ = s.database.CreateShot(&shot)
		createdShots = append(createdShots, shot)
	}
	writeJSON(w, http.StatusOK, createdShots)
}

func (s *Server) handleGenerateShots(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		http.Error(w, "scene not found", 404)
		return
	}
	s.operation(w, r, &models.Op{Kind: "shots", SceneID: &sceneID, ProjectID: scene.ProjectID}, func(ctx context.Context) (any, error) {
		drafts, err := s.agents.GenerateShots(ctx, scene.Title, scene.Summary, scene.Duration)
		if err != nil {
			return nil, err
		}
		shots := []models.Shot{}
		for _, draft := range drafts {
			camera, movement := draft.Camera, draft.Movement
			shot := models.Shot{SceneID: scene.ID, ShotOrder: draft.Order, Duration: draft.Duration, Prompt: draft.Prompt, Camera: &camera, Movement: &movement}
			if err = s.database.CreateShot(&shot); err != nil {
				return nil, err
			}
			shots = append(shots, shot)
		}
		return map[string]any{"scene_id": scene.ID, "shots": shots}, nil
	})
}

func (s *Server) handleReorderShots(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	var body struct {
		OrderedIDs []string `json:"ordered_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.database.ReorderShots(sceneID, body.OrderedIDs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	shots, _ := s.database.ListShots(sceneID)
	if shots == nil {
		shots = []models.Shot{}
	}
	writeJSON(w, http.StatusOK, shots)
}

func (s *Server) handleRefineScene(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		http.Error(w, "scene not found", http.StatusNotFound)
		return
	}

	var body struct {
		Instruction string `json:"instruction"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if body.Instruction == "" {
		http.Error(w, "instruction is required", 400)
		return
	}
	var result struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
	}
	if err = s.agentJSON(r.Context(), "refine_agent", "Refine the scene according to the instruction. Return title and summary; preserve the scene's intent.", fmt.Sprintf("Title: %s\nSummary: %s\nInstruction: %s", scene.Title, scene.Summary, body.Instruction), nil, &result); err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	if result.Title == "" || result.Summary == "" {
		http.Error(w, "OpenRouter returned incomplete refined scene", 502)
		return
	}
	scene.Title = result.Title
	scene.Summary = result.Summary
	if err = s.database.UpdateScene(scene); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"scene": scene,
		"note":  "Refined scene fields based on instruction",
	})
}

func (s *Server) handleRefineShot(w http.ResponseWriter, r *http.Request) {
	shotID := chi.URLParam(r, "id")
	shot, err := s.database.GetShot(shotID)
	if err != nil {
		http.Error(w, "shot not found", http.StatusNotFound)
		return
	}

	var body struct {
		Instruction string `json:"instruction"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if body.Instruction == "" {
		http.Error(w, "instruction is required", 400)
		return
	}
	var result struct {
		Prompt string `json:"prompt"`
	}
	if err = s.agentJSON(r.Context(), "refine_agent", "Refine the video shot prompt according to the instruction. Return prompt.", shot.Prompt+"\nInstruction: "+body.Instruction, nil, &result); err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	if result.Prompt == "" {
		http.Error(w, "OpenRouter returned an empty shot prompt", 502)
		return
	}
	shot.Prompt = result.Prompt
	if err = s.database.UpdateShot(shot); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"shot": shot,
		"note": "Refined shot prompt based on instruction",
	})
}

func (s *Server) handleRenderScene(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		http.Error(w, "scene not found", http.StatusNotFound)
		return
	}

	projectID, _ := s.database.GetActiveProjectID()
	stage := "submitted"
	progress := "Whole scene render queued"
	job := &models.RenderJob{
		ProjectID:   &projectID,
		SceneID:     &scene.ID,
		Provider:    "openrouter",
		Model:       s.routes().MediaModel("video"),
		Status:      "pending",
		Stage:       &stage,
		Progress:    &progress,
		RequestJSON: []byte(fmt.Sprintf(`{"scene_id":"%s"}`, scene.ID)),
	}
	_ = s.database.CreateRenderJob(job)
	s.workers.Enqueue(job.ID)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"job_id": job.ID,
		"status": job.Status,
	})
}

func (s *Server) handleStoryboard(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		http.Error(w, "scene not found", 404)
		return
	}
	s.operation(w, r, &models.Op{Kind: "storyboard", SceneID: &sceneID, ProjectID: scene.ProjectID}, func(ctx context.Context) (any, error) {
		refs, err := s.sceneReferences(scene)
		if err != nil {
			return nil, err
		}
		assets, err := s.generateAssets(ctx, scene.ProjectID, nil, scene.ID, scene.Title+" storyboard", "storyboard", "Storyboard keyframe of "+scene.Title+". "+scene.Summary, scene.AspectRatio, refs)
		if err != nil {
			return nil, err
		}
		ids := []string{}
		_ = json.Unmarshal(scene.AssetIDsJSON, &ids)
		for _, asset := range assets {
			ids = append(ids, asset.ID)
		}
		scene.AssetIDsJSON, _ = json.Marshal(ids)
		if err = s.database.UpdateScene(scene); err != nil {
			return nil, err
		}
		return map[string]any{"scene_id": scene.ID, "asset_id": assets[0].ID, "storyboard_url": "/" + assets[0].FilePath}, nil
	})
}

func (s *Server) handlePlanSceneAssets(w http.ResponseWriter, r *http.Request) {
	scene, err := s.database.GetScene(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "scene not found", 404)
		return
	}
	var body struct {
		Instruction string `json:"instruction"`
		MaxAssets   int    `json:"max_assets"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	plan, err := s.planAssets(r.Context(), scene, body.Instruction, body.MaxAssets)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	writeJSON(w, 200, plan)
}
func (s *Server) handleGenerateSceneAssets(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		http.Error(w, "scene not found", 404)
		return
	}
	var body struct {
		Instruction string `json:"instruction"`
		MaxAssets   int    `json:"max_assets"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.operation(w, r, &models.Op{Kind: "assets", SceneID: &sceneID, ProjectID: scene.ProjectID}, func(ctx context.Context) (any, error) {
		plan, err := s.planAssets(ctx, scene, body.Instruction, body.MaxAssets)
		if err != nil {
			return nil, err
		}
		assets := []models.Asset{}
		ids := []string{}
		existing := []string{}
		_ = json.Unmarshal(scene.AssetIDsJSON, &existing)
		for _, item := range plan.Assets {
			generated, err := s.generateAssets(ctx, scene.ProjectID, nil, scene.ID, item.Name, item.Type, item.Description, "1:1", nil)
			if err != nil {
				return nil, err
			}
			assets = append(assets, generated...)
			for _, asset := range generated {
				ids = append(ids, asset.ID)
				existing = append(existing, asset.ID)
			}
		}
		scene.AssetIDsJSON, _ = json.Marshal(existing)
		if err = s.database.UpdateScene(scene); err != nil {
			return nil, err
		}
		return map[string]any{"scene_id": scene.ID, "asset_ids": ids, "assets": assets}, nil
	})
}

func (s *Server) sceneReferences(scene *models.Scene) ([]string, error) {
	characterIDs := []string{}
	_ = json.Unmarshal(scene.CharacterIDsJSON, &characterIDs)
	refs := []string{}
	for _, id := range characterIDs {
		character, err := s.database.GetCharacter(id)
		if err != nil {
			return nil, err
		}
		ids := []string{}
		_ = json.Unmarshal(character.ReferenceAssetIDsJSON, &ids)
		if len(ids) > 0 {
			asset, err := s.database.GetAsset(ids[0])
			if err != nil {
				return nil, err
			}
			image, err := s.imageURL(asset)
			if err != nil {
				return nil, err
			}
			refs = append(refs, image)
		}
	}
	return refs, nil
}

func (s *Server) handleCastMemberAdd(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	characterID := chi.URLParam(r, "character_id")
	if err := s.database.AddSceneCast(sceneID, characterID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scene, _ := s.database.GetScene(sceneID)
	writeJSON(w, http.StatusOK, scene)
}

func (s *Server) handleCastMemberDelete(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	characterID := chi.URLParam(r, "character_id")
	if err := s.database.RemoveSceneCast(sceneID, characterID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scene, _ := s.database.GetScene(sceneID)
	writeJSON(w, http.StatusOK, scene)
}

func (s *Server) handleShotAssetAttach(w http.ResponseWriter, r *http.Request) {
	shotID := chi.URLParam(r, "id")
	assetID := chi.URLParam(r, "asset_id")
	if err := s.database.AttachShotAsset(shotID, assetID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	shot, _ := s.database.GetShot(shotID)
	writeJSON(w, http.StatusOK, shot)
}

func (s *Server) handleShotAssetDetach(w http.ResponseWriter, r *http.Request) {
	shotID := chi.URLParam(r, "id")
	assetID := chi.URLParam(r, "asset_id")
	if err := s.database.DetachShotAsset(shotID, assetID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	shot, _ := s.database.GetShot(shotID)
	writeJSON(w, http.StatusOK, shot)
}

func (s *Server) handleConversationalize(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var body struct {
		SceneIDs     []string `json:"scene_ids"`
		Instruction  string   `json:"instruction"`
		IncludeShots bool     `json:"include_shots"`
		Preview      bool     `json:"preview"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.operation(w, r, &models.Op{Kind: "conversation", ProjectID: &projectID}, func(ctx context.Context) (any, error) {
		ids := body.SceneIDs
		if len(ids) == 0 {
			scenes, err := s.database.ListScenes(projectID)
			if err != nil {
				return nil, err
			}
			for _, scene := range scenes {
				ids = append(ids, scene.ID)
			}
		}
		updated := []models.Scene{}
		for _, id := range ids {
			scene, err := s.database.GetScene(id)
			if err != nil {
				return nil, err
			}
			if scene.ProjectID == nil || *scene.ProjectID != projectID {
				return nil, fmt.Errorf("scene is outside the active project")
			}
			var result struct {
				Summary string `json:"summary"`
			}
			err = s.agentJSON(ctx, "scene_agent", "Rewrite this scene to include natural character dialogue and clear staging. Preserve its events and duration. Return summary including dialogue.", scene.Title+"\n"+scene.Summary+"\nInstruction: "+body.Instruction, nil, &result)
			if err != nil {
				return nil, err
			}
			if result.Summary == "" {
				return nil, fmt.Errorf("OpenRouter returned no conversational scene")
			}
			scene.Summary = result.Summary
			if !body.Preview {
				if err = s.database.UpdateScene(scene); err != nil {
					return nil, err
				}
			}
			if body.IncludeShots && !body.Preview {
				drafts, err := s.agents.GenerateShots(ctx, scene.Title, scene.Summary, scene.Duration)
				if err != nil {
					return nil, err
				}
				for _, draft := range drafts {
					camera, movement := draft.Camera, draft.Movement
					shot := &models.Shot{SceneID: scene.ID, ShotOrder: draft.Order, Duration: draft.Duration, Prompt: draft.Prompt, Camera: &camera, Movement: &movement}
					if err = s.database.CreateShot(shot); err != nil {
						return nil, err
					}
				}
			}
			updated = append(updated, *scene)
		}
		return map[string]any{"status": "converted", "scenes": updated, "preview": body.Preview}, nil
	})
}

func (s *Server) handleSceneRevisions(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	revs, _ := s.database.ListRevisions("scene", sceneID)
	if revs == nil {
		revs = []models.Revision{}
	}
	writeJSON(w, http.StatusOK, revs)
}

func (s *Server) handleShotRevisions(w http.ResponseWriter, r *http.Request) {
	shotID := chi.URLParam(r, "id")
	revs, _ := s.database.ListRevisions("shot", shotID)
	if revs == nil {
		revs = []models.Revision{}
	}
	writeJSON(w, http.StatusOK, revs)
}

func (s *Server) handleRevertRevision(w http.ResponseWriter, r *http.Request) {
	revID := chi.URLParam(r, "id")
	rev, err := s.database.GetRevision(revID)
	if err != nil {
		http.Error(w, "revision not found", http.StatusNotFound)
		return
	}

	if rev.EntityType == "scene" {
		var sc models.Scene
		if err := json.Unmarshal(rev.SnapshotJSON, &sc); err == nil {
			_ = s.database.UpdateScene(&sc)
		}
	} else if rev.EntityType == "shot" {
		var sh models.Shot
		if err := json.Unmarshal(rev.SnapshotJSON, &sh); err == nil {
			_ = s.database.UpdateShot(&sh)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "reverted", "revision_id": revID})
}

func (s *Server) handleDevelopIdea(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Idea string `json:"idea"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Idea == "" {
		http.Error(w, "idea is required", http.StatusBadRequest)
		return
	}

	res, err := s.agents.DevelopIdea(r.Context(), body.Idea)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGenerateScript(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}

	var body struct {
		Idea           string `json:"idea"`
		TargetDuration int    `json:"target_duration"`
		SceneCount     int    `json:"scene_count"`
		AspectRatio    string `json:"aspect_ratio"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Idea == "" {
		http.Error(w, "idea is required", http.StatusBadRequest)
		return
	}

	draft, err := s.agents.GenerateScript(r.Context(), body.Idea, body.SceneCount, body.TargetDuration, body.AspectRatio)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	draftBytes, _ := json.Marshal(draft)
	script := &models.Script{
		ProjectID: &projectID,
		Idea:      body.Idea,
		Title:     draft.Title,
		Summary:   draft.Summary,
		DraftJSON: draftBytes,
	}
	_ = s.database.CreateScript(script)

	createdScenes := []models.Scene{}
	for i, sc := range draft.Scenes {
		scene := models.Scene{
			ProjectID:   &projectID,
			ScriptID:    &script.ID,
			SceneOrder:  i + 1,
			Title:       sc.Title,
			Summary:     sc.Summary,
			Duration:    sc.Duration,
			AspectRatio: sc.AspectRatio,
		}
		_ = s.database.CreateScene(&scene)
		createdScenes = append(createdScenes, scene)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"script": script,
		"draft":  draft,
		"scenes": createdScenes,
	})
}

func (s *Server) handleListScripts(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	scripts, err := s.database.ListScripts(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if scripts == nil {
		scripts = []models.Script{}
	}
	writeJSON(w, http.StatusOK, scripts)
}
