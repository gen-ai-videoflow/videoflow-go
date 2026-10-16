package cloud

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAIReadinessDoesNotExposeSecretsOrAdvertiseUnsupportedAdapters(t *testing.T) {
	cfg := Config{OpenRouterKey: "never-expose-this", VideoModel: "approved-video", StoryModel: "approved-story", QueueURL: "https://private-queue", OriginalsBucket: "private-bucket", CreditsPerSecond: 10, StoryCredits: 5, StoryMaxCostMicros: 100, ProviderMaxCostMicros: 100, ProviderDailyBudgetMicros: 1000}
	value := aiReadiness(cfg)
	encoded, _ := json.Marshal(value)
	if strings.Contains(string(encoded), cfg.OpenRouterKey) || strings.Contains(string(encoded), cfg.QueueURL) || strings.Contains(string(encoded), cfg.OriginalsBucket) {
		t.Fatal("readiness exposed runtime secrets")
	}
	if len(value.Tasks) != 7 {
		t.Fatalf("missing adapter inventory: %d", len(value.Tasks))
	}
	for _, task := range value.Tasks {
		if task.ID == "story" || task.ID == "video" {
			if !task.AdapterSupported || !task.Configured {
				t.Fatalf("supported configured task missing: %+v", task)
			}
		} else if task.AdapterSupported || task.Configured {
			t.Fatalf("unsupported cloud adapter advertised: %+v", task)
		}
	}
	if value.LiveProviderVerified {
		t.Fatal("configuration is not a live provider test")
	}
}
func TestAIReadinessMissingConfigurationIsExplicit(t *testing.T) {
	value := aiReadiness(Config{})
	for _, task := range value.Tasks {
		if task.Configured {
			t.Fatal("empty config enabled AI")
		}
		if task.AdapterSupported && len(task.Missing) == 0 {
			t.Fatal("missing setup must be explained")
		}
	}
}
