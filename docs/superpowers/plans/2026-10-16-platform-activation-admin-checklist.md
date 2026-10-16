# Platform activation, experience and admin checklist

Date: 2026-10-16. This is a scope and acceptance checklist, not a claim that these items have been implemented or a completed live UI audit. All unchecked items require implementation or verification. No infrastructure, paid model calls, payment activation or new repository creation occurs in this planning task.

## Decisions and boundaries

- Customer web app stays in `videoflow-web`; business logic, workers, payments and authoritative data stay in `videoflow-api`; create a separate repository named `admin` for the operator web application.
- Admin uses authenticated, role-scoped API endpoints; no browser database credentials or provider secrets. Reuse the API rather than duplicating business logic in admin.
- Support Stripe and CryptoRails as customer payment choices. Platform Premium covers eligible content across creators; channel follows are free and independent. Credits fund generation and are separate from Premium.
- All AI inference must go through OpenRouter, including story, image/character generation, video, transcription and other AI-assisted features. Unsupported tasks remain disabled with truthful UI; no silent direct-provider fallback. Deterministic encoding, editing, caption formatting and streaming are not AI inference. Models are platform-selected. Choose the cheapest eligible model for each task after capability, quality, reliability and commercial-use checks; a cheap text model is not a video generator.
- Keep AWS rollout deferred. Local/sandbox verification precedes production activation. Global product intent does not imply either provider supports every operating country, payment method or payout destination.
- PostgreSQL owns money, subscriptions, content, configuration and audit facts. Redis provides disposable acceleration and abuse controls.

## Existing baseline to verify, not rebuild blindly

The current repository contains application-owned authentication/MFA/passkeys, profile URL fields and links, follows, Premium entitlements, CryptoRails integration, historical Stripe processing, generation quotes/jobs, bilingual prompt templates, a canvas, browser editor, publishing and basic admin operations. Local tests and previous browser checks do not establish live provider readiness. Relevant files: `internal/cloud/config.go`, `profile.go`, `admin.go`, `crypto_admin.go`; docs: `BILINGUAL-PROMPTS.md`, `PRODUCT-EXPERIENCE-GAPS.md`, `EDITOR-OPERATIONS.md`, `CRYPTORAILS.md`, `ops/LEGACY-STRIPE.md`.

## P0 — AI service activation

- [x] Inventory the actual story, character/image, video, speech and caption adapters; label each working, stubbed, unsupported or disabled with code/test evidence.
- [ ] Fetch current provider capabilities and pricing; calculate cost per completed task including tokens, duration, resolution, audio, retries and failed work.
- [ ] Define separate approved task policies for story, image and video; do not advertise reference-image conditioning until the adapter really submits it.
- [ ] Evaluate shortlisted models against English/Mandarin prompts, character continuity, structured output and safety requirements; record the selected model and date.
- [ ] Configure backend-only OpenRouter credentials and approved model IDs; existing configuration includes `OPENROUTER_API_KEY`, `OPENROUTER_STORY_MODEL`, `OPENROUTER_VIDEO_MODEL`.
- [ ] Verify whether the chosen video/image models are exposed through the implemented protocol; add a supported adapter where necessary instead of treating chat completions as a video endpoint.
- [ ] Fund the provider account and run a deliberately capped real smoke test in staging when ready for activation.
- [ ] Version routing and pricing policies; freeze the relevant policy in each quote/job so admin changes do not alter confirmed purchases.
- [ ] Calculate customer credit prices with margin covering generation, payment fees, storage/delivery and operating costs; show the final charge before confirmation.
- [ ] Reserve credits once; settle actual approved usage and release unused reservations; explicitly reconcile ambiguous provider submissions before retrying.
- [ ] Add bounded retries, timeouts, cancellation rules, queue limits, per-user concurrency, global daily spend caps and an emergency generation stop.
- [ ] Persist provider request IDs and cost facts; redact credentials and sensitive prompt content from routine logs.
- [ ] Test success, rejection, timeout, duplicate requests, worker crash/restart, provider outage and insufficient credits with no double charges.
- [ ] Present useful unavailable/failed states, retained prompts, retry eligibility and accurate charge/refund information.

Acceptance: one English and one Mandarin project complete through real configured providers within an explicit spend cap; job and credit records reconcile exactly. No model selector or API key appears in customer UI.

## P0 — Stripe plus CryptoRails

- [x] Reconcile current CryptoRails-first code/docs and historical Stripe routes; list which new checkout paths need restoring rather than merely changing a feature flag.
- [x] Offer clearly labeled Card (Stripe) and Crypto (CryptoRails) choices for supported products.
- [ ] Define a shared product catalogue: credit packs, Premium duration/renewal terms, currency, taxes, expiry and refund policy; preserve provider-specific behavior.
- [x] Distinguish Stripe automatic recurring Premium from CryptoRails prepaid access unless recurring crypto collection is actually supported and verified.
- [x] Prevent overlapping/duplicate Premium purchases across providers; show the current provider and correct management action.
- [ ] Store canonical orders and provider-specific attempts, amounts, currencies, identifiers and status transitions; use stable idempotency keys.
- [ ] Restore/configure Stripe credit checkout, Premium Checkout and owned billing portal; use sandbox keys first.
- [ ] Verify signed Stripe webhooks and CryptoRails authentication; credit/entitle from verified server-side settlement only, never redirects.
- [ ] Test duplicate, reordered and delayed events; concurrent checkouts; abandoned sessions; pending, failed and expired crypto payments; under/overpayments where applicable.
- [ ] Reconcile cancellation, renewal, payment failures, refunds, partial refunds, disputes and already-spent credits without minting money or reviving revoked access.
- [ ] Provide pending-payment recovery, order history, receipts, failed-payment messages and an explicit refresh/reconcile action.
- [ ] Separate customer payments from creator payouts; do not imply collecting cards/crypto automatically enables global payouts.
- [ ] Verify the actual business entity/country, supported customer countries, currencies and creator payout coverage before live enablement.
- [x] Update contradictory legacy Stripe documentation and environment/feature flags; retain historical reconciliation.

Acceptance: sandbox purchases through both gateways grant the expected credits/access once; replay, refund and dispute tests reconcile; customers can understand renewal terms and manage their purchase.

## P1 — Avatar, banner and social links

- [ ] Replace URL-first profile editing with click-to-upload and drag/drop; retain an optional URL entry only where useful.
- [ ] Add avatar circle crop, zoom/reposition and mobile camera/library selection; banner crop with visible desktop/mobile safe area.
- [ ] Show immediate preview, upload progress, retry, replace, remove and unsaved-change states.
- [ ] Validate ownership, content type, decoded dimensions and file size on the server; strip metadata and encode safe image variants.
- [ ] Publish optimized avatar/banner variants; clean abandoned/replaced uploads without deleting shared or current assets.
- [ ] Add preset YouTube, TikTok, Instagram, X, Facebook, LinkedIn, Twitch, Discord, Telegram and Website options with consistent recognizable brand icons.
- [ ] Support handle-to-URL formatting where unambiguous, full URL validation, duplicates, ordering and accessible icon labels; block misleading service/domain combinations.
- [ ] Reuse the same avatar, banner, name and social-link presentation in Settings, public profile, comments/creator attribution and admin preview.

Acceptance: a mobile user updates both images and adds/reorders a social link without manually hosting an image or guessing crop dimensions.

## P1 — Create → edit → publish → watch

- [ ] Walk a new user through sign-up, verification, credits, first project and first generation; remove dead ends and unexplained terms.
- [ ] Keep entry choices clear: Generate with AI, Upload a video, Continue a project; templates are optional assistance.
- [ ] Make the creation stages visible: idea/script → cast/scenes → generate → edit → publish; allow returning without losing work.
- [ ] Present character and asset libraries with image/initial fallback, name, type, generation status, search/filter and clear selection.
- [ ] Separate reusable character identity from individual generated variants; support inspect, rename, reuse, retry and safe deletion with usage warnings.
- [ ] Audit per-node generation actions and loading/failed states; verify each visible action really works.
- [ ] Preserve English/Mandarin templates, editable prompts, saved defaults and draft/conflict recovery.
- [ ] Show quote, required credits, progress and failure recovery at the correct stage without duplicate control panels.
- [ ] Verify generated outputs import into the editor, with honest local-media persistence/relink messaging.
- [ ] Implement a direct editor export-to-publication handoff with progress/retry; retain Download as a first-class option. Current export/download/upload steps must not be labeled automatic direct publishing.
- [ ] Route generated exports and external uploads into one publication composer: title, description, cover, format, audience, visibility, captions and preview.
- [ ] Show upload/processing/moderation/published states; prevent publishing an unplayable or still-processing asset.
- [ ] Explain public/unlisted/private and Premium audience separately; show a viewer-facing preview and a final watch/share link.
- [ ] Test permission-denied, expired session, network loss, insufficient credits, provider failure, missing editor media, unsupported upload and failed encoding recovery.
- [ ] Verify published playback, Shorts discovery, profile inclusion, original download ownership and Premium denial with a second account.

Acceptance: first-time creator completes both an AI-generated and uploaded-video journey; a separate viewer can watch permitted content and cannot fetch protected media directly.

## P1 — UI consistency and navigation audit

- [ ] Inventory all screens, routes, sidebars, top bars, toolbars, modals, menus and notification patterns; capture desktop/mobile evidence before proposing removals.
- [ ] Keep viewer navigation focused on Discover, Shorts, Following and Library; group creator tasks inside Studio with one contextual navigation hierarchy.
- [ ] Remove duplicate sidebar/top-bar destinations and duplicate action icons; retain clear project context, active state and Back navigation.
- [ ] Define shared tokens for brand orange, backgrounds, borders, text, semantic statuses, typography, spacing, radii and focus rings.
- [ ] Check dark/light theme contrast, white-panel regressions, selected/disabled/loading states and canvas/editor token compatibility.
- [ ] Use one icon family and consistent sizes/labels for product actions; social brand marks are deliberate exceptions.
- [ ] Replace browser alert/confirm/prompt and inconsistent custom overlays with shared shadcn-style Dialog, AlertDialog, DropdownMenu, Select, Popover, Tooltip and toast components appropriate to the existing Svelte stack.
- [ ] Use inline validation for fixable inputs, toasts for transient outcomes, persistent banners for ongoing outages, and confirmation dialogs for destructive actions.
- [ ] Verify focus trap/return, Escape, keyboard navigation, screen-reader names, reduced motion and touch targets.
- [ ] Standardize table pagination/filtering, skeletons, empty states, errors, destructive menus and unsaved-change protection.
- [ ] Review canvas density, character/asset variants, generation history and editor tool panels at narrow laptop and mobile sizes.
- [ ] Audit placement by user task: creative controls in Studio; account/billing in Settings; content consumption in viewer routes; operator controls exclusively in admin.
- [ ] Record every finding with route, screenshot, severity, expected behavior, owner and retest evidence. This checklist alone does not mark a live audit complete.

## P1 — Follows, Premium and viewing analytics

- [ ] Establish consistent terminology: Follow creator (free), Premium (platform paid access), generation credits (usage balance); eliminate misleading channel-paid subscription labels.
- [ ] Show follow/unfollow state, follower counts, Following feed and notification preferences consistently across watch, Shorts and profiles.
- [ ] Test Premium checkout, renewal, cancellation, expiry, refund and cross-creator access across both payment gateways.
- [ ] Define versioned metric rules for playback start, qualified view, unique viewer, watch seconds, completion, retention, replay and Shorts swipe.
- [ ] Collect playback-session events with timestamps, event IDs, deduplication, heartbeat bounds and consent/privacy preferences; avoid trusting client totals.
- [ ] Exclude self-views, bots and suspicious repeated/background playback from reward-eligible metrics; distinguish raw, qualified and monetizable views.
- [ ] Provide creator overview and per-video reports: date range, views, watch time, retention, traffic sources, follower growth and Premium engagement with privacy thresholds.
- [ ] Show data freshness and timezone; reconcile aggregates after delayed events and ingestion outages.
- [ ] Define the capped revenue-share pool using reconciled eligible net revenue, reserve rules and qualified engagement weights; version formulas and prevent exceeding the pool.
- [ ] Present estimated earnings separately from finalized payable balances; enforce maturity, fraud holds and payout eligibility.
- [ ] Add recommendation controls for recent/popular/following/interest signals with diversity and moderation filters; never bypass privacy/Premium checks.

## P0/P1 — Separate `admin` repository

- [x] Create `gen-ai-videoflow/admin` using the current UI stack, shared visual tokens and generated API contracts, with independent CI/build/deployment configuration.
- [ ] Add a dedicated admin origin and explicit session/origin policies; no reliance on a hidden customer route for security.
- [ ] Require individual operator accounts, MFA/step-up authentication, session expiry/revocation and deny-by-default server authorization.
- [ ] Define roles: support, moderation, finance, operations and super-admin; scope permissions for reading private content, changing configuration, money adjustments and payouts.
- [ ] Dashboard: users/active users, public videos, generation queue/failures, views/watch time, revenue/refunds, provider spend, margin, storage/delivery cost and service health; label estimates/freshness.
- [ ] Users: search/filter, profile, verification, status, sessions, risk flags, restrictions, related content/orders/jobs and recorded support actions.
- [ ] Content: paginated videos/Shorts, private projects under explicit permission, uploads, covers, captions, audiences, ownership, processing and moderation status; audited restricted preview.
- [ ] Moderation: reports, reasons/evidence, queues, block/unblock, appeals, age/content flags and takedown lifecycle; preserve financial/audit evidence.
- [ ] AI operations: job detail/timeline, provider request IDs, queue health, costs, safe retry/cancel and ambiguous-job reconciliation; no blind retry of possibly billed jobs.
- [ ] Models: capability allowlists, approved model IDs, cost ceilings, routing/fallback policy, health, concurrency and kill switches.
- [ ] Prompts/templates: English/Mandarin versions, preview/validation, publish/retire, revision history and rollback; distinguish platform defaults from user-owned templates.
- [ ] Billing: both gateways, orders, invoices, pending settlements, webhook receipts/retries, reconciliation, refunds/disputes and immutable credit ledger.
- [ ] Premium: product terms/prices, provider subscriptions, paid-through access and cancellation state; no silent repricing of existing contracts.
- [ ] Creator finance: eligible revenue inputs, pool period/formula, fraud holds, estimated/finalized earnings, payout batches and reconciliation; approval gates for transfers.
- [ ] Analytics: content/view/fraud reports, metric definitions, retention/export controls and aggregate rebuild status.
- [ ] Configuration: typed allowlisted settings with units, bounds, defaults, environment scope, effective date and impact preview; reject arbitrary code/SQL or arbitrary environment editing.
- [ ] Cover credit pack/pricing, Premium pricing, margin, quotas, file limits, upload switches, feature flags, recommendation weights, moderation thresholds, maintenance and signup controls.
- [ ] Store secret references in settings; provider keys remain in a secret store and are masked/write-only. Bootstrap database/identity/network settings remain deployment-managed.
- [ ] Require reasons and before/after audit facts for changes; add optimistic concurrency, rollback/versioning and extra approval for high-impact financial/security changes.
- [ ] Support user content export/deletion requests, retention policies and legal holds; avoid automatic destruction of billing evidence.
- [ ] Add CSV exports with access controls, bounded queries, pagination and spreadsheet-formula escaping; prohibit unbounded customer-data dumps.
- [ ] Test every role directly against backend endpoints, including cross-user content access and denied financial actions; hiding controls is insufficient.

Acceptance: each operator can do their own job and cannot access another role's restricted actions; every sensitive action has an attributable audit record. The admin browser receives no provider secret or direct database access.

## Data, Redis and service boundaries

- [ ] PostgreSQL: accounts, follows, projects, media metadata, prompt revisions, jobs, quotes, credit ledger, orders, subscriptions, payouts, config revisions and operator audit.
- [ ] Object storage/delivery service: private originals, editor exports, profile images, covers, caption files and protected playback renditions; persist durable references in PostgreSQL.
- [ ] Redis: rate limits, short-lived playback presence/dedup hints, cached feeds/profile counts, configuration caches and disposable progress; define TTL and invalidation for each.
- [ ] Use durable event IDs/uniqueness for billable or payout-relevant events; Redis eviction or restart must not double-charge or inflate finalized earnings.
- [ ] Keep the durable existing job queue/database authoritative; use Redis locks only as optimization with durable lease/idempotency checks.
- [ ] Document outage behavior: safe payment/credit decisions fail closed, public cache misses rebuild, creator drafts remain recoverable and analytics ingestion is replayable.

## Delivery order and release gates

1. Baseline inventory and journey/UI audit; capture evidence and finalize provider/payment contracts.
2. AI adapters, quote/accounting rules and sandbox dual-gateway purchases.
3. Profile uploads/social presets and shared UI/navigation cleanup.
4. Direct editor-to-publishing journey and media-provider end-to-end validation.
5. Follow/Premium clarity, reliable analytics and fraud-aware revenue pool.
6. Admin foundation early; ship each management section alongside its underlying API subsystem.
7. Security/financial review, accessibility/responsive QA, recovery/load tests and operator documentation.
8. Controlled live activation with credentials, bounded spending and verified external accounts; AWS rollout remains a separate deferred action.

- [ ] Maintain traceability from each requirement to changed files, automated tests and browser evidence in its subsystem implementation plan.
- [ ] Run Go tests/race checks, frontend checks, editor checks, contract generation, production builds and both gateway replay suites after relevant changes.
- [ ] Verify migrations, backup/restore, restricted DB grants, rollback and environment-specific feature flags.
- [ ] Test desktop/mobile and English/Mandarin creation; test owner/viewer/anonymous/Premium/operator roles.
- [ ] Measure representative upload/export/playback performance and check the initial US$500–2,000 monthly operating budget with actual usage assumptions.
- [ ] Publish release notes and explicit known limits; do not equate green CI with production provider verification.

## References and research gaps

- OpenRouter integration and model catalogue: https://openrouter.ai/docs/quickstart . Current model/capability/price selection must be refreshed during implementation.
- Stripe webhooks: https://docs.stripe.com/webhooks . Signature verification, duplicate delivery and event-order handling are mandatory.
- CryptoRails: https://docs.cryptorails.tech/llms-full.txt . Fetch failed in this planning pass; re-fetch and compare against the existing integration before specifying new recurring-payment capabilities.


## P0/P1 addition — Formal authentication and account setup

- [ ] Audit sign-up, sign-in, verification, recovery, MFA and passkey flows as one account journey with shared layouts, concise copy and preserved return destination.
- [x] Add a Security overview showing password/sign-in methods, passkeys, authenticator status, recovery readiness and active sessions; explain recommended next steps without making all users complete every option.
- [ ] Design passkey enrollment as introduction → native device prompt → successful registration → name device → recovery guidance. Handle dismissal, unsupported devices, duplicate credentials and expired challenges without losing context.
- [ ] Provide a prominent Use a passkey sign-in action and supported conditional/autofill experience; test platform authenticators and cross-device QR flow in real compatible browsers.
- [ ] List passkeys with name, creation/last-use details where reliably recorded, rename and remove; require recent authentication and prevent removal of the last usable sign-in method.
- [ ] Design authenticator setup as scan QR or copy key → enter first code → confirm activation → save recovery codes → completion; never label MFA enabled before proof succeeds.
- [x] Make recovery codes readable, downloadable/copyable and explicitly one-time; require recent authentication for replacement and invalidate previous codes.
- [ ] Provide sign-in fallback between passkey, password and authenticator/recovery challenge where enrolled; explain lost-device recovery without weakening identity checks.
- [ ] Add Google and Apple sign-in as initial social options after provider configuration; offer email magic-link sign-in as a separately tested option rather than exposing unconfigured buttons.
- [x] Use authorization-code/OIDC flows with state, nonce and PKCE where applicable, strict redirect allowlists, secure cookies and enumeration-resistant messages.
- [ ] Link/unlink providers only with verified ownership and recent authentication; never auto-merge accounts solely because an untrusted provider reports the same email.
- [ ] Handle private relay emails, existing-account collisions, canceled social sign-in and changed provider email addresses; require a usable fallback before unlinking.
- [ ] Test first-time viewer and creator onboarding separately, accessible keyboard completion and return to the intended video or project after sign-in.

## P1 addition — Caption languages and selection controls

- [ ] Inventory language, audience, format, visibility, generation option and preference selectors; choose controls based on one choice versus multiple choices, not styling alone.
- [ ] Use RadioGroup for small mutually exclusive sets such as visibility and audience; Checkbox for independent choices; Switch for immediate binary settings; searchable Select/Combobox for long language lists.
- [ ] Use current-stack shadcn-svelte components and their supported accessible primitives; do not import React-only Radix components into Svelte pages. Match the embedded React editor visually using its own compatible primitives.
- [ ] Distinguish spoken/source language, automatic detection, transcription language, translation target, caption track language and interface/prompt language.
- [ ] Use stable language tags and readable native/English labels; distinguish Simplified and Traditional Chinese where supported. Do not claim universal language support from a selector.
- [ ] Show only languages supported by the selected platform capability; explain disabled/unavailable combinations and whether translation spends credits.
- [ ] Support multiple uploaded/generated caption tracks, default track selection, language editing, preview and removal; validate timestamps, encoding, format and media ownership.
- [ ] Persist caption preference consistently across Watch and Shorts; support off/auto and sensible fallback when a preferred track is absent.
- [ ] Test caption layout with Mandarin, long text and RTL languages where supported; verify keyboard/screen-reader labels, captions in fullscreen and mobile playback.

## P0 addition — Request amplification and backend capacity audit

### Static inspection on 2026-10-16

These are code observations, not load-test results or a conclusion that the service is fully optimized:

- `frontend/src/lib/cloud/api.ts`: shared request helper performs one fetch; no recursive retry in this helper. It currently has no explicit client request deadline or AbortSignal parameter.
- `GenerationJobs.svelte`: active jobs trigger a 10-second interval; its refresh function has no local in-flight guard. Slow refreshes can overlap at this call site; the parent callback still needs tracing before measuring actual duplicate traffic.
- Payments poll at 10 seconds and uploads at 5 seconds with busy checks; verify those checks cover all callers and visibility/unmount behavior.
- `VideoPlayer.svelte`: local accounting ticks every second, heartbeat scheduling every 15 seconds, and grant refresh has an in-flight flag. One-second timer frequency is not itself one request per second; verify failure and hidden-tab paths with network evidence.
- `internal/cloud/store.go`: PostgreSQL pool is bounded to 20 open/5 idle connections per process, with a 30-minute maximum connection lifetime. Replica/worker totals and database capacity are not yet validated together.
- `internal/cloud/server.go`: atomic Redis counter/expiry rate limiting exists; selected strict limits fail closed outside development if Redis is unavailable. Auth, quote, generation and mutation routes have limits; this does not establish read-route/global capacity protection.
- NewServer silently ignores a malformed Redis URL; `/health` checks PostgreSQL, not Redis. Production dependency readiness/config validation needs explicit review.
- `cmd/cloud/main.go`: HTTP header/read/idle timeouts exist; no WriteTimeout is set in the inspected cloud server literal. Long-running/streaming endpoints require deliberate per-route deadline policy rather than blindly adding a short global timeout.
- `internal/providers/openrouter.go`: structured-output correction loop is bounded to two attempts; this is not unbounded recursion.
- `internal/cloud/worker.go`: queue consumption uses long polling and outbox batches are bounded; dispatch performs network sends while its database transaction is open, which needs contention/latency measurement.

### Required implementation and evidence

- [ ] Map frontend effects/timers, API handlers, worker retries, provider polling and queue redelivery into one call graph; identify cycles and retry multiplication across layers.
- [ ] Capture per-journey request baselines for idle browsing, Shorts scrolling, active generation, editor autosave and pending payment; compare idle, active and background tabs.
- [ ] Add regression tests for slow requests, repeated clicks, component remounts, multi-tab use and network flapping; prove maximum concurrent requests and bounded retry counts.
- [ ] Replace overlapping interval requests with one in-flight request and completion-scheduled polling; stop on terminal state, pause/reduce when hidden, and cancel on unmount/navigation.
- [ ] Add request cancellation/deadlines, stale-response protection, GET deduplication and selective caching; never deduplicate different users or entitlements into shared private responses.
- [ ] Use capped exponential backoff with jitter and Retry-After for safe transient retries; do not retry validation/auth failures or replay mutations without durable idempotency.
- [ ] Ensure a single layer owns retries for each operation; cap total attempts/time/cost across HTTP client, worker, provider SDK and queue redelivery.
- [ ] Add route-appropriate read/write/user/IP limits, bounded expensive-operation concurrency, bounded queues and explicit overload responses; protect public search/feed and playback authorization as well as mutations.
- [ ] Bound fallback limiter memory under many unique keys; test trusted proxy identity and multi-replica limiter behavior.
- [ ] Fail startup on invalid required Redis configuration; configure connection pools, short operation deadlines, TLS/auth, key namespaces, TTLs, eviction policy and dependency readiness separately from liveness.
- [ ] Test Redis outage/slowdown/restart: strict security and money gates remain safe, disposable caches rebuild, and retries do not amplify the outage.
- [ ] Budget PostgreSQL pool totals across all processes/replicas; configure statement/lock deadlines, measure pool wait, inspect slow queries and EXPLAIN plans, and fix N+1 queries/pagination/index gaps.
- [ ] Measure transaction duration around provider/network calls; move network work outside locks where durable outbox/idempotency semantics can be preserved.
- [ ] Validate worker leases, visibility renewal, dead-letter handling, attempt budgets and poison-message behavior without duplicate provider charges.
- [ ] Add metrics for route request/error/latency, 429/503, active requests, DB waits, Redis latency, queue age, provider attempts, polling rate and cost per successful generation.
- [ ] Run staged baseline, burst, sustained, slow-provider and dependency-outage load tests; record concurrency, p50/p95/p99, CPU/memory, DB/Redis usage, errors and recovery before claiming capacity.
- [ ] Expose bounded operational controls and health reports in admin; keep permission/security invariants out of arbitrary runtime toggles.

Acceptance: no unbounded recursive retries; one poll per resource per active view in flight; bounded end-to-end attempts and queues; stable memory/pool use under the agreed measured load; overload returns controlled responses and recovers without duplicate financial/provider actions. Numeric service targets must be set from the baseline and declared deployment size before optimization sign-off.

### Payment implementation evidence — 2026-10-16

- Explicit Card/Stripe and Crypto/CryptoRails selection is implemented for credit packs and Premium; independent availability flags prevent advertising an unconfigured gateway. Premium explains monthly recurring versus 30-day prepaid terms.
- New card routes reuse durable purchase records, provider idempotency, signed webhook accounting and owned billing management. No live transactions were sent.
- Both Premium creation paths share the same PostgreSQL user advisory lock. Existing crypto access/unresolved invoices block recurring purchase; crypto reconciles outstanding Stripe sessions first. See `internal/cloud/dual_payment_test.go` and `docs/ops/LEGACY-STRIPE.md`.
- Unified canonical orders/attempt history, tax policy, sandbox delivery and live business/provider activation remain unchecked. Existing provider ledgers are preserved rather than pretending a new canonical-order migration exists.

### AI inventory and polling evidence — 2026-10-16

- `GET /admin/ai-readiness` reports the real cloud SaaS adapters, missing configuration names and supported task selection without exposing credentials, queue URLs or bucket values. Tests verify secrets do not serialize and unsupported image/speech/caption/reference-video adapters remain unavailable. Configuration is explicitly not live provider verification. See `docs/AI-ACTIVATION.md`.
- Payments now uses the shared completion-scheduled poller with abort on unmount, hidden-tab pause, bounded failure backoff and manual/automatic request exclusion. Provider selection, invoice recovery, and shared request lifecycle assertions pass; frontend type check reports zero errors/warnings.
- Payment/provider race integration suite passes locally against PostgreSQL with mocked transports. Live Stripe sandbox webhook delivery and funded CryptoRails settlement remain unchecked.

### Story routing and combined billing history — 2026-10-16

- Cloud story quotes now compare operator-approved OpenRouter text models using the current cached catalogue, conservative token/context limits, understood rational pricing and the worker's enforced provider price ceiling. The selected actual model is frozen in the pipeline plan; later worker execution must still find it in the approved allowlist. Unknown/surcharge pricing and zero budgets fail closed. Tests cover English/Mandarin inputs and cached catalogue reuse.
- Payments now has combined owned Stripe credits, CryptoRails invoices and Stripe Premium payment history with method/product shadcn filters, stable keyset pagination and explicit recovery links. No hosted receipt URL is invented; unsupported receipts report false. Integration tests verify owner isolation and mixed-provider pagination. A new canonical-order write model remains separate from this normalized history.


## Implementation release evidence — 2026-10-16

This release implements the major backend safeguards, dual payment paths, authentication/profile improvements and admin foundation. It does **not** sign off every mixed requirement above. Unchecked rows include partially implemented work as well as future work; detailed boundaries follow.

Delivered: request admission/deadlines; bounded Redis retries/pools/fallback limiter; proxy loop/body/deadline safeguards; single-flight polling; guided TOTP/passkeys and magic links; configured Google/Apple OIDC with safe linking; avatar/banner framing and owned upload lifecycle; eleven social presets with domain validation/reordering; single contextual Settings navigation; shared internal unsaved dialogs; editor export-to-upload handoff with download retained; dual Stripe/CryptoRails purchase paths and normalized history; cheapest eligible approved OpenRouter story/video policies; separate private admin repository with roles, recent MFA, moderation, suspension, audited feature switches and bounded operational views.

Evidence: fresh-database cloud race suite passed; all Go packages passed; targeted OAuth/JWKS, payment concurrency, profile image lifecycle/cleanup race and capacity tests passed. Frontend/editor checks, nine behavioral scripts and production builds passed. Independent review identified a stalled gateway body retaining admission; a never-ending-stream regression now proves cancellation/deadline and permit release. Desktop/mobile browser checks verified profile framing and internal draft guard; admin review verified real TOTP enrollment, a setting revision and its audit record.

External activation gates: real Google/Apple configuration and browser consent, delivered email/physical passkeys, actual OpenRouter capability/pricing smoke tests under a spend cap, Stripe/CryptoRails tenant sandbox/live reconciliation, deployed media encoding/delivery and production load measurements. AWS remains deferred.

Further implementation: complete lifetime/paginated admin search/export and granular operator roles, runtime versioned model/pricing editing, full net revenue/provider-cost/payout reconciliation UI, deeper privacy-preserving viewing/retention/fraud analytics and recommendation controls, additional supported OpenRouter image/speech/caption adapters, durable email outbox, scheduled profile-image cleanup, exhaustive screen/dialog/accessibility audit and declared deployment capacity sign-off. These are not represented as complete by green CI. New media handoff requires configured video uploads; unsupported AI tasks remain explicitly unavailable.

### Versioned runtime controls — 2026-10-16

- Owner-only versioned/audited runtime policy and admin settings UI now control approved model allowlists, generation credit prices, provider spending ceilings, new Premium price and active-generation quota. The API/worker consume effective snapshots; secrets/activation/infrastructure remain deployment managed.
- Fresh catalogue eligibility, bounded numeric inputs and a fixed-pack minimum provider-cost buffer reject unsafe writes. Revision conflicts fail rather than overwrite concurrent edits. Catalogue refresh submits no inference.
- Tests verify unchanged stored quote credits and Premium access, new quote prices, same-key quota recovery, stale revisions/audit, unknown models and frozen pending checkout prices. See `docs/RUNTIME-POLICY.md`.
