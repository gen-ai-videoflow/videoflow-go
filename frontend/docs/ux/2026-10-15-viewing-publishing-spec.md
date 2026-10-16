# 6eek settings, watching, Shorts and publishing

Status: proposed detailed scope for review. User confirmed both generated Studio outputs and uploaded files can be published, and that the current shadcn interface should be extended directly without mockups. The existing visual identity and original canvas are binding. Upload support is in scope as a separate deliverable. Numeric upload limits, Shorts thresholds and view-policy changes remain proposals. This document does not authorise production deployment.

## 1. Product contract

One account can watch and create. Watching never requires creator onboarding or purchasing generation credits. Free following and one platform-wide Premium entitlement remain distinct; there are no per-channel paid subscriptions. The platform selects generation models. AWS deployment, live payments and rewards activation remain deferred.

Use familiar viewing conventions while retaining 6eek branding. Preserve the original canvas, not a replacement editor. Do not claim every interface is correct before reviewing real populated states.

Visitor modes: settings and Studio are Operate; standard watch and Shorts are Experience with familiar, accessible controls; discovery supports choosing something to watch. These are surface behaviours, not separate visual identities.

## 2. Navigation and routes

| Area | Routes | Primary navigation and purpose |
| --- | --- | --- |
| Viewer | /discover, /shorts, /following, /search | Discover, Shorts, Following; search visible on desktop; compact search entry on mobile |
| Watch | /watch/:id, /shorts/:id | Shareable deep links; browser Back restores feed position, query and active item |
| Personal library | /library, /history | Saved videos and continue watching; authenticated, no generation tools |
| Public creator | /channels/:handle | Featured video, Videos/Shorts/About tabs, Follow for free; editing links only for owner |
| Settings | /settings/account, /settings/profile, /settings/security, /settings/playback, /settings/privacy, /settings/billing | One shared settings shell; current section indicated; /settings defaults to account |
| Studio | /studio, /studio/projects, /studio/templates, /studio/content, /studio/outputs/:jobId, /studio/publish | Existing canvas; focused project, output and content management; explicit Back to watching |
| Uploaded source | /studio/upload | File picker/progress, then same publication composer as generated outputs |
| Creator business | /studio/analytics, /studio/earnings | Watch/format analytics and gated rewards, separated from viewer settings |

Desktop: viewer header plus restrained navigation; settings uses a secondary left rail and a readable content column. Mobile: compact header and bottom viewer navigation (Discover, Shorts, Following, You), with search reachable from header. Shorts suppresses duplicate chrome while retaining an accessible exit. Settings uses a section index and a labelled section selector/back link, not a horizontally clipped desktop rail. Studio keeps its own task navigation.

Keep existing links (/profile, /security, /payments, /credits, /channel, /render and current /studio aliases) through explicit redirects/compatible entry points, preserving query parameters and safe authentication return targets.

## 3. Settings journeys

### Account

Show email, verified status, display name and username. Username remains read-only until a handle migration policy exists. Display-name edit links to profile. Verification gets one clearly grouped block with resend cooldown, inbox instructions and enter-code action; development tokens only appear in development mode. Email change is a later real backend flow requiring recent authentication and verification of the new address, never an inert edit control.

### Public profile

Reuse the current profile editor and user-profile1 public composition. Edit display name, bio, location, avatar/banner and HTTPS links; feature a published public video; preview public profile. Save only on explicit submission. Preserve input on errors, show saved feedback, and warn before discarding dirty edits. Failed image loads use initials/neutral fallback. Current URL fields remain until a real image-upload path is built.

### Security

MFA, recovery codes, passkeys and sessions use existing server endpoints. Separate sections have independent loading/error states. No secret in analytics, query strings or external QR URLs. Recovery codes remain one-time display with copy/acknowledgement. Reauthentication requirements stay server-authoritative. Sessions show Current and activity dates; one-session or other-session revocation preserves the current session. Password reset preserves factors. Add password-change/device naming only with corresponding backend support.

### Playback and accessibility

Persist captions preference/language, autoplay-next (default off), Shorts autoplay (proposed default on while muted), reduced-motion preference (system by default), and data-saving preference. Browser volume/mute can be local; authenticated preferences sync through a dedicated settings endpoint. Browser capability or Data Saver may prevent autoplay; always retain a visible Play control. Do not imply selecting data saving guarantees exact bandwidth or overrides browser/network policy.

### Privacy

History on/off, clear history, hidden creators and account deletion request. Clear-history removes resume/history data and applicable recommendation history, not financial evidence or public aggregate counters. Pause-history stops new history persistence. Deletion uses a dedicated confirmation, recent authentication and accurate retention copy. It never silently deletes immediately or promises a retention deadline not in policy.

### Billing

Separate Premium access/expiry, generation credit balance, invoices and payment availability. Preserve existing CryptoRails invoice recovery and settlement logic. No automatic-renewal language for prepaid 30-day Premium. Creator payout onboarding stays in Studio earnings. Disabled payments explain availability without an actionable purchase button.

Notification settings are not shown until a real notification preference/service exists. No decorative nonfunctional toggles.

## 4. Standard watching

Arrival: public metadata/poster loads first; access status is explicit before asking the user to play. A viewer may browse free public content as a guest. Sign-in prompts preserve the target for following/saving/reporting.

Desktop: main column with contained 16:9 player, title, creator row, Follow, Share, Save, like count if implemented, publish date/views and expandable description. Related videos sit in a secondary column. Mobile stacks player, title, creator/actions, description and related videos. Portrait videos letterbox/contain without forced cropping; “Watch as Short” is available only when eligible.

Player: native accessible controls initially, captions, fullscreen and supported picture-in-picture; keyboard interaction never steals keys from text inputs. Preserve position on retry/re-authorisation. Do not reset playback on an unrelated save/follow update. Autoplay-next is opt-in; warn about next video and allow cancellation. Captions/transcripts have language labels rather than claiming unknown language is user language.

Access states: sign-in required; Premium required (correct platform-wide pricing); private/not found without leaking private metadata; processing; blocked/unavailable; unsupported format; expired authorisation; offline/buffering. Premium metadata can be visible, protected segments cannot be prefetched. No “Play” button that simply leads to a generic unauthorised error when access is known to be locked.

Related results exclude current, private, unlisted, blocked/deleted and hidden creators. Explain simple signals (“More from this creator”, “Recently published”); do not pretend a learned algorithm exists. Likes and saves require idempotent server-backed mutations; controls appear only when implemented. Comments are a separate moderated feature phase, not a fake box in the first watch release.

## 5. Shorts journey

A Short is a viewing format for an existing publication, not a duplicate paid product or duplicate upload. Proposed initial eligibility: measured portrait media, duration 3–180 seconds, published public status. Owner can select Standard or Short only if probe validation permits it; requested generation aspect ratio alone is not trusted. Thresholds are configurable server policy pending product confirmation.

Mobile: one centred video per viewport with CSS scroll snapping, swipe navigation, visible Previous/Next alternatives, creator/title/captions, follow/share/save/report actions and mute/play controls. Desktop: contained portrait player with context and visible up/down navigation. Deep link identifies the active item; replacing feed position must not add a browser-history entry on every swipe. Back returns to the originating page.

Playback manager owns one active player. Pause/destroy offscreen players, suspend hidden-tab playback, stop analytics on buffering/seeking, and release HLS instances. At most current plus one nearby player is mounted; at most one next item is lightly prefetched when entitled and data-saving rules allow. Do not stream an infinite list in parallel. Locked Premium items show a gate without fetching protected media. Muted autoplay is attempted only if allowed; rejection yields Play, not an error loop. User pausing a clip remains paused until they explicitly resume or choose another clip. Save mute/caption choice across items. Reduced motion disables smooth auto-scrolling.

Feed state: cursor-based pagination, deduplicate IDs, retain short-term seen IDs, show retry per failed page, preserve current item on fetch failure, support finite/end-of-feed and empty states. Modal, settings or report entry pauses the video. Swipe gestures do not override screen-reader navigation or page controls. Announce the new title without reading every timer update.

## 6. Finished output: direct publication or download

Canonical flow:

Studio project → quote/confirm generation → processing → finished output → preview → **Publish on 6eek** or **Download MP4**.

The finished-output page is owner-only and carries the completed job/source, output duration/orientation and real processing status. Downloading does not publish. Publishing does not require downloading/re-uploading. Both actions remain available after either one succeeds; an existing publication opens Edit publication rather than making a duplicate.

Publish composer stages:

1. **Details:** source preview, title, description, thumbnail/frame, caption status/language, origin disclosure (AI-created for generated sources; uploaded sources require truthful selection).
2. **Audience:** Standard/Short if eligible; Public/Unlisted/Private; Free/Premium. Explain that Premium controls who watches, visibility controls where it is discoverable. Default Private/Free. Unsupported combinations are rejected with a field-level explanation.
3. **Review and publish:** explicit summary of visibility, audience, format and source version. Save draft is secondary. Request processing once using an idempotency key; “Processing” does not mean publicly watchable. Show public link only after published and playable, or a clearly labelled owner processing preview beforehand.

Sources are immutable versions. Editing a project after publishing does not silently change its published video. A replacement render requires an explicit replacement/version workflow and re-encoding; retention of the old source follows policy.

Download: owner-authorised grant for a succeeded output, safe filename and Content-Disposition attachment. Prefer a visible signed link/direct navigation after obtaining the grant, not an async popup. Retry obtains a new grant. Show Preparing/Ready/Failed and “Download again”; never claim browser download completion when only a transfer was initiated. Generated output offers its actual ready MP4; an uploaded original retains its true format/filename. Offer MP4 export only when a real MP4 derivative exists, never rename WebM bytes to MP4. Captions download separately when present. Extra resolutions require real export jobs and pricing, not switches that return the same file.

Viewer download of creator originals is off in the initial scope. Premium viewing does not grant original-file download rights. This plan does not add OAuth posting to YouTube/TikTok; downloading permits the creator to upload elsewhere themselves.

## 7. Upload existing videos — confirmed scope, separate phase

Picker/drop area with file type/size policy before selection, progress/cancel/retry and persisted upload session. Reserve storage/quota before granting uploads. Direct browser-to-private object storage; backend handles metadata and grants, never proxies video bytes through the API.

Draft limits for confirmation: MP4/WebM inputs, 500 MiB per video, at most 10 minutes per source, one active upload per account; Short remains 3–180 seconds. These differ from the current 100 MiB generic asset/1 GiB workspace policy and must not silently raise it. Enforce a separate video quota and budget config before rollout. Same-file re-selection may resume only after server validation and matching ownership/checksum.

Source states: reserved → uploading → uploaded → validating → ready or rejected; aborted/expired states retain quota until object/multipart cleanup is confirmed. Verify actual size/checksum, ownership, container/codecs, decoded duration/dimensions/rotation and resource limits using a bounded isolated media probe. Filename/MIME are hints, not proof. Invalid or oversized files never become public. Handle duplicates, abandoned parts and completion retries safely.

Validated assets feed the same draft/publication/encoding flow as generated media. Do not fabricate succeeded generation jobs to satisfy current job_id foreign keys. Uploading a file does not charge AI generation credits; storage/encoding costs still need caps. Rights/disclosure confirmation is required; actual moderation and blocked-content handling must exist before general public uploads.

## 8. Backend and data ownership

Introduce an owned `media_asset` boundary for both generation outputs and uploaded sources. Publication references a ready source asset; legacy job-backed videos are backfilled with ownership and unique-source constraints before nullable job links are accepted. Publication source constraint: exactly one resolved asset; no arbitrary client object key. Keep the existing job link for provenance, not as the only way to publish.

Durable PostgreSQL additions (proposed, exact migration numbers assigned at implementation):

- cloud_media_assets: owner/workspace, origin, private object key/version/checksum, measured dimensions/duration/rotation/codecs, validation status and timestamps.
- cloud_video_uploads: owner, storage multipart ID (server-only), reserved bytes, expiry/status; bounded part manifests.
- cloud_video_preferences: account playback/privacy settings with schema version; no security factor data mixed in.
- cloud_video_progress: user/video, position, duration version, updated_at; separate from earnings/watch evidence.
- cloud_video_reactions and cloud_saved_videos: unique(user,video), idempotent state, access-safe reads.
- cloud_videos additions: source_asset_id, format, origin/disclosure, metadata version, publication intent and render/encoding status. Keep moderation, visibility and playable readiness distinct.

Proposed API families: GET/PATCH /settings/preferences; GET /viewer/history and /viewer/saved; PUT/DELETE /videos/:id/save and /videos/:id/like; PUT /videos/:id/progress; GET /feed?format=standard|short&cursor=…; GET /videos/:id/related; POST/PATCH /publications for drafts; POST /publications/:id/publish with Idempotency-Key; GET /jobs/:id/output; GET /media-assets/:id/download; POST /video-uploads, POST /video-uploads/:id/parts, POST /video-uploads/:id/complete, DELETE /video-uploads/:id. Retain existing routes via adapters until clients migrate. All mutation requests follow the current JSON/origin contract.

Workers: durable outbox → queue → source validation/encoding → authenticated completion event → database transition. Idempotency and correlation bind source version, publication and encoder job. Stale callbacks cannot publish old/private/rejected content. Retries use bounded backoff/dead-letter recovery. Deletion/visibility changes take effect in API authorisation immediately; issued CDN grants remain bounded by their short lifetime, not claimed instantly revoked.

Media service stays separate from application logic. Existing AWS S3/private originals, encoding queue/worker, MediaConvert and CloudFront integrations remain the planned path. Local adapters/test fixtures verify transitions before any cloud apply. Postgres carries truth; CDN serves bytes; API issues grants.

## 9. Redis versus database

| Concern | Redis | PostgreSQL/object storage |
| --- | --- | --- |
| Rate limits/upload part throttles/download grant throttles | Short-lived counters; existing fail behaviour reviewed per endpoint | Durable quota/reservation and audit records |
| Feed pages | Small TTL caches of public eligible IDs/metadata, versioned/invalidate on moderation/visibility | Authoritative eligibility and cursor query; private/personalised responses not shared |
| Shorts duplicate suppression | Optional short-lived user/feed seen IDs; browser session fallback | Not a payout or permanent viewing record |
| Playback presence/concurrent-stream hints | Expiring leases, never sole entitlement authority | Premium/ownership checks and bounded signed grants |
| Progress write coalescing | Optional, loss-tolerant only | Persist actual resume/history state; privacy setting checked before write |
| Jobs/encoding/uploads/payment state | No durable source of truth | Postgres state/outbox, queue delivery, private objects |
| Views/likes/saves/rewards | Cache/display only; not authoritative | Deduplicated validated events and unique constraints; capped financial ledger stays separate |

Keep first release operable when optional caches miss; sensitive authorisation never trusts cached public eligibility alone. Define cache keys by viewer entitlement/context or cache only public metadata. Playback lifecycle endpoints must not exhaust generation or payment rate-limit buckets.

## 10. Watch metrics, recommendations and rewards

Current qualifying views require 10 validated seconds, which is unsuitable for a 5-second clip. Propose duration-aware qualifying public views, e.g. ceil(min(10, 0.8 × measured duration)), capped to the actual duration and validated once per viewer/video/day. This is a proposal to validate on 3/5/10/180-second clips, not an approved financial formula. Repeated loops are not new views, seeking is not watch time, and background/parallel playback does not multiply counted time. Keep monotonic time bounds, event idempotency, owner exclusion and server entitlement checks.

Maintain distinct numbers for starts, qualified public views, watch time, completions and reward-eligible views. Reward eligibility is server policy and cannot change with a UI label. Anonymous views may be displayed only after a separately reviewed privacy/abuse policy; they are not automatically payout eligible. Existing capped revenue-share activation remains off.

Initial related/Shorts ordering: eligible public releases, follows, explicit hidden-creator preferences, recent items and validated engagement, with diversity and repetition caps. No browser-supplied ranking/view count is trusted. “Not interested” writes a real preference. No machine-learning or earnings guarantee is advertised.

## 11. States, accessibility and visual acceptance

Every surface must handle: guest/signed-in/owner/Premium; first run and empty; slow/loading; validation/error/retry; offline; long names/titles/non-Latin content; single versus hundreds of results; denied/private/blocked/deleted content. Preserve dirty forms and feed position. Route-load responses are cancelled/versioned so late results cannot replace a newer route.

Retain current dark neutral tokens and shadcn primitives. Use consistent semantic headings, readable body text, clear action groups, real thumbnails and contained video. Functional states use text plus colour. Primary actions have one clear hierarchy. Touch controls target 44×44 CSS px; keyboard focus is visible; menus escape clipping containers. Remove decorative eyebrow labels where they add no task information. Do not replace the Studio canvas or invent stats/thumbnails/testimonials.

Acceptance review sizes: 390×844, 768×1024 and 1440×900, plus 320px and 200% zoom overflow checks. Screen-reader/keyboard review covers route/section navigation, focus return, player controls, Shorts alternatives, dialogs and status announcements. Captions controls remain available; absence is explicit, not silently faked.

Browser verification requires real local media fixtures (labelled, rights-cleared) for public/free, Premium, unlisted, private, processing and blocked; portrait/landscape; clips shorter than 10s; no-audio and captioned clips. Test S3/grant expiry and >5-minute paused resume. Local fixtures prove local behaviour; deployed CDN/encoder/load verification remains a separate launch gate.

Impeccable implementation budget: build complete task flows, inspect desktop/mobile together once, fix in one batch, confirm once. Run detector once at finish on changed UI and use the skill's independent finish-review/documentation handoffs. This planning audit is not that implementation approval.

## 12. Build order and completion gates

1. Settings shell + mobile spacing + truthful route-specific states; no media dependency.
2. Output detail + unified publish/download for generated sources + explicit access state; preserve existing accounting.
3. Measured media/source model + standard watch data/related/progress + content manager.
4. Shorts player/feed and duration-aware public metrics, after player lifecycle tests and policy review.
5. Uploaded-source pipeline, using the same publication path; quota/moderation/probe gates first.
6. Likes/saves/library and richer recommendations; comments only after moderation/deletion/abuse design.
7. Production media, email, physical passkey and performance tests when environment is authorised; no automatic AWS deployment in this plan.

A phase is complete only when its UI works with its actual API, ownership/privacy tests pass, meaningful empty/error states work, local media behaviour is verified where relevant, and both extracted repositories' CI passes. No static button or mock-only screen counts as a completed system.

## Primary implementation references

- Browser autoplay policy and recoverable muted playback: https://developer.mozilla.org/en-US/docs/Web/Media/Guides/Autoplay
- Accessible captions/transcripts/player planning: https://www.w3.org/WAI/media/av/
- Multipart upload lifecycle and abort obligations: https://docs.aws.amazon.com/AmazonS3/latest/userguide/mpuoverview.html
- Checksums and object integrity: https://docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity-upload.html
