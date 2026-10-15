# VideoFlow Viewing and Publishing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver coherent settings, standard watching, Shorts and publication/download of generated and uploaded videos.

**Architecture:** Keep the original canvas and current shadcn design. Split the large CloudApp route body into focused settings, viewing and publication features. Introduce owned media assets as the shared source for job outputs and uploads, with durable publication/validation state and bounded playback lifecycles.

**Tech Stack:** SvelteKit, Svelte 5, shadcn-svelte, Go, PostgreSQL, Redis, HLS.js; existing AWS media adapters and local fixtures. No AWS apply.

**Spec:** `docs/ux/2026-10-15-viewing-publishing-spec.md`; audit: `docs/ux/2026-10-15-experience-audit.md`; confirmed product: `PRODUCT.md`.

## Global Constraints

- User confirmed generated and uploaded video publication, with direct extension of the incumbent shadcn interface.
- Preserve original Studio canvas and authentication/factor/payment protections.
- Default publication is Private/Free; Premium is platform-wide viewing, credits pay for generation.
- Owner downloads do not imply public publication; Premium does not grant original download rights.
- No deployment, live payment/reward activation or external social-post OAuth in this scope.
- Shorts duration/view thresholds and upload quotas in the spec are proposed configurable policies, not silent changes to reward accounting.
- Every exposed control has real state persistence and loading/error feedback. No mock-only completed screens.
- Build files named below are proposed new modules, not existing implementations. Exact migration numbers must use the next unused value when work begins.

## Review Focus

1. Late responses/navigation and expiring grants: never replace a new page or lose playback/form state.
2. Ownership/Premium/privacy changes during upload, encode or playback: no source/segment/download leakage; short grants have documented revocation bounds.
3. Short clips, rapid swipes, loops, hidden tabs and concurrent tabs: one active player, bounded resources and no multiplied validated engagement.
4. Interrupted multipart uploads and repeated publication: resumable/retriable work, durable quota and one resulting publication.
5. Viewer-only accounts, small screens, keyboard/screen readers and long real content: creator UI stays out of browsing; controls and destinations remain usable.

## Sequence and review gates

| Phase | Deliverable | Plan | Dependency |
| --- | --- | --- | --- |
| A | Unified settings and shell corrections | 2026-10-15-settings-experience.md | None |
| B | Media source boundary, output detail and publish/download | 2026-10-15-output-publishing.md | Source migration before uploads/watch metadata |
| C | Standard watch, format metadata and Shorts | 2026-10-15-watch-shorts.md | B source metadata and stable player contract |
| D | Validated resumable file uploads to the same composer | 2026-10-15-video-upload.md | B; public upload moderation/quota gates |

Within each phase: write behavioural regression tests, observe failure, implement, run relevant checks, review complete real states, commit and sync extracted repositories. Keep each deliverable independently testable. Authoritative parent paths below map to `src/...` in videoflow-web and `internal/...` in videoflow-api.

## First implementation batch

- [x] Confirm this detailed scope and rollout/policy values. Execution uses the existing 100 MiB/file and 1000 MiB/workspace quota; uploads stay gated off.
- [x] Phase A: settings sections, mobile verification action spacing and route-specific loading/empty states.
- [x] Phase B: shared source boundary and completed-output preview with Publish/Download actions; then publication composer/content manager.
- [x] Phase C: standard watch player/data contract before the Shorts feed.
- [x] Phase D: uploading, validation, resumability, quota and cleanup.
- [x] Add save/like/library/related-video controls only with their durable APIs; moderated comments are a later subsystem.

## Verification and delivery

- Go race/integration tests against isolated PostgreSQL/Redis, including restricted database-role grants and migration/backfill behaviour.
- Svelte check, desktop build and cloud build; relevant account/payment regression scripts; secret-canary scan.
- Local labelled media fixture catalogue; real backend calls for settings, publishing, uploads and download authorisation. Local fixtures must not be represented as provider output.
- One batched desktop/mobile inspection, one fix batch and one confirmation; include tablet/320px/zoom checks in those batches. Run Impeccable detector once at finish and follow its independent finish review/documentation workflow.
- Update status only for implemented/verified features. Sync both independent repos, push, observe CI and report actual result. No production-readiness claim from empty screenshots or local fixture playback.

## Open decisions carried explicitly

- Implementation keeps the existing conservative cap: 100 MiB/10 minutes, 1000 MiB combined workspace reservations and one active video upload. Enabling remains an operations gate.
- Proposed Short eligibility portrait/3–180s and proposed duration-aware public views: policy review before migration/rollout; reward formula stays gated and separate.
- Existing image URL editing remains until a first-class image asset path exists.
- Anonymous qualified-view counting, comment moderation, creator collections and external social integrations are separate follow-ups.

## Delivery evidence and remaining verification boundaries

The four implementation phases are delivered in source. The child plans below remain detailed acceptance inventories: a compound unchecked item must not be interpreted as a verified browser/device scenario solely because its feature exists. The verification record in `docs/ux/2026-10-15-delivery-verification.md` is the authoritative account of checks actually run.

No AWS apply, real MediaConvert/CDN load test, delivered-email test, physical passkey test or live payment/rewards activation is claimed. No comments, creator collections or external social publishing integration was added. Unknown multipart creation outcomes retain quota pending explicit operator reconciliation.
