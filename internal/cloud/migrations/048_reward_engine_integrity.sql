CREATE FUNCTION cloud_reward_engine_proposal_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR OLD.status='approved' OR ROW(NEW.id,NEW.base_revision,NEW.policy,NEW.reason,NEW.proposed_by,NEW.created_at) IS DISTINCT FROM ROW(OLD.id,OLD.base_revision,OLD.policy,OLD.reason,OLD.proposed_by,OLD.created_at) THEN RAISE EXCEPTION 'policy proposal evidence is immutable'; END IF; RETURN NEW; END $$;
CREATE TRIGGER cloud_reward_engine_proposal_guard BEFORE UPDATE OR DELETE ON cloud_reward_engine_proposals FOR EACH ROW EXECUTE FUNCTION cloud_reward_engine_proposal_guard();
CREATE TABLE cloud_reward_quality_history(video_id text NOT NULL REFERENCES cloud_videos(id),revision bigint NOT NULL,snapshot jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(video_id,revision));
CREATE FUNCTION cloud_reward_quality_history_record() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 INSERT INTO cloud_reward_quality_history(video_id,revision,snapshot)VALUES(NEW.video_id,NEW.revision,to_jsonb(NEW));RETURN NEW;END $$;
CREATE TRIGGER cloud_reward_quality_history_record AFTER INSERT OR UPDATE ON cloud_reward_quality_reviews FOR EACH ROW EXECUTE FUNCTION cloud_reward_quality_history_record();
CREATE TRIGGER cloud_reward_quality_history_immutable BEFORE UPDATE OR DELETE ON cloud_reward_quality_history FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE FUNCTION cloud_reward_calculation_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR ROW(NEW.id,NEW.day,NEW.shadow,NEW.policy_version,NEW.policy_snapshot,NEW.actor,NEW.reason,NEW.created_at) IS DISTINCT FROM ROW(OLD.id,OLD.day,OLD.shadow,OLD.policy_version,OLD.policy_snapshot,OLD.actor,OLD.reason,OLD.created_at) OR (OLD.evidence_complete AND NOT NEW.evidence_complete) THEN RAISE EXCEPTION 'calculation evidence identity is immutable'; END IF; RETURN NEW; END $$;
CREATE TRIGGER cloud_reward_calculation_guard BEFORE UPDATE OR DELETE ON cloud_reward_calculation_runs FOR EACH ROW EXECUTE FUNCTION cloud_reward_calculation_guard();
CREATE TRIGGER cloud_reward_estimates_immutable BEFORE UPDATE OR DELETE ON cloud_reward_estimates FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
