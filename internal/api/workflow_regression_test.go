package api

import (
	"6eek-go/internal/models"
	"encoding/json"
	"net/http"
	"testing"
)

func auditMock(t *testing.T, content string) *Server {
	return routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	})
}
func TestAuditDevelopIdeaContract(t *testing.T) {
	s := auditMock(t, `{"option_a":{"title":"A","narrative":"First"},"option_b":{"title":"B","narrative":"Second"},"recommendation":"A"}`)
	r := call(t, s, "POST", "/ideas/develop", `{"idea":"Cat story"}`)
	var body map[string]any
	json.Unmarshal(r.Body.Bytes(), &body)
	if _, ok := body["options"].([]any); !ok {
		t.Fatalf("UI expects options array; response=%s", r.Body.String())
	}
}
func TestAuditChatStateOnEmptyProject(t *testing.T) {
	s := auditMock(t, `{"reply":"Hello","action":"none"}`)
	r := call(t, s, "POST", "/chat", `{"message":"Hello"}`)
	var body struct {
		State struct {
			Scripts  int
			HasStyle bool `json:"has_style"`
		}
	}
	json.Unmarshal(r.Body.Bytes(), &body)
	if body.State.Scripts != 0 || body.State.HasStyle {
		t.Fatalf("empty project reported scripts=%d has_style=%v", body.State.Scripts, body.State.HasStyle)
	}
}
func TestAuditSceneExpansionHonorsCast(t *testing.T) {
	s := auditMock(t, `{"shots":[{"shot_order":1,"duration":5,"prompt":"A cat","camera":"wide","movement":"still"}]}`)
	r := call(t, s, "POST", "/characters", `{"name":"Mimi"}`)
	var c models.Character
	json.Unmarshal(r.Body.Bytes(), &c)
	r = call(t, s, "POST", "/scenes", `{"title":"Scene","summary":"Cat story"}`)
	var scene models.Scene
	json.Unmarshal(r.Body.Bytes(), &scene)
	r = call(t, s, "POST", "/scenes/"+scene.ID+"/generate", `{"character_ids":["`+c.ID+`"]}`)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	scenePtr, _ := s.database.GetScene(scene.ID)
	var ids []string
	json.Unmarshal(scenePtr.CharacterIDsJSON, &ids)
	if len(ids) != 1 {
		t.Fatalf("selected cast discarded: %s", scenePtr.CharacterIDsJSON)
	}
}
func TestCaptionFailurePreservesPreviousOutput(t *testing.T) {
	s := auditMock(t, `{}`)
	job := &models.RenderJob{Status: "succeeded", Provider: "openrouter", Model: "fixture"}
	if err := s.database.CreateRenderJob(job); err != nil {
		t.Fatal(err)
	}
	path := "storage/old-captioned.mp4"
	out := &models.RenderOutput{RenderJobID: job.ID, VideoPath: "storage/raw.mp4", CaptionedPath: &path}
	if err := s.database.CreateRenderOutput(out); err != nil {
		t.Fatal(err)
	}
	r := call(t, s, "PUT", "/outputs/"+out.ID+"/captions", `{"segments":[{"start":0,"end":1,"text":"Hi"}],"style":"clean"}`)
	if r.Code < 400 {
		t.Fatalf("caption save falsely succeeded: %d", r.Code)
	}
	updated, _ := s.database.GetRenderOutput(out.ID)
	if updated.CaptionedPath == nil || *updated.CaptionedPath != path {
		t.Fatalf("editor promises re-burn, endpoint only saves segments and removes captioned output: %s", r.Body.String())
	}
}

func TestCastRemovalInvalidatesConditioningSnapshot(t *testing.T) {
	s := auditMock(t, `{}`)
	project, _ := s.database.GetActiveProjectID()
	c := &models.Character{ProjectID: &project, Name: "Mimi"}
	if err := s.database.CreateCharacter(c); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal([]string{c.ID})
	snapshot, _ := json.Marshal(map[string]any{"generation_brief": generationBrief{Idea: "Cat", CharacterIDs: []string{c.ID}, CharacterNotes: "old cat", ReferenceAssetIDs: []string{"old-reference"}}})
	scene := &models.Scene{ProjectID: &project, Title: "Cat", CharacterIDsJSON: raw, SceneJSON: snapshot, AssetIDsJSON: json.RawMessage(`["old-keyframe"]`)}
	if err := s.database.CreateScene(scene); err != nil {
		t.Fatal(err)
	}
	r := call(t, s, "DELETE", "/scenes/"+scene.ID+"/cast/"+c.ID, "")
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	saved, _ := s.database.GetScene(scene.ID)
	b := sceneBrief(saved)
	if len(b.ReferenceAssetIDs) > 0 || b.CharacterNotes != "" || len(b.CharacterIDs) > 0 || string(saved.AssetIDsJSON) != "[]" {
		t.Fatalf("removed cast retained conditioning: %+v assets %s", b, saved.AssetIDsJSON)
	}
}
