CREATE TABLE cloud_reward_quality_reviews (
 video_id text PRIMARY KEY REFERENCES cloud_videos(id),originality_bps bigint NOT NULL CHECK(originality_bps BETWEEN 0 AND 10000),
 trust_bps bigint NOT NULL CHECK(trust_bps BETWEEN 0 AND 10000),eligible boolean NOT NULL,
 reason text NOT NULL,actor text NOT NULL,revision bigint NOT NULL DEFAULT 1,updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_reward_calculation_runs (
 id text PRIMARY KEY,day date NOT NULL,shadow boolean NOT NULL,policy_version text NOT NULL REFERENCES cloud_reward_policies(version),
 policy_snapshot jsonb NOT NULL,status text NOT NULL DEFAULT 'queued' CHECK(status IN('queued','running','succeeded','failed')),
 actor text NOT NULL,reason text NOT NULL,attempt integer NOT NULL DEFAULT 0,next_attempt_at timestamptz NOT NULL DEFAULT now(),lease_until timestamptz,
 evidence_complete boolean NOT NULL DEFAULT false,videos integer NOT NULL DEFAULT 0,estimated_cents bigint NOT NULL DEFAULT 0,draft_batches integer NOT NULL DEFAULT 0,error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(day,shadow,policy_version)
);
CREATE TABLE cloud_reward_evidence_snapshots (
 run_id text NOT NULL REFERENCES cloud_reward_calculation_runs(id),video_id text NOT NULL REFERENCES cloud_videos(id),user_id text NOT NULL REFERENCES cloud_users(id),
 views bigint NOT NULL,seconds bigint NOT NULL,retention_bps bigint NOT NULL,originality_bps bigint NOT NULL,trust_bps bigint NOT NULL,score bigint NOT NULL,weight bigint NOT NULL,
 cutoff timestamptz NOT NULL,PRIMARY KEY(run_id,video_id)
);
CREATE TRIGGER cloud_reward_evidence_snapshot_immutable BEFORE UPDATE OR DELETE ON cloud_reward_evidence_snapshots FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE TABLE cloud_reward_estimates (
 run_id text NOT NULL REFERENCES cloud_reward_calculation_runs(id),video_id text NOT NULL REFERENCES cloud_videos(id),user_id text NOT NULL REFERENCES cloud_users(id),
 batch_id text REFERENCES cloud_reward_batches(id),cents bigint NOT NULL CHECK(cents>=0),calculated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(run_id,video_id,batch_id)
);
CREATE INDEX cloud_reward_estimates_owner ON cloud_reward_estimates(user_id,calculated_at DESC);
CREATE TABLE cloud_reward_payout_runs (
 id text PRIMARY KEY,period date NOT NULL UNIQUE,actor text NOT NULL,reason text NOT NULL,status text NOT NULL DEFAULT 'prepared',
 batches integer NOT NULL DEFAULT 0,failures integer NOT NULL DEFAULT 0,created_at timestamptz NOT NULL DEFAULT now()
);
