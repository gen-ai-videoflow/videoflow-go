package api

import (
	"6eek-go/internal/models"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) handleAssembleOutputs(w http.ResponseWriter, r *http.Request) {
	project, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var body struct {
		OutputIDs []string `json:"output_ids"`
		Aspect    string   `json:"aspect_ratio"`
		Captioned bool     `json:"captioned"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.OutputIDs) < 2 || len(body.OutputIDs) > 20 {
		http.Error(w, "choose 2 to 20 clips in their playback order", 400)
		return
	}
	if body.Aspect == "" {
		body.Aspect = s.routes().Setting("default_aspect_ratio", "9:16")
	}
	if body.Aspect != "9:16" && body.Aspect != "16:9" && body.Aspect != "1:1" {
		http.Error(w, "unsupported assembly aspect ratio", 400)
		return
	}
	inputs := []string{}
	seen := map[string]bool{}
	for _, id := range body.OutputIDs {
		if seen[id] {
			http.Error(w, "a clip can only be selected once", 400)
			return
		}
		seen[id] = true
		output, err := s.database.GetRenderOutput(id)
		if err != nil {
			http.Error(w, "output not found", 404)
			return
		}
		job, err := s.database.GetRenderJob(output.RenderJobID)
		if err != nil || job.ProjectID == nil || *job.ProjectID != project || job.Status != "succeeded" {
			http.Error(w, "choose completed outputs from the active project", 400)
			return
		}
		path := output.VideoPath
		if body.Captioned {
			if output.CaptionedPath == nil {
				http.Error(w, "each selected clip needs a captioned export first", 400)
				return
			}
			path = *output.CaptionedPath
		}
		full, err := s.storagePath(path)
		if err != nil {
			http.Error(w, "invalid output path", 400)
			return
		}
		if info, err := os.Stat(full); err != nil || !info.Mode().IsRegular() {
			http.Error(w, "output file is missing", 400)
			return
		}
		inputs = append(inputs, full)
	}
	if !s.media.Available() {
		http.Error(w, "FFmpeg is required for local video assembly", 400)
		return
	}
	s.operation(w, r, &models.Op{Kind: "assembly", ProjectID: &project}, func(ctx context.Context) (any, error) {
		name := "assembly-" + uuid.NewString() + ".mp4"
		dest := filepath.Join(s.cfg.StorageRoot, "outputs", name)
		if err := s.media.Assemble(ctx, inputs, dest, body.Aspect); err != nil {
			return nil, err
		}
		raw, _ := json.Marshal(body)
		job := &models.RenderJob{ProjectID: &project, Provider: "local", Model: "ffmpeg-assembly", Status: "succeeded", RequestJSON: raw}
		output := &models.RenderOutput{VideoPath: filepath.ToSlash(filepath.Join("storage", "outputs", name))}
		if err := s.database.SaveLocalOutput(job, output); err != nil {
			os.Remove(dest)
			return nil, err
		}
		return map[string]any{"output_id": output.ID, "job_id": job.ID, "video_path": output.VideoPath, "download_url": "/outputs/" + output.ID + "/download"}, nil
	})
}
