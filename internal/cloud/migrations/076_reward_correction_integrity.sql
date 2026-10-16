CREATE FUNCTION cloud_reward_correction_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR OLD.approved_by IS NOT NULL OR
 (NEW.id,NEW.original_batch_id,NEW.user_id,NEW.cents,NEW.source_id,NEW.reason,NEW.evidence_reference,NEW.vesting_seconds,NEW.proposed_by,NEW.created_at) IS DISTINCT FROM
 (OLD.id,OLD.original_batch_id,OLD.user_id,OLD.cents,OLD.source_id,OLD.reason,OLD.evidence_reference,OLD.vesting_seconds,OLD.proposed_by,OLD.created_at) THEN RAISE EXCEPTION 'correction evidence and approved decisions are immutable'; END IF; RETURN NEW;
END $$;
CREATE TRIGGER cloud_reward_correction_guard BEFORE UPDATE OR DELETE ON cloud_reward_corrections FOR EACH ROW EXECUTE FUNCTION cloud_reward_correction_guard();
CREATE TRIGGER cloud_reward_correction_events_immutable BEFORE UPDATE OR DELETE ON cloud_reward_correction_events FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE FUNCTION cloud_reward_correction_release_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'correction release cannot be deleted'; END IF;
 IF NOT EXISTS(SELECT 1 FROM cloud_reward_corrections WHERE id=NEW.correction_id AND approved_by IS NOT NULL AND cents>0 AND cents>=NEW.released_cents) THEN RAISE EXCEPTION 'invalid correction principal'; END IF;
 IF TG_OP='INSERT' THEN IF NEW.released_cents<>0 THEN RAISE EXCEPTION 'correction starts unmatured'; END IF; RETURN NEW; END IF;
 IF NEW.correction_id<>OLD.correction_id OR NEW.released_cents<OLD.released_cents THEN RAISE EXCEPTION 'invalid correction maturity'; END IF; RETURN NEW;
END $$;
CREATE TRIGGER cloud_reward_correction_release_guard BEFORE INSERT OR UPDATE OR DELETE ON cloud_reward_correction_releases FOR EACH ROW EXECUTE FUNCTION cloud_reward_correction_release_guard();
