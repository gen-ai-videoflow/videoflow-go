CREATE TABLE cloud_commercial_policy (
 id boolean PRIMARY KEY DEFAULT true CHECK(id), revision bigint NOT NULL CHECK(revision>0),
 policy jsonb NOT NULL, updated_at timestamptz NOT NULL DEFAULT now(), updated_by text NOT NULL
);
CREATE TABLE cloud_commercial_proposals (
 id text PRIMARY KEY, base_revision bigint NOT NULL CHECK(base_revision>=0), policy jsonb NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 3 AND 1000), proposed_by text NOT NULL,
 approved_by text, approved_at timestamptz, approval_reason text,
 created_at timestamptz NOT NULL DEFAULT now(), CHECK(approved_by IS NULL OR approved_by<>proposed_by)
);
