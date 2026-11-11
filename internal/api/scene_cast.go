package api

import (
	"6eek-go/internal/models"
	"encoding/json"
	"fmt"
)

// A changed cast needs newly approved visuals; retained files remain available as assets.
func (s *Server) setSceneCast(scene *models.Scene, ids []string) error {
	if scene.ProjectID == nil {
		return fmt.Errorf("scene has no project")
	}
	b := sceneBrief(scene)
	b.CharacterIDs = ids
	if b.Idea == "" {
		b.Idea = scene.Title + " " + scene.Summary
	}
	if b.SceneCount == 0 {
		b.SceneCount = 1
	}
	style := b.Style
	if err := s.prepareBrief(*scene.ProjectID, &b); err != nil {
		return err
	}
	if style != "" {
		b.Style = style
	}
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(scene.SceneJSON, &snapshot); err != nil {
		return err
	}
	if snapshot == nil {
		snapshot = map[string]json.RawMessage{}
	}
	snapshot["generation_brief"], _ = json.Marshal(b)
	scene.SceneJSON, _ = json.Marshal(snapshot)
	scene.CharacterIDsJSON, _ = json.Marshal(ids)
	if len(ids) == 0 {
		scene.CharacterIDsJSON = json.RawMessage(`[]`)
	}
	scene.AssetIDsJSON = json.RawMessage(`[]`)
	return nil
}
func (s *Server) changeSceneCast(sceneID, characterID string, add bool) (*models.Scene, error) {
	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		return nil, err
	}
	var ids []string
	json.Unmarshal(scene.CharacterIDsJSON, &ids)
	next := []string{}
	found := false
	for _, id := range ids {
		if id == characterID {
			found = true
			if !add {
				continue
			}
		}
		next = append(next, id)
	}
	if add && !found {
		next = append(next, characterID)
	}
	if err = s.setSceneCast(scene, next); err != nil {
		return nil, err
	}
	if err = s.database.UpdateScene(scene); err != nil {
		return nil, err
	}
	return scene, nil
}
