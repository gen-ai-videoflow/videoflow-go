package db

import (
	"6eek-go/internal/models"
	"path/filepath"
	"testing"
)

func TestScriptSceneFailureRollsBackWholeSnapshot(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	project, _ := d.GetActiveProjectID()
	script := &models.Script{ProjectID: &project, Title: "Test"}
	scenes := []models.Scene{{ID: "duplicate", Title: "A"}, {ID: "duplicate", Title: "B"}}
	if err = d.SaveScriptScenes(script, scenes); err == nil {
		t.Fatal("bad snapshot committed")
	}
	saved, _ := d.ListScripts(project)
	stored, _ := d.ListScenes(project)
	if len(saved) != 0 || len(stored) != 0 {
		t.Fatal("partial snapshot left behind")
	}
}
func TestShotReplacementFailurePreservesApprovedShots(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	project, _ := d.GetActiveProjectID()
	scene := &models.Scene{ProjectID: &project, Title: "A", CharacterIDsJSON: []byte("[]"), SceneJSON: []byte("{}")}
	d.CreateScene(scene)
	old := &models.Shot{SceneID: scene.ID, Prompt: "approved"}
	d.CreateShot(old)
	shots := []models.Shot{{ID: "duplicate"}, {ID: "duplicate"}}
	if err = d.ReplaceSceneShots(scene, shots); err == nil {
		t.Fatal("invalid replacement committed")
	}
	saved, _ := d.ListShots(scene.ID)
	if len(saved) != 1 || saved[0].ID != old.ID {
		t.Fatal("approved shots lost")
	}
}

func TestShotEditFencesLateRegeneration(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	project, _ := d.GetActiveProjectID()
	scene := &models.Scene{ProjectID: &project, Title: "Scene", CharacterIDsJSON: []byte("[]"), SceneJSON: []byte("{}")}
	d.CreateScene(scene)
	shot := &models.Shot{SceneID: scene.ID, Prompt: "Old"}
	d.CreateShot(shot)
	snapshot, _ := d.GetScene(scene.ID)
	shot.Prompt = "New approved edit"
	if err = d.UpdateShot(shot); err != nil {
		t.Fatal(err)
	}
	if err = d.ReplaceSceneShots(snapshot, []models.Shot{{Prompt: "Late AI result"}}); err == nil {
		t.Fatal("late regeneration deleted the approved edit")
	}
}
func TestStaleOutputCannotOverwriteNewCaptions(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	job := &models.RenderJob{Provider: "local", Model: "fixture", Status: "succeeded"}
	d.CreateRenderJob(job)
	output := &models.RenderOutput{RenderJobID: job.ID, VideoPath: "storage/raw.mp4"}
	d.CreateRenderOutput(output)
	old, _ := d.GetRenderOutput(output.ID)
	fresh, _ := d.GetRenderOutput(output.ID)
	fresh.CaptionsJSON = []byte(`{"segments":[],"style":"clean"}`)
	d.UpdateRenderOutput(fresh)
	old.CaptionsJSON = []byte(`{"segments":[{"start":0,"end":1,"text":"Stale"}],"style":"clean"}`)
	if err = d.UpdateRenderOutput(old); err == nil {
		t.Fatal("late caption result overwrote a newer save")
	}
}
func TestInterruptedAcceptedOperationKeepsRecoveryFence(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	op := &models.Op{Kind: "storyboard", Status: "running"}
	d.CreateOp(op)
	d.BeginRequest("old-key", "same-input")
	body := []byte(`{"op_id":"` + op.ID + `"}`)
	d.FinishRequest("old-key", 202, body)
	d.InterruptOperations()
	replay, err := d.BeginRequest("new-key", "same-input")
	if err != nil || replay == nil {
		t.Fatal("interrupted accepted paid operation can be repeated without review", err)
	}
	requests, _ := d.PendingRequests()
	if len(requests) != 1 {
		t.Fatal("interrupted accepted request is absent from recovery")
	}
}

func TestRecoveryAcknowledgementRequiresInterruptedOperation(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	op := &models.Op{Kind: "storyboard", Status: "running"}
	d.CreateOp(op)
	d.BeginRequest("key", "fingerprint")
	d.FinishRequest("key", 202, []byte(`{"op_id":"`+op.ID+`"}`))
	if err = d.AcknowledgeRequest("key", "Reviewed provider activity"); err == nil {
		t.Fatal("active operation was unlocked")
	}
	d.InterruptOperations()
	if err = d.AcknowledgeRequest("key", "Reviewed provider activity"); err != nil {
		t.Fatal(err)
	}
	replay, err := d.BeginRequest("new-key", "fingerprint")
	if err != nil || replay != nil {
		t.Fatal("reviewed operation did not unlock", err)
	}
}
func TestOperationJobBatchRollsBackOnFailure(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	op := &models.Op{Kind: "video", Status: "running"}
	d.CreateOp(op)
	jobs := []models.RenderJob{{ID: "duplicate", Provider: "openrouter", Status: "pending"}, {ID: "duplicate", Provider: "openrouter", Status: "pending"}}
	if err = d.SaveOperationJobs(op.ID, jobs, map[string]any{}); err == nil {
		t.Fatal("invalid batch committed")
	}
	if _, err = d.GetRenderJob("duplicate"); err == nil {
		t.Fatal("partial batch survived")
	}
}
