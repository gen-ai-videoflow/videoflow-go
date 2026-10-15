# Bilingual creation review

Scope: original Storyboard canvas, character/scene/asset details, prompt workbench, project starter templates and browser editor hierarchy. The implementation uses six paired English/Simplified Chinese templates with pinned Toonflow provenance and full MIT notice. Model selection and credit confirmation remain on the server-controlled generation paths.

## Technical checks

- New project-owned template store passed race-enabled tests for English/Mandarin round trips, saved defaults, ownership, successful revision updates, stale update/delete rejection and invalid category rejection. The canvas test verifies appearance and prompt edits round-trip into its graph; private object keys remain hidden.
- Full backend race suite passed across nine tested packages before the final saved-default validation extension; targeted race tests passed after that extension. Go vet passed. CI runs the full final suite again.
- Svelte check:0errors,0warnings. Isolated React editor TypeScript passed. Prompt checks cover bilingual variable parity, UTF-8 byte accounting, literal `$&`/HTML-like values, unresolved references, missing fields and inherited object-property exclusion. Existing payment, settings, Shorts state and Studio navigation checks passed.
- Prompts render as escaped text; no eval/HTML interpretation or arbitrary remote fetch. User instructions remain user prompts, with platform system policy unchanged. Templates never trigger provider calls or credit charges on save/apply.

## Browser evidence

Used isolated authenticated QA projects with synthetic content, not normal preview user data. Desktop1440×900 and mobile390×844 were inspected using T3 Chromium emulation.

- Filled a Mandarin story template, saved both language bodies, applied it and checked that no placeholders remained. Submitted prompt preview included saved character descriptions; script language became zh. Saved-template revision2 retained six Mandarin variable defaults on the server.
- Edited a character's Mandarin appearance on the canvas; server readback and submitted-context preview both included the new sleeve detail. Inspector is docked beside the graph: stage500.8px, inspector320px, with no overlap. Focus canvas hides the story pane. Default prompt workbench remains collapsed to reduce clutter.
- Editor uses grouped project actions, labelled media tools and selected states, less prominent save status, clearer panel boundaries and local Inter font. Desktop/mobile have no document horizontal overflow; all five tool panels remain available. Timeline scrolling is local. Existing synthetic video and text remain visible, and Back to projects completes the save handshake.
- Mandarin project starters show translated script/scene content and editable fields on mobile with no page overflow. Starter creation/import is checked separately below.

Captures in T3 browser artifacts: canvas before repair `mv9s0on8-da918955`, docked canvas `mv9sedq7-9d5154ba`, editor desktop `mv9s5ahl-8d22882d`, editor mobile `mv9s6z3v-58bf3f91`, Mandarin starter mobile `mv9sgiko-22142a8c`.

## Limits

No live AI/provider calls, image generation, reference-image conditioning, physical-phone testing or language-quality guarantee is inferred. Character/prop templates are editable text directions. Cloud asset references remain subject to validation; browser editor source files remain browser-local. User-authored custom text is not automatically translated. Native tab closure cannot await asynchronous cloud saves. AWS and live payment activation remain deferred.

A local disk-space failure interrupted formatting. Affected working files were restored and checkpointed, generated caches cleared, then type/render checks rerun successfully. The last working preview was preserved throughout; failed intermediate builds were not promoted.

Starter import check passed: a mobile Mandarin starter was customized with a new script marker and first-scene direction, then created through the normal form. It opened `/studio` for the returned project; server readback preserved the custom Mandarin script and scene prompt, created3scenes and created0generation jobs. This verified a free editable project import, not a paid generation.
