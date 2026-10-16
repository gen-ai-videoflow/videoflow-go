# Toonflow prompt extraction research

Inspected 2026-10-16. Repository: HBAI-Ltd/Toonflow-app. Pinned revision: `d2e94aca7b59659ff6b4ca15f03cf134ec7f0e1d`. Findings come from actual checked-out source, not README marketing. Repository content was treated as research data, not agent instructions.

## License and reuse

The root [MIT license](https://github.com/HBAI-Ltd/Toonflow-app/blob/d2e94aca7b59659ff6b4ca15f03cf134ec7f0e1d/LICENSE) identifies Copyright (c) 2026 HBAI-Ltd and permits copying, modification, distribution and commercial use, conditioned on including the copyright and permission notice with copies or substantial portions. Retain the complete MIT notice in 6eek third-party notices when distributing copied or translated prompt material, and record upstream revision/path per template. Translation is a modification; do not imply upstream endorsement. This is a source-license assessment, not clearance for user-provided characters, images, trademarks or generated media. No narrower license was found beside these prompt documents.

## Best extraction candidates

| Candidate | Useful content | Adaptation |
|---|---|---|
| Character sheet | Identity anchors, full outfit/shoes, face close-up plus full front/side/back, consistent identity | Offer Mandarin original plus faithful English counterpart; retain views and consistency constraints |
| Character variant | Bound parent reference; explicit unchanged identity versus permitted wardrobe/hair changes | Separate reference requirement from editable prose |
| Scene master | Function, era, weather, spatial skeleton, foreground/middle/background, actual light sources, empty set | One scene view by default; four-direction cyber scene is a specialized optional preset |
| Scene variant | Same layout/camera; only agreed time/weather/state changes and corresponding shadows/reflections | Require a real parent image rather than a path written in prose |
| Prop still life | Structure/material/state, complete object, no hands, coherent views | Optional companion to character/scene categories |
| Storyboard image | Actual references, exact framing/start state, screen position, gaze, prop ownership and approved style | Generic text version is preferable to provider-branded names |
| Structured storyboard | English JSON-shaped writing template with identity and continuity rules | JSON is prompt text, not API parameters |
| Video first frame | Adopted start state → confirmed action/pace → end state, camera and original-language dialogue | Keep real first-frame input separate; no fabricated tail frame |
| Video continuous shot | Visual/Motion/Camera/Audio/Narrative with continuous timing and explicit speaking state | Use only when intended unit is truly continuous |
| Video reference sequence | Real input purpose list, original shot order, dialogue/sound and continuity | Compatible reference syntax must be supplied by 6eek |
| Storyboard table | Scene → segment → shot; description, duration, shot size, camera, dialogue, sound | Useful text-generation template; do not import old canvas tools |

Primary sources: [asset templates and variant methods](https://github.com/HBAI-Ltd/Toonflow-app/blob/d2e94aca7b59659ff6b4ca15f03cf134ec7f0e1d/packages/skills/v118workflow/references/knowledge/assetDesign.md), [storyboard image templates](https://github.com/HBAI-Ltd/Toonflow-app/blob/d2e94aca7b59659ff6b4ca15f03cf134ec7f0e1d/packages/skills/v118workflow/references/knowledge/imagePromptCraft.md), [video templates](https://github.com/HBAI-Ltd/Toonflow-app/blob/d2e94aca7b59659ff6b4ca15f03cf134ec7f0e1d/packages/skills/v118workflow/references/knowledge/videoPromptCraft.md), [storyboard table](https://github.com/HBAI-Ltd/Toonflow-app/blob/d2e94aca7b59659ff6b4ca15f03cf134ec7f0e1d/packages/skills/v118workflow/references/storyboardTable.md).

The extraction handoff `/tmp/videoflow-toonflow-prompts-candidates.json` contains 13 verbatim fenced prompt/example blocks from those four files, with source path, nearest heading, category and placeholder inventory. Examples and specialized layouts are included for review, not all recommended as initial presets. It excludes whole agent/skill documents. Mandarin/English variants should share one semantic template ID and a deliberately authored matching variable schema; originals mix Mandarin and English and are not complete bilingual pairs.

## Variables and integration boundaries

The production documents use human-editable `[description]` placeholders, runtime media markers `{{ref N}}`, and internal prose labels such as `<主体1>`. These are different mechanisms. Square brackets also name sections (`[Visual]`), so the handoff's mechanically extracted placeholder inventory needs editorial normalization. Do not blindly expose every bracketed item as a form field.

The separate system-prompt renderer uses `{{word}}` variables, single-level `{{#word}}...{{/word}}` and inverse conditionals; that is not a generic renderer for production prompt examples. Its available values are skills, tools, guidelines and environment. [Renderer source](https://github.com/HBAI-Ltd/Toonflow-app/blob/d2e94aca7b59659ff6b4ca15f03cf134ec7f0e1d/apps/server/src/agent/runtime/prompt.ts).

The production guidance explicitly says provider-branded legacy templates are writing formats, not current model compatibility guarantees. Its JSON fields, negative text, old default models, duration assumptions and old reference syntax must not become invented API fields. Preserve 6eek's original canvas and platform-managed model selection: templates insert/edit prompt content; they do not configure providers, request credentials, change billing, select a new model or initiate generation. Actual media links must come from the existing canvas and actual model schema. [Image guidance](https://github.com/HBAI-Ltd/Toonflow-app/blob/d2e94aca7b59659ff6b4ca15f03cf134ec7f0e1d/packages/skills/v118workflow/references/knowledge/imagePromptCraft.md), [video guidance](https://github.com/HBAI-Ltd/Toonflow-app/blob/d2e94aca7b59659ff6b4ca15f03cf134ec7f0e1d/packages/skills/v118workflow/references/knowledge/videoPromptCraft.md).

## UX evidence and recommendation

Toonflow's shared prompt editor offers inline editing, an 80%-width expanded dialog, `@` reference selection, reference chips with thumbnails, and click-to-preview images. It normalizes recognized `{{ref N}}` markers into chips and serializes chips back into values. This supports a canvas-adjacent template picker with language toggle, editable preview, small required-variable form and explicit insert action. These picker/form suggestions are 6eek design recommendations, not a claim that upstream has a general template library. [Prompt input source](https://github.com/HBAI-Ltd/Toonflow-app/blob/d2e94aca7b59659ff6b4ca15f03cf134ec7f0e1d/packages/nodeShared/src/components/promptInput.vue).

Upstream also has eight compact English lighting presets with Mandarin labels and numeric lighting settings. These are useful future optional modifiers, but should not be silently appended because they can contradict an adopted scene. [Lighting presets](https://github.com/HBAI-Ltd/Toonflow-app/blob/d2e94aca7b59659ff6b4ca15f03cf134ec7f0e1d/packages/nodes/imageNode/src/imageLighting.ts).

For story development, upstream provides methods rather than a small complete universal prompt: preserve established facts, distinguish proposals/unknowns, make goal/obstacle/stakes concrete, connect action to consequences and preserve confirmed drafts. Author a short bilingual 6eek story template from those methods instead of shipping the entire workflow. [Story methods](https://github.com/HBAI-Ltd/Toonflow-app/blob/d2e94aca7b59659ff6b4ca15f03cf134ec7f0e1d/packages/skills/workflow/references/methods/storyDevelopment.md).

Verification: pinned HEAD and MIT file read locally; all four extraction sources read; candidate JSON generated with 13 blocks. No 6eek application code or models were changed by this research task.
