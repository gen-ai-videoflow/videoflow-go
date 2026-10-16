# Integrated workspace and help

The Studio overview now resumes the Story view and uses the same Story, Canvas and Edit destinations as the project workspace. Recent projects retain their IDs. Canvas controls move into the shared toolbar, removing its separate control row; project switching also updates the URL. Empty canvas job history is hidden.

A contextual guide action is available in the project toolbar. Help is public and uses the focused viewing shell, with its own topic contents rather than a second permanently open global sidebar. Search filters the complete guide content. Narrow layouts stack contents and articles. The downloadable Markdown guide and in-app content share one source and a regeneration check.

The cloud guide is deliberately separate from the existing desktop handbook. It explicitly distinguishes saved project data, generated clips, browser-local editor media and published videos. It explains private defaults, quotes, payment settlement, Premium, security, publication and unsupported services without promising unconfigured capabilities.

## Verification

- Svelte check:0 errors,0 warnings; production cloud build passed.
- Guide generator validates unique sections, required content, internal action links and exact downloadable-content parity; this check also runs in CI.
- Existing project-navigation and editor-export handoff checks passed.
- Browser checks:11 guide topics, search, no-results recovery, caption-topic discovery and Markdown download passed;390px guide has no horizontal overflow.
- Authenticated Canvas:one project toolbar, correct selected project and links, no horizontal overflow at320px,390px and1280px.
- Shared T3 browser was used for the initial desktop pass. After it explicitly disconnected, the bounded mobile and functional checks used local headless Chrome.
