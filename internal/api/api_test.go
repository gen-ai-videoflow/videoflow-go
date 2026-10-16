package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"6eek-go/internal/agents"
	"6eek-go/internal/api"
	"6eek-go/internal/config"
	"6eek-go/internal/db"
	"6eek-go/internal/events"
	"6eek-go/internal/jobs"
	"6eek-go/internal/media"
	"6eek-go/internal/models"
	"6eek-go/internal/providers"
)

func setupTestServer(t *testing.T) (*api.Server, func()) {
	testDBPath := filepath.Join(t.TempDir(), "test_api.sqlite")
	cfg := config.Load()
	cfg.DatabasePath = testDBPath
	cfg.StorageRoot = t.TempDir()
	cfg.OpenRouterAPIKey = ""

	database, err := db.Open(testDBPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	broker := events.NewBroker()
	mediaEngine := media.NewMediaEngine()
	openrouter := providers.NewOpenRouterClient(cfg.OpenRouterAPIKey)
	agentsEngine := agents.NewAgentEngine(openrouter, "openai/gpt-4o-mini")

	workers := jobs.NewWorkerPool(cfg, database, broker, mediaEngine)
	workers.Start()

	server := api.NewServer(cfg, database, broker, workers, agentsEngine, openrouter, mediaEngine)

	cleanup := func() {
		server.Close()
		workers.Stop()
		database.Close()
		os.Remove(testDBPath)
	}

	return server, cleanup
}

func TestHealthEndpoint(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	server.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse json: %v", err)
	}
	if res["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", res["status"])
	}
}

func TestProjectsAndScenesEndpoints(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Create Project
	createPayload := []byte(`{"name":"New Test Studio","description":"Testing Chi API"}`)
	req := httptest.NewRequest("POST", "/projects", bytes.NewReader(createPayload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	// 2. List Projects
	req = httptest.NewRequest("GET", "/projects", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	// 3. Create Scene
	scenePayload := []byte(`{"title":"Act 1 Scene 1","summary":"Beginning of journey","duration":10,"aspect_ratio":"16:9"}`)
	req = httptest.NewRequest("POST", "/scenes", bytes.NewReader(scenePayload))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()

	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	// 4. Graph endpoint
	req = httptest.NewRequest("GET", "/graph", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
}

func TestMissingEndpointsResolution(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. GET /settings/providers/openrouter
	req := httptest.NewRequest("GET", "/settings/providers/openrouter", nil)
	rr := httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /settings/providers/openrouter, got %d", rr.Code)
	}

	// 2. GET /settings/providers/atlas
	req = httptest.NewRequest("GET", "/settings/providers/atlas", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /settings/providers/atlas, got %d", rr.Code)
	}

	// 3. GET /settings/providers
	req = httptest.NewRequest("GET", "/settings/providers", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /settings/providers, got %d", rr.Code)
	}

	// 4. GET /settings/agents
	req = httptest.NewRequest("GET", "/settings/agents", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /settings/agents, got %d", rr.Code)
	}
	var agents []map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &agents); err != nil || len(agents) < 10 {
		t.Fatalf("expected full agent list, got %v", agents)
	}

	// 5. GET /assets/types
	req = httptest.NewRequest("GET", "/assets/types", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /assets/types, got %d", rr.Code)
	}
	var typesRes map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &typesRes); err != nil || typesRes["types"] == nil {
		t.Fatalf("expected types object wrapper, got %v", typesRes)
	}

	// 6. GET /caption-styles and /caption-config
	req = httptest.NewRequest("GET", "/caption-styles", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /caption-styles, got %d", rr.Code)
	}

	req = httptest.NewRequest("GET", "/caption-config", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /caption-config, got %d", rr.Code)
	}

	// 7. Create scene and test scene action endpoints
	scenePayload := []byte(`{"title":"Act 2","summary":"Middle scene","duration":5,"aspect_ratio":"16:9"}`)
	req = httptest.NewRequest("POST", "/scenes", bytes.NewReader(scenePayload))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 for scene create, got %d", rr.Code)
	}
	var sc map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &sc)
	sceneID := sc["id"].(string)

	// POST /scenes/{id}/render
	req = httptest.NewRequest("POST", "/scenes/"+sceneID+"/render", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for scene render, got %d", rr.Code)
	}

	// POST /scenes/{id}/shots/generate?background=true
	req = httptest.NewRequest("POST", "/scenes/"+sceneID+"/shots/generate?background=true", bytes.NewReader([]byte(`{"auto_assets":true}`)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for shots generate background, got %d", rr.Code)
	}

	// POST /scenes/{id}/storyboard?background=true
	req = httptest.NewRequest("POST", "/scenes/"+sceneID+"/storyboard?background=true", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for storyboard background, got %d", rr.Code)
	}
}

func TestEditorAndPanelEndpoints(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	database := server.Database()
	projectID, err := database.GetActiveProjectID()
	if err != nil {
		t.Fatalf("expected active project: %v", err)
	}

	// 1. Create a scene, shot, render job, and output
	scene := &models.Scene{
		ProjectID:   &projectID,
		Title:       "Opening Scene",
		Summary:     "A sunrise over mountains",
		Duration:    5,
		AspectRatio: "16:9",
	}
	_ = database.CreateScene(scene)

	shot := &models.Shot{
		SceneID:   scene.ID,
		ShotOrder: 0,
		Duration:  5,
		Prompt:    "Slow cinematic pan of mountains",
	}
	_ = database.CreateShot(shot)

	stage := "completed"
	progress := "100%"
	job := &models.RenderJob{
		ProjectID:   &projectID,
		SceneID:     &scene.ID,
		ShotID:      &shot.ID,
		Provider:    "atlascloud",
		Model:       "kling-v1-6",
		Status:      "succeeded",
		Stage:       &stage,
		Progress:    &progress,
		RequestJSON: []byte(fmt.Sprintf(`{"spec":{"scene_id":"%s","duration":5,"prompt":"%s"}}`, scene.ID, shot.Prompt)),
	}
	_ = database.CreateRenderJob(job)

	score := 9.0
	out := &models.RenderOutput{
		RenderJobID:  job.ID,
		VideoPath:    "storage/outputs/sample.mp4",
		CaptionsJSON: []byte(`[{"start":0,"end":2.5,"text":"Hello world"}]`),
		Score:        &score,
		QAJSON:       []byte(`{"issues":["slight motion jitter"]}`),
		Selected:     true,
	}
	_ = database.CreateRenderOutput(out)

	// 2. Test GET /outputs/{id}/editor
	req := httptest.NewRequest("GET", "/outputs/"+out.ID+"/editor", nil)
	rr := httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for editor data, got %d", rr.Code)
	}
	var editorData map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &editorData); err != nil {
		t.Fatalf("failed to decode editor data: %v", err)
	}
	outputBlock := editorData["output"].(map[string]interface{})
	if outputBlock["qa_issues"] == nil {
		t.Fatalf("expected non-nil qa_issues array")
	}
	qaIssues := outputBlock["qa_issues"].([]interface{})
	if len(qaIssues) != 1 || qaIssues[0] != "slight motion jitter" {
		t.Fatalf("unexpected qa_issues: %v", qaIssues)
	}
	captionsBlock := editorData["captions"].(map[string]interface{})
	if captionsBlock["available"] != true {
		t.Fatalf("expected captions available=true")
	}
	shotsBlock := editorData["shots"].([]interface{})
	if len(shotsBlock) != 1 {
		t.Fatalf("expected 1 shot in shots block, got %d", len(shotsBlock))
	}
	sceneBlock := editorData["scene"].(map[string]interface{})
	if sceneBlock["title"] != "Opening Scene" {
		t.Fatalf("expected scene title 'Opening Scene', got %v", sceneBlock["title"])
	}

	// 3. Test POST /outputs/{id}/retry
	req = httptest.NewRequest("POST", "/outputs/"+out.ID+"/retry", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for retry output, got %d", rr.Code)
	}
	var retryRes map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &retryRes)
	if retryRes["job_id"] == nil || retryRes["job_id"] == "" {
		t.Fatalf("expected job_id in retry output response, got %v", retryRes)
	}

	// 4. Test POST /outputs/{id}/run-qa?background=true
	req = httptest.NewRequest("POST", "/outputs/"+out.ID+"/run-qa?background=true", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for run-qa background, got %d", rr.Code)
	}
	var qaOpRes map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &qaOpRes)
	if qaOpRes["op_id"] == nil {
		t.Fatalf("expected op_id for run-qa background, got %v", qaOpRes)
	}

	// 5. Test PUT /outputs/{id}/captions?background=true
	capsPayload := []byte(`{"segments":[{"start":0,"end":3.0,"text":"Edited caption"}],"style":"cinematic"}`)
	req = httptest.NewRequest("PUT", "/outputs/"+out.ID+"/captions?background=true", bytes.NewReader(capsPayload))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for update captions background, got %d", rr.Code)
	}

	// 6. Test POST /style/ingest?background=true
	req = httptest.NewRequest("POST", "/style/ingest?background=true", bytes.NewReader([]byte(`{"asset_ids":[]}`)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for style ingest background, got %d: %s", rr.Code, rr.Body.String())
	}

	// 7. Test GET /graph includes render_job and output nodes
	req = httptest.NewRequest("GET", "/graph", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /graph, got %d", rr.Code)
	}
	var graphRes map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &graphRes)
	nodes := graphRes["nodes"].([]interface{})
	hasJobNode := false
	hasOutputNode := false
	for _, n := range nodes {
		nodeMap := n.(map[string]interface{})
		if nodeMap["type"] == "render_job" {
			hasJobNode = true
		}
		if nodeMap["type"] == "output" {
			hasOutputNode = true
		}
	}
	if !hasJobNode {
		t.Fatalf("expected graph to contain render_job node")
	}
	if !hasOutputNode {
		t.Fatalf("expected graph to contain output node")
	}
}
