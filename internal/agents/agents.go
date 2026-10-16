package agents

import (
	"context"
	"encoding/json"
	"fmt"

	"6eek-go/internal/providers"
)

type AgentEngine struct {
	openrouter *providers.OpenRouterClient
	model      string
	language   func() string
	resolve    func(context.Context, string) (*providers.OpenRouterClient, string, error)
}

func NewAgentEngine(openrouter *providers.OpenRouterClient, defaultModel string) *AgentEngine {
	if defaultModel == "" {
		defaultModel = "openai/gpt-4o-mini"
	}
	return &AgentEngine{
		openrouter: openrouter,
		model:      defaultModel,
	}
}

type dialogueLanguageKey struct{}

func WithDialogueLanguage(ctx context.Context, language string) context.Context {
	return context.WithValue(ctx, dialogueLanguageKey{}, language)
}
func (a *AgentEngine) SetLanguageResolver(resolve func() string) { a.language = resolve }

// SetResolver supplies credentials and model settings for every agent call.
func (a *AgentEngine) SetResolver(resolve func(context.Context, string) (*providers.OpenRouterClient, string, error)) {
	a.resolve = resolve
}
func (a *AgentEngine) structured(ctx context.Context, name, system, prompt string, target any) error {
	client, model := a.openrouter, a.model
	if a.resolve != nil {
		var err error
		client, model, err = a.resolve(ctx, name)
		if err != nil {
			return err
		}
	}
	language := "English"
	if a.language != nil {
		language = a.language()
	}
	if value, _ := ctx.Value(dialogueLanguageKey{}).(string); value != "" {
		language = value
	}
	system += "\nWrite all character dialogue in " + language + "."
	return client.StructuredJSON(ctx, model, system, prompt, target)
}

// --- Develop Idea ---

type ConceptOption struct {
	Title       string `json:"title"`
	Hook        string `json:"hook"`
	Narrative   string `json:"narrative"`
	VisualStyle string `json:"visual_style"`
}

type DevelopIdeaResponse struct {
	OptionA        ConceptOption `json:"option_a"`
	OptionB        ConceptOption `json:"option_b"`
	Recommendation string        `json:"recommendation"`
}

func (a *AgentEngine) DevelopIdea(ctx context.Context, idea string) (*DevelopIdeaResponse, error) {
	system := "You are a professional film and animation director. Expand the user's idea into two mature, distinct concept directions (Option A and Option B), and give a recommendation."
	prompt := fmt.Sprintf("Idea: %s", idea)

	var res DevelopIdeaResponse
	err := a.structured(ctx, "idea_agent", system, prompt, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

// --- Generate Script & Scenes ---

type SceneDraft struct {
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Duration    int    `json:"duration"`
	AspectRatio string `json:"aspect_ratio"`
}

type ScriptDraft struct {
	Title   string       `json:"title"`
	Summary string       `json:"summary"`
	Scenes  []SceneDraft `json:"scenes"`
}

func (a *AgentEngine) GenerateScript(ctx context.Context, idea string, sceneCount int, targetDuration int, aspectRatio string) (*ScriptDraft, error) {
	if sceneCount <= 0 {
		sceneCount = 3
	}
	if targetDuration <= 0 {
		targetDuration = sceneCount * 5
	}
	if aspectRatio == "" {
		aspectRatio = "9:16"
	}

	system := fmt.Sprintf(`You are a scriptwriter. Break down the story idea into exactly %d structured scenes.
Total target duration is %d seconds. Aspect ratio is %s.
Return a title, overall summary, and the list of scenes.`, sceneCount, targetDuration, aspectRatio)

	prompt := fmt.Sprintf("Idea: %s", idea)

	var res ScriptDraft
	err := a.structured(ctx, "script_agent", system, prompt, &res)
	if err != nil {
		return nil, err
	}

	if len(res.Scenes) != sceneCount {
		return nil, fmt.Errorf("OpenRouter returned an unexpected scene count")
	}
	for i := range res.Scenes {
		scene := &res.Scenes[i]
		if scene.Title == "" || scene.Summary == "" || scene.Duration <= 0 {
			return nil, fmt.Errorf("OpenRouter returned an incomplete scene")
		}
		if scene.AspectRatio == "" {
			scene.AspectRatio = aspectRatio
		}
	}
	return &res, nil
}

// --- Generate Shots ---

type ShotDraft struct {
	Order    int    `json:"shot_order"`
	Duration int    `json:"duration"`
	Prompt   string `json:"prompt"`
	Camera   string `json:"camera"`
	Movement string `json:"movement"`
}

type ShotsListDraft struct {
	Shots []ShotDraft `json:"shots"`
}

func (a *AgentEngine) GenerateShots(ctx context.Context, title, summary string, duration int) ([]ShotDraft, error) {
	system := "You are a cinematographer. Break down the scene into 2 to 4 detailed camera shots (angles, movements, and visual prompts for video generation AI)."
	prompt := fmt.Sprintf("Scene Title: %s\nSummary: %s\nTotal Duration: %d seconds", title, summary, duration)

	var res ShotsListDraft
	err := a.structured(ctx, "shot_agent", system, prompt, &res)
	if err != nil {
		return nil, err
	}

	if len(res.Shots) == 0 {
		return nil, fmt.Errorf("OpenRouter returned no shots")
	}
	for _, shot := range res.Shots {
		if shot.Order <= 0 || shot.Duration <= 0 || shot.Prompt == "" {
			return nil, fmt.Errorf("OpenRouter returned an incomplete shot")
		}
	}
	return res.Shots, nil
}

// --- Classify Chat Intent ---

type ChatIntentResponse struct {
	Reply        string                 `json:"reply"`
	Action       string                 `json:"action"` // create_script, add_scene, render_video, none
	ActionParams map[string]interface{} `json:"action_params,omitempty"`
}

func (a *AgentEngine) Chat(ctx context.Context, message string, history []map[string]string) (*ChatIntentResponse, error) {
	system := `You are the 6eek Studio assistant. Help the user direct their story, build characters, scenes, and shots.
Respond in JSON with a natural conversational 'reply', and an optional 'action' (create_script, add_scene, none).`

	historyBytes, _ := json.Marshal(history)
	prompt := fmt.Sprintf("Conversation history: %s\nUser message: %s", string(historyBytes), message)

	var res ChatIntentResponse
	err := a.structured(ctx, "intent_agent", system, prompt, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}
