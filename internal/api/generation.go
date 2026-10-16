package api

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"6eek-go/internal/media"
	"6eek-go/internal/models"
)

// operation preserves the existing synchronous/background API contract and records failures.
func (s *Server) operation(w http.ResponseWriter, r *http.Request, op *models.Op, work func(context.Context) (any, error)) {
	if r.URL.Query().Get("background") != "true" {
		result, err := work(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, 200, result)
		return
	}
	op.Status = "running"
	if err := s.database.CreateOp(op); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.tasks.Add(1)
	go func() {
		defer s.tasks.Done()
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
		defer cancel()
		result, err := work(ctx)
		status := "succeeded"
		var errorText *string
		if err != nil {
			status = "failed"
			message := err.Error()
			errorText = &message
		}
		raw, _ := json.Marshal(result)
		_ = s.database.UpdateOp(op.ID, status, raw, errorText)
		s.broker.Publish(map[string]any{"type": "op_done", "op_id": op.ID, "kind": op.Kind, "status": status, "scene_id": op.SceneID, "output_id": op.OutputID, "error": errorText})
	}()
	writeJSON(w, 202, map[string]any{"op_id": op.ID, "status": "running"})
}

func (s *Server) agentJSON(ctx context.Context, agent, system, prompt string, images []string, target any) error {
	client, model, err := s.routes().Agent(ctx, agent)
	if err != nil {
		return err
	}
	if len(images) > 0 {
		if _, err = client.Model(ctx, model, "vision"); err != nil {
			return err
		}
	}
	return client.StructuredVision(ctx, model, system, prompt, images, target)
}

// storagePath maps public storage URLs and legacy paths to the configured storage root.
func (s *Server) storagePath(path string) (string, error) {
	return media.StoragePath(s.cfg.StorageRoot, path)
}
func (s *Server) imageURL(asset *models.Asset) (string, error) {
	return media.ImageDataURL(s.cfg.StorageRoot, asset.FilePath)
}

func (s *Server) stylePrompt(projectID *string) string {
	if projectID != nil {
		if style, err := s.database.GetStyleGuide(*projectID); err == nil {
			return style.StylePrompt + ". Palette: " + style.Palette + ". Lighting: " + style.Lighting
		}
	}
	return "Consistent cinematic illustration"
}

func (s *Server) generateAssets(ctx context.Context, projectID *string, characterID *string, sceneID, name, kind, prompt, aspect string, references []string) ([]models.Asset, error) {
	client, err := s.routes().Client("openrouter")
	if err != nil {
		return nil, err
	}
	modelKind := "image"
	if characterID != nil || kind == "prop" {
		modelKind = "reference"
	}
	var prepared struct {
		Prompt string `json:"prompt"`
	}
	if err = s.agentJSON(ctx, "prompt_agent", "Write a precise image-generation prompt from the brief. Preserve the characters, reference consistency, composition and visual style. Return prompt only as JSON.", prompt+"\nVisual style: "+s.stylePrompt(projectID), nil, &prepared); err != nil {
		return nil, err
	}
	if strings.TrimSpace(prepared.Prompt) == "" {
		return nil, fmt.Errorf("OpenRouter returned no image prompt")
	}
	payload := map[string]any{"model": s.routes().MediaModel(modelKind), "prompt": prepared.Prompt}
	if aspect != "" {
		payload["aspect_ratio"] = aspect
	}
	if len(references) > 0 {
		refs := []map[string]any{}
		for _, image := range references {
			refs = append(refs, map[string]any{"type": "image_url", "image_url": map[string]string{"url": image}})
		}
		payload["input_references"] = refs
	}
	images, err := client.GenerateImages(ctx, payload)
	if err != nil {
		return nil, err
	}
	assets := []models.Asset{}
	for _, image := range images {
		id := "asset_" + uuid.New().String()[:8]
		ext := ""
		switch image.MediaType {
		case "image/png":
			ext = ".png"
		case "image/jpeg":
			ext = ".jpg"
		case "image/webp":
			ext = ".webp"
		case "image/svg+xml":
			ext = ".svg"
		default:
			if extensions, _ := mime.ExtensionsByType(image.MediaType); len(extensions) > 0 {
				ext = extensions[0]
			}
		}
		if ext == "" {
			return nil, fmt.Errorf("unsupported generated media type %q", image.MediaType)
		}
		path := filepath.Join(s.cfg.StorageRoot, "assets", id+ext)
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		if err = os.WriteFile(path, image.Data, 0644); err != nil {
			return nil, err
		}
		metadata, _ := json.Marshal(map[string]any{"scene_id": sceneID, "provider": "openrouter", "model": payload["model"], "media_type": image.MediaType})
		asset := models.Asset{ID: id, ProjectID: projectID, CharacterID: characterID, Type: kind, Name: name, Description: prompt, FilePath: filepath.ToSlash(filepath.Join("storage", "assets", id+ext)), TagsJSON: json.RawMessage(`[]`), MetadataJSON: metadata}
		if err = s.database.CreateAsset(&asset); err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

type plannedAsset struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}
type assetPlan struct {
	SceneID        string         `json:"scene_id"`
	Assets         []plannedAsset `json:"assets"`
	SuggestedShots []string       `json:"suggested_shots"`
}

func (s *Server) planAssets(ctx context.Context, scene *models.Scene, instruction string, maxAssets int) (*assetPlan, error) {
	if maxAssets <= 0 {
		maxAssets = 3
	}
	if maxAssets > 10 {
		maxAssets = 10
	}
	plan := &assetPlan{Assets: []plannedAsset{}, SuggestedShots: []string{}}
	err := s.agentJSON(ctx, "asset_planner", "Plan only essential props and locations. Return assets with name, type (prop or location), description, and suggested_shots.", fmt.Sprintf("Scene: %s\n%s\nInstructions: %s\nMaximum assets: %d", scene.Title, scene.Summary, instruction, maxAssets), nil, plan)
	if err != nil {
		return nil, err
	}
	if len(plan.Assets) > maxAssets {
		plan.Assets = plan.Assets[:maxAssets]
	}
	for _, asset := range plan.Assets {
		if strings.TrimSpace(asset.Name) == "" || strings.TrimSpace(asset.Description) == "" || (asset.Type != "prop" && asset.Type != "location") {
			return nil, fmt.Errorf("OpenRouter returned an invalid asset plan")
		}
	}
	plan.SceneID = scene.ID
	return plan, nil
}
