CREATE TABLE cloud_crypto_invoices (
 id text PRIMARY KEY,user_id text NOT NULL REFERENCES cloud_users(id),workspace_id text NOT NULL REFERENCES cloud_workspaces(id),
 purpose text NOT NULL CHECK(purpose IN ('credits','premium')),pack_id text NOT NULL DEFAULT '',credits bigint NOT NULL CHECK(credits>=0),price_cents bigint NOT NULL CHECK(price_cents>0),
 idempotency_key text NOT NULL,reference text UNIQUE NOT NULL,address text NOT NULL DEFAULT '',gateway_id text NOT NULL DEFAULT '',
 network text NOT NULL,vm_type text NOT NULL,token text NOT NULL,token_address text NOT NULL,treasury_address text NOT NULL,
 amount text NOT NULL,consolidate_amount text NOT NULL,amount_net text NOT NULL,fee_amount text NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','settled','review')),
 created_at timestamptz NOT NULL DEFAULT now(),expires_at timestamptz NOT NULL DEFAULT now()+interval '30 minutes',last_notify_at timestamptz,
 UNIQUE(user_id,idempotency_key)
);
CREATE TABLE cloud_crypto_settlements (
 reference text PRIMARY KEY REFERENCES cloud_crypto_invoices(reference),payload_hash text NOT NULL,tx_hash text NOT NULL,network text NOT NULL,
 gross_amount text NOT NULL,provider_fee text NOT NULL,net_amount text NOT NULL,UNIQUE(network,tx_hash),created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_crypto_premium (
 user_id text PRIMARY KEY REFERENCES cloud_users(id),paid_through timestamptz NOT NULL,updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON cloud_crypto_invoices(user_id,created_at DESC);

CREATE UNIQUE INDEX ON cloud_crypto_invoices(network,address) WHERE address<>'';
