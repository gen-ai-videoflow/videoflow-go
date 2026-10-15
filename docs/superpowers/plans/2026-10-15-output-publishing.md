# Output, Publication and Download Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish finished videos on VideoFlow or download them from one owner output workflow.

**Architecture:** Resolve generated/uploaded sources through an owned media-asset module; publication references immutable validated source versions. Keep legacy job-backed API requests compatible while moving encoding away from mandatory job/quote joins.

**Tech Stack:** Go/PostgreSQL/outbox, existing media worker/storage grants, Svelte/shadcn.

**Spec:** `docs/ux/2026-10-15-viewing-publishing-spec.md` sections 6, 8–9, 11; master plan constraints/review focus apply.

## Task B1: Shared source boundary and safe migration

**Files:** create internal/cloud/media_assets.go and media_assets_test.go; add next migrations for owned assets and staged video source references; modify publishing.go, worker.go (encode currently joins cloud_jobs/cloud_quotes), store tests and cmd/cloud-migrate/main_test.go.

**Interface:** ResolveOwnedSource(ctx,user,sourceRef) returns an immutable source containing asset ID/version/private key/origin/status/measured metadata. Input is owned asset_id or compatible job_id, never an arbitrary object key. Origin is generated or upload; source job is nullable for uploaded assets. Only ready sources can be used for publication; Short eligibility additionally requires trusted measured metadata.

- [ ] Add failing tests for foreign source/job, incomplete/failed output, duplicate resolution and legacy videos. Test unknown measured metadata is ineligible for Shorts until probed.
- [ ] Create assets with unique source job/storage version and owner/workspace foreign keys. Backfill eligible successful job outputs idempotently; leave invalid legacy sources explicitly unresolved/unavailable rather than inventing readiness.
- [ ] Stage source_asset_id compatibility before relaxing job_id constraints; prove migration leaves existing watch/download/publication links working. Add runtime-role grants coverage.
- [ ] Refactor encoder source lookup to asset metadata with legacy adapter. Bind encoder callback to publication/source version; stale, duplicate or rejected callbacks cannot publish. Preserve visibility changes and moderation blocks during processing.
- [ ] Test migration/backfill, queue retry, source replacement and idempotent completion; targeted Go race suite before commit. No cloud apply.

## Task B2: Output detail and owner download

**Files:** create frontend/src/lib/cloud/publishing/OutputDetail.svelte and output.ts; modify GenerationJobs.svelte, CloudApp.svelte and server.go download handler; modify aws.go DownloadGrant, add output_test.go/download tests.

**Interface:** GET /jobs/:id/output → owned source summary, state, safe preview availability, formats and existing_publication_id. GET /media-assets/:id/download supports both ready generated/uploaded owned sources. Existing GET /jobs/:id/download remains a compatible adapter resolving its source, returning short-lived URL plus expiry/filename; file response has safe attachment Content-Disposition. Owner checks precede grant creation. Extra exports require distinct jobs, not invented options.

- [ ] Write failing tests for foreign job, processing/failed output, expired grant retry and safe filenames; no public original key in API metadata.
- [ ] Build /studio/outputs/:jobId with real preview/loading/failed states and Publish on VideoFlow / Download MP4 actions. Job lists link to this page.
- [ ] Replace post-await popup with visible download link/direct-navigation fallback. A transfer start is not shown as download completion. Maintain error/retry on the detail page.
- [ ] Link already-published output to its existing publication. Publishing and downloading do not consume another generation quote or charge credits again.
- [ ] Verify browser download navigation and signed grant renewal with local adapter/fixture; sensitive owner/download tests and Svelte checks before commit.

## Task B3: Composer, publication state and content management

**Files:** create internal/cloud/publications.go/publications_test.go; adapt publishing.go/server.go to existing /videos callers; add publication-intent/idempotency migration; create frontend/src/lib/cloud/publishing/{PublishComposer,ContentManager}.svelte; replace inline channel form in CloudApp.svelte; update contracts.

**Interface:** draft creation accepts source_asset_id (legacy job_id adapter), title/description, thumbnail/captions metadata, visibility, audience, format and disclosure. POST /publications/:id/publish uses Idempotency-Key and returns durable state; PATCH edits only authorised fields. Existing /videos and /videos/:id/publish remain adapters during migration. Stored publication intent is separate from playable status.

- [ ] Write failing tests for duplicate publish/submission retries, foreign sources, invalid format/audience/visibility, missing rights/disclosure, and changed source/visibility while encoding.
- [ ] Implement save draft, validation, idempotency and transactional outbox. Public discovery requires ready playable output, public visibility and approved status. Preserve Private/Free default.
- [ ] Build Details → Audience → Review composer with source preselected, field errors, save draft, dirty-state guard and real processing feedback. Never show successful public publication just because encoding was queued.
- [ ] Build /studio/content with status/visibility/format filters and real edit/view/download actions; no viewer original-download permission. Source replacement remains explicit.
- [ ] Test unpublished/direct public links, Premium locks, unauthorised download and stable source snapshots. Run publish/playback race tests and completed frontend state review before commit.
