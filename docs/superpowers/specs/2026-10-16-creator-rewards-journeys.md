# Creator rewards: product requirements and user journeys

Status: proposed design and implementation checklist, not shipped behavior.
Date: 2026-10-16. Product remains 6eek; the pasted “6eek” discussion supplies reward requirements, not a rebrand instruction.
Related: [implementation plan](../plans/2026-10-16-creator-rewards.md).

## 1. Outcome and boundaries

Creators can create or upload a video, publish it, understand whether it qualifies for rewards, track honest estimates, see funded confirmed rewards vest, and receive an auditable payment. Viewers can enjoy the platform without buying credits or joining rewards. Finance can prove every confirmed liability is funded. Support can explain states without exposing private scoring weights.

This design replaces the previous monthly-only engagement allocation lifecycle for new rewards. Existing earnings and historical payouts remain intact. It does not activate payments, providers or AWS as part of planning.

### Proposed pilot defaults

| Decision | Proposed rule |
| --- | --- |
| Earning window | 30 × 24 hours after first successful publication, UTC, half-open interval [start, end). |
| Confirmation | Daily UTC batches, after evidence qualification and funding reconciliation. Late evidence can settle through an explicit correction batch. |
| Vesting | Each confirmed batch vests linearly over 30 × 24 hours from confirmation. |
| Individual upside | No cap tied to the creator’s spending. Total awards remain constrained by funded pools. |
| Generation purchases | Purchase a creation service; do not buy eligibility, views, score or a promised financial return. |
| Source eligibility | Proposed: original eligible uploads and generated outputs can both participate without a minimum spend. |
| Continuity support | Pilot uses non-withdrawable generation credits for published quality milestones, from a separately funded support budget. No loss-triggered bonuses. |
| Funding proportions | No promised 60%, fixed per-view rate or example split. Explicitly versioned policy must be approved before activation. |
| Audience metrics | Actual playback metrics only. Do not invent “views” by converting rewards into audience counts. |
| External evidence | Optional verified inputs only; no requirement to connect a social account to watch or create. |
| Payout cadence | Monthly approved payout runs; creator can see the next scheduled run and any unmet requirements. |
| Threshold | Existing USD 50 threshold retained for pilot review; UI reads the server policy, never hardcodes it. |
| Currency | USD reward accounting initially; supported countries/currencies come from provider capabilities, not a worldwide promise. |

Commercial approval still needed before activation: funding basis and percentage, pool cap/reserve policy, score version, supported regions/ages, participation terms, payout rules and support-credit limits. Country/provider eligibility review belongs to launch operations. These are explicit release gates, not invented approved policy.

## 2. Actual starting point

Verified from source on this date, not a claim that live payments have been exercised:

- `internal/cloud/commerce.go`: AllocateRewards uses integer money, deterministic remainders and a total pool cap; no individual spend cap. CloseRewardPeriod accepts administrator-supplied contribution and weights for a closed calendar month.
- `internal/cloud/admin.go`: payout preparation matures pending entries after a 30-day cliff hold, applies negative adjustments, checks disputes/provider setup and requires at least 5000 USD cents. This is not linear vesting.
- `internal/cloud/migrations/009_billing_integrity.sql`: balanced money postings exist, but these do not prove cash backing or enforce a complete immutable funding ledger.
- `frontend/src/lib/cloud/CloudApp.svelte`: creator balances are Available, Pending and Paid. There are no per-video earning windows, estimated rewards or vesting schedules.
- Watch-event validation and creator analytics exist separately from financial allocation; they are useful inputs, not proof of reward-grade fraud qualification.
- A standalone API repository mirrors the reviewed commerce implementation. Build new work in the published API/web/admin repos; keep the aggregate source synchronized deliberately, not with broad directory copies.

## 3. Information architecture and screen contracts

Preserve current brand, shadcn-svelte controls, original canvas and connected project context. Do not add another competing project dashboard.

### Viewer navigation

Discover / Shorts / Following / Library. Premium and account controls remain accessible. Studio is an intentional mode switch, not a second sidebar embedded in browsing. Public channel profiles display real content and audience signals, never private earnings or identity-verification details.

### Creator navigation

Overview / Content / Create / Analytics / Earnings. Story, Canvas, Edit and Resources stay within one project workspace. Earnings is one destination with two simple tabs, following the user's simplification request on 2026-10-16:

| Tab | Default experience |
| --- | --- |
| Earnings | Available earnings as the main figure, confirmed funds still unlocking only when nonzero, and one next payment action. No empty estimate card. |
| Videos | Video title, a plain-language reward status, and expandable window/confirmed-reward details. |

Payment settings/history, audience analytics, nonempty estimates and program rules are secondary expandable sections within Earnings. Financial terminology such as batches, evidence cutoffs and ledger entries belongs in details or administration. Program/account blockers remain visible. Generation credits never appear as cash earnings. Opening a secondary section loads its optional data on demand.

Account settings owns identity and payout destination setup. Earnings links directly to the relevant setting and returns to the same tab. Avoid duplicating editable forms across both locations.

### Balance semantics (mandatory)

- **Estimated additional rewards:** provisional rewards for qualified activity not yet confirmed. Can decrease; updated-at and evidence-cutoff timestamps shown. Never count a confirmed batch again. Return “Not enough data” rather than a fabricated number.
- **Confirmed, vesting:** funded confirmed principal minus vested principal and audited reversals. Money is reserved, not available to spend on new reward pools.
- **Available for payout:** vested net amount minus paid/submitted/reserved payouts and applicable account holds. Minimum threshold can prevent scheduling without erasing this balance.
- **Paid to date:** historical successful provider transfers with settlement terminology kept accurate. A Stripe transfer to a connected account is not proof of arrival at the creator’s bank.
- Do not add these overlapping lifecycle totals into a misleading “total balance.” Separate lifetime confirmed from current balances.
- Generation-credit balance and promotional support credits are displayed separately from cash earnings, with expiry and restrictions.

### Detail rather than clutter

Overview emphasizes one available-earnings amount and one compact payout-status action. Put formulas, batch records, source splits and full schedules in a side sheet/detail page. On mobile use a detail page, compact tab navigation and stacked summaries; tables become labelled lists. Preserve filters on return.

Use Tabs for distinct tasks; RadioGroup for exclusive choices; Checkbox for independently required acknowledgements; Select/Combobox for long lists; Alert for persistent blockers; Dialog for consequential confirmation; toast for brief completed-action feedback. Important financial warnings and errors must not live only in a toast. Loading is not zero. Use locale-aware currency/dates, keyboard access, visible focus, named icons and reduced motion. Supply English and Mandarin from the same status/message keys.

## 4. Journeys from each perspective

### A. Viewer who never creates

Discover → watch/Shorts → follow/save → optionally subscribe to platform Premium → manage subscription.

- No creator onboarding, credits purchase or payout setup required.
- Premium remains platform-wide; following a channel is free. No per-channel subscription pricing is introduced.
- Sponsored reach is labelled and never represented as organic reward-eligible engagement.
- If choosing “Create,” preserve the watched video return location; do not enroll automatically in rewards.

Success: viewer can complete browsing and membership tasks without encountering reward configuration.

### B. New creator generating their first video

Switch to Studio → create project → Story/Canvas → review generation cost → generate → Edit → preview/export → publish → optional reward enrollment.

- Explain credit cost before generation; show job states and retry/refund outcomes. Never pitch generation as buying chances to win.
- Publishing accepts finished generated video, editor export or upload in one consistent flow.
- Show title, thumbnail, format, rights acknowledgement, public/Premium visibility, then reward eligibility summary.
- Enrollment explains variable funded rewards, qualification, earning window, vesting and availability; exact scoring weights stay private.
- Eligibility snapshot records terms and policy version. Identity setup is requested when required, without blocking ordinary creation unnecessarily.
- After publication show one success surface: Watch video / View content; reward window is secondary. No fake opening estimate.

Success: published video keeps project/source linkage, its first-publication timestamp and an understandable reward state.

### C. Creator uploading existing original work

Content → Upload → validate media → edit metadata/access → attest rights → publish → review reward status.

- Same eligibility path as generated output; no generation-credit purchase requirement under the proposed pilot.
- Explain why duplicate, unsupported, restricted or non-original media cannot qualify; preserve the draft.
- Replacing media creates a revision; it does not reset or extend the earning window. Re-uploads are deduplicated for reward eligibility.

Success: an eligible upload earns under the same published program rules without purchasing AI services.

### D. Creator monitoring a successful video

Earnings overview → Videos → active video → evidence and reward detail → daily confirmed batch → vesting schedule → payout.

- Audience views, qualified engagement and rewards are separate labelled figures.
- Detail shows publication/window dates, evidence cutoff, last calculation, policy version and broad scoring categories.
- Window countdown uses server time; at expiry it becomes “Earning window ended,” while outstanding reconciliation and vesting continue.
- A creator may earn more than they spent; the UI never frames this as a promised return.
- Final confirmed amount can differ from an earlier estimate. Explain the adjustment category and show a support reference.

Success: creator can distinguish forecast, committed liability, vesting and actual payment.

### E. Smaller creator with little or no reward

Content performance → practical retention/originality guidance → optional published support milestone → verified completion → promotional credit receipt.

- Explain insufficient evidence, low qualified performance or exhausted allocation without implying a near win.
- Milestones reward useful original work, not total spend, previous losses, referrals who purchase, or repeated low-quality uploads.
- Show milestone criteria, budget availability, limits, verification status and credit restrictions before participation.
- Zero rewards remain possible; no “spend more to unlock your earnings” messaging.

Success: useful feedback and a bounded, disclosed support option independent of cash-loss history.

### F. Creator who cannot receive payment yet

Earnings → blocker explanation → account setup or verification → return to earnings → scheduled payout.

- Distinguish unverified identity, unsupported country, below threshold, account review, unresolved dispute and temporary provider outage.
- Show what the creator can do, what the platform must do, and whether earning/vesting can continue under policy.
- Never route a creator into further purchases to unlock existing earnings.
- Payout destination changes require recent authentication/MFA, notification and a review hold under published policy.
- Failed or ambiguous provider responses remain pending reconciliation; do not retry under a new transfer identity.

Success: money is neither lost nor marked paid prematurely, and the next action is clear.

### G. Returning, inactive or departing creator

Return → catch-up summary → ended windows / remaining vesting / payout state. Account deletion → explain financial retention and outstanding balances → deliberate confirmation.

- No new publication required to release already confirmed funds.
- Pausing generation or cancelling Premium does not revoke earned rewards.
- Unpublish/delete stops new qualifying activity from its effective time; it does not silently confiscate confirmed rewards.
- Policy violations and chargebacks use documented holds/adjustments and appeal records. Do not silently mutate financial history.
- Republish does not restart the original window. Cross-account media copying cannot create fresh eligible windows.

Success: creator retains access to financial records and a fair settlement path.

### H. Support, trust/safety and finance

Support: inspect user-safe timeline → explain state → collect evidence → escalate/appeal. Cannot edit scores or balances.

Trust/safety: review suspicious evidence → apply scoped hold with reason → approve/reject evidence → create appeal result. Confidential fraud signals are not exposed in creator messages.

Finance: reconciled funding → draft daily allocation → liability/coverage preview → independent approval → commit batch → monitor vesting → approve monthly payout → reconcile provider.

Administrator: version policy prospectively → compare changes → validate budgets → preview affected cohorts → approve effective date. Cannot retroactively rewrite a confirmed batch through a policy edit.

Success: every material action has actor, reason, policy version, timestamp and immutable evidence reference. No one-click arbitrary money edits.

### I. Advertiser/sponsor (later phase)

Campaign brief → clearly labelled sponsored placement/content agreement → payment reconciliation → delivery verification → sponsor-funded award → normal confirmation/vesting or explicit contract schedule.

- Separate gross receipts, fees, refunds and eligible funding; no unfunded promised campaign budget.
- Paid impressions are excluded from organic performance rewards by default.
- Phase-one users see no empty advertiser portal or misleading sponsorship balance.

## 5. Lifecycle and accounting contract

Video: draft → processing → published → eligible/ineligible/review → window ended. Reward eligibility is independent of visibility and media readiness; publication must succeed before a window exists.

Reward batch: draft estimate → evidence frozen → funding verified → approved → confirmed/reserved → vesting → vested → reserved for payout → submitted → transfer confirmed/reconciled. Held, rejected and correction records are explicit branches, not silent deletion.

For principal A in integer cents, confirmation time t0 and duration D = 30 days:

`vested(t) = floor(A * clamp(t - t0, 0, D) / D)`

At t0 nothing is vested; at t0+D all cents are vested. Calculate using integer-safe arithmetic and UTC instants. The dashboard can render current vested value on read; a job records durable releases with unique batch/time identities so retries cannot duplicate them.

For each funding source and currency:

`spendable_pool = reconciled_committed_funding - confirmed_obligations - committed_support_cost - other_source_reservations`

Transfers between reserve and performance allocations are explicit ledger entries. Confirming a batch atomically reserves its full liability. Vesting changes maturity, not the amount of backing required. Unspent customer credit deposits are not automatically available revenue. Promotional credits reserve expected service cost and do not become cash liabilities at face value.

Allocation uses versioned accepted weights and deterministic rounding; sum of awards never exceeds the batch budget. Existing AllocateRewards can be reused as arithmetic, not as revenue reconciliation. Late evidence uses a separate funded correction; it cannot rewrite a confirmed snapshot. Refunds, disputes and sponsor reversals create compensating entries and explicit holds; insolvency cannot be hidden by changing an estimate label.

## 6. Data ownership, security and load

PostgreSQL is authoritative for funding, immutable postings, reward policies, enrollment, first publication, eligibility, snapshots, confirmations, vesting, support grants, holds, appeals and payouts. Financial facts and append-only evidence must survive Redis loss. Enforce immutable financial values with database permissions/triggers plus balanced transaction checks; mutable workflow status is separately audited.

Redis: short-lived read caches, request limits, dedup hints and bounded worker coordination only. A PostgreSQL uniqueness constraint/transaction must still prevent duplicate allocation, vesting and payout even if a Redis lock expires. Public views and reward eligibility have separate aggregates.

Workers: persistent jobs/outbox, bounded concurrency, idempotency, backoff with jitter, retry ceiling and dead-letter review. No recursive HTTP calls or browser-triggered closing jobs. Provider timeout leaves an ambiguous financial action for reconciliation, not automatic replacement.

UI: one summary request on entry; poll only active processing/confirmation screens with visibility-aware backoff and a stop condition. No full-history polling. Paginate batch/video/activity records; index eligibility windows, owner/date, batch state and source/currency. Use server-provided freshness timestamps and ETags or equivalent cache validation.

## 7. Master checklist

Existing foundations below are source-inspected, not newly executed payment tests. All unchecked boxes are work or activation still required.

### Foundation retained

- [x] Integer allocation, total pool cap and deterministic remainders exist.
- [x] Monthly close deduplication/input hash exists.
- [x] Earnings postings, adjustment entries, dispute controls and payout batch records exist.
- [x] Real playback metrics are separate from financial reward weights.
- [x] Creator analytics and payout setup surfaces exist.

### Policy and disclosures — launch blockers

- [ ] Approve/version funding basis, percentage, source splits, caps, reserves and unused-pool treatment.
- [ ] Approve eligibility, uploaded-content parity, age/region rules and evidence requirements.
- [ ] Approve 30-day window, daily confirmation, 30-day vesting, payout cadence and correction deadlines.
- [ ] Publish broad score factors and funding categories without exposing proprietary weights/fraud signals.
- [ ] Publish estimates, support-credit restrictions, hold/appeal and account-departure rules in English/Mandarin.
- [ ] Complete provider/country and applicable participation/payment policy reviews before enabling money flows.

### Funding and ledger — launch blockers

- [ ] Link reward funding to reconciled eligible consumed-service revenue and external revenue separately.
- [ ] Implement source/currency ledgers and full-liability reservation at confirmation.
- [ ] Enforce immutable amounts/evidence, compensating adjustments and auditable policy versions.
- [ ] Reject duplicate settlement, insufficient backing and concurrent over-allocation transactionally.
- [ ] Reconcile refunds/chargebacks, source reversals and payout reservations without double subtraction.

### Eligibility and scoring

- [ ] Store first successful publication, 30-day end, media fingerprint, enrollment and policy version.
- [ ] Define Premium/public, private/unlisted, moderation, deletion and re-upload eligibility behavior.
- [ ] Build qualified aggregates excluding self/bot/replayed/paid traffic and quarantined evidence.
- [ ] Add versioned normalization/scoring with evidence minimums; no spend/future-purchase ranking boost.
- [ ] Add evidence provenance for optional external signals; omit unsupported inputs rather than invent them.
- [ ] Freeze daily snapshots; support delayed evidence, zero evidence and funded corrections.

### Reward lifecycle

- [ ] Persist provisional estimates separately from confirmed liabilities, with freshness/cutoff metadata.
- [ ] Implement daily preview/approval/confirmation with atomic funding reservations.
- [ ] Implement exact linear vesting, partial releases, holds and payout reservations.
- [ ] Migrate legacy cliff-held earnings without reducing already promised balances or backdating new rules.
- [ ] Add monthly payout scheduling, threshold/country eligibility and provider reconciliation.
- [ ] Keep payment gateway collection separate from actual cash payout-provider support.

### Creator and viewer UX

- [ ] Keep viewer navigation free of forced creator/reward onboarding.
- [ ] Integrate optional enrollment and eligibility explanation into the existing publish flow.
- [ ] Build two simple Earnings tabs, expandable secondary sections and per-video detail with distinct estimate/vesting/available semantics.
- [ ] Add understandable empty, stale, unavailable, held, rejected, zero and failed states.
- [ ] Add accessible setup handoff/return, payout timeline and support/appeal entry.
- [ ] Add milestone support-credit flow without spending/loss-triggered pressure.
- [ ] Deliver English/Mandarin, keyboard, screen-reader, mobile, light/dark and large-number checks.

### Admin and operations

- [ ] Add separate admin sections: Funding, Batches, Eligibility/reviews, Liabilities/vesting, Payouts, Support grants, Policies, Audit.
- [ ] Require role separation, recent MFA, reasons and independent approval for money-moving actions.
- [ ] Add dry-run reconciliation and liability coverage reports before each confirmation/payout.
- [ ] Provide pause switches independently for new estimates, confirmation and payouts; preserve reads and accrued obligations.
- [ ] Alert on funding mismatch, delayed batches, abnormal scoring, payout ambiguity and backlog saturation.
- [ ] Keep operational guides private; publish only creator-facing reward explanations in Fumadocs.

### Verification and release

- [ ] Test exact window/vesting boundaries, cent rounding, zero inputs and overflow.
- [ ] Test duplicate events/jobs, simultaneous confirmations and concurrent refunds/payouts.
- [ ] Test destination changes, deleted/unpublished media, policy changes and inactive creators.
- [ ] Test source isolation, insufficient funding, negative corrections and paused services.
- [ ] Run shadow allocation with no payable entries; reconcile against independent calculations.
- [ ] Run a limited explicitly enrolled pilot only after activation gates pass.
- [ ] Record technical and UI/UX audit evidence; broaden availability only after reconciliation and support review.

## 8. Release sequence and success measures

1. Accounting/policy model and migration tests; no new public earning claims.
2. Shadow evidence/scoring/estimates and internal budget coverage dashboard; no money confirmations.
3. Creator views showing honest program status; enrollment remains gated.
4. Limited eligible pilot with funded daily confirmations and full liabilities reserved.
5. Monthly payout reconciliation and audit; then continuity credits; advertiser funding later.

Track creation completion, publication completion, viewer retention, eligible-video ratio, support resolution time, estimate-to-confirmed variance, settlement delay, funding coverage and duplicate-payment count. Observe spend independently; do not optimize payouts to manipulate a user into further purchases. Budget utilization and creator concentration inform prospective policy review, not hidden retroactive outcome engineering.
