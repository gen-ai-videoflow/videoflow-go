# CryptoRails checkout and settlement

This is the implemented local contract for new credit purchases and platform Premium. New checkout offers independently enabled Stripe and CryptoRails choices; Stripe receipts, subscriptions, disputes and refunds remain reconcilable. Card checkout and recurring Premium are described in [Stripe operations](ops/LEGACY-STRIPE.md). No live tenant credentials, funded-wallet transaction, production webhook delivery or AWS deployment were exercised while writing this document. Payment activation stays off until the execution checklist is verified in the intended environment.

## Product boundaries and journeys

| Surface | Route | What the user does | Payment relationship |
| --- | --- | --- | --- |
| Studio | `/` or `/studio` | Sign in; create/select a private project; use the original canvas to edit scripts, scenes, shots and assets; review generation quotes; follow durable jobs; export completed media. | Generation consumes platform credits after explicit quote confirmation. Premium viewing access does not purchase generation credits. |
| Templates | `/templates` | Choose a starter; import it into a real owned project; continue in `/studio?project={id}`. | Choosing/importing a template does not silently submit a paid generation request. |
| Discover | `/discover` | Browse published public videos/channels and open a watch page. | Free publications need no Premium purchase. Studio credits do not grant Premium viewing rights. |
| Watch | `/watch/{videoId}` | Read public metadata, play eligible media, follow/report where available, or obtain Premium for protected content. | One platform Premium grant covers eligible Premium videos across creators. Private, blocked and deleted videos remain protected. |
| Credits | `/credits` | Inspect durable available/reserved/debt balances, choose a configured credit pack and open its crypto invoice. | Credits are non-withdrawable generation units, distinct from Premium and creator earnings. |
| Premium | `/premium` | View current platform-wide access and expiry; purchase or explicitly renew 30 days using a new crypto invoice. | Prepaid **30-day** access; no automatic crypto renewal or recurring wallet debit. |
| Payment invoice | `/payments?invoice={id}` | Reopen an owned invoice, inspect its exact token/network/address and amount, copy or scan the locally rendered QR, transfer with an external wallet, and follow server-confirmed status/history. | Browser detection, wallet submission and transaction hashes are progress hints; only signed confirmed net settlement grants value. |

A creator works in Studio and publishes a selected immutable output with explicit visibility and free/Premium audience controls. Discover is the public viewing surface, not the private editor. Templates create editable projects, not fake dashboard jobs or public posts. Publishing and encoding do not make a private project public implicitly.

Do not describe Premium as “per channel,” a credit pack, or an automatically renewing monthly crypto subscription. A successful prepaid renewal must extend access once from the later of the current eligible paid-through time and application settlement time. Refreshing the page or replaying an invoice event must not extend it twice. Stripe Premium is a monthly recurring subscription with owned billing management. CryptoRails Premium is prepaid. The UI explains this distinction before checkout. Active prepaid access and unresolved crypto Premium invoices block a new Stripe subscription. Crypto Premium creation reconciles every prior Stripe checkout/subscription first; ambiguous or renewable sessions block switching gateways.

## Trust and settlement specification

1. Authenticate the 6eek user and freeze a server-owned invoice: purpose, pack/entitlement, price, exact accepted asset/network/contract, fresh deposit address, tenant treasury address, fee policy and minimum required net settlement. Keep the stable invoice/transaction reference through address creation, consolidation, reload and recovery. Never accept a client-selected credit quantity, price, treasury, chain or token as authority.
2. Display the exact invoice deposit instruction and any gateway/network fee or minimum-balance requirement before transfer. Use decimal strings and integer token units, with the gateway's 18-decimal settlement representation. Never calculate crypto amounts with floating point, silently round away a mismatch, assume every stablecoin stays worth USD 1, or accept a token symbol without its configured network/contract.
3. The browser talks to 6eek. Backend gateway calls use `x-tenant-api-key`; the tenant key and signing secret never appear in frontend environment variables, RPC headers, URLs, QR data, client logs or bundles. A fresh address per invoice keeps old funds from falsely proving a new purchase.
4. Browser deposit detection and the authenticated notify action may request server-controlled consolidation. They never grant credits or Premium. An API-accepted consolidation, an on-chain browser balance, a gateway status lookup, or a checkout return page is not settlement authority.
5. Verify `x-cr-webhook-signature` as constant-time HMAC-SHA256 over the **exact raw body**, before decoding. Match the frozen reference, deposit source, expected treasury destination, asset contract, blockchain, operation/type, confirmed status and exact amount/fee/net policy. Require coherent positive confirmed net settlement. The credited economic value is `netAmount`, not gross `amount`; the frozen consolidation gross must match exactly, `gross - providerFee` must equal signed `netAmount`, and net must be at least the frozen required target. Extra net caused by a lower fee or rounding does not buy extra credits or days: only the frozen pack/30-day grant is fulfilled. Underpayment, identity/amount mismatch and unknown references require an explicit supported recovery/reconciliation state, never guessed proportional value.
6. Commit the settlement receipt and the credit ledger posting or Premium period grant in one PostgreSQL transaction. Enforce unique gateway transaction/reference identity and immutable invoice intent. Duplicate or concurrent callbacks cannot mint value twice. A signed failed/pending message cannot undo an already confirmed receipt or create value. Creator revenue allocation is separate; purchasing Premium does not automatically pay a creator or create generation credits.
7. Preserve failed/ambiguous delivery and consolidation evidence for operations. The gateway guide says failed webhook deliveries are recorded and **automatic retries are not enabled**. A successful status poll therefore does not justify marking an invoice paid, and waiting for automatic delivery retries is not a recovery plan.

The gateway's fixed/percentage provider fee and its separate gas/treasury-side requirements must match the configured tenant. The published guide describes consolidation balance requirements as well as a net amount after provider fees; verify a funded staging example against the actual tenant settings before publishing a customer deposit amount. Do not infer a universal fee from example gas-fee tables or invent wallet addresses/asset contracts.

## Application API contract

The current new-payment endpoints are:

- `GET /v1/payments/options`: server-approved payment configuration/availability.
- `POST /v1/payments/invoices`: authenticated JSON `{purpose: "credits", pack_id: "configured-pack-id"}` or `{purpose: "premium"}`, with a stable `Idempotency-Key`. Price, asset, amounts and credit/day grant are server controlled.
- `GET /v1/payments/invoices` and `GET /v1/payments/invoices/{id}`: owned history/reopen/status.
- `POST /v1/payments/invoices/{id}/notify` with `{}`: trigger the stored consolidation instruction, never accept a replacement amount/reference/treasury from the browser.
- `POST /v1/webhooks/cryptorails`: raw signed callback, not an authenticated browser route.

Invoice amounts are 18-decimal strings. `amount` is the displayed deposit funding requirement, the server-only stored `consolidate_amount` is the exact frozen sweep gross, `amount_net` is the required purchase target, and `fee_amount` is the **total extra funding reserve**, including fee allowance, rounding and possible residual; it is not a promise that the entire difference is a fee. The gross and deposit quote are conservatively rounded upward to the gateway's five-decimal request limit. Deposit funding includes the consolidation gross plus its provider-fee allowance to satisfy the guide's stated balance check. An unspent residual may remain on the invoice address and requires manual reconciliation/recovery; it must not be described as an automatic refund or free credit.

Statuses are `pending`, `processing`, `settled` and `review`. A `review` state with no known address is not permission to mint another deposit address blindly. Premium reports `interval: "30_days"` and `auto_renew: false`; Stripe checkout is independently enabled; owned Stripe management remains available when new sales are disabled. Before a new invoice is created, the backend validates the live gateway token/chain/fee policy using exact provider filters. Token precision is explicit operator-reviewed configuration; optional expanded token metadata is checked strictly when present, and its absence does not establish precision. Inactive assets or changed tenant fees fail closed. This check and the configured fee arithmetic still require a funded tenant test.

## AWS and local configuration

Set both `BILLING_ENABLED=false` and `CRYPTORAILS_ENABLED=false` until integration and tenant setup are verified. The API also requires an explicit accepted stablecoin pricing policy through `CRYPTORAILS_USD_PEG_ACCEPTED=true`; that flag records approval of the pricing assumption, not an exchange-rate guarantee.

| Environment setting | Meaning / safe default |
| --- | --- |
| `CRYPTORAILS_API_URL` | Server endpoint, default `https://api.cryptorails.tech/v1`; production must use HTTPS. |
| `CRYPTORAILS_API_KEY` | Secret tenant API key; backend only. |
| `CRYPTORAILS_WEBHOOK_SECRET` | Secret raw-body HMAC key; backend only. |
| `CRYPTORAILS_NETWORK` | Exact approved gateway blockchain name; blank is unconfigured. |
| `CRYPTORAILS_VM_TYPE` | `EVM` or `TRON`, matching the approved chain; blank is unconfigured. |
| `CRYPTORAILS_TOKEN` | Explicitly approved `USDC` or `USDT`; no native-token USD pricing is assumed. |
| `CRYPTORAILS_TOKEN_DECIMALS` | Explicitly reviewed contract precision: integer 5–18; default 0 is unconfigured. Never infer this from the token symbol. |
| `CRYPTORAILS_TOKEN_ADDRESS` | Exact approved contract on that network. |
| `CRYPTORAILS_TREASURY_ADDRESS` | Expected configured consolidation destination. |
| `CRYPTORAILS_DEPOSIT_FIXED` | Exact decimal fixed provider fee, up to five decimal places; blank is unconfigured. Enter `0` only if the approved tenant has no fixed fee. |
| `CRYPTORAILS_DEPOSIT_FEE_BPS` | Integer percentage fee in basis points, 0–9999; must match the tenant. |
| `CRYPTORAILS_USD_PEG_ACCEPTED` | Explicit stablecoin USD policy approval; default false. |
| `PREMIUM_PRICE_CENTS` | Reviewed platform Premium price; zero keeps Premium sales unavailable. The new grant is 30 days, not automatic monthly renewal. |

Terraform variables use the corresponding lower-case names. Supply the two credentials in the separate `cryptorails_secret_arns` map as full Secrets Manager ARNs, not plaintext tfvars. The API task receives those secrets; frontend and worker container definitions do not. The common runtime secret map explicitly rejects CryptoRails credential keys to prevent accidental worker injection. Terraform's activation validation requires the two secret ARNs, billing/peg switches and the exact chain/token/wallet/fixed-fee fields and reviewed token decimals. Empty defaults permit infrastructure review without inventing credentials or activating payments.

Local compose reads `.env.cloud`; it does not emulate a real gateway or blockchain settlement. LocalStack covers storage/queues, not money. Keep credentials blank and payment switches off for ordinary development. The published callback must reach the versioned backend webhook through HTTPS/ALB/CloudFront, bypass authenticated UI routing and retain the raw body unchanged. Configure the tenant callback `/v1/webhooks/cryptorails` (or its supported `/api/v1` alias) at the actual approved API origin. Confirm the application route before saving a live tenant setting.

## Recovery and operational procedure

- Distinguish address creation ambiguity, consolidation accepted/in-progress, insufficient funds/fees, confirmed transaction with failed delivery, mismatched signed settlement and application settlement failure. Record invoice/reference, gateway transaction ID, asset/network, timestamps and the error state without secrets.
- Before retrying a gateway write, reconcile its stable reference. `409` “transaction already exists” and “consolidation processing” are not new invoices or rate-limit errors. Respect `Retry-After` for actual throttling and use bounded retries only for documented transient failures. Never create a new reference to escape an uncertain old transfer.
- A signed confirmed transaction with failed webhook delivery requires the gateway's supported manual delivery/replay process or operator-assisted recovery that obtains the original authoritative signed payload. The application must re-run normal signature/correlation/idempotency checks. A UI “recheck” may trigger that process or inspect state; it cannot convert a status lookup into a synthetic signed confirmation.
- If the deployed gateway exposes no supported manual replay mechanism, keep the invoice unsettled and route it to financial operations. Preserve evidence and coordinate with the provider. Do not manufacture an HMAC in an operator script to turn an unsigned query into a confirmation.
- Confirm gateway and application receipt states agree after recovery. If provider inspection reports the consolidation transaction failed, the application moves the invoice to `review` without granting value. The original failed reference cannot simply be submitted again: the gateway already owns it. Arrange supported external recovery/refund or provider-assisted funds reconciliation; do not manufacture a second reference and blindly sweep the same money. Record the operator action and final reference. Replaying a callback after an application timeout must either complete the original atomic settlement or return the existing result.
- No automatic withdrawal, crypto creator payout, refund transfer, chain swap or currency conversion is implied by checkout. Those require their own approved contract and reconciliation. Historical Stripe webhooks and dispute holds remain supported independently of CryptoRails activation.

## Sources and verification boundary

Integration details were read from the supplied `/tmp/videoflow-cryptorails-docs.txt` and checked against the provider's [current complete integration guide](https://docs.cryptorails.tech/llms-full.txt) and [documentation index](https://docs.cryptorails.tech/llms.txt). The provider guide explicitly separates browser detection from signed settlement, requires the tenant API header, and describes failed delivery without automatic retries. This document does not attest that a tenant, supported token configuration, production RPC service, treasury wallet or funded end-to-end transaction has been configured.

Customers select **Check payment** after transferring to request consolidation. Subsequent status polling reads durable application state; it does not infer settlement from wallet balances. Returning to the invoice recovers that action after leaving the page. Provider throttling uses an ephemeral tenant-scoped Redis cooldown (in-memory development fallback), while all financial records remain in PostgreSQL.
