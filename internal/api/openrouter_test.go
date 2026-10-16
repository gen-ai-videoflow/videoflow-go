package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"6eek-go/internal/agents"
	"6eek-go/internal/config"
	"6eek-go/internal/db"
	"6eek-go/internal/events"
	"6eek-go/internal/jobs"
	"6eek-go/internal/media"
	"6eek-go/internal/models"
	"6eek-go/internal/providers"
)

func routerServer(t *testing.T, h http.HandlerFunc) *Server {
	t.Helper()
	remote := httptest.NewServer(h)
	t.Cleanup(remote.Close)
	cfg := config.Load()
	cfg.StorageRoot = t.TempDir()
	cfg.DatabasePath = filepath.Join(t.TempDir(), "db.sqlite")
	cfg.OpenRouterAPIKey = ""
	cfg.OpenRouterBaseURL = remote.URL
	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err = database.SetProviderSecret("openrouter", "saved-key", nil); err != nil {
		t.Fatal(err)
	}
	broker := events.NewBroker()
	engine := media.NewMediaEngine()
	client := providers.NewOpenRouterClientAt("", remote.URL)
	server := NewServer(cfg, database, broker, jobs.NewWorkerPool(cfg, database, broker, engine), agents.NewAgentEngine(client, "openai/gpt-4o-mini"), client, engine)
	t.Cleanup(server.Close)
	return server
}
func call(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, r)
	return w
}
func TestOpenRouterChecksAreReal(t *testing.T) {
	calls := 0
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"invalid key"}}`))
	})
	for _, path := range []string{"/settings/providers/openrouter/test", "/settings/providers/openrouter/verify-model"} {
		w := call(t, s, "POST", path, `{"model":"missing"}`)
		var result map[string]any
		json.Unmarshal(w.Body.Bytes(), &result)
		if result["ok"] != false || result["error"] == nil {
			t.Errorf("false success %s: %s", path, w.Body.String())
		}
	}
	if calls != 2 {
		t.Errorf("expected upstream checks, got %d", calls)
	}
}
func TestDiscoveryUsesLiveCatalog(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/videos/models" {
			t.Error(r.URL.Path)
		}
		w.Write([]byte(`{"data":[{"id":"new/video","supported_durations":[4,8]}]}`))
	})
	w := call(t, s, "GET", "/settings/providers/openrouter/models?modality=video", "")
	if !bytes.Contains(w.Body.Bytes(), []byte("new/video")) {
		t.Fatal(w.Body.String())
	}
}
func TestSavedKeyAndAgentSelectionUsed(t *testing.T) {
	var actualModel string
	var systemPrompt string
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer saved-key" {
			t.Error("DB key not used")
		}
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"selected/model","architecture":{"input_modalities":["text"],"output_modalities":["text"]}}]}`))
			return
		}
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		actualModel, _ = payload["model"].(string)
		systemPrompt = payload["messages"].([]any)[0].(map[string]any)["content"].(string)
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"reply\":\"hello\",\"action\":\"none\"}"}}]}`))
	})
	s.database.SetAppSetting("dialogue_language", `"Malay"`)
	for _, model := range []string{"first/model", "selected/model"} {
		if err := s.database.SetAgentSetting("intent_agent", "openrouter", model); err != nil {
			t.Fatal(err)
		}
	}
	w := call(t, s, "POST", "/chat", `{"message":"hello"}`)
	if w.Code != 200 || actualModel != "selected/model" {
		t.Fatalf("status %d model %q body %s", w.Code, actualModel, w.Body.String())
	}
	if !strings.Contains(systemPrompt, "Malay") {
		t.Fatal("dialogue language ignored", systemPrompt)
	}
	settings, err := s.database.GetAgentSettings()
	if err != nil || settings["intent_agent"].Model != "selected/model" {
		t.Fatal(settings, err)
	}
}
func TestSavingURLPreservesKey(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {})
	w := call(t, s, "PUT", "/settings/providers/openrouter", `{"base_url":"https://openrouter.ai/api/v1"}`)
	key, _, _, err := s.database.GetProviderSecret("openrouter")
	if w.Code != 200 || err != nil || key != "saved-key" {
		t.Fatalf("key lost: status %d err %v", w.Code, err)
	}
}
func TestRejectOtherProviders(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not contact upstream") })
	for _, test := range []struct{ path, body string }{
		{"/settings/app", `{"default_video_provider":"atlascloud"}`},
		{"/settings/agents/script_agent", `{"provider":"openai","model":"gpt-4o"}`},
		{"/settings/connections", `{"label":"Other","preset":"openai","protocol":"chat","base_url":"https://api.openai.com/v1"}`},
	} {
		method := "PUT"
		if test.path == "/settings/connections" {
			method = "POST"
		}
		w := call(t, s, method, test.path, test.body)
		if w.Code != 400 {
			t.Errorf("accepted non OpenRouter: %s %d %s", test.path, w.Code, w.Body.String())
		}
	}
}

func TestImagesStoredAndCharacterLinked(t *testing.T) {
	images := 0
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
		case "/chat/completions":
			w.Write([]byte(`{"choices":[{"message":{"content":"{\"prompt\":\"Image brief\"}"}}]}`))
		case "/images/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"aspect_ratio":{"type":"enum","values":["1:1","9:16"]}}}]}`))
		case "/images":
			images++
			w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgo=","media_type":"image/png"}]}`))
		default:
			t.Error(r.URL.Path)
			w.WriteHeader(404)
		}
	})
	w := call(t, s, "POST", "/characters", `{"name":"Alice","appearance":"Red hat"}`)
	var character map[string]any
	json.Unmarshal(w.Body.Bytes(), &character)
	id, _ := character["id"].(string)
	if id == "" {
		t.Fatal(w.Body.String())
	}
	w = call(t, s, "POST", "/characters/"+id+"/reference-sheets", `{}`)
	if w.Code != 200 || images != 1 {
		t.Fatalf("expected actual image generation: %d %d %s", w.Code, images, w.Body.String())
	}
	saved, err := s.database.GetCharacter(id)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	json.Unmarshal(saved.ReferenceAssetIDsJSON, &ids)
	if len(ids) != 1 {
		t.Fatal("reference not linked", saved)
	}
	asset, err := s.database.GetAsset(ids[0])
	if err != nil || asset.CharacterID == nil || *asset.CharacterID != id {
		t.Fatal(asset, err)
	}
}

func TestMediaFailurePropagates(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(402)
		w.Write([]byte(`{"error":"insufficient credits"}`))
	})
	w := call(t, s, "POST", "/scenes", `{"title":"Scene","summary":"Ocean","duration":4,"aspect_ratio":"9:16"}`)
	var scene map[string]any
	json.Unmarshal(w.Body.Bytes(), &scene)
	id, _ := scene["id"].(string)
	w = call(t, s, "POST", "/scenes/"+id+"/storyboard", `{}`)
	if w.Code != 502 {
		t.Fatalf("provider error must fail: %d %s", w.Code, w.Body.String())
	}
}

func TestMissingOutputNeverReturnsSample(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {})
	for _, path := range []string{"/outputs/missing/editor", "/outputs/missing/download"} {
		w := call(t, s, "GET", path, "")
		if w.Code != 404 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
}
func TestRefineUsesSelectedAgent(t *testing.T) {
	requests := 0
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
			return
		}
		requests++
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"title\":\"Refined\",\"summary\":\"Actual model response\"}"}}]}`))
	})
	w := call(t, s, "POST", "/scenes", `{"title":"Before","summary":"Before"}`)
	var scene map[string]any
	json.Unmarshal(w.Body.Bytes(), &scene)
	w = call(t, s, "POST", "/scenes/"+scene["id"].(string)+"/refine", `{"instruction":"Improve it"}`)
	if w.Code != 200 || requests != 1 || !bytes.Contains(w.Body.Bytes(), []byte("Actual model response")) {
		t.Fatal(requests, w.Code, w.Body.String())
	}
}

func TestStoryboardHasFrontendAssetType(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
			return
		}
		if r.URL.Path == "/chat/completions" {
			w.Write([]byte(`{"choices":[{"message":{"content":"{\"prompt\":\"Storyboard brief\"}"}}]}`))
			return
		}
		if r.URL.Path == "/images/models" {
			w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"aspect_ratio":{"type":"enum","values":["9:16"]}}}]}`))
			return
		}
		w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgo=","media_type":"image/png"}]}`))
	})
	w := call(t, s, "POST", "/scenes", `{"title":"Scene","summary":"Ocean","duration":4,"aspect_ratio":"9:16"}`)
	var scene map[string]any
	json.Unmarshal(w.Body.Bytes(), &scene)
	w = call(t, s, "POST", "/scenes/"+scene["id"].(string)+"/storyboard", `{}`)
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	id, _ := result["asset_id"].(string)
	asset, err := s.database.GetAsset(id)
	if err != nil || asset.Type != "storyboard" {
		t.Fatalf("storyboard invisible in UI: %+v %v", asset, err)
	}
}
func TestResubmitCreatesFreshProviderJob(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {})
	oldID := "failed-upstream"
	job := &models.RenderJob{Provider: "openrouter", Model: "test/video", Status: "failed", ProviderJobID: &oldID, RequestJSON: json.RawMessage(`{"prompt":"Retry me"}`)}
	if err := s.database.CreateRenderJob(job); err != nil {
		t.Fatal(err)
	}
	w := call(t, s, "POST", "/render-jobs/"+job.ID+"/resubmit", `{}`)
	var res map[string]any
	json.Unmarshal(w.Body.Bytes(), &res)
	id, _ := res["job_id"].(string)
	fresh, err := s.database.GetRenderJob(id)
	if err != nil || id == job.ID || fresh.ProviderJobID != nil {
		t.Fatal("retry retained terminal ID", id, fresh, err)
	}
}

func TestSavedSceneDefaultsAreUsed(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {})
	w := call(t, s, "PUT", "/settings/app", `{"default_scene_duration":8,"default_aspect_ratio":"1:1"}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = call(t, s, "POST", "/scenes", `{"title":"Scene"}`)
	var scene models.Scene
	json.Unmarshal(w.Body.Bytes(), &scene)
	if scene.Duration != 8 || scene.AspectRatio != "1:1" {
		t.Fatal(scene)
	}
}

func TestImagePromptAgentSelectionIsUsed(t *testing.T) {
	promptModel := ""
	imagePrompt := ""
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			w.Write([]byte(`{"data":[{"id":"selected/prompter","architecture":{"output_modalities":["text"]}}]}`))
		case "/chat/completions":
			var p map[string]any
			json.NewDecoder(r.Body).Decode(&p)
			promptModel, _ = p["model"].(string)
			w.Write([]byte(`{"choices":[{"message":{"content":"{\"prompt\":\"Prepared image prompt\"}"}}]}`))
		case "/images/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"aspect_ratio":{"type":"enum","values":["9:16"]}}}]}`))
		case "/images":
			var p map[string]any
			json.NewDecoder(r.Body).Decode(&p)
			imagePrompt, _ = p["prompt"].(string)
			w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgo=","media_type":"image/png"}]}`))
		}
	})
	s.database.SetAgentSetting("prompt_agent", "openrouter", "selected/prompter")
	w := call(t, s, "POST", "/scenes", `{"title":"Scene","aspect_ratio":"9:16"}`)
	var scene models.Scene
	json.Unmarshal(w.Body.Bytes(), &scene)
	w = call(t, s, "POST", "/scenes/"+scene.ID+"/storyboard", `{}`)
	if w.Code != 200 || promptModel != "selected/prompter" || imagePrompt != "Prepared image prompt" {
		t.Fatal(w.Code, promptModel, imagePrompt, w.Body.String())
	}
}

func TestOpenRouterOnlyGuidedWorkflow(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
		case "/chat/completions":
			var payload struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			json.NewDecoder(r.Body).Decode(&payload)
			content := `{"prompt":"Consistent ocean scene"}`
			if strings.Contains(payload.Messages[0].Content, "scriptwriter") {
				content = `{"title":"Ocean","summary":"A wave","scenes":[{"title":"Wave","summary":"The ocean wave rises","duration":4,"aspect_ratio":"9:16"}]}`
			}
			if strings.Contains(payload.Messages[0].Content, "cinematographer") {
				content = `{"shots":[{"shot_order":1,"duration":4,"prompt":"Wave rising","camera":"wide","movement":"pan"}]}`
			}
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
		case "/images/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"aspect_ratio":{"type":"enum","values":["9:16"]}}}]}`))
		case "/images":
			w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgo=","media_type":"image/png"}]}`))
		case "/videos/models":
			w.Write([]byte(`{"data":[{"id":"google/veo-3.1-lite","supported_durations":[4],"supported_aspect_ratios":["9:16"],"supported_frame_images":["first_frame"]}]}`))
		case "/videos":
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			if payload["frame_images"] == nil {
				t.Error("guided storyboard was not sent")
			}
			w.WriteHeader(202)
			w.Write([]byte(`{"id":"guided-upstream"}`))
		case "/videos/guided-upstream":
			w.Write([]byte(`{"status":"completed"}`))
		case "/videos/guided-upstream/content":
			w.Write([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"))
		default:
			t.Errorf("unexpected provider path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	s.workers.Start()
	t.Cleanup(s.workers.Stop)
	w := call(t, s, "POST", "/videos/generate", `{"idea":"An ocean wave","scene_count":1,"target_duration":4,"aspect_ratio":"9:16"}`)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var started struct {
		ID string `json:"op_id"`
	}
	json.Unmarshal(w.Body.Bytes(), &started)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		op, err := s.database.GetOp(started.ID)
		if err != nil {
			t.Fatal(err)
		}
		if op.Status == "failed" {
			t.Fatal("pipeline failed", op.Error)
		}
		if op.Status == "succeeded" {
			var result struct {
				Jobs []string `json:"render_job_ids"`
			}
			json.Unmarshal(op.ResultJSON, &result)
			if len(result.Jobs) != 1 {
				t.Fatal(result)
			}
			job, _ := s.database.GetRenderJob(result.Jobs[0])
			if job.Status == "failed" {
				t.Fatal(job.Error)
			}
			if job.Status == "succeeded" {
				outputs, _ := s.database.ListOutputsForJob(job.ID)
				if len(outputs) != 1 || outputs[0].Score != nil {
					t.Fatal(outputs)
				}
				if !bytes.Contains(job.RequestJSON, []byte("frame_images")) {
					t.Fatal("actual request not persisted")
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("guided workflow did not complete")
}
