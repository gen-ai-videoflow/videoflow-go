package providers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"
)

const OpenRouterURL = "https://openrouter.ai/api/v1"

type OpenRouterClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewOpenRouterClient(apiKey string) *OpenRouterClient {
	return NewOpenRouterClientAt(apiKey, OpenRouterURL)
}
func NewOpenRouterClientAt(apiKey, baseURL string) *OpenRouterClient {
	if baseURL == "" {
		baseURL = OpenRouterURL
	}
	return &OpenRouterClient{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), httpClient: &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if req.URL.Scheme != "https" && req.URL.Host != via[0].URL.Host {
				return fmt.Errorf("insecure provider redirect")
			}
			if req.URL.Host != via[0].URL.Host || req.URL.Scheme != via[0].URL.Scheme {
				req.Header.Del("Authorization")
			}
			return nil
		},
	}}
}

func (c *OpenRouterClient) request(ctx context.Context, method, path string, payload any) ([]byte, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("OpenRouter API key is not configured")
	}
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "http://localhost:5173")
	req.Header.Set("X-Title", "6eek Go")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenRouter request: %s", strings.ReplaceAll(err.Error(), c.apiKey, "[redacted]"))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (256<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 256<<20 {
		return nil, fmt.Errorf("OpenRouter response exceeds 256 MB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.ReplaceAll(string(data), c.apiKey, "[redacted]")
		if len(message) > 2000 {
			message = message[:2000]
		}
		return nil, fmt.Errorf("OpenRouter HTTP %d: %s", resp.StatusCode, message)
	}
	return data, nil
}

func (c *OpenRouterClient) CheckKey(ctx context.Context) error {
	_, err := c.request(ctx, "GET", "/key", nil)
	return err
}

type Model struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Architecture struct {
		Input  []string `json:"input_modalities"`
		Output []string `json:"output_modalities"`
	} `json:"architecture"`
	SupportedParameters json.RawMessage `json:"supported_parameters"`
	Durations           []int           `json:"supported_durations"`
	AspectRatios        []string        `json:"supported_aspect_ratios"`
	Resolutions         []string        `json:"supported_resolutions"`
	FrameImages         []string        `json:"supported_frame_images"`
	InputReferences     json.RawMessage `json:"supported_input_references"`
	GenerateAudio       bool            `json:"generate_audio"`
}

func (c *OpenRouterClient) Models(ctx context.Context, modality string) ([]Model, error) {
	path := "/models"
	switch modality {
	case "image":
		path = "/images/models"
	case "video":
		path = "/videos/models"
	case "transcription":
		path = "/models?output_modalities=transcription"
	case "text", "vision", "":
	default:
		return nil, fmt.Errorf("unsupported modality %q", modality)
	}
	data, err := c.request(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Data []Model `json:"data"`
	}
	if err = json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	result := make([]Model, 0, len(res.Data))
	for _, m := range res.Data {
		if modality == "vision" && !contains(m.Architecture.Input, "image") {
			continue
		}
		if (modality == "text" || modality == "vision") && !contains(m.Architecture.Output, "text") {
			continue
		}
		result = append(result, m)
	}
	return result, nil
}

func (c *OpenRouterClient) Model(ctx context.Context, id, modality string) (*Model, error) {
	models, err := c.Models(ctx, modality)
	if err != nil {
		return nil, err
	}
	for _, m := range models {
		if m.ID == id {
			return &m, nil
		}
	}
	return nil, fmt.Errorf("model %q is not in OpenRouter's %s catalog", id, modality)
}

func contains[T comparable](values []T, value T) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func (c *OpenRouterClient) chat(ctx context.Context, model, system string, content any) (string, error) {
	payload := map[string]any{"model": model, "messages": []map[string]any{{"role": "system", "content": system}, {"role": "user", "content": content}}}
	data, err := c.request(ctx, "POST", "/chat/completions", payload)
	if err != nil {
		return "", err
	}
	var res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error json.RawMessage `json:"error"`
	}
	if err = json.Unmarshal(data, &res); err != nil {
		return "", err
	}
	if len(res.Choices) == 0 || strings.TrimSpace(res.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("OpenRouter returned no text completion")
	}
	return res.Choices[0].Message.Content, nil
}

func (c *OpenRouterClient) ChatCompletion(ctx context.Context, model, system, user string) (string, error) {
	return c.chat(ctx, model, system, user)
}

func (c *OpenRouterClient) StructuredJSON(ctx context.Context, model, system, user string, target any) error {
	return c.StructuredVision(ctx, model, system, user, nil, target)
}

func (c *OpenRouterClient) StructuredVision(ctx context.Context, model, system, user string, images []string, target any) error {
	template, _ := json.Marshal(exampleShape(reflect.TypeOf(target)))
	system += "\nReturn only a JSON object matching this shape (replace zero values with the requested content): " + string(template) + ". Do not use markdown."
	for attempt := 0; attempt < 2; attempt++ {
		var content any = user
		if len(images) > 0 {
			parts := []map[string]any{{"type": "text", "text": user}}
			for _, img := range images {
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": img}})
			}
			content = parts
		}
		result, err := c.chat(ctx, model, system, content)
		if err != nil {
			return err
		}
		result = strings.TrimSpace(result)
		result = strings.TrimPrefix(result, "```json")
		result = strings.TrimPrefix(result, "```")
		result = strings.TrimSuffix(result, "```")
		if err = json.Unmarshal([]byte(strings.TrimSpace(result)), target); err == nil {
			return nil
		}
		user += "\nYour last response was not valid JSON. Return the requested JSON object only."
	}
	return fmt.Errorf("OpenRouter model %s returned invalid JSON after two attempts", model)
}

type GeneratedImage struct {
	Data      []byte
	MediaType string
}

func (c *OpenRouterClient) GenerateImages(ctx context.Context, payload map[string]any) ([]GeneratedImage, error) {
	id, _ := payload["model"].(string)
	m, err := c.Model(ctx, id, "image")
	if err != nil {
		return nil, err
	}
	if err = m.validateImage(payload); err != nil {
		return nil, err
	}
	data, err := c.request(ctx, "POST", "/images", payload)
	if err != nil {
		return nil, err
	}
	var res struct {
		Data []struct {
			Base64    string `json:"b64_json"`
			MediaType string `json:"media_type"`
		} `json:"data"`
	}
	if err = json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	if len(res.Data) == 0 {
		return nil, fmt.Errorf("OpenRouter returned no images")
	}
	images := make([]GeneratedImage, 0, len(res.Data))
	for _, item := range res.Data {
		decoded, err := base64.StdEncoding.DecodeString(item.Base64)
		if err != nil || len(decoded) == 0 {
			return nil, fmt.Errorf("OpenRouter returned invalid image data")
		}
		mediaType := item.MediaType
		if mediaType == "" {
			mediaType = http.DetectContentType(decoded)
		}
		images = append(images, GeneratedImage{Data: decoded, MediaType: mediaType})
	}
	return images, nil
}

func (m *Model) validateImage(payload map[string]any) error {
	var caps map[string]struct {
		Type   string   `json:"type"`
		Values []string `json:"values"`
		Min    float64  `json:"min"`
		Max    float64  `json:"max"`
	}
	if err := json.Unmarshal(m.SupportedParameters, &caps); err != nil {
		return fmt.Errorf("image model has no capability metadata")
	}
	for key, value := range payload {
		if key == "model" || key == "prompt" || key == "provider" {
			continue
		}
		cap, ok := caps[key]
		if !ok {
			return fmt.Errorf("image model %s does not support %s", m.ID, key)
		}
		if cap.Type == "enum" {
			v, _ := value.(string)
			if !contains(cap.Values, v) {
				return fmt.Errorf("unsupported %s %q for image model %s", key, v, m.ID)
			}
		}
		if cap.Type == "range" {
			number := 0.0
			switch v := value.(type) {
			case float64:
				number = v
			case int:
				number = float64(v)
			case []any:
				number = float64(len(v))
			case []map[string]any:
				number = float64(len(v))
			}
			if number < cap.Min || number > cap.Max {
				return fmt.Errorf("%s must be between %g and %g for %s", key, cap.Min, cap.Max, m.ID)
			}
		}
	}
	if prompt, _ := payload["prompt"].(string); strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("image prompt is required")
	}
	return nil
}

func (c *OpenRouterClient) ValidateVideo(ctx context.Context, payload map[string]any) error {
	id, _ := payload["model"].(string)
	m, err := c.Model(ctx, id, "video")
	if err != nil {
		return err
	}
	prompt, _ := payload["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("video prompt is required")
	}
	if raw, ok := payload["duration"]; ok {
		var n int
		switch v := raw.(type) {
		case int:
			n = v
		case float64:
			if v != float64(int(v)) {
				return fmt.Errorf("video duration must be an integer")
			}
			n = int(v)
		default:
			return fmt.Errorf("video duration must be an integer")
		}
		if !contains(m.Durations, n) {
			return fmt.Errorf("model %s supports durations %v, requested %d", id, m.Durations, n)
		}
	}
	for key, allowed := range map[string][]string{"resolution": m.Resolutions, "aspect_ratio": m.AspectRatios} {
		if raw, ok := payload[key]; ok {
			v, _ := raw.(string)
			if !contains(allowed, v) {
				return fmt.Errorf("model %s supports %s %v, requested %q", id, key, allowed, v)
			}
		}
	}
	if audio, ok := payload["generate_audio"]; ok {
		b, valid := audio.(bool)
		if !valid {
			return fmt.Errorf("generate_audio must be boolean")
		}
		if b && !m.GenerateAudio {
			return fmt.Errorf("model %s does not support audio generation", id)
		}
	}
	if refs, ok := payload["frame_images"]; ok {
		data, _ := json.Marshal(refs)
		var frames []struct {
			FrameType string `json:"frame_type"`
		}
		if err := json.Unmarshal(data, &frames); err != nil {
			return fmt.Errorf("invalid frame_images")
		}
		if len(frames) == 0 || len(frames) > 2 {
			return fmt.Errorf("provide one or two frame images")
		}
		seen := map[string]bool{}
		for _, f := range frames {
			if !contains(m.FrameImages, f.FrameType) || seen[f.FrameType] {
				return fmt.Errorf("unsupported or duplicate frame type %q for %s", f.FrameType, id)
			}
			seen[f.FrameType] = true
		}
	}
	if refs, ok := payload["input_references"]; ok {
		data, _ := json.Marshal(refs)
		var inputs []any
		if json.Unmarshal(data, &inputs) != nil || len(inputs) == 0 {
			return fmt.Errorf("invalid input_references")
		}
		if len(m.InputReferences) == 0 || string(m.InputReferences) == "null" || string(m.InputReferences) == "false" {
			return fmt.Errorf("model %s does not advertise guidance image references; use supported frame_images", id)
		}
	}
	return nil
}

func (c *OpenRouterClient) CreateVideo(ctx context.Context, payload map[string]any) (string, error) {
	if err := c.ValidateVideo(ctx, payload); err != nil {
		return "", err
	}
	data, err := c.request(ctx, "POST", "/videos", payload)
	if err != nil {
		return "", err
	}
	var res struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(data, &res); err != nil {
		return "", err
	}
	if res.ID == "" {
		return "", fmt.Errorf("OpenRouter returned no video job ID")
	}
	return res.ID, nil
}

func (c *OpenRouterClient) GetVideo(ctx context.Context, jobID string) (string, []string, error) {
	data, err := c.request(ctx, "GET", "/videos/"+url.PathEscape(jobID), nil)
	if err != nil {
		return "", nil, err
	}
	var res struct {
		Status string          `json:"status"`
		URLs   []string        `json:"unsigned_urls"`
		Error  json.RawMessage `json:"error"`
	}
	if err = json.Unmarshal(data, &res); err != nil {
		return "", nil, err
	}
	if len(res.Error) > 0 && string(res.Error) != "null" && string(res.Error) != `""` {
		return "failed", nil, fmt.Errorf("OpenRouter video error: %s", strings.ReplaceAll(string(res.Error), c.apiKey, "[redacted]"))
	}
	if res.Status == "" {
		return "", nil, fmt.Errorf("OpenRouter returned no video status")
	}
	return res.Status, res.URLs, nil
}

func (c *OpenRouterClient) VideoContent(ctx context.Context, jobID string, index int) ([]byte, error) {
	data, err := c.request(ctx, "GET", fmt.Sprintf("/videos/%s/content?index=%d", url.PathEscape(jobID), index), nil)
	if err == nil {
		kind := http.DetectContentType(data)
		if !strings.HasPrefix(kind, "video/") {
			err = fmt.Errorf("OpenRouter returned non-video content (%s)", kind)
		}
	}
	return data, err
}

type TranscriptSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}
type Transcript struct {
	Text     string              `json:"text"`
	Segments []TranscriptSegment `json:"segments"`
}

func (c *OpenRouterClient) Transcribe(ctx context.Context, model string, audio []byte, format, language string) (*Transcript, error) {
	if len(audio) == 0 {
		return nil, fmt.Errorf("audio is empty")
	}
	if _, err := c.Model(ctx, model, "transcription"); err != nil {
		return nil, err
	}
	payload := map[string]any{"model": model, "input_audio": map[string]string{"data": base64.StdEncoding.EncodeToString(audio), "format": format}, "response_format": "verbose_json", "timestamp_granularities": []string{"segment"}}
	if language != "" && language != "auto" {
		payload["language"] = language
	}
	data, err := c.request(ctx, "POST", "/audio/transcriptions", payload)
	if err != nil {
		return nil, err
	}
	var transcript Transcript
	if err = json.Unmarshal(data, &transcript); err != nil {
		return nil, err
	}
	if strings.TrimSpace(transcript.Text) != "" && len(transcript.Segments) == 0 {
		return nil, fmt.Errorf("selected OpenRouter transcription model did not return timestamps; use a Whisper model with verbose_json support")
	}
	for _, segment := range transcript.Segments {
		if segment.Start < 0 || segment.End <= segment.Start {
			return nil, fmt.Errorf("invalid transcription timestamps")
		}
	}
	return &transcript, nil
}

// exampleShape gives slice element fields as well as object fields to prompt-only JSON models.
func exampleShape(t reflect.Type) any {
	if t == nil {
		return nil
	}
	if t.Kind() == reflect.Pointer {
		return exampleShape(t.Elem())
	}
	if t == reflect.TypeOf(json.RawMessage{}) {
		return map[string]any{}
	}
	switch t.Kind() {
	case reflect.Struct:
		fields := map[string]any{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			fields[name] = exampleShape(f.Type)
		}
		return fields
	case reflect.Slice, reflect.Array:
		return []any{exampleShape(t.Elem())}
	case reflect.Map:
		return map[string]any{}
	case reflect.String:
		return ""
	case reflect.Bool:
		return false
	default:
		return 0
	}
}
