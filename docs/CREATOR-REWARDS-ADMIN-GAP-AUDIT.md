# Reward engines, administration and API documentation: implementation audit

Date: 2026-10-16. Scope: canonical API, web, admin and documentation feature branches. This is a code audit, not evidence of live provider activation. It supersedes any implication that the whole reward launch checklist is complete.

## Main finding

The funded allocation and vesting foundation exists. An automatic, end-to-end creator economy does not yet exist. Finance currently supplies reviewed evidence weights and reconciles funding manually. Having arithmetic and an admin form is not the same as having the scoring, reconciliation, fraud and settlement engines that generate trustworthy inputs.

The admin must expose all intended business policies as validated, versioned controls. It should not expose arbitrary database mutation, raw environment editing, or overwrite measured history. Creator-facing earnings remain simple; internal detail belongs here.

## 1. Engine inventory

| Engine | Actual state | Evidence / missing work |
| --- | --- | --- |
| AI generation/model selection | Implemented platform-controlled quotes/jobs and approved-model selection; operational activation separate. | runtime_policy.go, generation adapters; not a reward scoring engine. |
| Playback measurement | Implemented session/event validation, time bounds, owner exclusion and daily deduplication. | engagement.go; these are useful inputs, not comprehensive bot qualification. |
| Creator audience analytics | Implemented measured activity and retention breakdowns. | creator_analytics.go; current creator view can expand analytics. |
| Feed ranking | Implemented deterministic SQL ranking, not configurable end to end. | viewing.go embeds watch/view caps, followed/liked/history channel weights and candidate limits; legacy DiscoveryFeed is another path. |
| Search | Implemented PostgreSQL full-text title/description matching and admin record-prefix search. | No unified search-policy console, synonyms, evaluated language-aware relevance or search diagnostics. |
| Reward scoring | **Missing automatic engine.** | reward_batches.go accepts reviewed input weights; no normalized quality/originality/trust/external-signal pipeline. |
| Reward estimates | **Missing calculation pipeline.** | RewardBalance intentionally returns null with insufficient_data, not fabricated potential earnings. |
| Pool allocation | Implemented integer proportional allocation, pool bound and deterministic remainders. | AllocateRewards and daily reviewed drafts. No per-creator spending cap. |
| Revenue reconciliation | **Manual foundation only.** | Independent funding proposal/reconciliation records and evidence reference; automatic consumed-revenue/cost/refund/settlement adapters missing. |
| Reward confirmation | Implemented independent approval, input snapshot, recipient preview and atomic full-liability reservation. | reward_batches.go, admin_rewards.go; remains activation gated. |
| Vesting | Implemented exact 30-day linear maturity, release events, bounded worker and payout catch-up. | reward_vesting.go, reward_worker.go, cmd/cloud-rewards. |
| Cash transfers | Existing manual batch preparation/execution with provider checks and idempotency. | admin.go; monthly scheduler, notifications and full bank settlement reconciliation still missing. |
| Continuity/milestone support | **Missing.** | No separate promotional-credit expiry/consumption/grant engine; support funding source alone is not a support program. |
| Advertising/sponsorship | **Missing campaign engine.** | Advertising can be a reviewed funding source; no campaign lifecycle, delivery reconciliation or sponsor allocation engine. |
| Corrections/appeals | Partial existing refunds/disputes/holds. | Dedicated late-evidence correction, reward funding shortfall/reversal and appeal workflows missing. |

## 2. Current admin controls versus required controls

| Area | Available now | Required next |
| --- | --- | --- |
| People/content | Searchable, paginated records, user content drill-down, restrictions, video block/restore and audit. | A unified creator detail page with audience, earning eligibility, balances, holds and action history. |
| Audience views | Read-only video views/watch time and channel follower totals. | Per-creator/video time series, organic/paid/invalid breakdown, dedup/replay investigation and audited correction/rebuild workflow. |
| Generation/pricing | Approved story/video model lists, credits per second/story/assembly, provider budgets/ceilings, Premium price, active-job limit. | Credit pack management, target margin configuration, commercial policy publishing and impact preview. |
| Rewards | Funding requests/reconciliation, reviewed batch drafts, input/recipient inspection, independent confirmation, policy status. | Editable reward policy console, scoring factor policy, source percentages/caps/reserves, preview/simulation, prospective scheduling and rollback. |
| Earning window / vesting | 30-day pilot defaults enforced in schema; not an arbitrary live UI setting. | New versioned policies with bounded durations and effective dates; old confirmed obligations keep original schedules. |
| Payouts | Existing operator transfer workflow, verification checks, disputes and reward-hold API. | Finance UI for liabilities, holds/appeals, payout review/reconciliation, threshold/cadence policy and approved destination-change rules. |
| Discovery | Implemented product ranking with hardcoded values. | Versioned ranking policy, creator/video boost/suppress controls, expiry, reasons, previews and a kill switch. |
| Search | Product full text; admin record search/filter. | Query playground, relevance weights, synonyms, pins/exclusions, language handling, indexing status and relevance evaluation. |
| Notifications | Existing account/payment feedback. | Reward confirmation, hold/release, payout and appeal notifications with delivery/retry records. |
| Operations | Readiness, capacity, basic generation/publication switches and runtime policy history. | Reward-specific queue health, funding coverage, unvested liabilities, failed runs, anomaly alerts and independent pause controls. |
| API docs | Generated OpenAPI contract existed; authenticated Swagger viewer added in this audit. | Fill incomplete request/response schemas, examples and pagination docs; enforce route/contract parity in CI. |

## 3. Define what “adjust every variable” means

### Editable policy — prospective and audited

Owner/authorized policy roles may change bounded policy fields through schemas and revision checks. Each change records before/after, actor, reason, effective date and affected policy version. Financial changes need independent approval and a preview. Validate combinations, not only individual ranges. Never accept arbitrary SQL, script expressions or unrestricted environment names.

Required policy groups:

- Reward source funding basis, percentages, absolute caps, reserve targets, carry-forward and source/currency separation.
- Scoring factor weights, factor normalization, minimum evidence, eligible traffic, evidence cutoff and correction grace period.
- Earning-window duration, per-batch vesting duration, eligibility requirements, enrollment/terms versions and program region rules.
- Payout threshold, cadence, risk holds, supported provider/currency/country rules and operational limits.
- Continuity milestone definitions, grant/creator caps, expected service cost budget, expiry and abuse limits.
- Discovery signal weights, freshness decay, diversity/exploration, followed/liked/history channel boosts, traffic caps and candidate/page limits.
- Search relevance, permitted filters, synonyms, curation/pinning, exclusion reasons, index version, language analysis and cache lifetimes.
- Generation pricing/packs, approved OpenRouter model policy, margin floors, cost ceilings, concurrency, upload/processing and media limits.
- Notification templates/languages, batching, delivery retry limits and feature announcements.

Secrets remain in a secret manager/deployment configuration. Admin can inspect masked configuration status and initiate a reviewed rotation workflow; secrets are never returned as normal editable variables.

### Measured views — inspect and correct, not overwrite

The requested creator view controls need three separate tools:

1. **Audience facts:** inspect measured real views, watch time, retention and provenance. Correct invalid events using an append-only exclusion/adjustment with reason and rebuild the affected aggregate. Preserve original evidence and metric version.
2. **Distribution controls:** explicitly boost/suppress a creator or video for a bounded period, with reason, placement type, limits, preview and automatic expiry. A reach boost does not increment view counters by itself. Label paid/sponsored placement.
3. **Reward eligibility:** include/exclude qualified evidence through documented review and appeals. Promotional/paid impressions cannot silently become organic payable activity. Ranking changes do not rewrite already confirmed earnings.

No production “set creator views to N” control should masquerade as actual human audience. Test/demo data belongs only in isolated fixtures. Optional synthetic reward points must be explicitly labelled and separate from audience views and cash.

## 4. Consolidated remaining checklist

### P0 — truthful automated money lifecycle

- [ ] Implement automatic consumed-generation revenue recognition with actual direct costs, fees, taxes, refunds and settled receipts as inputs.
- [ ] Reconcile committed funding, confirmed liabilities, payout reservations and provider balances; report shortfalls and source reversals.
- [ ] Implement frozen, reproducible qualified evidence snapshots and original/re-upload fraud review.
- [ ] Implement versioned multifactor normalization/scoring; validate factors, ranges and weights. Spending/loss history must not secretly boost scores.
- [ ] Implement provisional estimates derived from accepted evidence and actual budget assumptions, with cutoff/freshness and no double-counting of confirmed slices.
- [ ] Add daily draft generation, approval queues, durable retry/dead-letter handling and source-backed confirmation orchestration.
- [ ] Add funded late-evidence corrections, explicit adjustments and creator appeals; preserve confirmed input history.
- [ ] Add monthly payout scheduling/notifications and ambiguous-transfer/provider reconciliation.
- [ ] Reconcile legacy balances without inventing historical backing; document migration exceptions.

### P0 — complete finance/admin controls

- [ ] Build a typed reward policy editor with independent approval, prospective effective dates and historical version comparison.
- [ ] Add funding percentage/cap/reserve and scoring/window/vesting/payout controls with server-side validation and simulation.
- [ ] Add liability coverage/aging dashboards and a creator financial detail timeline.
- [ ] Expose the existing hold/release API through a reasoned, audited finance workflow; add appeals and account recovery handoff.
- [ ] Add independent switches for new estimates, new confirmations and cash submission. Reads and existing obligations remain intact.
- [ ] Add complete audit/export, RBAC, recent MFA and conflicting-update handling to every new mutation.

### P1 — views, recommendation and search administration

- [ ] Add creator/video audience drill-down with date, country/device where privacy-safe, source, organic/paid and invalid-traffic filters.
- [ ] Add audited event exclusion/correction and aggregate replay, with before/after values and metric versions.
- [ ] Move hardcoded ranking constants into a validated versioned ranking policy; unify legacy/current feed paths where intended.
- [ ] Add creator/video boost/suppress controls with scope, expiry, disclosure and a recommendation preview.
- [ ] Add query playground, search relevance/synonyms/pins/exclusions and language-aware evaluation, including Mandarin.
- [ ] Add indexing/reindex/cache invalidation diagnostics, bounded jobs, queue monitoring and rollback.
- [ ] Prove hidden/private/blocked/Premium content cannot leak through search previews or ranking overrides.

### P1 — support and sponsorship

- [ ] Implement milestone eligibility, independent support budget, promotional-credit ledger, expiry and service-cost reservation.
- [ ] Separate promotional consumption from purchased credits and withdrawable earnings.
- [ ] Add abuse controls, grant caps, rejection reasons and creator support-credit explanations.
- [ ] Implement campaign contracts, settled sponsor funding, labelled delivery and sponsor-funded award reconciliation as a separate later increment.

### P1 — API documentation and quality gates

- [x] Generate OpenAPI from the repository's contract source (195 operations at this audit).
- [x] Add authenticated `/admin/openapi.json` behind role and MFA checks.
- [x] Add self-hosted, lazy-loaded Swagger UI in Admin → API docs; no external validation or write actions from the viewer.
- [ ] Fill typed responses/examples/errors and query/pagination details for currently underspecified operations.
- [ ] Add automatic live-route-to-contract parity and schema-validation tests in CI.
- [ ] Document future controls as they ship; do not publish unavailable endpoints as working features.
- [ ] Verify complete admin keyboard/mobile/screen-reader flows and full English/Mandarin application locale integration.
- [ ] Complete shadow allocation, independent budget reconciliation and a limited enrolled pilot before general activation.

## 5. Admin navigation and UX

Keep the ordinary creator interface to Earnings/Videos with secondary disclosures. Admin can be deeper because its job is operating the platform, but avoid one giant settings form:

- People → Creator detail → Content / Audience / Earnings / Reviews.
- Rewards → Funding / Calculations / Liabilities / Payouts / Reviews.
- Discovery → Ranking policy / Creator overrides / Preview.
- Search → Query playground / Relevance policy / Curation / Index health.
- Platform controls → Generation/pricing / Program policies / Operational limits.
- API docs → searchable Swagger reference and OpenAPI download.
- Audit → actor/resource/date filters and approved exports.

Show current effective values first. Open an editor only for the selected policy group. Review impact and changes before applying; keep uncommon controls collapsed. Read-only facts never look like editable policy inputs.

## 6. Implementation order

1. Versioned policy/control registry plus funding coverage and correction rules.
2. Qualified evidence/scoring/estimate pipeline and shadow comparison.
3. Finance review/liability/payout console and scheduling.
4. Creator analytics corrections, ranking/search controls and relevance evaluation.
5. Continuity credits, notifications and later sponsor operations.
6. Contract parity, end-to-end audits and approved pilot activation.

Swagger is added now as a developer/admin reference, separately from user-facing Fumadocs. Its viewer uses the [official Swagger UI distribution](https://swagger.io/docs/open-source-tools/swagger-ui/usage/installation/), pinned to swagger-ui-dist 5.33.3 with integrity and license recorded under admin/static/swagger. The calculation/admin gaps above are still work, not implicitly completed by adding Swagger.

## Verification for the Swagger addition

- The protected-contract test first failed with a missing route, then passed after implementation. It checks anonymous/ordinary-user rejection, administrator MFA requirement, authorized response and private cache policy.
- Full API `go test ./... -count=1` passed with isolated PostgreSQL. Admin type/component checks report zero errors and warnings; production build passed.
- Browser verification in an isolated MFA-session fixture loaded grouped Swagger operations and all six creator reward routes. No write/try-out buttons were present. Desktop 1280px and mobile 390px had no page-level horizontal overflow or alert errors. This is not evidence of a physical authenticator ceremony or production deployment.
- The public Fumadocs site is unchanged by the private Swagger reference.
