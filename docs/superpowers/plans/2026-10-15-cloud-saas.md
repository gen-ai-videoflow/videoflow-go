# Cloud SaaS Implementation Plan

**Goal:** Implement the approved cloud platform specification using Go, SvelteKit and shadcn-svelte.
**Spec:** ../specs/2026-10-15-cloud-saas-platform-design.md
**Architecture:** Separate cloud server and worker commands preserve desktop compatibility. PostgreSQL owns durable facts; Redis handles temporary coordination; SQS carries jobs; S3/MediaConvert/CloudFront deliver media.
**Execution:** User authorized implementation without further questions. Work on feat/cloud-saas in the current checkout to keep results visible. Parallel work is limited to disjoint files.

## Global constraints
- Never expose the legacy unscoped server as the cloud API.
- No Cognito, provider keys or model selectors in customer flows.
- Financial operations use database transactions and durable idempotency.
- Production activation requires actual credentials and country/policy configuration; no fake successful payments or payouts.
- Preserve existing desktop tests and routes.

## Review focus
- Concurrent credit reservations and duplicate webhooks cannot create money.
- Cross-user IDs and playback must never bypass ownership checks.
- Provider submission timeouts must not trigger blind paid retries.
- Cloud integrations must fail clearly when unconfigured.
- Redis loss must not lose durable records.

## Tasks
- [x] 1. Cloud persistence, identity, migrations and secure sessions. Files: internal/cloud/{store,identity}*.go, internal/cloud/migrations/*.sql. Verify credentials, cookie/CSRF, cross-user isolation and PostgreSQL integration.
- [x] 2. Billing, generation quotes/jobs, publishing and discovery API. Files: internal/cloud/{server,billing,generation,publishing}*.go, cmd/cloud/main.go. Verify transactions, quote expiry and idempotency before implementation; expose no unscoped legacy handlers.
- [x] 3. Worker, AWS media adapters, Redis admission and SQS outbox. Files: internal/cloud/{worker,aws,redis}*.go, cmd/cloud-worker/main.go. Verify replay, missing credentials and bounded retries.
- [x] 4. SaaS UI with current shadcn-svelte components. Files: frontend/src/lib/cloud/*, frontend/src/routes/(cloud)/*. Keep desktop available, add authenticated creation, credits, channel, discovery, watch, membership and earnings states. Run check/build and browser validation.
- [x] 5. AWS Terraform, local cloud compose, environment contract and repo extraction tooling. Files: infra/*, deploy/*cloud*, scripts/*split*, docs/CLOUD*. Validate configuration and document required secrets.
- [ ] 6. Integrate and review the whole branch. Run Go tests/race where practical, frontend checks/build, PostgreSQL integration, browser account/project/billing flows and infrastructure validation. Record remaining spec work honestly.

## Execution ledger
- Baseline: Go not installed; installing Go 1.26.0 in /tmp/videoflow-tools. Existing checkout was clean.
- Ruling: implement cloud separately from desktop; retrofitting global SQLite routes would expose cross-user data. Keep cloud PostgreSQL-only.
- Ruling: implementation may construct all approved features, while live billing/payout/deployment activation remains disabled until credentials and business configuration exist.

- Implemented: cloud Go/PostgreSQL API, Redis sensitive admission/progress hints, SQS worker, bounded AWS media, memberships, operator rewards/payouts, shadcn cloud UI, Terraform and CI.
- Review fixes: scoped media cookies, public playback grants with current authorization, bounded unversioned quarantine uploads, atomic cancellation, trusted edge addresses, paid-spend admission, credit refund debt, balanced postings, viewer-wide elapsed clock.
- Verification: PostgreSQL cloud race suite passed (8.08s); full Go suite passed before final corrections. Final checks in progress. HTTP gateway smoke passed registration/session/project/verification/channel/logout.
- Limitation: browser preview automation timed out/failed snapshots; no visual QA claimed. No AWS deployment, live AI generation or live money movement performed.
- Scope coverage is recorded in docs/CLOUD-IMPLEMENTATION.md; remaining advanced studio migration and production operations are not represented as finished.

- Additional delivered scope: workspace-scoped advanced studio CRUD and captions, protected WebVTT sidecars, import/export ID remapping, S3 archive-before-delete maintenance, confirmed upload quota reclamation, history clearing and account-deletion requests.
- Final checks: Go race tests across all packages passed; go vet passed; Svelte check reports zero errors/warnings; desktop static and Node cloud builds passed; Terraform fmt/validate and compose config passed; API/worker/web container builds passed.
- Independent repositories created at /home/jienweng/projects/videoflow-saas/videoflow-web and /home/jienweng/projects/videoflow-saas/videoflow-api. Backend tests pass outside the source repository; standalone web Docker build passes. They are source snapshots with no remotes or rewritten source history.
- Local runtime uses a separate videoflow_app database, so integration test public-video fixtures are not shown in the app.
- Remaining verification gates: browser visual automation (tool failed), deployed AWS HLS/SES/provider tests, payment sandbox integration, restore/load drills and commercial activation. Full master-spec remaining features are explicit in docs/CLOUD-IMPLEMENTATION.md.

- Continuation delivered durable screenplay/scene/clip assembly, flow graph/timing UI, current-day spend fencing, dispute/payout race fixes and explicit restricted-role migrations. See [continuation ledger](2026-10-15-cloud-saas-continuation.md) for final tests and local runtime evidence. Task 6 remains open for the unverified browser/deployed-service gates.
