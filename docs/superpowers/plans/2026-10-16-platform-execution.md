# Platform hardening and operator experience implementation plan

> Implement the approved 148-item checklist in subsystem phases. Keep an evidence ledger here; unchecked requirements remain open until their acceptance evidence exists.

**Spec:** `2026-10-16-platform-activation-admin-checklist.md` in this directory.
**Architecture:** Keep durable state and authorization in the Go API. Customer and admin Svelte apps consume protected APIs. All AI inference routes through backend OpenRouter adapters. Production switches remain off during local/sandbox verification.
**Workspace:** Existing isolated feature branch `feat/cloud-saas`; preserve the approved original canvas and browser editor.

## Task 1 — Bounded frontend requests and polling

Files: `frontend/src/lib/cloud/api.ts`, new `request.mjs`/`.d.mts`, new `poll.mjs`/`.d.mts`, `GenerationJobs.svelte`, behavioral tests in `frontend/scripts/check-request-lifecycle.mjs`.
- [ ] Test one in-flight refresh under slow responses, completion scheduling, stop cancellation, failure backoff and hidden-tab pause.
- [ ] Implement reusable single-flight polling and deadline/cancellation support without automatic mutation retries.
- [ ] Integrate generation activity and targeted other pollers without duplicate data ownership.
- [ ] Run behavioral tests and Svelte checks.

## Task 2 — Dependency and overload safeguards

Files: `internal/cloud/config.go`, `server.go`, new `capacity.go`/`capacity_test.go`; existing config/server tests.
- [ ] Reject malformed Redis configuration; distinguish readiness from liveness with bounded dependency checks.
- [ ] Add bounded request admission, overload Retry-After, Redis operation deadlines and bounded fallback-limiter memory.
- [ ] Verify overload permits release after errors/panics, outage behavior, limits and unchanged financial idempotency.
- [ ] Record measured limitations; load claims require a declared deployment and load test, not source inspection.

## Task 3 — Dual gateway purchasing

Files: existing Stripe/CryptoRails payment handlers, Premium and customer payment views; payment docs/tests.
- [ ] Restore explicit new Stripe checkout capability alongside CryptoRails with both disabled by default.
- [ ] Test duplicate/overlapping purchases across gateways, webhook replay and refund/entitlement handling.
- [ ] Present provider-specific renewal and management truthfully.

## Task 4 — Authentication, profile and language controls

Files: customer security/profile/settings components and new owned profile upload module where supported.
- [ ] Implement guided passkey/TOTP states and recovery with existing server validation.
- [ ] Add real profile upload/crop flow and preset recognizable social links.
- [ ] Use accessible single/multi-selection controls; distinguish caption languages from UI/prompt language.
- [ ] Provider OAuth/magic-link additions require complete protocol, account-linking and recovery tests; never show unconfigured provider buttons.

## Task 5 — Admin application and protected management APIs

Files: separate local repo `../videoflow-saas/admin`, new `internal/cloud/admin_console*.go`, migrations031+; admin operations docs.
- [ ] Build a real dashboard and API-backed management of existing resources.
- [ ] Enforce server permissions, audited typed allowlisted configuration, no browser secrets or arbitrary DB editing.
- [ ] Test access denial, configuration validation/conflicts and existing operation integrations.

## Task 6 — Creation, publishing, analytics and UI audit

- [ ] Verify actual create→generate→edit→publish/upload→watch flows with separate viewer identity.
- [ ] Implement direct export-to-upload handoff using validated existing upload protocol; preserve local download.
- [ ] Audit duplicate navigation/icons, dialogs, status language, responsive layout and color consistency with browser evidence.
- [ ] Measure view ingestion/recommendation/earnings behavior before changing financial metrics; preserve existing capped pool/accounting invariants.

## Task 7 — AI activation and release controls

- [ ] Inventory every AI call and enforce OpenRouter-only inference.
- [ ] Publish real capability/configuration readiness and operational cost limits without secret exposure.
- [ ] Run provider-independent tests; only run paid live provider verification after explicit bounded activation configuration exists.
- [ ] Run relevant Go/frontend/editor/contracts/build checks, record external gates, and synchronize the split repositories after verified changes.

## Review focus

Slow requests must not overlap polls. Redis loss must not disable security gates silently. Retries must not duplicate paid actions. Admin configuration must not bypass financial constraints. Profile/media uploads must not leak another user's originals. Each relevant subsystem owns these regression checks.

## Execution ledger

- Planning: user authorized implementation of the approved checklist; no repeated design approval required. Parallel implementation is used for independent auth/profile, dual-payment and admin slices; root owns request/backend capacity safeguards and integration.
- External gates: provider credentials, actual business/payment country eligibility, production domains and paid model smoke tests remain environmental prerequisites; implementation must show unavailable states rather than fake success.

- Task1 implemented: deadline/cancellation helper, completion-scheduled polls, loop-safe bounded Node gateway and behavioral regression scripts. Root found and restored four viewing routes accidentally removed during route extraction; existing viewing test caught the regression and now passes.
- Task2 implemented: default128 API permits, bounded request contexts, Redis configuration/deadlines/pool/no hidden retries, bounded local limiter, liveness/readiness and protected process telemetry. Synthetic200-request burst atcapacity8 admittedmax8/rejected192/releasedall. Production sizing remains unmeasured.
- Task3 implemented: both gateway choices, cross-gateway Premium serialization, durable replay safeguards and normalized owned paginated history; no live settlement performed.
- Task4 implemented: guided security, magic/OIDC, image framing/upload ownership lifecycle, social presets/domain checks, clear language controls and shared internal draft dialogs. Provider/device activation and full caption adapters remain gates.
- Task5 implemented foundation and published private admin repo; latest100 bounded readviews and reviewed safe switches. Runtime money/model editing and lifetime analytics intentionally remain open until their versioning/reconciliation controls are implemented.
- Task6 implemented direct export/upload handoff and settings sidebar cleanup; existing all-package viewing/publication tests pass. Complete live media journeys/load tests and exhaustive UI coverage remain open.
- Task7 implemented OpenRouter-only inference policy (local Whisper removed), supported-task inventory and cheapest approved story selection; unsupported image/speech/caption cloud tasks are clearly disabled. No live provider verification or AWS rollout claimed.
- Independent review: gateway stalled-body permit leak reproduced, regression wentred, fixed so body cancellation/deadline releasespermit; no other critical findings in targeted review. Evidence summarized in docs/ops/REQUEST-CAPACITY.md and docs/ux/2026-10-16-platform-delivery-audit.md.
