# Account, creator and viewing experience status

The payment execution checklist is distinct from the wider product roadmap. The user deferred AWS deployment while local payment hardening and recovery verification continue.

| Area | Implemented locally | Further product work |
| --- | --- | --- |
| Account/authentication | shadcn login-02/signup-02 layout, application-owned credentials, email verification/recovery tokens, session expiry/revocation, origin protection, request limits, safe post-login destination, encrypted TOTP/recovery codes, WebAuthn passkeys, shared recovery/MFA layouts | User-visible session/device management, deeper abuse controls and delivered-email verification in a real environment; do not describe the module as perfect |
| Public creator profile | Editable avatar/banner/bio/location/HTTPS links, true public counts, published public videos, persistent follows, official user-profile1 Svelte layout and Premium gating | Collections and user-visible pinned/featured video controls |
| Creator management | Original project canvas, templates, generation quotes/jobs, publishing privacy/audience settings, basic analytics/earnings | A focused content-management dashboard, scheduled publication, comments/moderation workflow and deeper analytics |
| Discover | Default Discover viewer navigation, Following and video search, separate Studio tools, public channel/watch routes and protected playback | Richer recommendation controls, watch-next and continue-watching surfaces |
| Short-form viewing | Short video outputs can be published through the normal video path | Dedicated vertical swipe/keyboard Shorts feed, autoplay/mute controls, lifecycle/accessibility controls and format-aware recommendations |
| Streaming | Private originals, HLS encode/delivery integration, short-lived scoped playback authorization, caption snapshots | End-to-end media-provider tests and performance/load verification against deployed services; no claim of live streaming readiness from a local empty feed |

Recommended information architecture: **Discover** for general browsing, **Shorts** for vertical viewing, public **Creator profiles** for their work, and private **Studio** for projects/templates/publishing/analytics. A single account moves between viewing and creating; Premium viewing and generation credits remain separate. These roadmap features are not marked implemented by completing payment checks.

## Security verification limits

Automated verification includes real signed ES256 WebAuthn software-authenticator fixtures, origin/user-verification/counter checks, MFA/recovery replay and restricted PostgreSQL runtime grants. A physical/platform authenticator enrollment still needs a compatible device/browser check; automated fixtures are not a claim of hardware testing. Delivered email and deployed media/load checks remain external environment gates. AWS and live payments remain disabled/deferred.
