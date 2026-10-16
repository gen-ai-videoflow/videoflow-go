ALTER TABLE cloud_reward_policies DROP CONSTRAINT cloud_reward_policies_window_seconds_check;
ALTER TABLE cloud_reward_policies ADD CHECK(window_seconds BETWEEN 86400 AND 7776000);
ALTER TABLE cloud_reward_policies DROP CONSTRAINT cloud_reward_policies_vesting_seconds_check;
ALTER TABLE cloud_reward_policies ADD CHECK(vesting_seconds BETWEEN 86400 AND 31536000);
ALTER TABLE cloud_payout_batches DROP CONSTRAINT cloud_payout_batches_amount_cents_check;
ALTER TABLE cloud_payout_batches ADD CHECK(amount_cents>=100);
CREATE TABLE cloud_reward_engine_policy (
 id boolean PRIMARY KEY DEFAULT true CHECK(id),revision bigint NOT NULL CHECK(revision>=0),
 policy_version text NOT NULL REFERENCES cloud_reward_policies(version),policy jsonb NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),updated_by text NOT NULL
);
CREATE TABLE cloud_reward_engine_proposals (
 id text PRIMARY KEY,base_revision bigint NOT NULL,policy jsonb NOT NULL,reason text NOT NULL,
 proposed_by text NOT NULL,approved_by text,created_at timestamptz NOT NULL DEFAULT now(),approved_at timestamptz,
 status text NOT NULL DEFAULT 'proposed' CHECK(status IN('proposed','approved')),
 CHECK(approved_by IS NULL OR approved_by<>proposed_by)
);
CREATE TABLE cloud_reward_engine_policy_history (
 revision bigint PRIMARY KEY,policy_version text NOT NULL REFERENCES cloud_reward_policies(version),policy jsonb NOT NULL,
 reason text NOT NULL,proposed_by text NOT NULL,approved_by text NOT NULL,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER cloud_reward_engine_policy_history_immutable BEFORE UPDATE OR DELETE ON cloud_reward_engine_policy_history FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
ALTER TABLE cloud_videos ADD COLUMN reward_window_seconds bigint NOT NULL DEFAULT 2592000 CHECK(reward_window_seconds BETWEEN 86400 AND 7776000);
CREATE OR REPLACE FUNCTION cloud_reward_publication_clock() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND OLD.first_published_at IS NOT NULL THEN
  NEW.first_published_at=OLD.first_published_at;NEW.reward_window_seconds=OLD.reward_window_seconds;
 ELSIF NEW.status='published' THEN
  NEW.first_published_at=COALESCE(NEW.first_published_at,NEW.published_at,now());
  NEW.reward_window_seconds=COALESCE((SELECT window_seconds FROM cloud_reward_policies WHERE active LIMIT 1),2592000);
 END IF;
 RETURN NEW;
END $$;
