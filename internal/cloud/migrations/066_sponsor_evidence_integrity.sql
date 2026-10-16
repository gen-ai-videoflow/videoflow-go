CREATE FUNCTION cloud_sponsor_award_integrity() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'campaign award evidence is immutable'; END IF;
 IF ROW(NEW.id,NEW.campaign_id,NEW.cents,NEW.evidence_reference,NEW.proposed_by,NEW.created_at) IS DISTINCT FROM ROW(OLD.id,OLD.campaign_id,OLD.cents,OLD.evidence_reference,OLD.proposed_by,OLD.created_at) OR OLD.approved_by IS NOT NULL AND ROW(NEW.approved_by,NEW.approved_at) IS DISTINCT FROM ROW(OLD.approved_by,OLD.approved_at) OR OLD.batch_id IS NOT NULL AND NEW.batch_id IS DISTINCT FROM OLD.batch_id OR OLD.confirmed_at IS NOT NULL AND NEW.confirmed_at IS DISTINCT FROM OLD.confirmed_at THEN RAISE EXCEPTION 'campaign award evidence cannot be rewritten'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_sponsor_award_integrity BEFORE UPDATE OR DELETE ON cloud_sponsor_awards FOR EACH ROW EXECUTE FUNCTION cloud_sponsor_award_integrity();
CREATE FUNCTION cloud_watch_traffic_integrity() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF ROW(NEW.traffic_source,NEW.campaign_id) IS DISTINCT FROM ROW(OLD.traffic_source,OLD.campaign_id) AND (OLD.traffic_source='paid' OR EXISTS(SELECT 1 FROM cloud_watch_events e WHERE e.session_id=OLD.id)) THEN RAISE EXCEPTION 'recorded watch traffic provenance is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_watch_traffic_integrity BEFORE UPDATE ON cloud_watch_sessions FOR EACH ROW EXECUTE FUNCTION cloud_watch_traffic_integrity();
