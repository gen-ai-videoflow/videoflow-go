CREATE TABLE cloud_reward_corrections (
 id text PRIMARY KEY, original_batch_id text NOT NULL REFERENCES cloud_reward_batches(id),
 user_id text NOT NULL REFERENCES cloud_users(id), cents bigint NOT NULL CHECK(cents BETWEEN -100000000 AND 100000000 AND cents<>0),
 source_id text REFERENCES cloud_reward_sources(id), reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 1000),
 evidence_reference text NOT NULL CHECK(length(evidence_reference) BETWEEN 1 AND 2000),
 vesting_seconds bigint NOT NULL CHECK(vesting_seconds BETWEEN 86400 AND 31536000),
 proposed_by text NOT NULL, approved_by text, approved_at timestamptz, approval_reason text,
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK((cents>0 AND source_id IS NOT NULL) OR (cents<0 AND source_id IS NULL)),
 CHECK((approved_by IS NULL AND approved_at IS NULL AND approval_reason IS NULL) OR (approved_by IS NOT NULL AND approved_by<>proposed_by AND approved_at IS NOT NULL AND length(approval_reason)>0))
);
CREATE INDEX cloud_reward_correction_owner ON cloud_reward_corrections(user_id,created_at DESC);
CREATE TABLE cloud_reward_correction_releases (
 correction_id text PRIMARY KEY REFERENCES cloud_reward_corrections(id), released_cents bigint NOT NULL DEFAULT 0 CHECK(released_cents>=0), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_reward_correction_events (
 id text PRIMARY KEY, correction_id text NOT NULL REFERENCES cloud_reward_corrections(id),
 through_cents bigint NOT NULL CHECK(through_cents>0), earning_id text NOT NULL UNIQUE REFERENCES cloud_earnings(id),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(correction_id,through_cents)
);
