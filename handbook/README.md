# 6eek handbook

This handbook describes the local 6eek workflow inspected on 27 September 2026. Start here whether you have an empty computer, an empty 6eek project, or an existing story.

**Current readiness:** Go backend and SvelteKit frontend tests pass, and zero-dependency single binary build is operational. Confirm credentials, account access, and model behavior before production use. AtlasCloud and OpenRouter routes are supported. See [provider setup](03-providers.md).

## Read in order

| Part | What you will learn |
|---|---|
| [1. How 6eek works](01-how-it-works.md) | Concepts, workflow philosophy, and what automation actually does |
| [2. Install from zero](02-installation.md) | Prerequisites, download, launch, health checks, and stopping safely |
| [3. Configure providers](03-providers.md) | Keys, models, agents, media engines, and OpenRouter limitations |
| [4. Projects](04-projects.md) | Create, switch, export, import, and back up work |
| [5. First video](05-first-video.md) | Start with an idea and no characters or assets |
| [6. Characters and assets](06-characters-assets.md) | Reusable identities, descriptions, reference images, and style |
| [7. Scenes and shots](07-scenes-shots.md) | Build and inspect a story in controllable stages |
| [8. Studio](08-studio.md) | Use the relationship canvas and guided chat |
| [9. Renders and outputs](09-renders.md) | Understand jobs, QA, retries, and downloads |
| [10. Captions and editor](10-captions-editor.md) | Transcribe, correct timing/text, and export captioned video |
| [11. Troubleshooting](11-troubleshooting.md) | Recover from setup, generation, and media errors |
| [12. Reference](12-reference.md) | Glossary, configuration, routes, and maintainer commands |

## Choose a starting point

- **Nothing installed:** begin with Part 2. You do not need an existing code project, database, video, character, or GPU to open the app.
- **App running, empty project:** configure Part 3, create a project in Part 4, and follow Part 5.
- **Need controlled production:** follow Parts 6–10, reviewing each stage before the next paid request.

Examples describe expected behavior, not a promise that every external model succeeds. Provider access, selected models, account credit, and the limitations recorded in the audit still apply.
