package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (s *Server) characterNotes(project *string, raw []byte) (string, error) {
	var ids []string
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &ids); err != nil {
			return "", fmt.Errorf("invalid cast")
		}
	}
	var notes strings.Builder
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		c, err := s.database.GetCharacter(id)
		if err != nil || project == nil || c.ProjectID == nil || *project != *c.ProjectID {
			return "", fmt.Errorf("character not found in this project")
		}
		fmt.Fprintf(&notes, "\nCharacter %s (%s): %s. Appearance: %s. Personality: %s. Visual rules: %s. Voice rules: %s.", c.Name, c.ID, c.Description, c.Appearance, c.Personality, c.VisualRulesJSON, c.VoiceRulesJSON)
	}
	return notes.String(), nil
}
