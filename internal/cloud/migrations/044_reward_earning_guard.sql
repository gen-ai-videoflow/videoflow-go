CREATE FUNCTION cloud_reward_earning_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.reference LIKE 'vesting:%' THEN
  IF TG_OP='DELETE' THEN RAISE EXCEPTION 'vested earning is immutable'; END IF;
  IF ROW(NEW.id,NEW.user_id,NEW.reference,NEW.amount_cents,NEW.currency,NEW.created_at) IS DISTINCT FROM ROW(OLD.id,OLD.user_id,OLD.reference,OLD.amount_cents,OLD.currency,OLD.created_at) THEN RAISE EXCEPTION 'use an audited reward adjustment'; END IF;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_reward_earning_guard BEFORE UPDATE OR DELETE ON cloud_earnings FOR EACH ROW EXECUTE FUNCTION cloud_reward_earning_guard();
