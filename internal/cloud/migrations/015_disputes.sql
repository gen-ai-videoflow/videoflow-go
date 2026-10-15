-- Keep dispute holds distinct from operator restrictions. Closed lost disputes
-- remain held until an operator reconciles the loss; winning clears only this hold.
ALTER TABLE cloud_credit_accounts ADD COLUMN dispute_hold boolean NOT NULL DEFAULT false;
CREATE TABLE cloud_payment_disputes (
 id text PRIMARY KEY, payment_intent text NOT NULL, status text NOT NULL,
 amount_cents bigint NOT NULL CHECK(amount_cents>0),currency text NOT NULL,
 workspace_id text REFERENCES cloud_workspaces(id), creator_id text REFERENCES cloud_users(id),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(workspace_id IS NOT NULL OR creator_id IS NOT NULL)
);
CREATE INDEX ON cloud_payment_disputes(workspace_id);
CREATE INDEX ON cloud_payment_disputes(creator_id);
