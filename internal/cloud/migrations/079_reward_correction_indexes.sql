CREATE INDEX cloud_reward_correction_original ON cloud_reward_corrections(original_batch_id,user_id) WHERE approved_by IS NOT NULL;
CREATE INDEX cloud_reward_correction_pending ON cloud_reward_corrections(created_at) WHERE approved_by IS NULL;
CREATE INDEX cloud_reward_correction_source ON cloud_reward_corrections(source_id) WHERE approved_by IS NOT NULL AND cents>0;
CREATE FUNCTION cloud_reward_correction_earning_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF OLD.reference LIKE 'correction:%' OR OLD.reference LIKE 'correction-release:%' THEN
  IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction earning is immutable'; END IF;
  IF (NEW.id,NEW.user_id,NEW.reference,NEW.amount_cents,NEW.currency,NEW.created_at) IS DISTINCT FROM (OLD.id,OLD.user_id,OLD.reference,OLD.amount_cents,OLD.currency,OLD.created_at) THEN RAISE EXCEPTION 'correction earning is immutable'; END IF;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW;
END $$;
CREATE TRIGGER cloud_reward_correction_earning_guard BEFORE UPDATE OR DELETE ON cloud_earnings FOR EACH ROW EXECUTE FUNCTION cloud_reward_correction_earning_guard();
