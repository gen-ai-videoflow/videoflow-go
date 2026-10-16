# Unified project workspace

The creator should experience one project from idea to published video. Story, Canvas and Edit are views of that project, with one shared project identity and navigation. Resource management supports these views rather than competing with them.

## Journey

1. Projects: start or resume a named private project.
2. Story: describe an idea, review the credit quote, confirm generation, then inspect scenes and shots. Once scenes exist, generation controls collapse so the story takes priority.
3. Canvas: explore the same project's characters, scenes and assets spatially. Generation remains an optional panel.
4. Edit: assemble completed clips and local media in the embedded OpenCut timeline; edit audio, text and captions. Leaving waits for the editor's save handshake.
5. Export: download, or continue to the publishing form. Publishing requires a finished video, metadata and rights confirmation; users select visibility and Premium access before publication.

Resources contains scripts, characters, scenes, shots, assets and generated captions. These remain available without seven competing top-level destinations. Existing URLs remain valid and project selection follows each transition.

## Interface changes

- Shared project switcher and Story / Canvas / Edit navigation across all three surfaces.
- One Resources entry for supporting material.
- Remove duplicate editor / storyboard / workspace sidebar destinations.
- Storyboard uses the application's surface, foreground, border and primary tokens; no hard-coded white or olive panels.
- One empty storyboard with an action to add scenes, instead of two stacked empty panels.
- Populated story shows the graph beside the selected scene inspector. Remove the redundant scene ribbon and second scene list.
- Generation and assembly controls become a disclosure after scenes exist.
- Preserve quotation, explicit paid confirmation, unsaved-change protection and private-by-default publishing.

## Acceptance

- Project ID survives Story → Canvas → Edit and Resources transitions.
- Switching project while editing preserves existing unsaved-change protection.
- Dark and light themes have readable nodes and graph controls.
- Narrow screens stack graph and inspector, and workflow navigation fits without horizontal overflow.
- OpenCut export still reaches download or upload/publish using the existing validated handoff.

## Boundaries

Shared project context does not mean scene metadata edits automatically rewrite generated video or the OpenCut timeline. Generation creates outputs; importing those outputs into Edit is explicit. Export remains required before publishing an edited timeline. These distinctions are retained in contextual copy.

## Verification evidence

- Full cloud race suite passed (85.185s), including admin, runtime policy and creator analytics integration.
- Svelte check: zero errors and zero warnings after the workspace integration.
- Existing studio navigation, settings navigation and validated editor handoff scripts passed.
- Browser inspection: shared project links retain the selected project;390px Story view has no horizontal overflow; desktop graph and scene inspector share one row.
- Final preview packaging and editor save-handshake verification are recorded after the production build.
