package api

import (
	"6eek-go/internal/models"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"os"
	"path/filepath"
)

// Keep the previous export until the replacement has actually been written.
func (s *Server) saveBurnedCaptions(ctx context.Context, output *models.RenderOutput, segments []models.CaptionSegment, style string) (any, error) {
	if len(segments) == 0 {
		output.CaptionsJSON, _ = json.Marshal(map[string]any{"segments": segments, "style": style})
		output.CaptionedPath = nil
		if err := s.database.UpdateRenderOutput(output); err != nil {
			return nil, err
		}
		return map[string]any{"output_id": output.ID, "status": "cleared"}, nil
	}
	source, err := s.storagePath(output.VideoPath)
	if err != nil {
		return nil, err
	}
	name := output.ID + "-" + uuid.NewString() + "-captioned.mp4"
	dest := filepath.Join(s.cfg.StorageRoot, "outputs", name)
	if err = os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return nil, err
	}
	if err = s.media.BurnCaptions(ctx, source, dest, segments, style); err != nil {
		os.Remove(dest)
		return nil, err
	}
	raw, _ := json.Marshal(map[string]any{"segments": segments, "style": style})
	public := filepath.ToSlash(filepath.Join("storage", "outputs", name))
	output.CaptionsJSON = raw
	output.CaptionedPath = &public
	if err = s.database.UpdateRenderOutput(output); err != nil {
		os.Remove(dest)
		return nil, err
	}
	return map[string]any{"output_id": output.ID, "status": "burned", "captioned_path": public}, nil
}
