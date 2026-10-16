# Bilingual prompts and creation templates

6eek supports paired English and Simplified Chinese prompt templates in Storyboard, single-clip generation, and character/scene/shot/asset editing. Open Prompts & templates, choose a language, fill named fields, review the rendered text and Apply prompt. Applying a template changes the draft only; the existing credit quote and confirmation still control generation. An English template does not translate quoted dialogue or a user's custom text automatically.

Six built-in templates cover character sheets, environments, props, storyboard frames, continuous video direction and story outlines. Built-ins remain immutable; Save my template creates a project-owned copy. Edit English and Mandarin separately; a missing language is stated rather than silently substituted. Updates and deletes require the current revision. A stale-tab conflict preserves the draft; refresh the saved list and reconcile before retrying. Each project permits100 personal templates. Template text is limited to16KB per language; final generation prompts are limited to8KB, measured in UTF-8 bytes so Mandarin is counted accurately. Rendering is literal text, with no evaluation or HTML interpolation. Variables use `{{subject}}` syntax; unsupported `{{ref N}}` tokens cannot be applied as fake media references.

Saved character and scene design text can be included in the Storyboard generation request. The complete submitted text is available for review. This is text conditioning only: no reference images, audio inputs or unsupported provider capabilities are attached automatically. Asset-design prompts are editable directions, not evidence of a new image-generation provider. Studio media references remain private and subject to the existing validation gates.

Project starters also have English and Mandarin outlines. Customize script & scene directions before creating a project. This creates editable script/scenes/shots and spends no generation credits. Further edits and generation remain in the original Storyboard canvas. The browser editor remains separate, with generated-source import, browser-local media, real timeline editing and export.

## Provenance

Toonflow reference: https://github.com/HBAI-Ltd/Toonflow-app/tree/d2e94aca7b59659ff6b4ca15f03cf134ec7f0e1d/packages/skills/v118workflow/references/knowledge . Character/scene/prop templates adapt the corresponding Mandarin blocks; the English versions are 6eek translations. Storyboard/video templates adapt general content patterns without named-model assumptions or unbound references. The story-outline template and starter translations are authored for 6eek. Per-template source URLs are in `frontend/src/lib/cloud/prompts/catalogue.json`; preserve the full MIT notice in `frontend/static/licenses/toonflow-MIT.txt`. This is not a Toonflow endorsement.

## Deployment

Run migration030 before the new API/UI. `/projects/{project}/prompt-templates` GET/PUT and its DELETE child route enforce project ownership and optimistic revisions. PostgreSQL is authoritative; Redis is not used for template persistence. Existing cloud secret, provider and credit policies are unchanged. No AWS apply or live-provider activation is included.

Personal templates also save filled variable defaults. Restoring a preset restores these values as literal text; switching languages does not translate them. Defaults are bounded and validated on the server. The docked canvas inspector and Focus canvas control keep editing separate from the story assistant, and unsaved design edits are guarded during in-app navigation.
