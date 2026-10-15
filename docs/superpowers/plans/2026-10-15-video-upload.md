# Uploaded Video Publication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upload an existing file and publish or download it through the same source/publication system as generated videos.

**Architecture:** Durable owned upload sessions reserve quota, grant direct private multipart uploads and enqueue bounded validation. Only validated media assets reach Phase B's composer; upload bytes never pass through Go API handlers.

**Tech Stack:** Existing Go/PostgreSQL/queue/outbox, S3 adapter/local emulator, isolated media probe, Svelte upload UI. No AWS deployment.

**Spec:** `docs/ux/2026-10-15-viewing-publishing-spec.md` sections 6–9, 11; master plan constraints/review focus apply. Both uploaded and generated publishing were confirmed by the user. Depends on Phase B's asset/composer interface.

## Task D1: Durable session, quota and storage contract

**Files:** create internal/cloud/video_uploads.go/video_uploads_test.go; add upload-session/reservation migration; extend aws.go, server.go, maintenance.go and cmd/cloud-migrate/main_test.go; update contracts.

**Interface:** POST /video-uploads reserves an owned session using filename/type/bytes/checksum hints and an idempotency key. POST /video-uploads/:id/parts grants a bounded batch of upload parts; POST /video-uploads/:id/complete validates the stored multipart identity/part list and returns validating state. DELETE /video-uploads/:id requests abort. GET /video-uploads/:id resumes owned status. Clients cannot select arbitrary storage keys, owner IDs or multipart IDs.

- [ ] Confirm configurable upload/file/duration/workspace caps and probe budget before enabling the feature; current generic asset quota is not silently increased.
- [ ] Write failing tests for concurrent quota reservation, foreign IDs, wrong part/object checksum/size, completion replay, repeated abort and expired sessions.
- [ ] Implement atomic reservation plus external storage-operation reconciliation, not a database transaction that assumes a remote call cannot partially succeed. Use unique opaque object keys under a private owned prefix.
- [ ] Issue short-lived bounded part grants; verify final object via server storage metadata, record actual version/checksum/bytes, enqueue validation transactionally. Do not equate multipart ETag with a full file checksum.
- [ ] Abort/expiry cleanup confirms remote part/object removal before releasing reserved quota. Add retry/backoff and orphan reconciliation tests; restricted runtime-role and local multipart integration checks before commit.

## Task D2: Validated source and moderation boundary

**Files:** create internal/cloud/media_probe.go/media_probe_test.go; modify worker.go/outbox dispatch and media_assets.go; local fixture files/manifest; extend maintenance cleanup tests.

**Interface:** validation consumes owned asset/version and produces measured duration, display dimensions accounting for rotation, codecs/container, safe thumbnail and accepted/rejected status. Worker execution has timeout, CPU/memory/file limits and no arbitrary network fetching. Public upload feature remains gated if this validation/moderation path is unavailable.

- [ ] Add failing fixture tests for spoofed MIME/extensions, corrupt/truncated media, oversized dimensions/duration, unsupported codec, portrait rotation metadata, no-audio files and resource-limit expiry.
- [ ] Validate real stored bytes under bounded isolation; generate safe derivatives; never serve originals from public storage. Rejected media cannot create a publishable source.
- [ ] Apply upload origin/rights/AI-disclosure input and actual moderation policy/status. Automated AI detection is not claimed; uploader disclosure and review are separate data.
- [ ] Handle duplicate/stale validation jobs and source replacements idempotently. Correlate completion with immutable object version; validate claimed size against actual usage and clean rejected objects safely.
- [ ] Verify accepted uploads enter the shared composer/encoder without a synthetic generation job or generation-credit charge. Public publication requires all readiness/policy checks. Run race/fixture/cleanup tests before commit.

## Task D3: Resumable UI and unified publication

**Files:** create frontend/src/lib/cloud/publishing/{UploadVideo,UploadProgress}.svelte and upload-controller.ts; extend studio navigation/output/content pages; reuse PublishComposer; frontend/scripts/check-upload.mjs for real state transitions.

**Interface:** /studio/upload picker/drop area → upload session/progress → validating → ready source → composer. State stores a nonsecret owned upload ID, completed part bookkeeping and file fingerprint, not authentication or permanent storage credentials. Reload requires reselecting matching file before resuming bytes when browser file access is unavailable.

- [ ] Write failing state tests for cancel/retry, expired part grants, refresh/reselect, offline recovery, mismatched file and completion retries.
- [ ] Build pre-selection constraints, accessible picker, progress/time-independent status, cancel/retry and recoverable error states. Never claim upload completion before server validation completes.
- [ ] Resume known parts after authoritative server reconciliation; do not silently restart and double reserve quota. Aborting never immediately reports released storage until cleanup is confirmed.
- [ ] Open the shared composer with source preselected; preserve title/draft on validation delays or errors. Successful publish gives the true public/owner link according to state; authorised source download uses Phase B's rights checks.
- [ ] End-to-end local multipart upload, validation, publish/private/Premium and owner download using rights-cleared fixtures. Batched desktop/mobile inspection and one confirmation; secret scan, both builds and both repositories' CI before completion.
