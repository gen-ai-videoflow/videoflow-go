package db_test

import (
	"os"
	"testing"

	"6eek-go/internal/db"
	"6eek-go/internal/models"
)

func TestDatabaseOperations(t *testing.T) {
	testDBPath := "test_db_ops.sqlite"
	defer os.Remove(testDBPath)

	database, err := db.Open(testDBPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// 1. Projects
	activeID, err := database.GetActiveProjectID()
	if err != nil || activeID == "" {
		t.Fatalf("expected active project ID, got: %s (err: %v)", activeID, err)
	}

	p, err := database.CreateProject("Test Project", "Testing DB")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	err = database.ActivateProject(p.ID)
	if err != nil {
		t.Fatalf("failed to activate project: %v", err)
	}

	currentActive, _ := database.GetActiveProjectID()
	if currentActive != p.ID {
		t.Fatalf("expected active project to be %s, got %s", p.ID, currentActive)
	}

	// 2. Characters
	char := models.Character{
		ProjectID:   &p.ID,
		Name:        "Alice",
		Description: "A brave explorer",
	}
	if err := database.CreateCharacter(&char); err != nil {
		t.Fatalf("failed to create character: %v", err)
	}

	chars, err := database.ListCharacters(p.ID)
	if err != nil || len(chars) != 1 {
		t.Fatalf("expected 1 character, got %d", len(chars))
	}

	// 3. Scenes & Shots
	scene := models.Scene{
		ProjectID:   &p.ID,
		Title:       "Opening Scene",
		Summary:     "Alice stands on a ridge",
		Duration:    10,
		AspectRatio: "16:9",
	}
	if err := database.CreateScene(&scene); err != nil {
		t.Fatalf("failed to create scene: %v", err)
	}

	shot := models.Shot{
		SceneID:   scene.ID,
		ShotOrder: 1,
		Duration:  5,
		Prompt:    "Wide shot of sunrise",
	}
	if err := database.CreateShot(&shot); err != nil {
		t.Fatalf("failed to create shot: %v", err)
	}

	shots, err := database.ListShots(scene.ID)
	if err != nil || len(shots) != 1 {
		t.Fatalf("expected 1 shot, got %d", len(shots))
	}

	// 4. Graph Data
	graph, err := database.GetGraphData(p.ID)
	if err != nil {
		t.Fatalf("failed to get graph data: %v", err)
	}
	if len(graph.Nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(graph.Nodes))
	}
}
