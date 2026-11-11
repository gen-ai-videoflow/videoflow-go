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

	previousCast := string(scene.CharacterIDsJSON)
	previousProject := scene.ProjectID
	previousUpdated := scene.UpdatedAt
	// Snapshot revision before update
	oldSnap, _ := json.Marshal(scene)
	if _, err := s.database.CreateRevision("scene", id, oldSnap); err != nil {
		http.Error(w, "cannot save scene revision", 500)
		return
	}

	if err := json.NewDecoder(r.Body).Decode(scene); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	scene.ID = id
	scene.ProjectID = previousProject
	scene.UpdatedAt = previousUpdated
	if string(scene.CharacterIDsJSON) != previousCast {
		var ids []string
		if json.Unmarshal(scene.CharacterIDsJSON, &ids) != nil {
			http.Error(w, "invalid cast", 400)
			return
		}
		if err := s.setSceneCast(scene, ids); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}
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
	if _, err := s.database.CreateRevision("shot", id, oldSnap); err != nil {
		http.Error(w, "cannot save shot revision", 500)
		return
	}

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

// Both entry points use the same validated, atomic scene-generation operation.
func (s *Server) handleExpandScene(w http.ResponseWriter, r *http.Request) {
	s.handleGenerateShots(w, r)
}
func (s *Server) handleGenerateShots(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		http.Error(w, "scene not found", 404)
		return
	}
	var body struct {
		CharacterIDs *[]string `json:"character_ids"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", 400)
			return
		}
	}
	if body.CharacterIDs != nil {
		scene.CharacterIDsJSON, _ = json.Marshal(*body.CharacterIDs)
		brief := sceneBrief(scene)
		brief.CharacterIDs = *body.CharacterIDs
		if scene.ProjectID != nil {
			if brief.Idea == "" {
				brief.Idea = scene.Summary
			}
			if err = s.prepareBrief(*scene.ProjectID, &brief); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
		}
		var snapshot map[string]any
		json.Unmarshal(scene.SceneJSON, &snapshot)
		if snapshot == nil {
			snapshot = map[string]any{}
		}
		snapshot["generation_brief"] = brief
		scene.SceneJSON, _ = json.Marshal(snapshot)
	}
	if len(scene.CharacterIDsJSON) == 0 {
		scene.CharacterIDsJSON = []byte("[]")
	}
	notes, err := s.characterNotes(scene.ProjectID, scene.CharacterIDsJSON)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.operation(w, r, &models.Op{Kind: "shots", SceneID: &sceneID, ProjectID: scene.ProjectID}, func(ctx context.Context) (any, error) {
		drafts, err := s.agents.GenerateShots(sceneBrief(scene).context(ctx), scene.Title, scene.Summary+"\n"+sceneBrief(scene).prompt()+notes, scene.Duration)
		if err != nil {
			return nil, err
		}
		shots := []models.Shot{}
		for _, draft := range drafts {
			camera, movement := draft.Camera, draft.Movement
			shots = append(shots, models.Shot{SceneID: scene.ID, ShotOrder: draft.Order, Duration: draft.Duration, Prompt: draft.Prompt, Camera: &camera, Movement: &movement})
		}
		if err = s.database.ReplaceSceneShots(scene, shots); err != nil {
			return nil, err
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

	projectID, err := s.database.GetActiveProjectID()
	if err != nil || scene.ProjectID == nil || *scene.ProjectID != projectID {
		http.Error(w, "scene not in active project", 400)
		return
	}
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
	if err := s.database.CreateRenderJob(job); err != nil {
		http.Error(w, "failed to save render job", 500)
		return
	}
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
		assets, err := s.generateAssets(ctx, scene.ProjectID, nil, scene.ID, scene.Title+" storyboard", "storyboard", "Storyboard keyframe of "+scene.Title+". "+scene.Summary+"\n"+sceneBrief(scene).prompt(), scene.AspectRatio, refs)
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
	assetIDs := sceneBrief(scene).ReferenceAssetIDs
	if len(assetIDs) == 0 {
		var characterIDs []string
		json.Unmarshal(scene.CharacterIDsJSON, &characterIDs)
		if _, err := s.characterNotes(scene.ProjectID, scene.CharacterIDsJSON); err != nil {
			return nil, err
		}
		for _, id := range characterIDs {
			c, err := s.database.GetCharacter(id)
			if err != nil {
				return nil, err
			}
			var ids []string
			json.Unmarshal(c.ReferenceAssetIDsJSON, &ids)
			assetIDs = append(assetIDs, ids...)
		}
	}
	refs := []string{}
	seen := map[string]bool{}
	for _, id := range assetIDs {
		if seen[id] || len(refs) >= 8 {
			continue
		}
		seen[id] = true
		asset, err := s.database.GetAsset(id)
		if err != nil {
			return nil, err
		}
		if scene.ProjectID == nil || asset.ProjectID == nil || *scene.ProjectID != *asset.ProjectID {
			return nil, fmt.Errorf("reference project mismatch")
		}
		image, err := s.imageURL(asset)
		if err != nil {
			return nil, err
		}
		refs = append(refs, image)
	}

	return refs, nil
}

func (s *Server) handleCastMemberAdd(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	characterID := chi.URLParam(r, "character_id")
	scene, err := s.changeSceneCast(sceneID, characterID, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, scene)
}

func (s *Server) handleCastMemberDelete(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	characterID := chi.URLParam(r, "character_id")
	scene, err := s.changeSceneCast(sceneID, characterID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
			current, err := s.database.GetScene(sc.ID)
			if err != nil {
				http.Error(w, "scene not found", 404)
				return
			}
			sc.UpdatedAt = current.UpdatedAt
			if err := s.database.UpdateScene(&sc); err != nil {
				http.Error(w, "cannot restore scene", 500)
				return
			}
		}
	} else if rev.EntityType == "shot" {
		var sh models.Shot
		if err := json.Unmarshal(rev.SnapshotJSON, &sh); err == nil {
			if err := s.database.UpdateShot(&sh); err != nil {
				http.Error(w, "cannot restore shot", 500)
				return
			}
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

	var body generationBrief
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if err = s.prepareBrief(projectID, &body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	draft, err := s.agents.GenerateScript(body.context(r.Context()), body.prompt(), body.SceneCount, body.TargetDuration, body.AspectRatio)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	script, createdScenes, err := s.persistDraft(projectID, body, draft)
	if err != nil {
		http.Error(w, "failed to save generated script", 500)
		return
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
