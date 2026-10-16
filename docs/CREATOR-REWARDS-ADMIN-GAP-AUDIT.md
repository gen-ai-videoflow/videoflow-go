# Creator economy implementation and remaining release gates

Updated 2026-10-16. Canonical API/web/admin/docs feature branches. This supersedes the earlier foundation-only audit. Runtime activation and real-money proof are separate from implemented software.

## Resolved engineering gaps

- [x] **Revenue engine:** independently approved settled USD receipts and explicit fees/taxes/refunds/provider/media costs; FIFO purchased-credit consumption; promotional credits excluded; closed-day net recognition, loss carry-forward, funding percentage/reserve/cap, idempotent funding. Missing evidence stays pending. Stripe statement intake is an explicit read; token quantities never imply USD realization.
- [x] **Funding recovery:** refund/dispute freezes, immutable source holds, independent fresh-funding replacement, recursive replacement coverage and creator holds. Confirmed obligations remain intact.
- [x] **Evidence/scoring:** frozen per-video UTC-day snapshots, verified non-owner viewers, organic session provenance, exclusions, deduplication, bounded retention, explicit originality/trust review, normalized versioned weights and integer allocation. Spending and prior losses do not increase scores.
- [x] **Estimates:** derived from reviewed evidence and available funding; no estimates without qualifying data; confirmed slices excluded. Current policy/effective time prevents retrospective reuse. Day locks recheck conflicting confirmations. Drafts remain provisional.
- [x] **Durable jobs:** persistent daily keys, bounded claims, lease/attempt fencing, backoff and five-attempt review state. Shadow jobs run while money is paused. Worker creates scheduled drafts/confirmations only under approved policy and deployment switches; page requests never run the engine.
- [x] **Corrections:** positive corrections reserve fresh funds and vest from approval; negative corrections append debt and withdrawal holds. Independent approval, audit, notifications and appeals preserve original principal, schedules and measured views.
- [x] **Payout preparation:** configurable threshold/cadence, bounded per-account durable outcomes and retries, maturity catch-up, holds and destination checks. Scheduled preparation never submits transfers. Known ambiguous Stripe transfers can be reconciled by GET with exact amount/currency/destination/metadata checks and rejection of partial/full reversals.
- [x] **Policy administration:** typed bounded policies, optimistic revisions, independent approval, immutable history, snapshot comparison, shadow preview and separate estimate/confirmation/payout switches. Earning windows are captured on first publication; confirmed schedules retain their original duration. Unchanged terms preserve existing enrollment; changed durations require new acceptance.
- [x] **Commercial pricing:** independently reviewed USD pack registry, target margin and fee/media reserves, cost-ceiling preview, revision/history, dynamic catalog and runtime margin validation. Existing pending Stripe/CryptoRails invoices preserve their original sale snapshot and retry identity.
- [x] **Legacy reconciliation:** quarantine unbacked positive balances, independently reserve fresh per-earning backing, reject unbacked provider submissions and retain explicit historical transfer exceptions after verified provider evidence. Paid history remains unchanged.
- [x] **Finance UX:** Funding / Calculations / Liabilities / Payouts / Reviews / Policies; source/recipient inspection, run evidence, failed jobs, holds, appeals, corrections, creator financial timeline and actual JSON exports. Recent MFA and server-side roles protect mutations.
- [x] **Audience facts:** privacy-thresholded creator/video cohorts, organic/paid/unclassified provenance, append-only invalid-event exclusions and aggregate replay preserving unverifiable legacy counts. No arbitrary fake view counter controls.
- [x] **Discovery/search:** validated versioned weights, bounded candidate/history/page limits and freshness; unified legacy/current feed policy with response compatibility; expiring creator/video boosts, suppressions, query pins, synonyms, Mandarin literal matching, preview, index health and access checks. Overrides cannot reveal private/blocked/inaccessible Premium content.
- [x] **Continuity support:** independently reviewed service-cost budgets, milestone claims/reviews, finite grants/creator caps, separate nonwithdrawable promotional lots, expiry, earliest-expiry reservations and purchased/promotional split. Reserved lots settle safely after expiry; conservative backing is retained.
- [x] **Sponsorship:** dedicated settled funding sources, finite contracts, creator terms acceptance, labelled paid watch sessions, actual delivery reports and independently approved sponsored awards. Paid sessions cannot become organic reward evidence. Sponsor money cannot be reused by ordinary allocation.
- [x] **Creator communications:** durable deduplicated in-app confirmation/release/hold/payout/support/appeal updates and read state. Unsupported external delivery is recorded honestly.
- [x] **Simple creator UX:** Earnings and Videos only; available balance first, relevant unlocking amount, no fabricated potential earnings, optional payment/program/audience/support/sponsor/history disclosures. English/Mandarin reward copy and public guides.
- [x] **Swagger and parity:** private authenticated OpenAPI; self-hosted searchable Swagger without write actions or external validators; 263 operations and65 schemas; live chi routes checked both directions against generated contract; reference resolution tests. Public Fumadocs remains user-facing.

## Policies rather than unrestricted database edits

Admin controls intended business policy through validated schemas, revisions, reasons and review. Measured views are corrected from original evidence; ranking curation is separately recorded; rewards use funded allocation. Secrets and arbitrary SQL/scripts are not editable settings. Historical sources, confirmed principal and accepted terms are not overwritten to manufacture a result.

## Remaining external proof and activation gates

- [ ] Connect real payment/model/media credentials and prove actual receipts, direct costs, refunds and provider cash coverage. Software does not substitute for statements or independently reviewed costs.
- [ ] Verify CryptoRails actual settled USD conversion evidence. Raw token invoice amounts are insufficient.
- [ ] Reconcile historical legacy obligations against real backing; preserve exceptions and paid history. Do not invent historical cash or migrate provisional amounts into confirmed rewards automatically.
- [ ] Review commercial policy, country/provider availability and creator terms before enabling cash programs. Global browsing does not imply every payout provider supports every country.
- [ ] Run a funded shadow comparison, independent budget reconciliation and limited enrolled pilot. Test distribution/fraud behavior using actual eligible content before general activation.
- [ ] Test real passkey/authenticator ceremonies and provider transfer/bank-arrival reconciliation in the chosen deployment. A mocked transfer and an MFA database fixture are not those tests.
- [ ] Configure external notification delivery if desired; this increment provides truthful in-app updates, not an activated email/push service.
- [ ] Complete application-wide locale selection and assistive-technology testing. Reward copy supports English/Mandarin; this does not imply every existing application surface is localized.

Country/device audience attribution is not collected by this increment. Legacy traffic remains explicitly unclassified. Database index rebuilds remain controlled deployment operations, with transactional updates and diagnostic status in Admin. No fabricated external signals, full bot-detection claim, guaranteed earnings, or bank-deposit claim is made.

## Verification record

Detailed test/build and browser results are recorded in CREATOR-REWARDS-EXECUTION.md. New module suites passed independently; final integrated verification follows all migration and contract changes. Earlier full-suite attempts failed at linking because of temporary filesystem quota, not because they passed. Keep REWARDS_ENABLED and CASH_PAYOUTS_ENABLED off until the separate release gates are satisfied. No AWS provisioning or live transfer is required to review this implementation.
