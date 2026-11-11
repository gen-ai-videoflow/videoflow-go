package api

import (
	"6eek-go/internal/models"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGuidedCharacterToTwoOutputsAndCombinedExport(t *testing.T) {
	binary, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("real FFmpeg fixture required")
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mp4")
	if b, err := exec.Command(binary, "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=160x120:d=1", "-c:v", "libx264", "-pix_fmt", "yuv420p", clip).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, b)
	}
	clipBytes, _ := os.ReadFile(clip)
	var submissions, referenceImages atomic.Int32
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
		case r.URL.Path == "/chat/completions":
			var body struct{ Messages []struct{ Content string } }
			json.NewDecoder(r.Body).Decode(&body)
			system := body.Messages[0].Content
			content := `{"prompt":"Mimi the cat, cinematic framing"}`
			if strings.Contains(system, "scriptwriter") {
				content = `{"title":"Mimi","summary":"Cat story","scenes":[{"title":"One","summary":"Mimi arrives","duration":1,"aspect_ratio":"16:9"},{"title":"Two","summary":"Mimi plays","duration":1,"aspect_ratio":"16:9"}]}`
			}
			if strings.Contains(system, "cinematographer") {
				content = `{"shots":[{"shot_order":1,"duration":1,"prompt":"Mimi plays","camera":"wide","movement":"still"}]}`
			}
			if !strings.Contains(body.Messages[1].Content, "Mimi") {
				t.Error("character context did not reach text stage")
			}
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
		case r.URL.Path == "/images/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"aspect_ratio":{"type":"enum","values":["16:9"]},"input_references":{"type":"array","min":0,"max":8}}}]}`))
		case r.URL.Path == "/images":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["input_references"] == nil {
				t.Error("uploaded character photo not sent to image model")
			}
			referenceImages.Add(1)
			w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgo=","media_type":"image/png"}]}`))
		case r.URL.Path == "/videos/models":
			w.Write([]byte(`{"data":[{"id":"google/veo-3.1-lite","supported_durations":[1],"supported_aspect_ratios":["16:9"],"supported_frame_images":["first_frame"]}]}`))
		case r.URL.Path == "/videos":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			prompt, _ := body["prompt"].(string)
			if body["frame_images"] == nil || !strings.Contains(prompt, "Chinese") || !strings.Contains(prompt, "Mimi") || !strings.Contains(prompt, "Warm playful direction") {
				t.Errorf("brief or keyframe lost: %v", body)
			}
			fmt.Fprintf(w, `{"id":"upstream-%d"}`, submissions.Add(1))
		case strings.HasSuffix(r.URL.Path, "/content"):
			w.Write(clipBytes)
		case strings.HasPrefix(r.URL.Path, "/videos/upstream-"):
			w.Write([]byte(`{"status":"completed"}`))
		default:
			http.NotFound(w, r)
		}
	})
	project, _ := s.database.GetActiveProjectID()
	character := &models.Character{ProjectID: &project, Name: "Mimi", Description: "A cat", Appearance: "white fur"}
	s.database.CreateCharacter(character)
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aO1cAAAAASUVORK5CYII=")
	imagePath := filepath.Join(s.cfg.StorageRoot, "photo.png")
	os.WriteFile(imagePath, png, 0600)
	asset := &models.Asset{ProjectID: &project, CharacterID: &character.ID, Type: "character_reference", Name: "Photo", FilePath: imagePath}
	if err = s.database.CreateUploadedAsset(asset); err != nil {
		t.Fatal(err)
	}
	s.workers.Start()
	defer s.workers.Stop()
	request, _ := json.Marshal(map[string]any{"idea": "Mimi plays", "scene_count": 2, "target_duration": 2, "aspect_ratio": "16:9", "language": "Chinese", "style": "cinematic", "instruction": "Warm playful direction", "character_ids": []string{character.ID}})
	response := call(t, s, "POST", "/videos/generate", string(request))
	if response.Code != 202 {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
	var started struct {
		ID string `json:"op_id"`
	}
	json.Unmarshal(response.Body.Bytes(), &started)
	var outputIDs []string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		op, _ := s.database.GetOp(started.ID)
		if op.Status == "failed" {
			t.Fatalf("guided failed: %v", op.Error)
		}
		jobs, _ := s.database.ListRenderJobs(project)
		outputIDs = nil
		for _, job := range jobs {
			if job.Status == "failed" || job.Status == "reconciliation_required" {
				t.Fatalf("render failed: %+v", job)
			}
			outputs, _ := s.database.ListOutputsForJob(job.ID)
			for _, out := range outputs {
				outputIDs = append(outputIDs, out.ID)
			}
		}
		if len(outputIDs) == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(outputIDs) != 2 || submissions.Load() != 2 || referenceImages.Load() != 2 {
		t.Fatalf("incomplete flow: outputs=%v video=%d image=%d", outputIDs, submissions.Load(), referenceImages.Load())
	}
	scenes, _ := s.database.ListScenes(project)
	for _, scene := range scenes {
		var ids []string
		json.Unmarshal(scene.CharacterIDsJSON, &ids)
		if len(ids) != 1 || ids[0] != character.ID {
			t.Fatal("cast not persisted")
		}
	}
	// Captions must produce a real replacement file, not just save segments.
	response = call(t, s, "PUT", "/outputs/"+outputIDs[0]+"/captions", `{"segments":[{"start":0,"end":0.8,"text":"Mimi"}],"style":"clean"}`)
	if response.Code != 200 {
		t.Fatalf("caption burn: %d %s", response.Code, response.Body.String())
	}
	out, _ := s.database.GetRenderOutput(outputIDs[0])
	if out.CaptionedPath == nil {
		t.Fatal("captioned file missing")
	}
	raw, _ := json.Marshal(map[string]any{"output_ids": outputIDs, "aspect_ratio": "16:9"})
	response = call(t, s, "POST", "/outputs/assemble", string(raw))
	if response.Code != 200 {
		t.Fatalf("assembly: %d %s", response.Code, response.Body.String())
	}
	var assembled struct {
		ID string `json:"output_id"`
	}
	json.Unmarshal(response.Body.Bytes(), &assembled)
	combined, _ := s.database.GetRenderOutput(assembled.ID)
	full, _ := s.storagePath(combined.VideoPath)
	if b, err := exec.Command(binary, "-v", "error", "-i", full, "-c:v", "rawvideo", "-c:a", "pcm_s16le", "-f", "null", "-").CombinedOutput(); err != nil {
		t.Fatalf("export decode failed: %v %s", err, b)
	}
	response = call(t, s, "GET", "/outputs/"+assembled.ID+"/download", "")
	if response.Code != 200 || response.Body.Len() == 0 {
		t.Fatal("combined download unavailable")
	}

}
