# Configure OpenRouter

6eek Go uses OpenRouter for all AI tasks: story planning, scripts, shots, chat, image generation, character references, visual recognition, frame QA, video generation, and speech transcription. FFmpeg performs local frame extraction, audio extraction and caption burning; it is not an AI provider.

## Setup

1. Create an API key at https://openrouter.ai/keys and enable sufficient credits for the models you choose.
2. In **Settings → Providers**, save the built-in OpenRouter key. Alternatively set `OPENROUTER_API_KEY` before starting the Go server. Database credentials take precedence over the environment.
3. Run **Test connection**. It calls OpenRouter's `/key` endpoint and does not generate content.
4. Use **Load models** to fetch the current text catalog. **Test text model + JSON** makes a small billable completion and checks its JSON response. It does not certify image, vision, video, or transcription behavior.
5. Open **Engines** and refresh OpenRouter's model catalogs. Select models for images, character/prop images, video, frame vision and transcription. Saving an engine model checks that it appears in the appropriate live catalog.
6. In **Agents**, choose text or vision model IDs. Agent selections and saved keys are read on each call, including after a restart.

Defaults: `openai/gpt-4o-mini` for text, `qwen/qwen3-vl-30b-a3b-instruct` for vision, `openai/gpt-image-2` for images, `google/veo-3.1-lite` for video, and `openai/whisper-1` for timed transcription. Catalog availability does not guarantee account access or sufficient credits.

Named connections must use OpenRouter Chat Completions at `https://openrouter.ai/api/v1`. Each named connection uses its own saved key and default model; built-in media tasks use the built-in OpenRouter key. Other provider presets and arbitrary endpoint overrides are no longer available. Existing unrelated connection records are preserved but cannot be selected for new work. Credentials in the Go SQLite database are stored locally in plaintext; protect the database.

## Media behavior

Images use `POST /images` and are saved with their actual format. Storyboard images appear in the scene's storyboard view. Character reference sheets are saved as assets and linked to their character; character references can guide storyboard image generation.

Videos use `POST /videos`, `GET /videos/{id}`, and authenticated `GET /videos/{id}/content?index=N`. Models, explicit durations, aspect ratios, resolutions, audio flags and frame anchors are checked before paid submission. Local scene timings are mapped to the next supported duration, or the model's maximum when longer; the actual provider request is saved with the job. Shot beats are expressed in the prompt, not as a native multi-shot API field. Saved negative prompts and dialogue language are included in the request prompt.

When supported, the latest generated storyboard is used as a first-frame anchor. Explicit `frame_images` override that automatic anchor. Local references are embedded as data URLs. OpenRouter's image guide documents these; its video guide demonstrates remote URLs, so live reference-video acceptance remains model-dependent. Guidance `input_references` are accepted only when capability metadata advertises support; video/audio reference assets are not mapped.

The worker marks success only after downloading real video content and saving outputs. It stores the upstream job ID before polling and resumes it after shutdown/restart. Clicking retry/resubmit creates a fresh generation that may incur a new charge. Provider errors fail the job; no sample video or fabricated QA score is substituted.

Frame QA sends three sampled images to a vision model. Its score describes sampled visual frames, not an audio or continuous-motion assessment. It requires local FFmpeg. Transcription uses `/audio/transcriptions` with `verbose_json` and genuine provider timestamps; choose a Whisper model that returns segments. Caption burning requires an FFmpeg build with libass.

Official contracts: [image generation](https://openrouter.ai/docs/guides/overview/multimodal/image-generation), [video generation](https://openrouter.ai/docs/guides/overview/multimodal/video-generation), [transcription](https://openrouter.ai/docs/guides/overview/multimodal/stt).

See the [current verification report](../docs/audits/2026-10-01-openrouter-go.md) for what was tested. Paid live media quality and reference-video acceptance remain external checks.
