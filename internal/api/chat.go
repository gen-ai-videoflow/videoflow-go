package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message string              `json:"message"`
		History []map[string]string `json:"history"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	projectID, _ := s.database.GetActiveProjectID()
	scenes, _ := s.database.ListScenes(projectID)
	chars, _ := s.database.ListCharacters(projectID)
	jobs, _ := s.database.ListRenderJobs(projectID)

	outputsList := []map[string]interface{}{}
	for _, j := range jobs {
		outs, _ := s.database.ListOutputsForJob(j.ID)
		for _, o := range outs {
			outputsList = append(outputsList, map[string]interface{}{
				"id":    o.ID,
				"video": o.VideoPath,
			})
		}
	}

	sceneOptions := []map[string]interface{}{}
	for _, sc := range scenes {
		sceneOptions = append(sceneOptions, map[string]interface{}{
			"id":    sc.ID,
			"title": sc.Title,
		})
	}

	charOptions := []map[string]interface{}{}
	for _, c := range chars {
		charOptions = append(charOptions, map[string]interface{}{
			"id":   c.ID,
			"name": c.Name,
		})
	}

	res, err := s.agents.Chat(r.Context(), body.Message, body.History)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	action := canonicalChatAction(res.Action)

	intent := map[string]interface{}{
		"action":      action,
		"reply":       res.Reply,
		"idea":        body.Message,
		"scene_count": 3,
		"confidence":  0.95,
	}

	if len(scenes) > 0 {
		intent["scene_id"] = scenes[0].ID
	}
	if len(chars) > 0 {
		intent["character_id"] = chars[0].ID
	}
	if len(outputsList) > 0 {
		intent["output_id"] = outputsList[0]["id"]
	}

	options := map[string]interface{}{
		"scenes":         sceneOptions,
		"characters":     charOptions,
		"outputs":        outputsList,
		"caption_styles": defaultCaptionStyles,
	}

	sceneSnapshots := []map[string]interface{}{}
	for _, sc := range scenes {
		sceneSnapshots = append(sceneSnapshots, map[string]interface{}{
			"id":             sc.ID,
			"title":          sc.Title,
			"expanded":       true,
			"has_shots":      true,
			"has_storyboard": false,
			"rendered":       false,
		})
	}

	state := map[string]interface{}{
		"characters": len(chars),
		"has_style":  true,
		"scripts":    1,
		"scenes":     sceneSnapshots,
		"next_steps": []string{
			"Add characters to your studio",
			"Review and generate cinematography shots",
			"Render scenes into high-definition video",
		},
	}

	response := map[string]interface{}{
		"intent":   intent,
		"options":  options,
		"state":    state,
		"warnings": []string{},
	}

	writeJSON(w, http.StatusOK, response)
}

// Keep model output on the action-card contract, including legacy agent replies.
func canonicalChatAction(action string) string {
	switch action {
	case "create_script", "add_scene":
		return "generate_script"
	case "generate_script", "generate_scenes", "generate_shots", "storyboard", "render_scene", "render_shot", "caption", "retry_render", "generate_assets", "refine_scene", "refine_shot", "delete_scene", "style_ingest", "plan_assets":
		return action
	default:
		return "unknown"
	}
}
