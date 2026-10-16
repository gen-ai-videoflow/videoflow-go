# 6eek creator rewards operations (private)

This runbook is for authorized operators. Keep it outside the public documentation collection. The code implements a bounded, reviewed rewards pilot; deployment, real provider funding and commercial approval remain separate release gates. Use the current execution ledger and gap audit for launch decisions.

## Deployment and access

Apply all pending cloud migrations from the deployed version using the existing owner-only `cloud-migrate` process before updating runtime services. This feature spans the reward foundation, policy/calculation queues, revenue evidence/recovery, sponsorship and support/correction migrations; applying only 041–045 is insufficient. Back up the database and verify restoration first. UI previews do not authorize production migrations.

Run the API and `cloud-rewards` worker with the restricted runtime role. Outside development the worker checks that role. Finance and owner routes require the existing administrator authentication and recent MFA. Reasons and independent approvals are recorded; a proposer cannot approve their own policy, funding evidence, recovery or correction. Never use SQL or audience-counter edits as an operator workflow.

The default reward policy is inactive, funding and daily budget are zero, and estimates, confirmations and payouts are paused. Deployment gates `REWARDS_ENABLED` and `CASH_PAYOUTS_ENABLED` remain independent of approved policy switches. Do not activate production merely because a form or test passes.

## Prospective program policy

In Rewards → Policies, inspect effective values and revision before editing. Edit the selected policy, review the change and reason, and submit a proposal. A different administrator approves it. Compare approved snapshots in the history disclosure. Conflicting revisions require refreshing and reviewing the latest policy again.

Controls cover earning window, vesting duration, minimum qualified evidence, quality weights, funding/reserve basis points, daily cap, payout threshold and cadence, and separate operational switches. Quality weights total 10,000 basis points. Approved policy versions preserve the policy used by confirmed liabilities. Matching terms may carry enrollment forward; changed earning/vesting terms require fresh acceptance. Publication fixes each video's earning window; later policy edits do not restart that clock.

New live calculation evidence must fall on or after the selected policy's creation time. Historical readiness inspection uses shadow mode. Route late evidence or errors affecting confirmed awards through the correction workflow rather than replaying financial history under a new policy.

## Settled receipts, actual costs and funding

Open Funding → Consumed revenue, actual costs and receipt reconciliation. Candidate records are facts to verify, not approved reward backing. Lists are bounded; work through the oldest unreconciled consumption days and keep the source statements outside the application with stable evidence references.

1. Select a paid purchased-credit receipt. Stripe candidates must be paid USD purchases with a matching purchased-credit grant. The optional **Read Stripe statement** action makes an explicit provider GET and returns an unapproved draft only when an available USD balance transaction, actual payment fee, tax and refund fields are present.
2. For CryptoRails, verify a settled credit invoice, settlement receipt and matching credit grant. Its token amounts and quoted invoice price are not proof of realized USD. Supply actual USD conversion/realization evidence, fees, taxes and refunds. The application does not invent a fiat conversion rate.
3. Enter every actual receipt amount in USD cents. Missing fees, taxes or refunds remain missing; enter zero only when the statement proves zero. Include the UTC settlement time and evidence reference. A different administrator reviews and approves the immutable receipt lot.
4. For each succeeded job consuming purchased credits, enter independently reconciled actual provider and media/storage costs. Both amounts must be explicit, including attested zero. Budget ceilings, model quotes and estimates are not cost statements. A different administrator approves the cost evidence.
5. Review and run a completed UTC day's reconciliation. The service maps succeeded consumption FIFO to previously acquired, approved purchased-credit lots. Purchased consumption is `credits - promotional_credits`; promotional consumption cannot become receipt revenue. Unspent purchased-credit deposits do not become reward funding.

A missing receipt, missing approved cost statement or insufficient lot coverage leaves the entire day `pending_reconciliation`; partial FIFO consumption and funding are rolled back. Earlier unrecognized jobs block later days. The current bounded pass refuses days exceeding 10,000 purchased-credit jobs and requires operator review rather than silently truncating them.

Recognized receipt cents use cumulative integer proportions so consumption cannot recognize a receipt twice or create rounding money. Actual fees, taxes and recorded refunds reduce receipt contribution; actual job costs reduce the consumed contribution. Losses carry forward. Eligible funding is the positive net contribution after prior losses, multiplied by funding basis points and then by the unreserved share, capped by the daily budget. The day records its policy revision and amounts. A unique day/source key makes retries idempotent. A zero funding share or zero cap creates no reward source.

The separate manual funding proposal/reconciliation workflow remains available for independently verified eligible generation contribution, advertising funds and support budgets. Use unique receipt references and two administrators. It is a reviewed evidence workflow, not automatic provider settlement proof. Support funding cannot back withdrawable cash rewards; committed sponsorship sources remain separate from ordinary organic allocation.

## Quality reviews and calculation jobs

In Calculations, review originality, re-upload/trust evidence and explicit eligibility for each video. Enter originality and trust as whole basis points with a reason and evidence reference. Unreviewed videos do not enter automatic allocation. Load an existing review before editing its revision; stale updates must be refreshed. These judgments do not rewrite measured audience events.

Choose a closed UTC activity day and shadow or live calculation. Queue identity is day, mode and approved policy version. Inspect queued/running/succeeded/failed status, attempts, last error and frozen evidence in the calculation disclosure. The worker claims one bounded job, leases it and retries failures with backoff up to five attempts. Exhausted work requires operator investigation; do not reset attempts or rewrite snapshots directly. Claim fencing prevents stale workers from overwriting a newer run's status.

Accepted organic evidence requires authenticated, verified distinct viewers, matching watch sessions, minimum accepted watch time, publication/earning-window eligibility and enrollment. Owner watches, excluded events and paid/sponsored sessions do not become organic reward views. Originality/trust require an explicit eligible review. Purchasing volume and loss history are not scoring inputs.

Shadow processing remains available while new money operations are paused. It records reproducible evidence without drafting payable obligations. The detail view's allocation preview uses **current** available backing and the frozen scoring policy: it is a simulation, not a historical bank balance, promise or reservation. Refreshing a preview can change it as other work consumes funding.

Live calculation freezes evidence and prepares reviewed allocation drafts against eligible available sources and the daily cap. Inspect source evidence, policy, canonical input hash and recipient amounts before confirmation. Manual reviewed-weight drafts are an advanced workflow, separate from measured view history. Confirmation atomically reserves the full liability and requires an independent actor. The enabled scheduler can confirm its own system-generated successful drafts through a distinct system confirmation actor; manually submitted drafts remain in the finance review workflow.

## Worker and vesting

`make cloud-rewards` runs the dedicated bounded worker. A pass releases existing vested obligations, drains readiness shadow work, and, when activation gates permit, reconciles yesterday's revenue, queues/processes calculations and confirms eligible system drafts. It also prepares scheduled payout review batches when cash preparation is enabled. It never submits provider transfers.

The vesting pass visits bounded account pages and advances its cursor so one failure does not permanently starve later accounts. Monitor worker logs, failed calculation attempts and funding/review queues; a successful process launch is not evidence of completed financial reconciliation. Missing historical receipt or cost evidence needs finance intake; the yesterday scheduler does not replace a historical catch-up review.

Confirmed allocations retain their recorded vesting duration. Releases are append-only and idempotent, with `vesting:` earnings distinct from legacy pending earnings. Maturity continues for existing obligations when new estimates or confirmations are paused. Payout preparation catches up maturity under the creator locking protocol. Never pass browser-controlled time to vesting release routines.

## Refunds, source coverage and recovery

Increasing a known Stripe refund, an unresolved dispute or an independently approved receipt-lot reversal freezes linked backing. The source's recorded amount and confirmed reserves remain immutable. Creators with affected confirmed awards enter reward holds. Source-hold creation shares payout execution's creator locks. Do not delete a hold merely to make a payout succeed.

In Funding, inspect coverage holds and the reserved liability at risk. Recovery uses a new evidence proposal with the held source, a fresh independently reconciled replacement source, reference and reason. A different administrator approves it. Approval atomically reserves replacement money equal to the original source's confirmed reserve and writes an immutable recovery link. Support budgets, held sources and campaign-committed sources cannot be selected as ordinary replacement backing.

The original source remains unavailable for new awards. Existing confirmed obligations are unchanged. Creator holds can be released through the reasoned account-review workflow only after the full replacement chain has valid backing. If replacement backing is later reversed, affected creators are held again. Recovery does not prove bank settlement or silently forgive a debt.

Raw source reserve totals include replacement reservations alongside original source records. Inspect `recovery_reserved_cents`, source holds and confirmed allocations separately; a raw funded-minus-reserved figure alone does not establish recoverable coverage or provider liquidity.

## Corrections, appeals and support

Use Reviews → Late evidence corrections and funded adjustments for a specific creator and original confirmed batch. A positive correction requires fresh eligible backing and independent approval; it preserves the original obligation and creates a separate correction using the recorded vesting policy. Negative corrections record a separate adjustment and debt/hold consequences rather than rewriting the original award. Approval checks bounded clawback and existing payout state. A prepared or ambiguous provider payment may require separate recovery review; never assume a negative ledger entry reversed an external transfer.

Creators can submit appeals. Review the claim and evidence, record a resolution and use the correction workflow when an upheld appeal requires a financial change. An appeal resolution alone does not create a cash credit. Maintain reasons for holds and releases and inspect the creator financial timeline before acting.

Support programs require a separately reconciled support source, bounded grants, prospective criteria and independent activation. Approved support claims issue expiring promotional generation credits, not withdrawable earnings. Promotional-credit consumption remains separate from purchased-credit revenue. Sponsorship awards use their separately committed source and reviewed campaign/creator acceptance workflow; paid delivery does not become organic evidence.

## Payout review and transfer reconciliation

The payout threshold and cadence control preparation of reviewable batches. Cadence periods are aligned to UTC Unix-epoch intervals; they are not a promise of a particular calendar-month bank arrival. A period captures its candidate account set. Preparation processes at most 100 account outcomes per pass, preserves durable results and retries failed accounts up to five attempts. Inspect failures and exhausted accounts. Holds or below-threshold accounts can be skipped for that period; later eligibility is not a guarantee of an immediate additional payment.

Preparation catches up vesting, checks connected-account eligibility and holds/disputes, reserves eligible ledger entries and creates a batch. It never calls a transfer provider. Review the creator's allocations, released amounts, corrections, reserved earnings and payout records in Liabilities → Creator financial timeline. Bounded account history and JSON exports show recorded data; they are not permission to edit financial history.

Provider execution is a separate explicit operator action and remains subject to cash activation, supported country/currency, current account capabilities, holds, balance checks and provider idempotency. The first submission time is preserved. Old ambiguous submissions beyond the safe idempotency retry window require reconciliation, not another POST.

For a known ambiguous transfer, use Payouts → Reconcile an ambiguous provider transfer. Enter the payout batch ID, known `tr_` transfer ID and review reason. The server makes a **GET only** provider check for identity, amount, currency, destination, batch metadata and any full or partial reversal before recording the transfer. It does not issue a new transfer. A transfer to a connected payment account does not prove a bank deposit. Provider-side reversals and subsequent bank reconciliation still require monitored operational evidence.

## Legacy balances and external release gates

Legacy earnings retain their original references, pending/cliff rules and statuses. Unpaid positive legacy entries without independently reserved backing are excluded from spendable balances and new payout selection; `legacy_unbacked_cents` reports that quarantine separately. In Liabilities → Legacy unpaid balances and independently funded clearance, finance proposes a specific original unpaid earning and a fresh eligible source with evidence and reason. A different administrator, who is not the recipient, approves it. Approval verifies the unchanged original amount and atomically reserves that amount from actual available source funds. The immutable backing record is not evidence of historical revenue. Source reversals and funded recovery protections also apply to these legacy-backed obligations.

Already-paid history is preserved. An already-submitted ambiguous legacy transfer must be checked against the actual provider GET response: its immutable transfer exception records historical shortfall when backing is missing, rather than inventing funding or allowing another provider POST. Review and recover that shortfall separately.

Legacy earnings retain their original references, pending/cliff rules and statuses. The migration does not fabricate receipts, purchased-credit lots, provider costs or reward-source backing for old money. The legacy pending figure is a reconciliation exception, not proof that the balance is safe to pay. Existing legacy payout paths and commercial terms require separate finance review before activation. Historical jobs without reconciled receipt coverage or cost statements can block revenue catch-up; document the evidence gap rather than inventing a zero cost or synthetic receipt.

Before an enrolled production pilot, obtain commercial approval, actual settled funds and complete fee/tax/refund/cost evidence, actual CryptoRails USD realization where applicable, provider account/country/currency approval, reconciliation ownership and alerting, shadow allocation review and an independent liability/coverage comparison. Validate deployment secrets through the established secret manager and provider setup process; ordinary policy APIs do not expose secrets. Offline fixtures, successful unit tests and available UI controls do not prove live settlement, provider activation, fraud completeness or bank arrival.
