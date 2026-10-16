package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"6eek-go/internal/models"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListCharacters(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	chars, err := s.database.ListCharacters(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if chars == nil {
		chars = []models.Character{}
	}
	writeJSON(w, http.StatusOK, chars)
}

func (s *Server) handleGetCharacter(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := s.database.GetCharacter(id)
	if err != nil {
		http.Error(w, "character not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleCreateCharacter(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	var c models.Character
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	c.ProjectID = &projectID
	if err := s.database.CreateCharacter(&c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateCharacter(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := s.database.GetCharacter(id)
	if err != nil {
		http.Error(w, "character not found", http.StatusNotFound)
		return
	}
	if err := json.NewDecoder(r.Body).Decode(c); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	c.ID = id
	if err := s.database.UpdateCharacter(c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteCharacter(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.database.DeleteCharacter(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleCharacterRefSheets(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := s.database.GetCharacter(id)
	if err != nil {
		http.Error(w, "character not found", http.StatusNotFound)
		return
	}

	s.operation(w, r, &models.Op{Kind: "character_reference", ProjectID: c.ProjectID}, func(ctx context.Context) (any, error) {
		images, err := s.generateAssets(ctx, c.ProjectID, &c.ID, "", c.Name+" reference", "character_reference", fmt.Sprintf("Character reference sheet showing front, side and full body views of %s. Appearance: %s. Description: %s. Visual rules: %s", c.Name, c.Appearance, c.Description, c.VisualRulesJSON), "1:1", nil)
		if err != nil {
			return nil, err
		}
		ids := []string{}
		_ = json.Unmarshal(c.ReferenceAssetIDsJSON, &ids)
		for _, image := range images {
			ids = append(ids, image.ID)
		}
		c.ReferenceAssetIDsJSON, _ = json.Marshal(ids)
		if err = s.database.UpdateCharacter(c); err != nil {
			return nil, err
		}
		return c, nil
	})
}

func (s *Server) handleCharacterBible(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := s.database.GetCharacter(id)
	if err != nil {
		http.Error(w, "character not found", http.StatusNotFound)
		return
	}

	var body struct {
		Notes string `json:"notes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	var result struct {
		Description    string `json:"description"`
		Appearance     string `json:"appearance"`
		Personality    string `json:"personality"`
		SampleDialogue string `json:"sample_dialogue"`
	}
	existing, _ := json.Marshal(c)
	if err = s.agentJSON(r.Context(), "character_memory", "Develop a consistent character bible from the existing character and notes. Return description, appearance, personality and sample_dialogue.", string(existing)+"\nNotes: "+body.Notes, nil, &result); err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	if result.Description == "" {
		http.Error(w, "OpenRouter returned no character bible", 502)
		return
	}
	c.Description = result.Description
	c.Appearance = result.Appearance
	c.Personality = result.Personality
	c.SampleDialogue = result.SampleDialogue
	if err = s.database.UpdateCharacter(c); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	writeJSON(w, http.StatusOK, c)
}
