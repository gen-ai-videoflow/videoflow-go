# Account, creator and viewing experience status

The payment execution checklist is distinct from the wider product roadmap. The user deferred AWS deployment while local payment hardening and recovery verification continue.

| Area | Implemented locally | Further product work |
| --- | --- | --- |
| Account/authentication | shadcn login-02/signup-02 layout, application-owned credentials, email verification/recovery tokens, session expiry/revocation, origin protection, request limits, safe post-login destination | MFA/passkeys, user-visible session/device management, deeper abuse controls and delivered-email verification in a real environment; do not describe the module as perfect |
| Public creator profile | Channel name/description, published video list, follows and Premium audience gating | Avatar/banner management, richer profile/about links, clear public-profile versus private creator-management navigation, collections and pinned/featured videos |
| Creator management | Original project canvas, templates, generation quotes/jobs, publishing privacy/audience settings, basic analytics/earnings | A focused content-management dashboard, scheduled publication, comments/moderation workflow and deeper analytics |
| Discover | Separate public viewer navigation, published feed, public channel/watch routes, server-enforced protected playback | Search/category/filter UI polish, richer recommendation controls, watch-next and continue-watching surfaces |
| Short-form viewing | Short video outputs can be published through the normal video path | Dedicated vertical swipe/keyboard Shorts feed, autoplay/mute controls, lifecycle/accessibility controls and format-aware recommendations |
| Streaming | Private originals, HLS encode/delivery integration, short-lived scoped playback authorization, caption snapshots | End-to-end media-provider tests and performance/load verification against deployed services; no claim of live streaming readiness from a local empty feed |

Recommended information architecture: **Discover** for general browsing, **Shorts** for vertical viewing, public **Creator profiles** for their work, and private **Studio** for projects/templates/publishing/analytics. A single account moves between viewing and creating; Premium viewing and generation credits remain separate. These roadmap features are not marked implemented by completing payment checks.
