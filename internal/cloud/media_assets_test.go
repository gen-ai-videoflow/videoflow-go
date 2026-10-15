package cloud

import (
	"context"
	"encoding/json"
	"testing"
)

func TestOwnedGeneratedOutputAndPublication(t *testing.T) {
	s, u, token := securityTestServer(t)
	ctx := context.Background()
	p, e := s.Store.CreateProject(ctx, u, "Output test", "")
	if e != nil {
		t.Fatal(e)
	}
	s.Store.GrantCredits(ctx, u.WorkspaceID, "output-test", 100, "")
	q, e := s.Store.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "A story", Duration: 5, AspectRatio: "16:9"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	j, e := s.Store.SubmitJob(ctx, u, q.ID, "output-test")
	if e != nil {
		t.Fatal(e)
	}
	path := "/jobs/" + j.ID + "/output"
	if w := securityRequest(s, "GET", path, token, nil); w.Code != 409 {
		t.Fatalf("unready %d %s", w.Code, w.Body.String())
	}
	if e = s.Store.FinishJob(ctx, j.ID, "succeeded", "workspaces/"+u.WorkspaceID+"/output.mp4", ""); e != nil {
		t.Fatal(e)
	}
	w := securityRequest(s, "GET", path, token, nil)
	if w.Code != 200 {
		t.Fatalf("output %d %s", w.Code, w.Body.String())
	}
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	asset, _ := out["asset_id"].(string)
	if asset == "" {
		t.Fatal(out)
	}
	body := map[string]any{"source_asset_id": asset, "title": "Story", "visibility": "private", "audience": "free", "format": "standard"}
	w = securityRequest(s, "POST", "/publications", token, body)
	if w.Code != 200 {
		t.Fatalf("publication %d %s", w.Code, w.Body.String())
	}
	var v Video
	json.Unmarshal(w.Body.Bytes(), &v)
	again := securityRequest(s, "POST", "/publications", token, body)
	var same Video
	json.Unmarshal(again.Body.Bytes(), &same)
	if same.ID != v.ID {
		t.Fatalf("duplicate publication: %s", again.Body.String())
	}
	_, _, foreign := securityTestServer(t)
	if w = securityRequest(s, "GET", path, foreign, nil); w.Code != 404 {
		t.Fatalf("foreign output %d", w.Code)
	}
	if w = securityRequest(s, "POST", "/publications", foreign, body); w.Code != 404 {
		t.Fatalf("foreign publication %d", w.Code)
	}
	body["format"] = "short"
	body["source_asset_id"] = asset
	body["title"] = "Short"
	if w = securityRequest(s, "POST", "/publications", token, body); w.Code != 400 {
		t.Fatalf("unmeasured short %d", w.Code)
	}
}
