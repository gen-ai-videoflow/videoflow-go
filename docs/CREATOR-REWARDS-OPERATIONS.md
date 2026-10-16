# Creator rewards operations (private)

Do not copy this into the public documentation collection.

This branch implements a reviewed, funding-backed pilot foundation. It does not activate the complete commercial program. Read CREATOR-REWARDS-EXECUTION.md for remaining gates.

## Schema and compatibility

Apply 041–045 with the existing owner-only cloud-migrate process before updating runtime services. Back up and verify restoration first. Do not apply these migrations to production merely to preview the UI. Legacy earnings retain their existing statuses and cliff settlement rules; no migration invents historical funding backing. New `vesting:` earnings are immediately matured entries, distinct from legacy pending.

## Funding workflow

Finance proposes a unique funding receipt reference, eligible USD net reward budget and an evidence reference. A different finance administrator checks the actual reconciliation and approves the source. This is a manual evidence workflow, not automated settlement verification. Do not fund from unspent customer deposits, gross invoice values, missing cost estimates or disputed receipts.

After approved commercial rules exist, a qualified evidence reviewer prepares a closed UTC activity day's video weights and references the supporting evidence. The draft preserves canonical inputs and recipient amounts. Another finance administrator inspects source coverage, input hash, recipient allocation and policy before confirmation. Confirmation reserves the full amount atomically. Support sources cannot fund cash reward batches.

## Vesting service

`make cloud-rewards` runs the separate maturity worker. Use the restricted runtime role and ordinary cloud configuration. It checks migrations and, outside development, runtime-role restrictions. Each bounded pass visits up to 100 accounts with partially released confirmed allocations. Cursor progression prevents a single failure from starving later accounts. Failed passes retry after a delay; logs require operational monitoring. The worker does not approve new rewards or transfer money.

Maturity continues for confirmed obligations even if new rewards are paused. `PreparePayout` catches up maturity under the existing creator locking protocol. Never pass a browser-supplied time to ReleaseRewardVesting.

## Payment and holds

Existing payout preparation/execution remains explicit operator work, including threshold, provider account eligibility, disputes and unique transfer identity. A transfer is not bank-arrival confirmation. Admin reward-hold mutations share the same creator advisory lock as payout execution; all hold changes require reasons and audit entries. Do not change balances directly.

## Activation boundary

Keep the default policy inactive and REWARDS_ENABLED/CASH_PAYOUTS_ENABLED off until the execution ledger's outstanding release gates are resolved. The two boolean flags alone are not an approval or reconciliation process. Automated scoring, continuity grants, late-evidence correction and monthly scheduling are not enabled by this branch.
