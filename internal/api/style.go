package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"6eek-go/internal/models"
)

func (s *Server) handleGetStyle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	style, err := s.database.GetStyleGuide(projectID)
	if err != nil {
		// Return default preset
		style = &models.StyleGuide{
			ProjectID:   &projectID,
			Name:        "2D Picture Book",
			StylePrompt: "warm 2D children's picture-book illustration, clean shapes, expressive faces",
			Palette:     "soft pastel colors",
			Lighting:    "gentle even lighting",
			Audience:    "children aged 5 to 9",
			Tone:        "warm and playful",
		}
	}
	writeJSON(w, http.StatusOK, style)
}

func (s *Server) handleUpdateStyle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	var style models.StyleGuide
	if err := json.NewDecoder(r.Body).Decode(&style); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	style.ProjectID = &projectID
	if err := s.database.UpsertStyleGuide(&style); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, style)
}

func (s *Server) handleIngestStyle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var body struct {
		AssetIDs []string `json:"asset_ids"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.operation(w, r, &models.Op{Kind: "style_ingest", ProjectID: &projectID}, func(ctx context.Context) (any, error) {
		ids := body.AssetIDs
		if len(ids) == 0 {
			assets, err := s.database.ListAssets(projectID)
			if err != nil {
				return nil, err
			}
			for _, asset := range assets {
				if asset.Type == "character_reference" || asset.Type == "location" {
					ids = append(ids, asset.ID)
					if len(ids) == 4 {
						break
					}
				}
			}
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("upload at least one image reference before extracting style")
		}
		if len(ids) > 4 {
			ids = ids[:4]
		}
		images := []string{}
		for _, id := range ids {
			asset, err := s.database.GetAsset(id)
			if err != nil {
				return nil, err
			}
			if asset.ProjectID == nil || *asset.ProjectID != projectID {
				return nil, fmt.Errorf("reference is outside active project")
			}
			image, err := s.imageURL(asset)
			if err != nil {
				return nil, err
			}
			images = append(images, image)
		}
		var style models.StyleGuide
		if err = s.agentJSON(ctx, "style_agent", "Analyze the image references. Return name, style_prompt, palette, lighting, audience, tone. Describe only visible style features.", "Extract a consistent production style.", images, &style); err != nil {
			return nil, err
		}
		if style.StylePrompt == "" {
			return nil, fmt.Errorf("OpenRouter returned no style prompt")
		}
		style.ProjectID = &projectID
		if err = s.database.UpsertStyleGuide(&style); err != nil {
			return nil, err
		}
		return style, nil
	})
}
