package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

func TestEditorDocumentOwnershipAndRevision(t *testing.T) {
	s, u := testPolicyStore(t)
	_, other := testPolicyStore(t)
	ctx := context.Background()
	p, e := s.CreateProject(ctx, u, "Edit", "")
	if e != nil {
		t.Fatal(e)
	}
	d, e := s.EditorDocument(ctx, u, p.ID)
	if e != nil || d.Revision != 0 {
		t.Fatalf("new document: %+v %v", d, e)
	}
	body := json.RawMessage(`{"metadata":{"name":"Edit"},"scenes":[],"version":1}`)
	d, e = s.SaveEditorDocument(ctx, u, p.ID, 0, body)
	if e != nil || d.Revision != 1 {
		t.Fatalf("save: %+v %v", d, e)
	}
	d, e = s.SaveEditorDocument(ctx, u, p.ID, 1, body)
	if e != nil || d.Revision != 2 {
		t.Fatalf("second save: %+v %v", d, e)
	}
	if _, e = s.SaveEditorDocument(ctx, u, p.ID, 0, body); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale overwrite allowed: %v", e)
	}
	if _, e = s.EditorDocument(ctx, other, p.ID); !errors.Is(e, ErrNotFound) {
		t.Fatalf("foreign read: %v", e)
	}
	if _, e = s.SaveEditorDocument(ctx, other, p.ID, 2, body); !errors.Is(e, ErrNotFound) {
		t.Fatalf("foreign write: %v", e)
	}
	d, e = s.EditorDocument(ctx, u, p.ID)
	if e != nil || d.Revision != 2 {
		t.Fatalf("read: %+v %v", d, e)
	}
	if _, e = s.SaveEditorDocument(ctx, u, p.ID, 1, json.RawMessage(`[]`)); !errors.Is(e, ErrValidation) {
		t.Fatalf("invalid document: %v", e)
	}
}

func TestEditorDocumentConcurrentRevision(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Concurrent editor", "")
	if err != nil {
		t.Fatal(err)
	}
	body := json.RawMessage(`{"metadata":{"name":"Edit"},"scenes":[],"version":1}`)
	if _, err = s.SaveEditorDocument(ctx, u, p.ID, 0, body); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.SaveEditorDocument(ctx, u, p.ID, 1, body); results <- err }()
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one winner and one conflict, got %d/%d", successes, conflicts)
	}
}
