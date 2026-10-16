# Creator rewards execution ledger

Plan: aggregate repository docs/superpowers/plans/2026-10-16-creator-rewards.md.

- Base: d87f1d2. Canonical implementation starts in API feature branch, no production activation.
- Ruling: funding policy is unapproved; ship durable funding/confirmation primitives and disabled defaults before wiring automated revenue reconciliation. Never classify an administrator-entered number as verified provider funding.
- Ruling: do not expose private estimates without an implemented scoring/evidence source; use insufficient_data until qualified evidence exists.
- Task status: core accounting and user/admin surfaces implemented; complete commercial rollout remains gated by the outstanding items below.

## Implemented and verified

- Migrations 041–045 isolate new confirmed liabilities from legacy earnings; preserve first publication; make source, allocation, input snapshot, release events and vested principal immutable.
- Reviewed funding requests require a different reconciling administrator. Daily draft batches preserve canonical inputs; independent confirmation atomically reserves the complete liability. Existing legacy financial history is unchanged.
- Exact, overflow-safe 30-day linear vesting and idempotent release events feed the existing payout ledger. New `cmd/cloud-rewards` materializes maturity in bounded cursor passes. Payout preparation also catches up maturity. Holds and pending negative adjustments apply before payout.
- Owner-authorized creator program/enrollment, summary, paginated videos, per-video schedules and payout history endpoints; finance/owner admin review endpoints, existing recent MFA and source/recipient preview.
- Web: two simple Earnings tabs with secondary disclosures, nullable honest estimates, per-video windows/schedules, payout setup/history, enrollment gating, lazy audience analytics and publication handoff. Removed duplicate parent earnings requests. Wire types generated from API contracts.
- Admin: funding proposals/reconciliation, allocation drafts/recipient preview/independent confirmation, policy status. Explicit reward proxy allowlist has a Node test.
- Public English/Mandarin reward guides; operator material remains excluded. Fixed relative MDX Card links and added an export regression check.
- Fixed web npm build/dev scripts so cloud mode also reaches Vite, retaining Node cloud adapter/API gateway.

## Evidence

- Initial vesting test failed with missing VestedCents; lifecycle tests failed with missing implementation before the new service modules were added.
- `go test ./... -count=1` passed against a separate PostgreSQL database (`videoflow_rewards_test`), with all packages successful. Test output: /tmp/reward-all-tests.log.
- `go test -race ./internal/cloud -run TestReward -count=1` passed.
- Targeted reward tests cover funding concurrency, self-approval rejection, deterministic retries, immutable principal/inputs, vesting rounding/endpoints, first-publication preservation, expired windows, private reads, disabled enrollment and pending-refund availability.
- Web and admin `npm run check` report zero errors/warnings; web cloud Node-adapter production build and admin production build passed.
- Docs types/build/content/export passed: 32 localized pages, 124 explicit steps; exported relative MDX Card links are resolved.
- Browser: authenticated dedicated preview at :10550, 1280×800 Overview and 390×844 Videos; tabs switch, Payouts shows disabled setup correctly, no page-level horizontal overflow. This is an isolated test database, not production rollout. Physical provider/authenticator and live money flows were not exercised.
- Fresh code review found pending-refund availability mismatch and missing canonical allocation inputs. Both were fixed with regression coverage and explicit recipient/input preview.

## Remaining release gates (not claimed complete)

- Automatic consumed-revenue/cost reconciliation adapters and source reversal/shortfall workflows. Funding currently requires independent manual evidence review; a receipt reference alone does not prove provider cash.
- Automated multifactor scoring, originality/reupload fraud qualification, external evidence adapters and provisional estimate generation. Estimates intentionally remain null; manual reviewed weights are not described as an automatic model.
- Dedicated late-evidence correction and appeals workflows, policy-management UI and independent money-moving controls beyond the implemented confirmation workflow.
- Monthly payout scheduling/notifications, continuity promotional-credit accounting/expiry/milestone UI, advertiser campaign operations and cross-country activation review.
- Exhaustive admin visual/keyboard/screen-reader and Mandarin application-locale integration review. Creator component has translated copy but the existing application has no full locale switcher; do not claim full app localization.
- Full liability coverage reconciliation for preexisting legacy balances; no historical backing was fabricated in migration.

Ruling: keep the new policy inactive and existing REWARDS_ENABLED/CASH_PAYOUTS_ENABLED defaults off. The core is reviewable and tested, but the entire commercial launch checklist is not complete. No AWS provisioning, production migration, production deployment or real transfer was performed.

## UX refinement — 2026-10-16

User requested simpler earnings and progressive disclosure. The default now emphasizes available earnings and relevant unlocking funds; only Earnings/Videos remain as tabs. Payment history/settings, program rules and audience analytics expand on demand. Null estimates are omitted; important holds and payment unavailability remain visible. Optional video/history requests are lazy. This is recorded as durable product guidance.
