# Crypto checkout and user journeys implementation plan

**Goal:** Replace new card checkouts with CryptoRails payments and separate watching from creating while retaining the original 6eek canvas.

**Authorization:** The user approved the implementation checklist and requested completion without further questions. External provider onboarding prerequisites gate live activation, not implementation with mocked credentials.

**Architecture:** PostgreSQL owns invoices, settlement journals and entitlements. CryptoRails receives server-generated address/consolidation requests; signed confirmed webhooks are the only purchase authority. Redis holds rate limits and ephemeral status hints. The existing AWS media pipeline and original canvas remain in place.

## Tasks

- [x] Backend: add exact-decimal CryptoRails client, validated disabled-by-default configuration, frozen account-owned invoices and stable provider references.
- [x] Backend: verify raw webhook HMAC and frozen address/token/network/destination; settle net amounts once transactionally; reject conflicting replays.
- [x] Backend: grant credits through the existing ledger; extend platform Premium by 30 days per prepaid purchase; preserve historical Stripe event handling.
- [x] Frontend: separate Discover and Studio navigation; preserve original canvas; provide templates that create editable projects without paid generation.
- [x] Frontend: add crypto checkout, local QR, explicit token/network/fees, payment history and recoverable pending invoices; distinguish Premium from credits.
- [x] Operations: document provider setup, exact fee policy, missed-webhook recovery, partial/late/overpayment handling and independently gated creator payouts.
- [x] Infrastructure: wire AWS secret references and public non-secret settings; default live payments off.
- [x] Integration: update generated API contract and restricted database grants; audit payment invariants independently.
- [x] Verification: run Postgres/Redis-backed race tests, Go vet, Svelte checks and both builds, Terraform validation and scoped preview checks.
- [x] Delivery: update local running preview, synchronize clean independent frontend/backend repositories, commit verified changes.

## Acceptance and limits

No browser callback or unsigned request grants value. No arbitrary user-selected withdrawal is exposed. No automatic recurring billing is promised. Provider configuration and live end-to-end payment validation remain explicit activation requirements, and no live funds are moved during development. Unsupported or ambiguous provider states remain pending/review instead of producing money or credits.

Provider retry cooldowns are ephemeral Redis state; invoices, receipts, credit grants and Premium expiry are PostgreSQL state. The operator inspect command moves a provider-failed sweep to review but never grants value. Fund recovery from a failed transaction requires documented external operator remediation rather than a new untracked reference.

## Verified delivery

Full PostgreSQL/Redis-backed race suite passed (`/tmp/videoflow-crypto-complete-tests.log`); final focused crypto regression suite passed (`/tmp/videoflow-crypto-final-regression-suite.log`), including concurrent settlement, disputed-time renewal/expiry, cooldowns and disabled configuration. Restricted-role settlement tests passed; Go vet passed. Svelte check is clean; both build modes and all three images pass. Independent backend `go test ./...` and frontend `npm run check` pass after extraction.

Preview: `https://asus-dev.tail66f665.ts.net:10543/`; Studio `/studio`, Discover `/discover`, Templates `/templates`, invoices `/payments`. Schema 020 is applied only to the local app database; no AWS apply, real payment, withdrawal or provider activation occurred. Independent repositories are synchronized under `/home/jienweng/projects/videoflow-saas/`.
