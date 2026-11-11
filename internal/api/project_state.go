package api

import "encoding/json"

// projectState is derived from persisted work, never from an assistant's guesses.
func (s *Server) projectState(projectID string) (map[string]any, error) {
	scenes, err := s.database.ListScenes(projectID)
	if err != nil {
		return nil, err
	}
	scripts, err := s.database.ListScripts(projectID)
	if err != nil {
		return nil, err
	}
	chars, err := s.database.ListCharacters(projectID)
	if err != nil {
		return nil, err
	}
	assets, err := s.database.ListAssets(projectID)
	if err != nil {
		return nil, err
	}
	jobs, err := s.database.ListRenderJobs(projectID)
	if err != nil {
		return nil, err
	}
	style, _ := s.database.GetStyleGuide(projectID)
	snapshots := []map[string]any{}
	for _, sc := range scenes {
		shots, err := s.database.ListShots(sc.ID)
		if err != nil {
			return nil, err
		}
		storyboard, rendered := false, false
		var approved []string
		json.Unmarshal(sc.AssetIDsJSON, &approved)
		approvedSet := map[string]bool{}
		for _, id := range approved {
			approvedSet[id] = true
		}
		for _, a := range assets {
			var meta struct {
				SceneID string `json:"scene_id"`
			}
			json.Unmarshal(a.MetadataJSON, &meta)
			if a.Type == "storyboard" && meta.SceneID == sc.ID && approvedSet[a.ID] {
				storyboard = true
			}
		}
		for _, j := range jobs {
			if j.SceneID != nil && *j.SceneID == sc.ID && j.Status == "succeeded" {
				outputs, err := s.database.ListOutputsForJob(j.ID)
				if err != nil {
					return nil, err
				}
				rendered = rendered || len(outputs) > 0
			}
		}
		snapshots = append(snapshots, map[string]any{"id": sc.ID, "title": sc.Title, "expanded": len(shots) > 0, "has_shots": len(shots) > 0, "has_storyboard": storyboard, "rendered": rendered})
	}
	next := []string{}
	if len(chars) == 0 {
		next = append(next, "Add characters and reference photos")
	}
	if len(scripts) == 0 {
		next = append(next, "Generate or write a script")
	}
	if len(scenes) > 0 {
		next = append(next, "Review scene cast and shots before generating video")
	}
	return map[string]any{"characters": len(chars), "scripts": len(scripts), "has_style": style != nil && style.StylePrompt != "", "scenes": snapshots, "next_steps": next}, nil
}
