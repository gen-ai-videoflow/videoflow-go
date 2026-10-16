# Creator Rewards Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Delegate only when separately authorized.

**Goal:** Deliver the funded, per-video reward lifecycle and distinct viewer/creator/admin journeys in the linked design.

**Architecture:** PostgreSQL owns financial facts, eligibility and durable jobs; bounded workers qualify evidence, confirm funded batches and reconcile releases/payouts. Redis accelerates reads and throttling only. Extend the existing SvelteKit creator interface and separate admin application; keep proprietary calculations server-side.

**Tech Stack:** Go, PostgreSQL, Redis, SvelteKit, shadcn-svelte, existing payment adapters, bilingual Fumadocs.

**Spec:** [Creator rewards requirements and journeys](../specs/2026-10-16-creator-rewards-journeys.md). Read it before execution; proposed commercial defaults are not activation approval.

## Global constraints

- Earning window: 30 × 24 hours from first successful publication, UTC, half-open interval.
- Confirmation: daily funded batches; vesting: 30 × 24 hours per confirmed batch.
- No individual spend cap, guaranteed per-view rate, spend-based scoring, synthetic audience views or loss-triggered bonuses.
- No automatic provider/AWS activation. Preserve existing legacy balances and reward-off defaults.
- USD accounting initially. Public eligibility and payout availability must match real supported regions/providers.
- Public user docs exclude operations; maintain private admin/finance procedures separately.
- Canonical implementation repos: `/home/jienweng/projects/videoflow-saas/videoflow-api`, `videoflow-web`, `admin`, `docs`. API paths below are relative to videoflow-api; UI paths are labelled explicitly.
- Inspect each repo's AGENTS.md and current migrations before execution. Migration 041 is the next candidate after inspected 040; renumber if concurrent work has consumed it.

## Review focus

1. A late event or republished copy must not reopen a video's earning window (Tasks 2–3).
2. Two workers must never reserve the same available funding or vest the same cent twice (Tasks 1, 4–5).
3. A provider timeout must not become a second payout or a premature “paid” label (Task 5).
4. A stale estimate, empty result or held account must not display invented earnings or erase money (Tasks 6–7).
5. Policy changes, deletion and legacy migration must preserve historical commitments and explanation (Tasks 1–2, 8).

## Proposed file boundaries

| Repository | File/module | Responsibility |
| --- | --- | --- |
| API | `internal/cloud/reward_policy.go` | Versioned reward policy and activation validation. |
| API | `internal/cloud/reward_funding.go` | Reconciled source funding, adjustments, reservations and coverage. |
| API | `internal/cloud/reward_eligibility.go` | Publication window, enrollment, originality linkage and eligibility. |
| API | `internal/cloud/reward_scoring.go` | Evidence snapshots and versioned accepted weights. |
| API | `internal/cloud/reward_batches.go` | Estimate calculation, approval, funded confirmation and corrections. |
| API | `internal/cloud/reward_vesting.go` | Exact vested principal and durable release bookkeeping. |
| API | `internal/cloud/reward_handlers.go` | Authorized, paginated creator reward reads/enrollment. |
| API | `internal/cloud/admin_rewards.go` | Role-separated financial review/approval handlers. |
| API | `internal/cloud/reward_support.go` | Idempotent bounded promotional milestone grants. |
| API | `internal/cloud/migrations/041_creator_rewards.sql` | New tables, indexes, constraints, financial immutability and legacy mapping. |
| Web | `src/lib/cloud/rewards/` | Typed client, two task tabs and secondary disclosures, video detail and status components. |
| Admin | `src/lib/rewards/` | Funding, approvals, liabilities, payout reconciliation and audit views. |
| Docs | `content/docs/publish/creator-rewards.mdx`, `.zh.mdx` | Public program and payout explanations. |

Keep monetary arithmetic in small tested modules. Reuse AllocateRewards for rounding; do not expand commerce.go or CloudApp.svelte into another monolith. Legacy payout adapters remain behind a service seam during migration.

## Task 1: Policy, funding and ledger foundation

Files: create reward_policy.go, reward_funding.go, migration 041 and matching `_test.go`; modify runtime policy integration only where appropriate.

Interfaces: define RewardPolicy (version, effective time, funding rules, window/vesting durations, payout rules); FundingSource (id, currency, origin, reconciled amount); FundingReservation (source, batch, cents). Expose `ReserveRewardFunding(ctx, batchID string, cents int64) error` inside the confirmation transaction. No UI-supplied contribution amount is accepted as reconciliation evidence.

- [ ] Write failing integration tests for source isolation, duplicate funding receipt, conflicting policy version, negative adjustment and simultaneous reservations exceeding backing.
- [ ] Add schemas for policies, enrollments, sources, postings, reservations, holds, snapshots, batches, allocations, schedules and outbox; specify foreign keys and unique source/batch/event identities. Enforce financial immutability and balanced transactions.
- [ ] Implement server-owned funding reconciliation, including consumed-credit revenue recognition inputs, source cost deductions and explicit reserve movements. Keep unsupported revenue adapters disabled.
- [ ] Map legacy entries as legacy cliff-held liabilities with coverage exceptions reported explicitly; do not fabricate historical source funding or silently convert them to new vesting.
- [ ] Run integration tests against disposable PostgreSQL, prove concurrent over-allocation fails, review migration rollback/recovery and commit.

## Task 2: Enrollment and per-video eligibility

Files: create reward_eligibility.go/tests; modify existing publication handlers/store and server.go routes.

Interfaces: `RewardEligibility(ctx, userID, videoID string) (RewardVideoEligibility, error)` returns server-owned first_published_at, window_ends_at, status, reason_code and policy_version. Enrollment stores accepted terms/version/time. Mutations require owner authorization and validated requests.

- [ ] Write failing tests: event at start qualifies; event exactly at end does not; processing failure has no window; republish/reupload does not renew; owner change is rejected; legacy video treatment follows migration policy.
- [ ] Atomically assign first publication and window only on successful publication; connect media fingerprint/revision and moderation/access eligibility.
- [ ] Implement enrollment and eligibility API without requiring generation spending; implement privacy/unpublish and terms-change behaviors from the spec.
- [ ] Verify generated output and original-upload parity, authorization and account-departure preservation, then commit.

## Task 3: Qualified evidence and private scoring

Files: create reward_scoring.go/tests; extend creator_analytics.go and engagement.go via explicit shared aggregates.

Interfaces: immutable RewardEvidenceSnapshot identifies video, interval, cutoff, source provenance, accepted/excluded metrics and policy/scoring versions. Scoring emits nonnegative weights plus auditable internal factor contributions; creator responses expose only approved broad factors/reason codes.

- [ ] Write failing tests for self-viewing, replay, paid traffic exclusion, out-of-window events, deleted/quarantined evidence, absent external signals and zero qualified evidence.
- [ ] Build persistent daily aggregate jobs with bounded work and evidence cutoffs; integrate verified external signals only when an actual authorized adapter exists.
- [ ] Implement versioned deterministic normalization/scoring after factor policy approval. Spending, prior losses and predicted future purchases are not inputs.
- [ ] Test reproducibility, late evidence quarantine/correction routing and query plans on representative volumes; commit.

## Task 4: Estimates and daily funded confirmation

Files: create reward_batches.go/tests, admin_rewards.go; connect existing AllocateRewards and durable worker/outbox execution.

Interfaces: draft batch references evidence/policy and funding sources; confirmation accepts batch ID plus approved immutable input hash, never arbitrary client weights. `ConfirmRewardBatch(ctx, batchID, actor string) error` atomically reserves backing and creates liabilities/schedules. Estimate responses include calculated_at, evidence_cutoff, remaining unconfirmed estimate and nullable insufficient-data state.

- [ ] Write failing tests for two competing confirmations, retry after commit/timeout, changed approved hash, insufficient backing, empty evidence, deterministic cent rounding and correction replay.
- [ ] Implement daily UTC draft snapshots and estimates without posting payable money. Remove confirmed slices from remaining estimates.
- [ ] Add independent preview/approval and confirmation with full liability reservation and immutable batch input/output records.
- [ ] Run shadow mode: compare source funds, predicted allocation and independent totals; prove no payable entries are created. Commit only with invariant tests passing.

## Task 5: Vesting and payout settlement

Files: create reward_vesting.go/tests; adapt admin.go payout preparation/execution, commerce.go earnings reads and disputes/refunds services.

Interfaces: `VestedCents(principal int64, confirmedAt, at time.Time, duration time.Duration) (int64, error)` follows the exact spec formula with overflow-safe arithmetic. Durable release identities prevent duplicate postings. Payout selection reserves only eligible vested net amounts, with source references preserved.

- [ ] Write failing tests at confirmation, halfway (101 cents → 50), one instant before end, exact end (101), overflow/invalid duration, repeated release and concurrent release/payout.
- [ ] Implement schedules and read-time maturity; holds affect payout eligibility without erasing confirmed principal. Retain legacy cliff behavior explicitly.
- [ ] Test negative corrections, partial vesting, payout threshold, destination change, disputes and insufficient provider funds without releasing unbacked money.
- [ ] Add monthly payout scheduler with independent approval and original idempotency keys; distinguish submission, transfer confirmation and bank payout where supported.
- [ ] Simulate provider timeout/retry/reconciliation and prove one transfer only. Verify no unused unvested backing feeds new pools; commit.

## Task 6: Creator/public APIs and integrated publishing UX

Files: create reward_handlers.go/tests and paginated API contracts; Web rewards/client.ts, RewardEnrollment.svelte, RewardStatus.svelte; modify current publish flow and CloudApp.svelte integration point.

Interfaces: creator summary returns separate estimated_additional, confirmed_unvested, available, held, payout_reserved and historical_paid cents plus freshness/policy/eligibility. Video detail returns window and per-batch schedule. All IDs are authorization checked; financial amounts are server-owned.

- [ ] Write API tests for cross-account access, nullable estimate vs zero, pagination and overlapping totals.
- [ ] Implement summary/video/activity/program endpoints; generate matching client types using the repo's existing contract workflow.
- [ ] Integrate optional enrollment/status into the existing shared generated/export/upload publish flow; preserve drafts and return locations on auth/setup failure.
- [ ] Verify viewer mode remains independent and publishing succeeds without joining rewards; verify English/Mandarin acknowledgement, validation and failure states; commit.

## Task 7: Earnings screens, recovery and notifications

Files: Web rewards/EarningsOverview.svelte, RewardVideos.svelte, RewardVideoDetail.svelte, RewardPayouts.svelte, RewardProgram.svelte and shared states; integrate CreatorAnalytics and account settings links.

- [ ] Implement the two task tabs and secondary disclosures and compact overview from the spec. Keep generation credits outside the cash summaries.
- [ ] Add per-video window/evidence timeline and per-batch vesting detail, correct source labels and next payout action.
- [ ] Add support/appeal handoff and durable notification events for confirmation, hold/release and payout result; avoid notification per tiny daily release.
- [ ] Verify stale/offline data, zero/null/negative adjustments, large amounts, insufficient threshold, unsupported country and delayed provider state without misleading labels.
- [ ] Verify 390px/1280px, both themes/languages, keyboard/focus/screen-reader semantics. Poll only active views with backoff; no recursive refresh or background full-history fetch. Run `npm run check` and `npm run build`; commit.

## Task 8: Admin controls and audit

Files: API admin_rewards.go/tests and permissions; Admin src/lib/rewards/{Funding,Batches,Liabilities,Reviews,Policies,Audit}.svelte; integrate existing AdminRecords/RuntimePolicy shell.

- [ ] Write permission tests separating support reads, trust review, policy editing, finance approval and payout execution. Test same-actor approval rejection and recent-MFA requirement.
- [ ] Implement funding coverage, batch diffs, policy-effective dates, liability aging, hold/appeal reasons and source-to-payout trace.
- [ ] Add independent pause switches; changing policy cannot modify frozen/confirmed history or stop reads/owed maturity.
- [ ] Verify export/redaction, pagination, audit completeness, denied actions and ambiguous transfers; run API/admin checks and commit.

## Task 9: Creator continuity credits

Files: reward_support.go/tests; Web rewards/SupportMilestones.svelte; Admin support-budget view.

- [ ] Define approved milestone criteria/budgets before enabling; default to no grants while unconfigured.
- [ ] Write tests for repeated claims, multi-account abuse, exhausted budgets, grant expiry and simultaneous awards.
- [ ] Atomically reserve expected service cost and issue uniquely referenced non-withdrawable promotional credits; preserve cash separation.
- [ ] Verify qualification/pending/rejection/expiry UI, no loss-triggered targeting and no promise of recovered spend; commit.

## Task 10: Documentation, audit and pilot gate

Files: Docs public creator-rewards pages and locale metadata; private API/admin runbooks; root checklist evidence.

- [ ] Explain variable rewards, funding categories, real views, 30-day windows, estimates, confirmations, vesting, payout limits and appeals in both locales. No scoring weights or operator procedures in public guides.
- [ ] Run docs content/type/build/export checks; preserve the four user-facing documentation tabs.
- [ ] Run full API suite with disposable PostgreSQL/Redis and isolated provider fixtures; record tests skipped or unavailable. Go is currently absent from the host: establish a pinned Go toolchain/container before claiming tests passed.
- [ ] Complete technical audit of accounting invariants, authorization, retries, source reconciliation and query limits; complete UI/UX audit for all journeys and material states.
- [ ] Record commercial/provider/region activation approvals, independently reconcile shadow batches, then enable only a limited enrolled pilot. Do not enable worldwide payouts by changing one flag.
- [ ] Observe daily coverage, estimate variance, job delay and first monthly payout reconciliation before expansion. Sponsor funding is a later separately scoped increment, not implied by pilot completion.

## Completion evidence

Every task records changed files, exact commands/results, migrations applied, relevant screenshots and unresolved limitations. Existing source inspection is not test evidence. Final acceptance requires all launch-blocking checklist items in the spec, no unexplained funding differences, no duplicate financial events and passing end-to-end creator/payment fixture journeys. This plan itself makes no implementation or deployment claim.
