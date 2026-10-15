# CryptoRails execution and launch checklist

The user requested local completion before AWS deployment. All locally actionable payment checks below are complete. Real tenant onboarding, funded blockchain payments and deployment remain explicitly deferred; simulator results do not complete those external gates. The wider creator/Discover roadmap is tracked separately in [PRODUCT-EXPERIENCE-GAPS.md](../PRODUCT-EXPERIENCE-GAPS.md).

## Local implementation — complete

- [x] New credit and Premium checkouts use CryptoRails; historical Stripe receipts, refunds, disputes and owned subscription management remain reconcilable.
- [x] Server-owned invoices freeze purpose, account/workspace, pack/credits, exact asset/network/contract, treasury, fees and net target. No client-selected price, credit quantity or destination is authoritative.
- [x] Fresh deposit addresses and stable references survive reload. Ambiguous create requests are fenced in review; retries cannot create another address for that invoice.
- [x] Tenant keys remain server-side; redirects are rejected and provider Retry-After cooldowns are shared through Redis. Redis is ephemeral admission/status infrastructure, never the financial source of truth.
- [x] Raw-body HMAC verification precedes JSON parsing; only correctly correlated signed confirmed net settlement fulfills purchases.
- [x] Settlement receipt, credit ledger grant or 30-day Premium extension and invoice status commit atomically in PostgreSQL. Restricted runtime credentials preserve immutable invoice terms and append-only financial evidence.
- [x] Premium and generation credits are separate. Renewal is explicit; held/revoked legacy subscription time cannot be copied into a crypto entitlement or misleading expiry display.
- [x] Default and missing-input configurations fail closed, including disabled token precision 0. Rewards, creator cash payouts and withdrawals remain separately gated and off by default.
- [x] API-only AWS secret references and reviewed asset/fee settings are prepared with disabled defaults. Terraform and compose validation pass; no AWS apply occurred.

## Backend verification — complete locally

- [x] Exact decimal parsing and JSON money strings: 18 decimals, values above JavaScript's safe integer range, maximum price integers, zero/negative/malformed input, overprecision and five-decimal upward-rounding boundaries.
- [x] Twenty signed mutation cases reject mismatched source/destination, asset, network, operation, reference, gross/fee/net and underfunded or inconsistent amounts. Unknown invoices fail closed.
- [x] Duplicate and twelve concurrent callbacks grant once. Stale failure events and purpose/key substitution cannot change a completed purchase.
- [x] Deferred commit failure rolls back receipt, grant and invoice status for credits and Premium. Actual PostgreSQL connection termination during commit leaves no partial state; a new Server instance and original event recover once. Lost-response replay after commit changes nothing. This is a connection-loss/restart drill, not a claim of an OS process-kill experiment.
- [x] Expired Premium denies new playback grants and redemption of a previously issued grant. Private, blocked and deleted videos remain denied even with active Premium.
- [x] Restricted-role signed settlement succeeds and preserves an existing account dispute hold. Historical payment and desktop/local tests still pass.
- [x] Full PostgreSQL/Redis-backed race tests, real FFmpeg assembly integration and Go vet pass.

## Frontend and browser verification — complete locally

- [x] Original canvas/auth-02 screens remain; Discover is distinct from Studio. Templates create actual connected projects without paid generation.
- [x] Payment history, exact amount/network/token/contract, local address-only QR, copy/recheck/reopen controls and explicit 30-day renewal are present.
- [x] Pending/reload states cannot confirm payment. A simulated deposit with a deliberately withheld signed callback remains uncredited after browser reload.
- [x] Conditional shortfall/wrong-network/wrong-token troubleshooting does not invent a failure diagnosis. Review/failed/expired/unknown/missing-address states hide deposit controls. Failed loads hide stale payment details and expose retry; interrupted refresh is reported.
- [x] Persisted purchase attempts are validated. Thirty-three state/recovery assertions pass; no browser balance, wallet hint or return URL grants credits or access.
- [x] Keyboard history selection works with Enter; same-page invoice switching preserves focus/scroll and announces selection. Mobile/desktop layouts, local QR and both desktop/static and cloud/Node build modes pass.
- [x] Secret audit scanned 1,417 exported asset/log files against the two actual mock server-secret values: zero exposures. Browser HTML/API responses/resource URLs also contained no known secret and made no direct provider request. A deliberately secret-bearing invalid callback produced a safe constant rejection log, not a body/signature echo. A separate cloud build with three server-secret canaries scanned 1,416 public files with zero exposures; CI is wired to repeat it; this is bounded canary evidence, not a guarantee about all future configurations.
- [x] Actual shared-browser flows used the real local application and isolated simulator: Studio/credits → invoice → processing → recovered signed delivery → 1,000 credits once; Discover → Premium → invoice → signed confirmation → 30-day access. A duplicate Premium event did not extend access again or change generation credits. These are simulated gateway transactions, never funded blockchain payments.
- [x] The actual local HTTPS SvelteKit proxy accepted original raw signed replay and rejected a modified body and malformed signature (200/401/401). Production proxy behavior still requires deployed verification.

## Local operations and restore verification — complete

- [x] Authenticated loopback/private-Docker simulator implements deposit addresses, active token policy, exact fee/balance checks, stable consolidation references, failed delivery and preserved original raw/HMAC replay. Its three tests pass; it is explicitly test-only and cannot move real funds.
- [x] Audited `cloud-admin crypto-inspect`, `crypto-recover-address` and `crypto-replay` perform guarded recovery; no unsigned lookup or locally reconstructed signature grants value. A failed provider transaction moves to review; consumed failed references require external funds recovery rather than blind resubmission.
- [x] Audited `cloud-admin crypto-report` exposes aggregate statuses/stale ages and receipt/grant integrity checks without PII, secrets, provider calls or financial repair. Safe webhook logs distinguish invalid_signature, malformed_payload and settlement_rejected.
- [x] Disposable restored settled checkpoint: two original signed callbacks replayed twice left credits, receipts and Premium expiry unchanged.
- [x] Disposable older pending checkpoint: replay of the newer original callback granted exactly 1,000 credits once; second replay changed nothing. An event whose invoice was absent from that checkpoint was excluded rather than fabricated.
- [x] QA report after the browser drill showed two settled invoices and zero missing receipt, incorrect state or missing/incorrect credit-grant discrepancies.
- [x] Evidence, customer instructions, manual residual/refund recovery boundaries, simulator setup and safe restore commands are documented. No automatic crypto refund, withdrawal or creator transfer is represented as implemented by checkout.

See [LOCAL-CRYPTORAILS-DRILL.md](LOCAL-CRYPTORAILS-DRILL.md), [the integration contract](../CRYPTORAILS.md) and the implementation plan for commands and scope. Restore tooling refuses the main app/test databases and creates new named databases; no production or main app data was restored or overwritten.

## Deferred external/provider/deployment gates — not completed

These require actual tenant access, funded wallets or deployment-specific infrastructure. They are intentionally not checked during the requested no-AWS phase.

- [ ] Create/restrict a real tenant API key and store real API/webhook credentials in the chosen secret store.
- [ ] Configure and rotate the real webhook secret and approved public callback.
- [ ] Verify real chain/VM/token contract, token precision, activation requirements, fees, consolidation treasury and gas funding.
- [ ] Approve the actual stablecoin USD/depeg policy, supported jurisdictions, fees and customer refund/recovery terms.
- [ ] Deploy staging, run explicit migrations and restricted-role provisioning, and verify the deployed proxy preserves raw webhook bodies and rejects invalid signatures.
- [ ] Execute low-value funded credit and Premium transactions; reconcile deposit, consolidation fee/net and treasury receipt. Local simulator/browser tests do not complete this gate.
- [ ] Exercise the real provider's failed-delivery/manual recovery capability and timeout reconciliation. Automatic gateway webhook retries must not be assumed.
- [ ] Connect aggregate reports/logs to real monitoring, alerts and an assigned operator/on-call procedure; verify deployment budgets and backup/restore/failover behavior.
- [ ] Enable real checkout only after those gates pass. Keep creator payouts, withdrawals and rewards gated until their own launch checks pass.

No public RPC service is needed by the current checkout: it uses server-controlled consolidation and signed callbacks. Browser deposit-watch/RPC detection is optional future work and would require separately approved capacity; it must never become settlement authority.

Final local closure: the full race suite, Go vet, workflow lint, simulator tests, frontend checks/builds and independent repository checks passed. The temporary 10544 simulator preview and its three containers were stopped; the normal 10543 preview remains available with real payments disabled. No existing application database was overwritten.
