ALTER TABLE cloud_credit_accounts ADD COLUMN debt bigint NOT NULL DEFAULT 0 CHECK(debt>=0);
ALTER TABLE cloud_credit_accounts ADD COLUMN restricted boolean NOT NULL DEFAULT false;
ALTER TABLE cloud_purchases ADD COLUMN payment_intent text UNIQUE;
ALTER TABLE cloud_purchases ADD COLUMN refunded_cents bigint NOT NULL DEFAULT 0;
ALTER TABLE cloud_purchases ADD COLUMN revoked_credits bigint NOT NULL DEFAULT 0;
