package cloud

import "net/http"

// This inventory describes the cloud SaaS adapters, not legacy desktop settings.
// Configuration alone never proves provider capability or a successful paid call.
type AITaskReadiness struct {
	ID               string   `json:"id"`
	Provider         string   `json:"provider"`
	AdapterSupported bool     `json:"adapter_supported"`
	Configured       bool     `json:"configured"`
	Availability     string   `json:"availability"`
	Selection        string   `json:"selection"`
	Missing          []string `json:"missing"`
}
type AIReadiness struct {
	Provider             string            `json:"provider"`
	LiveProviderVerified bool              `json:"live_provider_verified"`
	Tasks                []AITaskReadiness `json:"tasks"`
	Note                 string            `json:"note"`
}

func aiReadiness(cfg Config) AIReadiness {
	value := AIReadiness{Provider: "openrouter", Tasks: []AITaskReadiness{}, Note: "Configuration inventory only. A quote still validates the current capability, approved pricing, duration, frame, budget and account before generation. Image, speech and caption adapters are not implemented in the cloud SaaS."}
	for _, id := range []string{"story", "video", "image", "speech", "transcription", "caption_translation", "image_reference_video"} {
		task := AITaskReadiness{ID: id, Provider: "openrouter", Missing: []string{}, Availability: "unsupported", Selection: "unavailable"}
		if id == "story" || id == "video" {
			task.AdapterSupported = true
			task.Selection = "cheapest_eligible_approved_video"
			checks := []struct {
				name string
				ok   bool
			}{{"OPENROUTER_API_KEY", cfg.OpenRouterKey != ""}, {"SQS_QUEUE_URL", cfg.QueueURL != ""}, {"S3_BUCKET_ORIGINALS", cfg.OriginalsBucket != ""}, {"PROVIDER_DAILY_BUDGET_MICROS", cfg.ProviderDailyBudgetMicros > 0}}
			if id == "story" {
				task.Selection = "cheapest_eligible_approved_story"
				checks = append(checks, struct {
					name string
					ok   bool
				}{"OPENROUTER_STORY_MODEL", cfg.StoryModel != ""}, struct {
					name string
					ok   bool
				}{"STORY_CREDITS", cfg.StoryCredits > 0}, struct {
					name string
					ok   bool
				}{"STORY_MAX_COST_MICROS", cfg.StoryMaxCostMicros > 0})
			} else {
				checks = append(checks, struct {
					name string
					ok   bool
				}{"OPENROUTER_VIDEO_MODEL", cfg.VideoModel != ""}, struct {
					name string
					ok   bool
				}{"CREDITS_PER_SECOND", cfg.CreditsPerSecond > 0}, struct {
					name string
					ok   bool
				}{"PROVIDER_MAX_COST_MICROS", cfg.ProviderMaxCostMicros > 0})
			}
			for _, check := range checks {
				if !check.ok {
					task.Missing = append(task.Missing, check.name)
				}
			}
			task.Configured = len(task.Missing) == 0
			task.Availability = "configuration_missing"
			if task.Configured {
				task.Availability = "quote_validation_required"
			}
		}
		value.Tasks = append(value.Tasks, task)
	}
	return value
}
func (s *Server) adminAIReadiness(w http.ResponseWriter, r *http.Request) {
	s.json(w, http.StatusOK, aiReadiness(s.Config))
}
