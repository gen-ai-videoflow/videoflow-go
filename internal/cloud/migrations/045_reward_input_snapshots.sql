ALTER TABLE cloud_reward_batches ADD COLUMN input_snapshot jsonb NOT NULL DEFAULT '{}';
CREATE FUNCTION cloud_reward_snapshot_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.input_snapshot IS DISTINCT FROM OLD.input_snapshot THEN RAISE EXCEPTION 'reward input snapshot is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_reward_snapshot_guard BEFORE UPDATE ON cloud_reward_batches FOR EACH ROW EXECUTE FUNCTION cloud_reward_snapshot_guard();
