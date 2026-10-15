CREATE TABLE cloud_membership_plans (
 channel_id text PRIMARY KEY REFERENCES cloud_channels(id),
 price_cents bigint NOT NULL CHECK(price_cents BETWEEN 100 AND 100000),
 currency text NOT NULL CHECK(currency='usd'), enabled boolean NOT NULL DEFAULT false,
 version bigint NOT NULL DEFAULT 1, updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_payout_accounts (
 user_id text PRIMARY KEY REFERENCES cloud_users(id), provider_id text UNIQUE NOT NULL,
 country text NOT NULL CHECK(length(country)=2), charges_enabled boolean NOT NULL DEFAULT false,
 payouts_enabled boolean NOT NULL DEFAULT false, updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_membership_checkouts (
 id text PRIMARY KEY, user_id text NOT NULL REFERENCES cloud_users(id), channel_id text NOT NULL REFERENCES cloud_channels(id),
 price_cents bigint NOT NULL CHECK(price_cents>0),currency text NOT NULL,
 commission_bps bigint NOT NULL CHECK(commission_bps BETWEEN 0 AND 10000),
 provider_session text UNIQUE, provider_subscription text UNIQUE, idempotency_key text, UNIQUE(user_id,idempotency_key),
 created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE cloud_memberships ADD COLUMN revoked_through timestamptz NOT NULL DEFAULT 'epoch';
CREATE TABLE cloud_membership_invoices (
 id text PRIMARY KEY, checkout_id text NOT NULL REFERENCES cloud_membership_checkouts(id),
 payment_intent text UNIQUE, gross_cents bigint NOT NULL CHECK(gross_cents>0),
 creator_cents bigint NOT NULL CHECK(creator_cents>=0), refunded_cents bigint NOT NULL DEFAULT 0 CHECK(refunded_cents>=0),
 currency text NOT NULL, period_end timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_reward_periods (
 period text PRIMARY KEY CHECK(period ~ '^[0-9]{4}-[0-9]{2}$'), currency text NOT NULL CHECK(currency='usd'),
 contribution_cents bigint NOT NULL CHECK(contribution_cents>=0), reward_bps bigint NOT NULL CHECK(reward_bps BETWEEN 0 AND 10000),
 cap_cents bigint NOT NULL CHECK(cap_cents>=0), allocated_cents bigint NOT NULL CHECK(allocated_cents>=0),
 input_hash text NOT NULL, closed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON cloud_membership_checkouts(user_id,channel_id);
CREATE INDEX ON cloud_earnings(user_id,created_at DESC);
