# Versioned runtime model and pricing policy

Migration `038_runtime_policy.sql` adds a singleton audited runtime policy. Migration `040_runtime_policy_versions.sql` retains immutable full JSON snapshots for every subsequent revision and backfills the existing current policy, if any. Until an owner saves its first revision, deployment configuration remains the bootstrap source. The API and worker read effective configuration without mutating their shared server/worker configuration objects.

`GET /admin/runtime-policy/versions` returns the most recent 20 immutable snapshots for authorized operators; older revisions remain stored. The settings UI shows revision reasons/timestamps and a concrete review of all proposed values before save.

The separate admin app exposes **Models, pricing and limits** under settings for owners. `GET /admin/runtime-policy` returns `{policy, revision, updated_at, source}`. `PUT /admin/runtime-policy` requires `{policy, revision, reason}` and the existing owner/recent second-factor guard. A stale revision returns 409. Policy changes, immutable full-value snapshots and their reasons append audit records in the same transaction; they do not grant credits, alter balances or activate providers.

Editable fields:

- Approved OpenRouter story/video model ID lists, each capped at 20 distinct IDs.
- Credits per video second, per story and per deterministic assembly.
- Daily provider budget, per-generation provider ceiling and story ceiling.
- New Premium purchase price (zero disables new Premium sales).
- Maximum active generations per account (1–20), enforced under the account reservation lock. Same-key job recovery remains available at the quota.

Credentials, payment activation, infrastructure endpoints/buckets/queues, global HTTP admission capacity, payout eligibility and fixed credit-pack prices remain deployment managed. There is no arbitrary environment editor or secret field in this policy.

## Model approval and price checks

An owner can refresh current public model metadata through `POST /admin/runtime-policy/refresh-catalogues`. This performs catalogue reads only; it does not run inference or enable a model. Story metadata is cached for one hour, video metadata for one day. Approval rejects unknown IDs, free model variants, unsupported capability/pricing and models exceeding the approved ceiling. Missing credentials or unavailable catalogues fail closed.

Numeric bounds cap daily AI budget at USD 1,000, a generation ceiling at USD 100 and story ceiling at USD 10. New Premium price must be zero or USD 1–1,000. Generation pricing must preserve at least a 20% revenue-over-provider-ceiling buffer using the lowest unit revenue of the fixed credit packs. This is an enforceable minimum buffer, not a complete profit forecast; payment fees, hosting and tax still require reviewed operating policy.

## Existing purchases and jobs

New quotes and purchases use the current effective policy. Confirmed quotes retain their credits, actual selected models and provider ceilings. Changing runtime prices never recalculates a stored quote or active Premium entitlement. Pending Stripe checkout retries retain their original frozen checkout price, even after a new-sales price update. Existing recurring subscriptions retain their processor price; the policy does not modify Stripe subscriptions.

Workers refresh current budget caps before submission and retain confirmed job payloads. Lowering limits or revoking a story model may prevent pending execution rather than silently replacing a model or increasing a charge. Budget reservation and failure/refund paths remain the existing durable accounting implementation.

## Evidence and limits

Local PostgreSQL/race tests verify revision conflict rejection, audit writes, unknown-model rejection, unsafe bounds/margin rejection, old/new quote pricing, preserved Premium access, frozen pending checkout price and quota idempotency. Mocked model-catalogue tests verify pricing and language-independent selection. No paid provider call, live Stripe transaction, funded CryptoRails transfer or AWS deployment is implied by these tests.
