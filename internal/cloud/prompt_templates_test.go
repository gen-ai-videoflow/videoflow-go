package cloud

import (
	"context"
	"errors"
	"testing"
)

func TestPromptTemplatesOwnershipAndRevision(t *testing.T) {
	s, u := testPolicyStore(t)
	_, other := testPolicyStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Bilingual prompts", "")
	if err != nil {
		t.Fatal(err)
	}
	input := PromptTemplate{Category: "character", TitleEn: "Character", TitleZh: "角色", BodyEn: "Describe {{subject}} consistently.", BodyZh: "一致地描述{{subject}}。", Defaults: map[string]string{"subject": "林安"}}
	saved, err := s.SavePromptTemplate(ctx, u, p.ID, input)
	if err != nil || saved.Revision != 1 {
		t.Fatalf("first save: %+v %v", saved, err)
	}
	if _, err = s.PromptTemplates(ctx, other, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign read: %v", err)
	}
	if _, err = s.SavePromptTemplate(ctx, other, p.ID, saved); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign write: %v", err)
	}
	original := saved
	saved.BodyZh = "保持{{subject}}的服装和外观一致。"
	saved, err = s.SavePromptTemplate(ctx, u, p.ID, saved)
	if err != nil || saved.Revision != 2 {
		t.Fatalf("second save: %+v %v", saved, err)
	}
	if _, err = s.SavePromptTemplate(ctx, u, p.ID, original); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update: %v", err)
	}
	rows, err := s.PromptTemplates(ctx, u, p.ID)
	if err != nil || len(rows) != 1 || rows[0].BodyZh != saved.BodyZh || rows[0].Defaults["subject"] != "林安" {
		t.Fatalf("roundtrip: %+v %v", rows, err)
	}
	if err = s.DeletePromptTemplate(ctx, other, p.ID, saved.ID, saved.Revision); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign delete", err)
	}
	if err = s.DeletePromptTemplate(ctx, u, p.ID, saved.ID, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("stale delete", err)
	}
	if err = s.DeletePromptTemplate(ctx, u, p.ID, saved.ID, 2); err != nil {
		t.Fatal(err)
	}
	input.Category = "model"
	if _, err = s.SavePromptTemplate(ctx, u, p.ID, input); !errors.Is(err, ErrValidation) {
		t.Fatal("invalid category", err)
	}
}
