package routing

import (
	"path/filepath"
	"testing"
	"6eek-go/internal/config"
	"6eek-go/internal/db"
	"6eek-go/internal/models"
)

func TestLegacyConnectionCannotLeakKey(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	endpoint := "https://other.example/v1"
	connection := &models.Connection{Name: "legacy", Preset: "openrouter", Protocol: "chat", BaseURL: &endpoint}
	if err = database.CreateConnection(connection); err != nil {
		t.Fatal(err)
	}
	database.SetProviderSecret("legacy", "private-key", nil)
	if _, _, _, err = (Resolver{Config: config.Load(), DB: database}).Credentials("legacy"); err == nil {
		t.Fatal("persisted arbitrary URL can receive OpenRouter key")
	}
}
