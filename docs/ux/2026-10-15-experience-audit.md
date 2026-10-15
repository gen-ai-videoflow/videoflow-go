# VideoFlow experience audit

Review date: 2026-10-15. Scope: current local web UI and code supporting settings, viewing, creation, publishing and downloads. This is a planning audit, not approval that all screens are complete or that production streaming works.

## Evidence

- Inspected live settings at 1440×900 and 390×844, and mobile Discover. No horizontal document overflow in these samples.
- Desktop settings capture: `/home/jienweng/.t3/userdata/browser-artifacts/browser-screenshot-asus-dev-tail66f665-ts-net-mv9j0sxk-26f960b4.png`.
- Mobile settings capture: `/home/jienweng/.t3/userdata/browser-artifacts/browser-screenshot-asus-dev-tail66f665-ts-net-mv9j1bwz-ad48845d.png`.
- Inspected current Svelte routing, viewer shell, player, generation jobs, account controls and Go publishing/playback/upload/watch-accounting paths.
- No populated production video catalogue was available. Watch/Shorts conclusions below are code-based; actual media delivery, quality changes and long playback were not visually verified.
- Historical console entries from earlier tests were excluded from new findings; they are not evidence of current failures.

## Findings, ordered by impact

| Priority | Finding | Evidence | Required result |
| --- | --- | --- | --- |
| P1 | No dedicated Shorts journey or discovery contract | No Shorts route/nav/player lifecycle or measured format metadata in current video response | Vertical browsing with one active player, public/format filtering, accessible navigation and bounded preloading |
| P1 | Uploading an existing video is not a complete publication flow | Generic upload grant exists; CreateVideo requires a successful generation job | First-class owned media assets and validated upload completion; no fake generation jobs for uploaded files |
| P1 | Publish and download are separate journeys | GenerationJobs exposes Download; channel form requires choosing a completed job separately | Finished output offers Preview, Publish on VideoFlow and Download with its source preselected |
| P1 | Watch is a basic player shell rather than a complete viewing page | CloudApp inline watch branch; no related-video region, saved progress or explicit Premium gate component | Player-led layout, creator/actions, description, related releases, clear unavailable/access states |
| P1 | Fixed 10-second view threshold does not suit short clips | RecordWatch in internal/cloud/engagement.go | Duration-aware public view policy, loop/seek/background protection; no automatic change to financial rewards |
| P2 | Settings actions visually collide and wrap into broken text | Verification button/link/Edit profile share inline flow; mobile “Enter verification token” wraps around neighbours | Separate verification block and wrapped action group with defined spacing; editing profile lives in its own settings section |
| P2 | Settings has no persistent subnavigation | Account, profile, security and payments are separate dropdown destinations | One settings shell with account, profile, security, playback, privacy and billing sections; old URLs redirect |
| P2 | Generic creator-oriented loading copy reaches viewer/account pages | “Opening your creative space…” for all routes | Route-specific skeletons and task-specific accessible loading text |
| P2 | Discover empty-state action loops back to itself | Empty Discover links to Discover | Explain the empty catalogue; contextual creator action for signed-in users, meaningful retry/back alternatives |
| P2 | Browsing lacks cursor pagination and richer metadata | Discovery returns at most 60 records; tiles have limited presentation metadata | Stable pagination, aspect/duration thumbnails, creator, publish age and truthful counts |
| P2 | Download waits on async request before opening a window | GenerationJobs.download calls window.open after await | Visible secure download link or direct navigation fallback; failure leaves output page intact |
| P2 | Controls are compact for touch | Sample settings buttons measured about 32 CSS px high | Aim for 44×44 CSS px touch controls on mobile; retain compact desktop density |
| P3 | Some headings use decorative eyebrow labels | Account settings “MAKE YOURSELF AT HOME”, generation stage label | Remove nonfunctional eyebrow copy during the relevant screen pass; retain actual statuses |

## Preserve

Original Studio canvas and project logic; shadcn login-02/signup-02 composition and shared recovery layout; existing dark neutral identity, typography, Lucide icons and shadcn primitives; real security/account state; ownership, privacy and Premium checks; separate generation credits and viewing Premium.

## Review boundaries

No product code changed in this audit. Full accessibility conformance, populated catalogue layouts and player behaviour require implementation-specific checks. Do not mark these findings resolved merely because a plan or empty-state screenshot exists.
