package api

import (
	"6eek-go/internal/agents"
	"6eek-go/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type generationBrief struct {
	Idea              string   `json:"idea"`
	TargetDuration    int      `json:"target_duration"`
	SceneCount        int      `json:"scene_count"`
	Style             string   `json:"style"`
	AspectRatio       string   `json:"aspect_ratio"`
	Language          string   `json:"language"`
	ConversationMode  string   `json:"conversation_mode"`
	Instruction       string   `json:"instruction"`
	CharacterIDs      []string `json:"character_ids"`
	CharacterNotes    string   `json:"character_notes,omitempty"`
	ReferenceAssetIDs []string `json:"reference_asset_ids,omitempty"`
}

func (s *Server) effectiveStyle(projectID, choice string) (string, bool) {
	if style, err := s.database.GetStyleGuide(projectID); err == nil && strings.TrimSpace(style.StylePrompt) != "" {
		return style.StylePrompt + ". Palette: " + style.Palette + ". Lighting: " + style.Lighting, true
	}
	if choice == "cinematic" {
		return "cinematic composition and lighting", false
	}
	return "warm 2D children's picture-book illustration, clean shapes, expressive faces", false
}
func (s *Server) prepareBrief(project string, b *generationBrief) error {
	if strings.TrimSpace(b.Idea) == "" || len(b.Idea) > 20000 || len(b.Instruction) > 5000 {
		return fmt.Errorf("a story idea of at most 20000 characters is required")
	}
	if b.SceneCount < 0 || b.TargetDuration < 0 {
		return fmt.Errorf("scene count and duration must be positive")
	}
	if len(b.CharacterIDs) > 8 {
		return fmt.Errorf("choose at most 8 characters")
	}
	if b.SceneCount <= 0 {
		b.SceneCount = 3
	}
	if b.SceneCount > 20 {
		return fmt.Errorf("choose at most 20 scenes")
	}
	if b.TargetDuration <= 0 {
		b.TargetDuration = b.SceneCount * int(s.routes().NumberSetting("default_scene_duration", 5))
	}
	if b.TargetDuration < b.SceneCount {
		return fmt.Errorf("allow at least one second per scene")
	}
	if b.TargetDuration > 600 {
		return fmt.Errorf("target duration exceeds 600 seconds")
	}
	if b.AspectRatio == "" {
		b.AspectRatio = s.routes().Setting("default_aspect_ratio", "9:16")
	}
	if b.Language == "" {
		b.Language = s.routes().Setting("dialogue_language", "English")
	}
	if b.ConversationMode == "" {
		b.ConversationMode = "dialogue"
	}
	if b.ConversationMode != "dialogue" && b.ConversationMode != "narration" && b.ConversationMode != "silent" {
		return fmt.Errorf("unsupported dialogue mode")
	}
	b.Style, _ = s.effectiveStyle(project, b.Style)
	raw, _ := json.Marshal(b.CharacterIDs)
	notes, err := s.characterNotes(&project, raw)
	if err != nil {
		return err
	}
	b.CharacterNotes = notes
	b.ReferenceAssetIDs = nil
	seen := map[string]bool{}
	for _, id := range b.CharacterIDs {
		c, _ := s.database.GetCharacter(id)
		var ids []string
		json.Unmarshal(c.ReferenceAssetIDsJSON, &ids)
		for _, assetID := range ids {
			if !seen[assetID] && len(b.ReferenceAssetIDs) < 8 {
				a, err := s.database.GetAsset(assetID)
				if err != nil || a.ProjectID == nil || *a.ProjectID != project {
					return fmt.Errorf("character reference does not belong to this project")
				}
				seen[assetID] = true
				b.ReferenceAssetIDs = append(b.ReferenceAssetIDs, assetID)
			}
		}
	}
	return nil
}
func (b generationBrief) prompt() string {
	return fmt.Sprintf("%s\nVisual style: %s\nSpoken language: %s\nMode: %s\nDirection: %s\n%s", b.Idea, b.Style, b.Language, b.ConversationMode, b.Instruction, b.CharacterNotes)
}
func (b generationBrief) context(ctx context.Context) context.Context {
	return agents.WithDialogueLanguage(ctx, b.Language)
}
func (s *Server) persistDraft(project string, b generationBrief, draft *agents.ScriptDraft) (*models.Script, []models.Scene, error) {
	raw, _ := json.Marshal(draft)
	script := &models.Script{ProjectID: &project, Idea: b.Idea, Title: draft.Title, Summary: draft.Summary, DraftJSON: raw}
	cast, _ := json.Marshal(b.CharacterIDs)
	if string(cast) == "null" {
		cast = []byte("[]")
	}
	snapshot, _ := json.Marshal(map[string]any{"generation_brief": b})
	scenes := []models.Scene{}
	for i, sc := range draft.Scenes {
		scenes = append(scenes, models.Scene{ProjectID: &project, SceneOrder: i + 1, Title: sc.Title, Summary: sc.Summary, Duration: sc.Duration, AspectRatio: sc.AspectRatio, CharacterIDsJSON: cast, SceneJSON: snapshot})
	}
	if err := s.database.SaveScriptScenes(script, scenes); err != nil {
		return nil, nil, err
	}
	return script, scenes, nil
}
func sceneBrief(scene *models.Scene) generationBrief {
	var snapshot struct {
		Brief generationBrief `json:"generation_brief"`
	}
	json.Unmarshal(scene.SceneJSON, &snapshot)
	return snapshot.Brief
}
