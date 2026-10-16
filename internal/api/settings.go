package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"6eek-go/internal/providers"
	"6eek-go/internal/routing"

	"6eek-go/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ProviderPresetInfo struct {
	Label           string   `json:"label"`
	Protocol        string   `json:"protocol"`
	URL             string   `json:"url"`
	DefaultModel    string   `json:"model"`
	SuggestedModels []string `json:"-"`
	EnvKey          string   `json:"-"`
}

var presetCatalog = map[string]ProviderPresetInfo{
	"openrouter": {Label: "OpenRouter", Protocol: "chat", URL: providers.OpenRouterURL, DefaultModel: "openai/gpt-4o-mini", SuggestedModels: []string{"openai/gpt-4o-mini", "meta-llama/llama-3.3-70b-instruct"}, EnvKey: "OPENROUTER_API_KEY"},
}
var presetOrder = []string{"openrouter"}

func maskKey(raw string) string {
	if raw == "" {
		return ""
	}
	if len(raw) <= 4 {
		return "••••"
	}
	return "••••" + raw[len(raw)-4:]
}

func (s *Server) routes() routing.Resolver { return routing.Resolver{Config: s.cfg, DB: s.database} }
func (s *Server) effectiveProviderKey(name string) (key string, baseURL *string, fromDB bool) {
	key, endpoint, fromDB, _ := s.routes().Credentials(name)
	return key, &endpoint, fromDB
}

func (s *Server) handleGetAppSettings(w http.ResponseWriter, r *http.Request) {
	appSettings := map[string]interface{}{
		"default_aspect_ratio":   "9:16",
		"default_video_provider": "openrouter",
		"default_image_provider": "openrouter",
		"default_scene_duration": 5.0,
		"caption_style":          "clean",
		"caption_language":       "auto",
		"whisper_model":          "openai/whisper-1",
		"dialogue_language":      "English",
		"image_model":            s.cfg.OpenRouterImageModel,
		"ref_image_model":        s.cfg.OpenRouterImageModel,
		"video_model":            s.cfg.OpenRouterVideoModel,
		"vl_model":               s.cfg.OpenRouterVisionModel,
		"max_video_refs":         3,
		"render_negatives":       []string{"blurry", "low quality", "distorted", "watermark"},
	}

	// Overlay any keys stored in app_settings table
	for k := range appSettings {
		if val, err := s.database.GetAppSetting(k); err == nil && val != "" {
			var parsed interface{}
			if err := json.Unmarshal([]byte(val), &parsed); err == nil {
				appSettings[k] = parsed
			}
		}
	}

	appSettings["default_image_provider"] = "openrouter"
	appSettings["default_video_provider"] = "openrouter"
	writeJSON(w, http.StatusOK, appSettings)
}

func (s *Server) handlePutAppSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	for _, key := range []string{"default_image_provider", "default_video_provider"} {
		if value, ok := body[key]; ok && value != "openrouter" {
			http.Error(w, "All generation uses OpenRouter", http.StatusBadRequest)
			return
		}
	}
	for key, modality := range map[string]string{"image_model": "image", "ref_image_model": "image", "video_model": "video", "vl_model": "vision", "whisper_model": "transcription"} {
		if value, ok := body[key]; ok {
			model, valid := value.(string)
			if !valid || strings.TrimSpace(model) == "" {
				http.Error(w, "model ID is required", 400)
				return
			}
			client, err := s.routes().Client("openrouter")
			if err == nil {
				_, err = client.Model(r.Context(), model, modality)
			}
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
		}
	}
	if raw, ok := body["default_scene_duration"]; ok {
		value, valid := raw.(float64)
		if !valid || value < 1 || value > 120 || value != float64(int(value)) {
			http.Error(w, "Scene duration must be an integer between 1 and 120 seconds", 400)
			return
		}
	}
	if raw, ok := body["render_negatives"]; ok {
		values, valid := raw.([]any)
		if !valid {
			http.Error(w, "render_negatives must be a list of strings", 400)
			return
		}
		for _, value := range values {
			if _, valid := value.(string); !valid {
				http.Error(w, "render_negatives must contain strings", 400)
				return
			}
		}
	}
	for k, v := range body {
		b, _ := json.Marshal(v)
		if err := s.database.SetAppSetting(k, string(b)); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	s.handleGetAppSettings(w, r)
}

var standardAgents = []struct {
	Agent           string
	Label           string
	DefaultProvider string
	DefaultModel    string
}{
	{"script_agent", "Script Agent", "openrouter", "openai/gpt-4o-mini"},
	{"scene_agent", "Scene Agent", "openrouter", "openai/gpt-4o-mini"},
	{"shot_agent", "Shot Agent", "openrouter", "openai/gpt-4o-mini"},
	{"prompt_agent", "Prompt Agent", "openrouter", "openai/gpt-4o-mini"},
	{"qa_agent", "QA Agent", "openrouter", "openai/gpt-4o-mini"},
	{"idea_agent", "Idea Agent", "openrouter", "openai/gpt-4o-mini"},
	{"asset_planner", "Asset Planner", "openrouter", "openai/gpt-4o-mini"},
	{"refine_agent", "Refine Agent", "openrouter", "openai/gpt-4o-mini"},
	{"intent_agent", "Intent Agent", "openrouter", "openai/gpt-4o-mini"},
	{"asset_recogniser", "Asset Recogniser", "openrouter", "openai/gpt-4o-mini"},
	{"character_memory", "Character Memory", "openrouter", "openai/gpt-4o-mini"},
	{"style_agent", "Style Agent", "openrouter", "openai/gpt-4o-mini"},
}

func (s *Server) defaultAgentModel(agent string) string {
	if agent == "qa_agent" || agent == "asset_recogniser" {
		return s.routes().Setting("vl_model", s.cfg.OpenRouterVisionModel)
	}
	return s.cfg.OpenRouterTextModel
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	overrides, _ := s.database.GetAgentSettings()
	if overrides == nil {
		overrides = make(map[string]models.AgentSetting)
	}

	res := make([]map[string]interface{}, 0, len(standardAgents))
	for _, a := range standardAgents {
		prov := a.DefaultProvider
		mod := s.defaultAgentModel(a.Agent)
		if ov, ok := overrides[a.Agent]; ok && ov.Provider != "" {
			prov = ov.Provider
			if ov.Model != "" {
				mod = ov.Model
			}
		}
		if selectedProvider, selectedModel, _, err := s.routes().Selection(a.Agent); err == nil {
			prov = selectedProvider
			mod = selectedModel
		}
		res = append(res, map[string]interface{}{
			"agent":            a.Agent,
			"label":            a.Label,
			"provider":         prov,
			"model":            mod,
			"default_provider": a.DefaultProvider,
			"default_model":    s.defaultAgentModel(a.Agent),
		})
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handlePutAgent(w http.ResponseWriter, r *http.Request) {
	agentName := chi.URLParam(r, "agent")
	var body struct {
		Provider *string `json:"provider"`
		Model    *string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var foundAgent *struct {
		Agent           string
		Label           string
		DefaultProvider string
		DefaultModel    string
	}
	for i := range standardAgents {
		if standardAgents[i].Agent == agentName {
			foundAgent = &standardAgents[i]
			break
		}
	}
	if foundAgent == nil {
		http.Error(w, "unknown agent", http.StatusNotFound)
		return
	}

	if body.Provider == nil || *body.Provider == "" {
		_ = s.database.ResetAgentSetting(agentName)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"agent":            foundAgent.Agent,
			"label":            foundAgent.Label,
			"provider":         foundAgent.DefaultProvider,
			"model":            s.defaultAgentModel(foundAgent.Agent),
			"default_provider": foundAgent.DefaultProvider,
			"default_model":    s.defaultAgentModel(foundAgent.Agent),
		})
		return
	}

	if *body.Provider != "openrouter" {
		conns, err := s.database.ListConnections()
		valid := false
		if err == nil {
			for _, c := range conns {
				if c.Name == *body.Provider && c.Preset == "openrouter" {
					valid = true
				}
			}
		}
		if !valid {
			http.Error(w, "Select an OpenRouter connection", 400)
			return
		}
	}
	modelStr := ""
	if body.Model != nil {
		modelStr = *body.Model
	}
	if err := s.database.SetAgentSetting(agentName, *body.Provider, modelStr); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"agent":    foundAgent.Agent,
		"label":    foundAgent.Label,
		"provider": *body.Provider,
		"model": func() string {
			_, model, _, err := s.routes().Selection(agentName)
			if err == nil {
				return model
			}
			return modelStr
		}(),
		"default_provider": foundAgent.DefaultProvider,
		"default_model":    s.defaultAgentModel(foundAgent.Agent),
	})
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	conns, _ := s.database.ListConnections()
	providers := []map[string]interface{}{}

	// Standard presets
	for _, name := range presetOrder {
		preset := presetCatalog[name]
		key, _, fromDB := s.effectiveProviderKey(name)
		isConfigured := key != ""
		if name == "codex" {
			isConfigured = false
		}

		models := preset.SuggestedModels
		if models == nil {
			models = []string{}
		}

		providers = append(providers, map[string]interface{}{
			"name":             name,
			"label":            preset.Label,
			"protocol":         preset.Protocol,
			"configured":       isConfigured,
			"models":           models,
			"suggested_models": models,
			"default_model":    preset.DefaultModel,
			"allow_custom":     true,
			"from_db":          fromDB,
		})
	}

	// User-created connections
	for _, c := range conns {
		if c.Preset != "openrouter" || c.Protocol != "chat" {
			continue
		}
		key, _, fromDB := s.effectiveProviderKey(c.Name)
		models := []string{}
		if c.Model != "" {
			models = []string{c.Model}
		}
		providers = append(providers, map[string]interface{}{
			"name":             c.Name,
			"label":            c.Label,
			"protocol":         c.Protocol,
			"configured":       key != "",
			"models":           models,
			"suggested_models": models,
			"default_model":    c.Model,
			"allow_custom":     true,
			"from_db":          fromDB,
		})
	}

	writeJSON(w, http.StatusOK, providers)
}

func (s *Server) handleGetProviderConfig(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	key, baseURL, fromDB := s.effectiveProviderKey(name)
	isConfigured := key != ""

	if baseURL == nil {
		if p, ok := presetCatalog[name]; ok && p.URL != "" {
			u := p.URL
			baseURL = &u
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":       name,
		"configured": isConfigured,
		"masked_key": maskKey(key),
		"base_url":   baseURL,
		"from_db":    fromDB,
	})
}

func (s *Server) handlePutProviderConfig(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body struct {
		APIKey  *string `json:"api_key"`
		BaseURL *string `json:"base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if name != "openrouter" {
		conns, _ := s.database.ListConnections()
		valid := false
		for _, c := range conns {
			if c.Name == name && c.Preset == "openrouter" {
				valid = true
			}
		}
		if !valid {
			http.Error(w, "Unknown OpenRouter connection", 400)
			return
		}
	}
	if body.BaseURL != nil && *body.BaseURL != "" && !routing.ValidBaseURL(*body.BaseURL) {
		http.Error(w, "Use https://openrouter.ai/api/v1", 400)
		return
	}
	keyStr, _, _, err := s.database.GetProviderSecret(name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if body.APIKey != nil {
		keyStr = strings.TrimSpace(*body.APIKey)
	}
	if err = s.database.SetProviderSecret(name, keyStr, body.BaseURL); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	s.handleGetProviderConfig(w, r)
}

func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	client, err := s.routes().Client(chi.URLParam(r, "name"))
	if err == nil {
		err = client.CheckKey(r.Context())
	}
	var message any
	if err != nil {
		message = err.Error()
	}
	writeJSON(w, 200, map[string]any{"ok": err == nil, "latency_ms": time.Since(start).Milliseconds(), "error": message})
}
func (s *Server) handleDiscoverModels(w http.ResponseWriter, r *http.Request) {
	modality := r.URL.Query().Get("modality")
	if modality == "" {
		modality = "text"
	}
	client, err := s.routes().Client(chi.URLParam(r, "name"))
	var catalog []providers.Model
	if err == nil {
		catalog, err = client.Models(r.Context(), modality)
	}
	ids := []string{}
	for _, model := range catalog {
		ids = append(ids, model.ID)
	}
	var message any
	if err != nil {
		message = err.Error()
	}
	writeJSON(w, 200, map[string]any{"models": ids, "capabilities": catalog, "modality": modality, "error": message})
}
func (s *Server) handleVerifyModel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model    string `json:"model"`
		Modality string `json:"modality"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Model) == "" {
		http.Error(w, "model is required", 400)
		return
	}
	if body.Modality == "" {
		body.Modality = "text"
	}
	client, err := s.routes().Client(chi.URLParam(r, "name"))
	if err == nil {
		_, err = client.Model(r.Context(), body.Model, body.Modality)
	}
	// A text verification performs one small, billable completion. Media verification only checks the catalog.
	if err == nil && (body.Modality == "text" || body.Modality == "vision") {
		var result struct {
			OK bool `json:"ok"`
		}
		err = client.StructuredJSON(r.Context(), body.Model, "Return the requested JSON.", "Return {\"ok\":true}", &result)
		if err == nil && !result.OK {
			err = fmt.Errorf("model did not return the requested JSON object")
		}
	}
	var message any
	if err != nil {
		message = err.Error()
	}
	writeJSON(w, 200, map[string]any{"ok": err == nil, "error": message, "modality": body.Modality, "generation_tested": body.Modality == "text" && err == nil})
}

func (s *Server) handleConnectionPresets(w http.ResponseWriter, r *http.Request) {
	presets := make(map[string]interface{})
	for k, v := range presetCatalog {
		presets[k] = map[string]interface{}{
			"label":    v.Label,
			"protocol": v.Protocol,
			"url":      v.URL,
			"model":    v.DefaultModel,
			"preset":   k,
			"mode":     "auto",
			"vision":   true,
		}
	}
	writeJSON(w, http.StatusOK, presets)
}

func (s *Server) handleCreateConnection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label    string  `json:"label"`
		Preset   string  `json:"preset"`
		Protocol string  `json:"protocol"`
		BaseURL  *string `json:"base_url"`
		Model    string  `json:"model"`
		Mode     string  `json:"mode"`
		Vision   bool    `json:"vision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Label == "" {
		http.Error(w, "invalid connection payload", http.StatusBadRequest)
		return
	}

	if body.Preset != "openrouter" || (body.Protocol != "" && body.Protocol != "chat") {
		http.Error(w, "Connections must use OpenRouter Chat Completions", 400)
		return
	}
	if body.BaseURL != nil && *body.BaseURL != "" && !routing.ValidBaseURL(*body.BaseURL) {
		http.Error(w, "Use https://openrouter.ai/api/v1", 400)
		return
	}
	body.Protocol = "chat"
	if body.Mode != "auto" && body.Mode != "prompt" && body.Mode != "" {
		http.Error(w, "Supported output modes are auto and prompt", 400)
		return
	}
	if body.BaseURL == nil || *body.BaseURL == "" {
		value := providers.OpenRouterURL
		body.BaseURL = &value
	}
	name := "conn_" + uuid.New().String()[:8]
	conn := &models.Connection{
		Name:     name,
		Label:    body.Label,
		Preset:   body.Preset,
		Protocol: body.Protocol,
		BaseURL:  body.BaseURL,
		Model:    body.Model,
		Mode:     body.Mode,
		Vision:   body.Vision,
	}
	if err := s.database.CreateConnection(conn); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"name": name})
}
