package api

import (
	"encoding/json"
	"net/http"

	"6eek-go/internal/models"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.database.ListProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if projects == nil {
		projects = []models.Project{}
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) handleGetActiveProject(w http.ResponseWriter, r *http.Request) {
	proj, err := s.database.GetActiveProject()
	if err != nil {
		http.Error(w, "no active project", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, proj)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		body.Name = "Untitled Project"
	}
	proj, err := s.database.CreateProject(body.Name, body.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, proj)
}

func (s *Server) handleActivateProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.database.ActivateProject(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "active_project_id": id})
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.database.UpdateProject(id, body.Name, body.Description); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p, err := s.database.GetProject(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.database.DeleteProject(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleExportProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := s.database.GetProject(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	chars, _ := s.database.ListCharacters(id)
	scenes, _ := s.database.ListScenes(id)
	assets, _ := s.database.ListAssets(id)
	style, _ := s.database.GetStyleGuide(id)

	exportData := map[string]interface{}{
		"project":    p,
		"characters": chars,
		"scenes":     scenes,
		"assets":     assets,
		"style":      style,
	}
	writeJSON(w, http.StatusOK, exportData)
}

func (s *Server) handleImportProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Project    models.Project     `json:"project"`
		Characters []models.Character `json:"characters"`
		Scenes     []models.Scene     `json:"scenes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid import payload", http.StatusBadRequest)
		return
	}
	newProj, err := s.database.CreateProject(body.Project.Name, body.Project.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, c := range body.Characters {
		c.ProjectID = &newProj.ID
		_ = s.database.CreateCharacter(&c)
	}
	for _, sc := range body.Scenes {
		sc.ProjectID = &newProj.ID
		_ = s.database.CreateScene(&sc)
	}
	writeJSON(w, http.StatusOK, newProj)
}
