-- Platform-wide Premium replaces new channel memberships. Legacy rows remain
-- intact for refund, webhook replay, accounting and operator reconciliation.
UPDATE cloud_membership_plans SET enabled=false;
CREATE TABLE cloud_premium_checkouts (
 id text PRIMARY KEY,user_id text NOT NULL REFERENCES cloud_users(id),price_cents bigint NOT NULL CHECK(price_cents BETWEEN 100 AND 100000),
 currency text NOT NULL DEFAULT 'usd' CHECK(currency='usd'),idempotency_key text NOT NULL,
 provider_session text UNIQUE,provider_subscription text UNIQUE,checkout_url text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(user_id,idempotency_key)
);
CREATE TABLE cloud_premium_subscriptions (
 user_id text PRIMARY KEY REFERENCES cloud_users(id),provider_subscription text UNIQUE NOT NULL,
 status text NOT NULL CHECK(status IN ('active','expired','revoked')),paid_through timestamptz NOT NULL,revoked_through timestamptz NOT NULL DEFAULT 'epoch',updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_premium_invoices (
 id text PRIMARY KEY,checkout_id text NOT NULL REFERENCES cloud_premium_checkouts(id),payment_intent text UNIQUE NOT NULL,
 gross_cents bigint NOT NULL CHECK(gross_cents>0),refunded_cents bigint NOT NULL DEFAULT 0 CHECK(refunded_cents>=0 AND refunded_cents<=gross_cents),
 currency text NOT NULL CHECK(currency='usd'),period_end timestamptz NOT NULL,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_premium_refunds (
 id text PRIMARY KEY,invoice_id text NOT NULL REFERENCES cloud_premium_invoices(id),cumulative_cents bigint NOT NULL CHECK(cumulative_cents>0),
 delta_cents bigint NOT NULL CHECK(delta_cents>0),created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(invoice_id,cumulative_cents)
);
CREATE TABLE cloud_premium_disputes (
 id text PRIMARY KEY,user_id text NOT NULL REFERENCES cloud_users(id),payment_intent text NOT NULL,status text NOT NULL,
 amount_cents bigint NOT NULL CHECK(amount_cents>0),currency text NOT NULL CHECK(currency='usd'),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON cloud_premium_checkouts(user_id,created_at);
CREATE INDEX ON cloud_premium_disputes(user_id,status);
