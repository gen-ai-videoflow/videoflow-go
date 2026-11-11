package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	Title      string `json:"title"`
	Premise    string `json:"premise"`
	Hook       string `json:"hook"`
	WhyItWorks string `json:"why_it_works"`
}
type DevelopIdeaResponse struct {
	Options          []ConceptOption `json:"options"`
	RecommendedIndex int             `json:"recommended_index"`
	Reasoning        string          `json:"reasoning"`
}

// UnmarshalJSON accepts old provider replies without advertising the legacy schema to new calls.
func (response *DevelopIdeaResponse) UnmarshalJSON(data []byte) error {
	type canonical DevelopIdeaResponse
	var current canonical
	if err := json.Unmarshal(data, &current); err != nil {
		return err
	}
	*response = DevelopIdeaResponse(current)
	if len(response.Options) > 0 {
		return nil
	}
	var legacy struct {
		OptionA, OptionB struct{ Title, Hook, Narrative string }
		Recommendation   string
	}
	// Legacy snake-case names are explicit; unknown historic fields are ignored.
	var raw struct {
		A              struct{ Title, Hook, Narrative string } `json:"option_a"`
		B              struct{ Title, Hook, Narrative string } `json:"option_b"`
		Recommendation string                                  `json:"recommendation"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	legacy.OptionA = raw.A
	legacy.OptionB = raw.B
	legacy.Recommendation = raw.Recommendation
	if legacy.OptionA.Title != "" && legacy.OptionB.Title != "" {
		response.Options = []ConceptOption{{Title: legacy.OptionA.Title, Premise: legacy.OptionA.Narrative, Hook: legacy.OptionA.Hook}, {Title: legacy.OptionB.Title, Premise: legacy.OptionB.Narrative, Hook: legacy.OptionB.Hook}}
		response.Reasoning = legacy.Recommendation
		if strings.EqualFold(strings.TrimSpace(legacy.Recommendation), "B") {
			response.RecommendedIndex = 1
		}
	}
	return nil
}
func (a *AgentEngine) DevelopIdea(ctx context.Context, idea string) (*DevelopIdeaResponse, error) {
	var response DevelopIdeaResponse
	if err := a.structured(ctx, "idea_agent", "Develop two distinct film concepts. Return JSON options (exactly two objects with title, premise, hook, why_it_works), recommended_index (0 or 1), and reasoning.", "Idea: "+idea, &response); err != nil {
		return nil, err
	}
	if len(response.Options) != 2 || response.RecommendedIndex < 0 || response.RecommendedIndex >= 2 {
		return nil, fmt.Errorf("OpenRouter returned invalid idea options")
	}
	for _, o := range response.Options {
		if strings.TrimSpace(o.Title) == "" || strings.TrimSpace(o.Premise) == "" {
			return nil, fmt.Errorf("OpenRouter returned incomplete idea options")
		}
	}
	return &response, nil
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
	Reply       string `json:"reply"`
	Action      string `json:"action"` // canonical action-card action, or none
	SceneCount  int    `json:"scene_count,omitempty"`
	SceneID     string `json:"scene_id,omitempty"`
	CharacterID string `json:"character_id,omitempty"`
	OutputID    string `json:"output_id,omitempty"`
}

func (a *AgentEngine) Chat(ctx context.Context, message string, history []map[string]string, projectContext ...string) (*ChatIntentResponse, error) {
	system := `You are the 6eek Studio assistant. Help the user direct their story, build characters, scenes, and shots.
Respond in JSON with a natural conversational 'reply' and an 'action'.
Use generate_script to write a new story, script, or scene from an idea. Do not return create_script or add_scene.
Other supported actions: generate_scenes (expand an existing scene), generate_shots, storyboard,
render_scene, render_shot, caption, retry_render, generate_assets, refine_scene, refine_shot,
delete_scene, style_ingest, plan_assets. Use none for conversation or requests that cannot be performed.
Never invent action names or IDs. Set scene_count only when the user specifies a scene count; otherwise use 1. Select scene_id, character_id or output_id only from the supplied project context when the request identifies that item. The user reviews and runs the action card; do not claim it has already executed.`

	historyBytes, _ := json.Marshal(history)
	prompt := fmt.Sprintf("Conversation history: %s\nUser message: %s\nProject context: %s", string(historyBytes), message, strings.Join(projectContext, "\n"))

	var res ChatIntentResponse
	err := a.structured(ctx, "intent_agent", system, prompt, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}
