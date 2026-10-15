# CryptoRails execution and launch checklist

Status convention: `[x]` means locally evidenced work; `[ ]` means pending verification or an external setup step. Code completion, mock tests, provider onboarding and live activation are separate states. The user has authorized implementation of this checklist; an unchecked external credential/funding step is not permission to invent an input or claim a successful payment.

## Implemented infrastructure evidence

- [x] Added production Terraform CryptoRails configuration with disabled defaults, HTTPS API URL validation, exact asset/fee configuration and explicit stablecoin policy approval.
- [x] Added API-only Secrets Manager ARN injection for tenant/signing credentials; no frontend/worker crypto credential injection.
- [x] Added blank development environment examples with payment switches off.
- [x] Recorded Studio, Templates, Discover, Credits, Premium and invoice journeys in [the payment specification](../CRYPTORAILS.md).
- [x] Read the supplied provider guide and fetched its current primary documentation.
- [x] Terraform `fmt -check` and `validate` passed for the CryptoRails infrastructure changes (Terraform 1.14, AWS provider 6.69). Re-run after any later infrastructure changes.
- [x] Compose configuration validated with the updated development environment example; no gateway/container money flow was executed.

## Backend implementation and deterministic verification

- [x] Confirm every new purchase endpoint creates a CryptoRails invoice; no new Stripe checkout path remains enabled.
- [x] Credit packs and prepaid Premium freeze distinct purposes and exact net targets; no cross-purpose replay or user/tenant substitution.
- [x] Fresh deposit address is durable per invoice; same-key retries never create another address. Concurrent/ambiguous address creation is fenced in review and recoverable through owned history.
- [x] API key header is `x-tenant-api-key`; HTTP redirects cannot leak it to a different origin.
- [x] Notify/consolidation accepts only stored invoice settings and stable references; client amount/network/token/treasury substitution is rejected.
- [ ] Decimal parsing, fee arithmetic and JSON serialization are exact; tests cover 18 decimals, zero/negative values, overprecision, large amounts and rounding boundaries.
- [x] Raw-body HMAC validation rejects modified bodies, malformed signatures and missing secrets before parsing.
- [x] Only signed confirmed **net** settlement with complete identity/fee/amount matching fulfills an invoice.
- [ ] Mismatched source, destination, token contract, chain, operation, reference, gross/provider fee/net, underfunded net or inconsistent gross/fee/net and unknown invoices fail closed.
- [ ] Duplicate/concurrent callbacks, stale failure messages, application crash before/after commit and payment-purpose replay cannot duplicate grants.
- [x] Premium renewal grants exactly 30 days once from the correct paid-through/confirmation boundary; credits remain unchanged by Premium purchase.
- [ ] Premium expiry denies new protected playback grants; private/blocked/deleted content remains denied despite Premium.
- [x] Recovery never settles from an unsigned transaction query, browser balance or synthetic locally signed event.
- [x] Restricted runtime database role supports the new invoice/receipt/ledger path and preserves append-only financial evidence.
- [x] Historical Stripe invoice/refund/dispute/subscription webhook reconciliation and owned management tests still pass.
- [x] Run PostgreSQL/Redis integration tests, race tests, Go vet and existing local/desktop tests; record commands/results.

## Frontend and user journey verification

- [x] `/` and `/studio` retain the original protected project canvas; public browsing lives at `/discover`.
- [x] Template selection creates an owned editable project and opens its actual canvas; no paid generation starts implicitly.
- [x] Credits and Premium screens expose separate purposes and do not claim Premium includes generation credits.
- [x] Premium copy says prepaid 30 days and explicit renewal; historical recurring Stripe management is clearly distinguished.
- [x] Invoice screen displays exact accepted network/token/contract, amount/address, applicable fee guidance, local QR, status and reopen/history/recheck actions.
- [ ] Secrets never appear in bundles, browser requests, QR text, logs or RPC headers.
- [ ] A browser detected deposit shows “detected/pending,” not “paid”; reload/reopen remains pending until authoritative settlement.
- [ ] Insufficient balance/fees, wrong network/asset, ambiguous creation, pending confirmation, failed delivery and reconciliation-needed states are actionable and truthful.
- [x] Cross-user invoice IDs and history are denied; no address/secret/financial metadata leaks.
- [ ] Verify responsive browser flows, keyboard/focus behavior, local QR rendering and new/classic build modes.
- [ ] Capture actual browser evidence for Discover → Premium invoice → confirmed grant and Studio → credits invoice → confirmed balance; mocks alone do not complete this item.

## Tenant and AWS staging setup

- [ ] Create/restrict the gateway tenant API key through its administrator UI and store it in Secrets Manager.
- [ ] Store and rotate the webhook signing secret; configure the exact public HTTPS callback in the tenant.
- [ ] Verify selected chain/VM/token contract and explicitly reviewed 5–18 token decimals, active token configuration, fixed/percentage fees, treasury/consolidation wallet and any required withdrawal-side gas balance.
- [ ] Verify wallet activation requirements for the selected network; no network/token/address is assumed by the example configuration.
- [ ] Approve/document stablecoin USD pricing and depeg behavior; do not enable `CRYPTORAILS_USD_PEG_ACCEPTED` by habit.
- [ ] Configure production RPC capacity if browser detection is used; backend/gateway credential keys must never become public RPC keys.
- [ ] Migrate staging explicitly, re-provision restricted runtime grants and deploy reviewed images/configuration to an isolated account.
- [ ] Verify the webhook path bypasses UI authentication while preserving raw bytes; test valid/invalid signatures through the real proxy chain.
- [ ] Fund a controlled low-value staging invoice with the exact displayed instructions and fees; compare deposit, consolidation gross/provider fee/net and final treasury receipt.
- [ ] Verify only one durable credit/30-day Premium grant after duplicate delivery and user refresh.
- [ ] Deliberately fail webhook delivery; observe the provider's failed-delivery record and exercise its actual supported manual replay/recovery path. **Do not assume automatic retries.**
- [ ] Reconcile application/gateway/chain state after an application timeout; never resubmit a new financial reference blindly.
- [ ] Verify monitoring and on-call routing for pending/ambiguous invoices, signature failures, mismatched net settlement, failed deliveries, treasury/gas insufficiency and provider outages.
- [ ] Exercise PostgreSQL restore and financial reconciliation with gateway state newer than the restore point; preserve receipts and prevent duplicate settlement.

## Operator recovery commands

Use a separately authorized operator identity and private database access. Keep the captured raw payload file private; its bytes must be exactly what the provider signed. Templates below do not generate signatures or settle from a poll:

```sh
cloud-admin crypto-inspect --id "$INVOICE_ID" --actor "$OPERATOR_ID"
cloud-admin crypto-recover-address --id "$INVOICE_ID" --address "$PROVIDER_VERIFIED_ADDRESS" --actor "$OPERATOR_ID" --reason "$RECOVERY_REASON"
cloud-admin crypto-replay --input "$ORIGINAL_RAW_BODY_FILE" --signature "$ORIGINAL_PROVIDER_HMAC" --actor "$OPERATOR_ID"
```

Inspection reads provider transaction state and records an audit event; a provider-confirmed failed transaction moves the invoice to `review` without granting value. A failed reference is already consumed at the gateway and cannot simply be retried. External operator/provider funds recovery or refund is required; never invent a new financial reference to bypass that state. Address recovery is for an ambiguous invoice in `review` with no bound address: an operator first obtains the real address through the gateway's supported administration/evidence workflow, and the application checks that exact address against the frozen tenant reference and VM before binding it. A transaction/address GET is not a substitute for a signed settlement event, and an unknown address cannot be recovered by inventing one or assuming reference-only lookup works.

Replay submits the **original raw body and original signature** through the ordinary webhook verifier and atomic/idempotent settlement path. Obtain those from supported gateway delivery recovery or a preserved original delivery. Never sign a reconstructed transaction query locally. If no original authoritative signed payload can be recovered, escalate to financial operations/provider support and leave the invoice unfulfilled. Review `crypto_inspect`, `crypto_recover_address` and `crypto_replay` audit records after the procedure. Reconciliation of residual address balances and any real refund transfer is a separate reviewed operation.

## Activation and closure

- [ ] Record test evidence, supported jurisdiction/token/network policy, fees, refund/recovery policy and customer-facing instructions.
- [ ] Set real secrets and reviewed asset/fee configuration without committing plaintext credentials.
- [ ] Enable new checkout only with explicit `BILLING_ENABLED` and `CRYPTORAILS_ENABLED`; verify disabled/missing-input configurations still fail closed.
- [ ] Keep rewards, creator payouts, withdrawals and unrelated commercial switches off unless their separate launch gates pass.
- [ ] Confirm budgets/alerts and gateway operational recovery ownership; a signed callback is settlement authority, not proof of tax/accounting compliance or final bank cash availability.
- [ ] Publish the final verification result with any unchecked items clearly listed. No AWS apply or live activation is claimed by this checklist's creation.

## Local execution evidence (2026-10-15)

- PostgreSQL/Redis-backed `go test -race ./...` passed, including the real FFmpeg assembly integration through the worker image. Historical desktop/local tests remain passing. Final additional payment/role checks cover signed settlement through restricted runtime credentials while retaining an existing dispute hold.
- Independent review found held legacy Premium time being copied into crypto renewals; both legacy-base queries now exclude unresolved disputes, with a 200-day held legacy / 30-day new crypto regression test.
- CryptoRails HTTP redirects are rejected before a tenant key can be forwarded. Provider `Retry-After` cooldowns use Redis across replicas and in-memory state for development; browser retries do not bypass the gateway backoff.
- The documented unexpanded tenant-token response is supported using exact network/contract query filters and explicitly reviewed token decimals. Expanded metadata, when supplied, must match. Fee changes reject new invoices.
- Svelte check reports zero errors/warnings; desktop static and cloud Node builds pass. API, worker and web images are built locally. Terraform formatting/schema validation and compose configuration validation pass; no AWS apply occurred.
- Shared T3 browser: mobile signup succeeded; template creation produced three scenes, three shots, three edges, zero jobs and zero credits spent; original canvas rendered at desktop and mobile sizes. Pending-invoice local QR and exact strings rendered without horizontal overflow at 390px and 1366px. An empty-address review invoice showed no QR/send/check-payment controls.
- Browser invoice layouts used two explicitly non-payable private `local-test-only` / `TEST-USDC` fixtures, not a funded provider transaction. Live confirmations, external-wallet transfer, production webhook delivery/replay and activation remain unchecked above. No synthetic callback was used to grant this preview account value.
- Login/signup preserve same-origin viewing or Premium destinations; 24 redirect-security cases reject external, encoded and auth-loop destinations.

Unchecked staging/activation items require the actual tenant, credentials, treasury/gas configuration and funded validation. They are not represented as completed by local mock or fixture tests.

Final local follow-up: 12 concurrent settlement attempts grant exactly once; the displayed Premium expiry also excludes held legacy time. Disabled Terraform precision `0` is accepted while enabled checkout requires reviewed precision. Login was exercised in the shared browser with `/premium` return intent and landed on `/premium`. Temporary non-payable invoice fixtures were removed after UI verification; no financial grant or funded test was created.
