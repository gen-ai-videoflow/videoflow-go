# Platform Premium

New Premium purchases use CryptoRails prepaid access for **30 days**, covering eligible Premium videos across all creators. Renewal requires another explicit payment; there is no automatic crypto renewal or wallet debit. Premium viewing rights do not include generation credits, and buying credits does not purchase Premium.

See [checkout, exact settlement rules and user journeys](CRYPTORAILS.md), and the [execution/recovery checklist](ops/CRYPTORAILS-EXECUTION-CHECKLIST.md). The server advertises `interval: "30_days"` and `auto_renew: false`. Only an authoritative signed confirmed settlement grants the frozen invoice entitlement once. Provider detection, browser callbacks, a transaction lookup and a success page cannot grant access.

Previous Stripe subscriptions may still be recurring at Stripe. Preserve their [historical management and reconciliation controls](ops/LEGACY-STRIPE.md), including cancellations, refunds, disputes and duplicate webhook recovery. Their existence does not enable new Stripe sales. Existing credit journals, creator earnings and legacy invoice history remain intact.

New checkout remains disabled without approved billing configuration, CryptoRails tenant/signing credentials, the exact supported chain/token/treasury/fee policy, explicit stablecoin USD pricing approval and a nonzero reviewed Premium price. No live activation or funded end-to-end payment has been verified by writing these documents.
