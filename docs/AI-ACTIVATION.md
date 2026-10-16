# Cloud AI activation

The SaaS sends supported AI inference through server-owned OpenRouter credentials. Customers do not configure or choose models. Saving prompt templates and importing starter projects do not call a provider or consume credits.

The protected admin endpoint `GET /api/v1/admin/ai-readiness` inventories the actual cloud adapters without returning credentials, queue URLs or bucket names. `configured` means required configuration is present; it does not mean connectivity, billing, capability or a paid generation has been tested. `live_provider_verified` is deliberately false in this inventory.

| Task | Cloud implementation | Selection |
| --- | --- | --- |
| Story/script | OpenRouter structured text request | Cheapest eligible model from the comma-separated operator-approved `OPENROUTER_STORY_MODEL` allowlist, using current text catalogue pricing and bounded prompt/output tokens |
| Text-to-video | OpenRouter video job adapter | Cheapest eligible model among the operator-approved `OPENROUTER_VIDEO_MODEL` list, current supported duration/frame/resolution and understood pricing |
| Image generation | Unsupported cloud adapter | Disabled |
| Speech generation | Unsupported cloud adapter | Disabled |
| Transcription | Unsupported cloud adapter | Disabled |
| Caption translation | Unsupported cloud adapter | Disabled |
| Image/reference-conditioned video | Unsupported cloud adapter | Disabled; character directions remain text context |

Caption editing and uploaded caption playback are separate from AI transcription. Browser editing/export and FFmpeg assembly are deterministic media operations, not AI inference.

## Setup and release gates

1. Configure `OPENROUTER_API_KEY` in the deployment secret store. Never save credentials in admin settings, project prompts or browser storage.
2. Review commercial-use eligibility, current pricing and supported parameters before approving model IDs. Free or unknown-pricing models are excluded by video policy. The selection promise is cheapest eligible approved model, not cheapest model regardless of capability or license.
3. Set positive per-request/daily provider budgets and reviewed generation credit prices. Story generation also needs `STORY_CREDITS` and `STORY_MAX_COST_MICROS`. Set an operator-reviewed story-model allowlist. The backend refreshes the text catalogue at most hourly, rejects unknown/additional nonzero pricing, checks context and required parameters, and compares exact rational prompt/completion costs. The worker enforces a conservative per-token ceiling. Selection is frozen in the pipeline plan; operators cannot silently replace the confirmed model.
4. Configure the queue, originals bucket and worker/media runtime. Verify readiness and run local fixture generation before sending a paid request.
5. Test the complete quote → explicit credit confirmation → queued job → output → browser editor → export/publish journey in staging. Validate failed jobs, cancellation, budget exhaustion, delayed provider responses and credit release. A successful configuration screen cannot replace this test.
6. Keep unsupported adapters disabled. Implement and verify an OpenRouter capability before adding a corresponding charged action. An uploaded character image is not automatically sent as a model reference.

No live AI generation or provider activation was performed by this implementation. Prices and credentials remain deployment configuration; changing the admin generation switch cannot supply a missing worker, credential or adapter.

The text catalogue cache uses migration `036_story_catalog.sql`. Story selection bounds the customer idea to 8KB, reserves conservative message overhead, and caps generated output at 2,048 tokens. Unsupported output sizes, request/audio/image surcharges, free/zero or unknown pricing, expired capability information and zero budgets do not silently become eligible. Local tests cover English/Mandarin selection, invalid pricing, budget limits and one cached refresh across repeated calls. Browser-local caption AI is disabled under the OpenRouter-only policy; AI transcription awaits a verified cloud adapter.
