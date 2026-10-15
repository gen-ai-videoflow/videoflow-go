package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

func TestStudioNestedOwnership(t *testing.T) {
	s, u := testPolicyStore(t)
	_, other := testPolicyStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Studio", "")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.CreateProject(ctx, u, "Other", "")
	if err != nil {
		t.Fatal(err)
	}
	scene, err := s.CreateStudioEntity(ctx, u, p.ID, "scenes", StudioEntity{Title: "Scene", Content: "A room"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StudioEntities(ctx, other, p.ID, "scenes", ""); err == nil {
		t.Fatal("cross-workspace read allowed")
	}
	if _, err = s.CreateStudioEntity(ctx, u, p2.ID, "shots", StudioEntity{SceneID: scene.ID, Title: "Wrong parent"}); err == nil {
		t.Fatal("cross-project scene substitution allowed")
	}
	shot, err := s.CreateStudioEntity(ctx, u, p.ID, "shots", StudioEntity{SceneID: scene.ID, Title: "Shot", Duration: 3})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateStudioEntity(ctx, u, p2.ID, "shots", shot.ID, map[string]any{"title": "stolen"}); err == nil {
		t.Fatal("cross-project update allowed")
	}
	if err = s.DeleteStudioEntity(ctx, other, p.ID, "shots", shot.ID); err == nil {
		t.Fatal("cross-workspace delete allowed")
	}
}

func TestStudioArchiveRemapsAndCaptionsBounded(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Archive", "")
	if err != nil {
		t.Fatal(err)
	}
	scene, err := s.CreateStudioEntity(ctx, u, p.ID, "scenes", StudioEntity{Title: "Scene"})
	if err != nil {
		t.Fatal(err)
	}
	shot, err := s.CreateStudioEntity(ctx, u, p.ID, "shots", StudioEntity{Title: "Shot", SceneID: scene.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateStudioEntity(ctx, u, p.ID, "shots", shot.ID, map[string]any{"project_id": "other"}); err == nil {
		t.Fatal("ownership field patched")
	}
	archive, err := s.StreamProjectExport(ctx, u, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(archive)
	if err != nil {
		t.Fatal(err)
	}
	var input ProjectArchive
	if err = json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	copy, err := s.ImportStudioProject(ctx, u, input)
	if err != nil {
		t.Fatal(err)
	}
	scenes, err := s.StudioEntities(ctx, u, copy.ID, "scenes", "")
	if err != nil || len(scenes) != 1 || scenes[0].ID == scene.ID {
		t.Fatalf("scene remapping: %+v %v", scenes, err)
	}
	shots, err := s.StudioEntities(ctx, u, copy.ID, "shots", scenes[0].ID)
	if err != nil || len(shots) != 1 || shots[0].ID == shot.ID {
		t.Fatalf("shot remapping: %+v %v", shots, err)
	}
	if err = s.GrantCredits(ctx, u.WorkspaceID, "captions", 10, ""); err != nil {
		t.Fatal(err)
	}
	q, err := s.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "test", Duration: 5, AspectRatio: "16:9"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.SubmitJob(ctx, u, q.ID, "captions-test-key")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveCaptions(ctx, u, j.ID, []CaptionSegment{}); err == nil {
		t.Fatal("queued source captions accepted")
	}
	if err = s.FinishJob(ctx, j.ID, "succeeded", "immutable.mp4", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveCaptions(ctx, u, j.ID, []CaptionSegment{{Start: 0, End: 6, Text: "too long"}}); err == nil {
		t.Fatal("caption beyond source accepted")
	}
	if err = s.SaveCaptions(ctx, u, j.ID, []CaptionSegment{{Start: 0, End: 2, Text: "Hello"}}); err != nil {
		t.Fatal(err)
	}
	captions, err := s.Captions(ctx, u, j.ID)
	if err != nil || captions.Duration != 5 || len(captions.Segments) != 1 {
		t.Fatalf("captions %+v %v", captions, err)
	}
	_, other := testPolicyStore(t)
	if _, err = s.Captions(ctx, other, j.ID); err == nil {
		t.Fatal("cross-user captions read")
	}
	got, err := s.Job(ctx, u, j.ID)
	if err != nil || got.OutputKey != "immutable.mp4" {
		t.Fatalf("caption edited original: %+v %v", got, err)
	}
}

func TestStudioWorkspaceQuotaConcurrent(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Quota", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `INSERT INTO cloud_studio_entities(id,project_id,kind,title)SELECT $1||'-'||i,$1,'scripts','Quota' FROM generate_series(1,499)i`, p.ID); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := s.CreateStudioEntity(ctx, u, p.ID, "scripts", StudioEntity{Title: "Final slot"})
			results <- err
		}()
	}
	group.Wait()
	close(results)
	accepted, rejected := 0, 0
	for err := range results {
		if err == nil {
			accepted++
		} else if errors.Is(err, ErrConflict) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("accepted=%d rejected=%d", accepted, rejected)
	}
	_, other := testPolicyStore(t)
	otherProject, err := s.CreateProject(ctx, other, "Independent", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateStudioEntity(ctx, other, otherProject.ID, "scripts", StudioEntity{Title: "Separate quota"}); err != nil {
		t.Fatal(err)
	}
}
func TestProjectWorkspaceQuota(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_projects(id,workspace_id,name)SELECT $1||'-'||i,$1,'Quota' FROM generate_series(1,100)i`, u.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProject(ctx, u, "Overflow", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("project cap: %v", err)
	}
	var archive ProjectArchive
	archive.Version = 1
	archive.Project.Name = "Import overflow"
	if _, err := s.ImportStudioProject(ctx, u, archive); !errors.Is(err, ErrConflict) {
		t.Fatalf("import bypassed project cap: %v", err)
	}

}
