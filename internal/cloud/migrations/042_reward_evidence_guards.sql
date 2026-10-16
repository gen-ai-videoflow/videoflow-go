CREATE FUNCTION cloud_reward_allocation_insert_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE batch_status text;
BEGIN
 SELECT status INTO batch_status FROM cloud_reward_batches WHERE id=NEW.batch_id FOR UPDATE;
 IF batch_status<>'draft' THEN RAISE EXCEPTION 'cannot append to confirmed reward batch'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_reward_allocation_insert_guard BEFORE INSERT ON cloud_reward_allocations FOR EACH ROW EXECUTE FUNCTION cloud_reward_allocation_insert_guard();
CREATE FUNCTION cloud_reward_policy_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'reward policy cannot be deleted'; END IF;
 IF ROW(NEW.version,NEW.terms_version,NEW.window_seconds,NEW.vesting_seconds,NEW.created_by,NEW.created_at) IS DISTINCT FROM ROW(OLD.version,OLD.terms_version,OLD.window_seconds,OLD.vesting_seconds,OLD.created_by,OLD.created_at) THEN RAISE EXCEPTION 'create a new reward policy version'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_reward_policy_guard BEFORE UPDATE OR DELETE ON cloud_reward_policies FOR EACH ROW EXECUTE FUNCTION cloud_reward_policy_guard();
