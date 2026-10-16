-- Support budgets are service-cost obligations, never cash awards.
CREATE TABLE cloud_reward_support_programs (
 id text PRIMARY KEY, title text NOT NULL CHECK(length(title) BETWEEN 1 AND 120),
 criteria text NOT NULL CHECK(length(criteria) BETWEEN 1 AND 2000),
 source_id text NOT NULL REFERENCES cloud_reward_sources(id),
 budget_cents bigint NOT NULL CHECK(budget_cents BETWEEN 1 AND 100000000),
 committed_cents bigint NOT NULL DEFAULT 0 CHECK(committed_cents BETWEEN 0 AND budget_cents),
 grant_credits bigint NOT NULL CHECK(grant_credits BETWEEN 1 AND 1000000),
 service_cost_cents bigint NOT NULL CHECK(service_cost_cents BETWEEN 1 AND 1000000),
 creator_cap integer NOT NULL CHECK(creator_cap BETWEEN 1 AND 10),
 grant_cap integer NOT NULL CHECK(grant_cap BETWEEN 1 AND 10000),
 granted integer NOT NULL DEFAULT 0 CHECK(granted BETWEEN 0 AND grant_cap),
 expiry_days integer NOT NULL CHECK(expiry_days BETWEEN 1 AND 365),
 minimum_publications integer NOT NULL CHECK(minimum_publications BETWEEN 1 AND 100),
 proposed_by text NOT NULL, approved_by text, enabled boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(), CHECK(approved_by IS NULL OR approved_by<>proposed_by)
);
CREATE TABLE cloud_reward_support_claims (
 id text PRIMARY KEY, program_id text NOT NULL REFERENCES cloud_reward_support_programs(id),
 user_id text NOT NULL REFERENCES cloud_users(id), workspace_id text NOT NULL REFERENCES cloud_workspaces(id),
 evidence text NOT NULL CHECK(length(evidence) BETWEEN 1 AND 2000),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN('pending','approved','rejected')),
 reviewed_by text, reason text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), reviewed_at timestamptz,
 CHECK(reviewed_by IS NULL OR reviewed_by<>user_id)
);
CREATE UNIQUE INDEX cloud_support_pending ON cloud_reward_support_claims(program_id,user_id) WHERE status='pending';
CREATE INDEX cloud_support_claim_owner ON cloud_reward_support_claims(user_id,created_at DESC);
CREATE TABLE cloud_promotional_credits (
 id text PRIMARY KEY REFERENCES cloud_reward_support_claims(id), workspace_id text NOT NULL REFERENCES cloud_workspaces(id),
 credits bigint NOT NULL CHECK(credits>0), consumed bigint NOT NULL DEFAULT 0 CHECK(consumed>=0),
 reserved bigint NOT NULL DEFAULT 0 CHECK(reserved>=0),
 service_cost_cents bigint NOT NULL CHECK(service_cost_cents>0), expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), CHECK(consumed+reserved<=credits)
);
CREATE INDEX cloud_promo_available ON cloud_promotional_credits(workspace_id,expires_at);
CREATE TABLE cloud_promotional_credit_reservations (
 job_id text NOT NULL REFERENCES cloud_jobs(id), grant_id text NOT NULL REFERENCES cloud_promotional_credits(id),
 credits bigint NOT NULL CHECK(credits>0), status text NOT NULL DEFAULT 'reserved' CHECK(status IN('reserved','consumed','released')),
 PRIMARY KEY(job_id,grant_id)
);
ALTER TABLE cloud_jobs ADD COLUMN promotional_credits bigint NOT NULL DEFAULT 0 CHECK(promotional_credits>=0 AND promotional_credits<=credits);
