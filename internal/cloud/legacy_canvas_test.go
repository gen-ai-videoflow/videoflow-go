package cloud

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCloudCanvasGraphAndRelationshipIsolation(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Canvas", "")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.CreateProject(ctx, u, "Other", "")
	if err != nil {
		t.Fatal(err)
	}
	scene, err := s.CreateCanvasEntity(ctx, u, p.ID, "scenes", map[string]any{"title": "Scene", "summary": "A room", "aspect_ratio": "9:16"})
	if err != nil {
		t.Fatal(err)
	}
	character, err := s.CreateCanvasEntity(ctx, u, p.ID, "characters", map[string]any{"name": "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := s.CreateCanvasEntity(ctx, u, p2.ID, "characters", map[string]any{"name": "Other"})
	if err != nil {
		t.Fatal(err)
	}
	if updateErr := s.PatchCanvasEntity(ctx, u, p.ID, "characters", character["id"].(string), map[string]any{"name": "安娜 / Ana", "description": "A patient explorer", "content": "短发，蓝色外套", "prompt": "Keep the blue coat consistent"}); updateErr != nil {
		t.Fatal(updateErr)
	}
	designGraph, graphErr := s.CloudCanvasGraph(ctx, u, p.ID)
	if graphErr != nil {
		t.Fatal(graphErr)
	}
	foundDesign := false
	for _, node := range designGraph.Nodes {
		if node.ID == character["id"] {
			foundDesign = node.Data["content"] == "短发，蓝色外套" && node.Data["prompt"] == "Keep the blue coat consistent"
		}
	}
	if !foundDesign {
		t.Fatal("character design missing from canvas graph")
	}
	edge := CanvasEdge{Source: scene["id"].(string), Target: character["id"].(string), Label: "casts"}
	if err = s.SetCanvasEdge(ctx, u, p.ID, edge, false); err != nil {
		t.Fatal(err)
	}
	if err = s.SetCanvasEdge(ctx, u, p.ID, edge, false); err != nil {
		t.Fatal(err)
	}
	edge.Target = foreign["id"].(string)
	if err = s.SetCanvasEdge(ctx, u, p.ID, edge, false); err == nil {
		t.Fatal("cross-project target allowed")
	}
	_, other := testPolicyStore(t)
	if _, err = s.CloudCanvasGraph(ctx, other, p.ID); err == nil {
		t.Fatal("cross-user graph leaked")
	}
	if err = s.PatchCanvasEntity(ctx, u, p2.ID, "scenes", scene["id"].(string), map[string]any{"summary": "stolen"}); err == nil {
		t.Fatal("cross-project edit allowed")
	}
	graph, err := s.CloudCanvasGraph(ctx, u, p.ID)
	if err != nil || len(graph.Nodes) != 2 || len(graph.Edges) != 1 {
		t.Fatalf("graph %+v %v", graph, err)
	}
	if err = s.PatchCanvasEntity(ctx, u, p.ID, "scenes", scene["id"].(string), map[string]any{"summary": "Updated", "aspect_ratio": "1:1"}); err != nil {
		t.Fatal(err)
	}
	if err = s.PatchCanvasEntity(ctx, u, p.ID, "scenes", scene["id"].(string), map[string]any{"workspace_id": "other"}); err == nil {
		t.Fatal("ownership patch accepted")
	}
	detached, err := s.DeleteCanvasEntity(ctx, u, p.ID, "characters", character["id"].(string))
	if err != nil || detached != 1 {
		t.Fatalf("detach count %d %v", detached, err)
	}
	graph, err = s.CloudCanvasGraph(ctx, u, p.ID)
	if err != nil || len(graph.Edges) != 0 || len(graph.Nodes) != 1 {
		t.Fatalf("deleted edges leaked %+v %v", graph, err)
	}

}

func TestCloudCanvasDoesNotExposePrivateObjectKeys(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Private graph", "")
	if err != nil {
		t.Fatal(err)
	}
	uploadKey := "workspaces/" + u.WorkspaceID + "/uploads/private.png"
	if err = s.ReserveUpload(ctx, u.WorkspaceID, uploadKey, 100); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateCanvasEntity(ctx, u, p.ID, "assets", map[string]any{"name": "Reference", "object_key": uploadKey, "asset_type": "image"}); err != nil {
		t.Fatal(err)
	}
	if err = s.GrantCredits(ctx, u.WorkspaceID, "graph-output", 10, ""); err != nil {
		t.Fatal(err)
	}
	q, err := s.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "test", Duration: 5, AspectRatio: "16:9"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.SubmitJob(ctx, u, q.ID, "canvas-output-key")
	if err != nil {
		t.Fatal(err)
	}
	outputKey := "workspaces/" + u.WorkspaceID + "/private/output.mp4"
	if err = s.FinishJob(ctx, j.ID, "succeeded", outputKey, ""); err != nil {
		t.Fatal(err)
	}
	graph, err := s.CloudCanvasGraph(ctx, u, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), uploadKey) || strings.Contains(string(raw), outputKey) {
		t.Fatal("private S3 key leaked in graph")
	}
	if len(graph.Nodes) != 3 || len(graph.Edges) != 1 {
		t.Fatalf("missing output graph %+v", graph)
	}
	for _, node := range graph.Nodes {
		if node.Type == "output" && node.Data["download_endpoint"] != "/v1/jobs/"+j.ID+"/download" {
			t.Fatal("output missing authorized download endpoint")
		}
	}
}
