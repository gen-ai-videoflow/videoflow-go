# Account, creator and viewing experience status

The payment execution checklist is distinct from the wider product roadmap. The user deferred AWS deployment while local payment hardening and recovery verification continue.

| Area | Implemented locally | Further product work |
| --- | --- | --- |
| Account/authentication | shadcn login-02/signup-02 layout, application-owned credentials, email verification/recovery tokens, session expiry/revocation, origin protection, request limits, safe post-login destination, encrypted TOTP/recovery codes, WebAuthn passkeys, shared recovery/MFA layouts, active session review/revocation and expired-session cleanup | Device naming/recognition, deeper abuse controls and delivered-email verification in a real environment; do not describe the module as perfect |
| Public creator profile | Editable avatar/banner/bio/location/HTTPS links, true public counts, published public videos, persistent follows, official user-profile1 Svelte layout, featured public-video selection and Premium gating | Collections |
| Creator management | Original project canvas, templates, generation quotes/jobs, publishing privacy/audience settings, basic analytics/earnings | Paginated creator content with status/format filters and a step-by-step publication composer now exist; scheduled publication, comments/moderation workflow and deeper analytics remain |
| Discover | Default Discover viewer navigation, Following and video search, separate Studio tools, public channel/watch routes and protected playback | Watch-next, saved videos and history now exist. Recommendation ranking/personalisation and richer recommendation controls remain |
| Short-form viewing | Dedicated portrait Shorts feed, previous/next and keyboard navigation, one active player, muted autoplay/preferences, format-aware feed and profile tabs | Recommendation ranking, additional real-device/browser checks and deployed load verification |
| Streaming | Private originals, HLS encode/delivery integration, short-lived scoped playback authorization, caption snapshots, owned output preview/download, shared asset publication, validated multipart upload implementation (disabled until configured) | End-to-end media-provider tests and performance/load verification against deployed services; no claim of live streaming readiness from a local empty feed |

Recommended information architecture: **Discover** for general browsing, **Shorts** for vertical viewing, public **Creator profiles** for their work, and private **Studio** for projects/templates/publishing/analytics. A single account moves between viewing and creating; Premium viewing and generation credits remain separate. These roadmap features are not marked implemented by completing payment checks.

## Security verification limits

Automated verification includes real signed ES256 WebAuthn software-authenticator fixtures, origin/user-verification/counter checks, MFA/recovery replay and restricted PostgreSQL runtime grants. A physical/platform authenticator enrollment still needs a compatible device/browser check; automated fixtures are not a claim of hardware testing. Delivered email and deployed media/load checks remain external environment gates. AWS and live payments remain disabled/deferred.

## Launch preparation

Reviewable messaging, a demo script, social/outreach drafts and the first-month checklist are in `docs/launch/LAUNCH-CONTENT.md`. These are prepared materials, not sent outreach or a claim of a public production launch.

## October 15 viewing and publication work

Settings now separate Account, Public profile, Security, Playback, Privacy and Billing, with persisted playback/history preferences and fresh credential checks for deletion requests. Viewers have standard Watch pages, related videos, durable likes/saves/history, and separate Shorts; creators keep the original canvas and use a focused publishing shell. Generated outputs and validated uploads share the same publication composer; owner-only original download links remain separate from public viewing access.

The file-upload gate remains off in the normal preview. Read `docs/ux/VIDEO-UPLOAD-OPERATIONS.md` before enabling it; uncertain multipart creation retains quota pending operator reconciliation. Local synthetic playback fixtures and protocol tests do not establish production cloud delivery readiness. These feature changes do not activate payments, creator rewards or AWS deployment.
