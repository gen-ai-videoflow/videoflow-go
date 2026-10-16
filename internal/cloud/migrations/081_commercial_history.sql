CREATE TABLE cloud_commercial_history (
 revision bigint PRIMARY KEY, proposal_id text NOT NULL UNIQUE REFERENCES cloud_commercial_proposals(id),
 policy jsonb NOT NULL, proposed_by text NOT NULL, approved_by text NOT NULL,
 reason text NOT NULL, approval_reason text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), CHECK(proposed_by<>approved_by)
);
CREATE TRIGGER cloud_commercial_history_immutable BEFORE UPDATE OR DELETE ON cloud_commercial_history FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE FUNCTION cloud_commercial_proposal_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR OLD.approved_by IS NOT NULL OR (NEW.id,NEW.base_revision,NEW.policy,NEW.reason,NEW.proposed_by,NEW.created_at) IS DISTINCT FROM (OLD.id,OLD.base_revision,OLD.policy,OLD.reason,OLD.proposed_by,OLD.created_at) THEN RAISE EXCEPTION 'commercial proposal evidence is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_commercial_proposal_guard BEFORE UPDATE OR DELETE ON cloud_commercial_proposals FOR EACH ROW EXECUTE FUNCTION cloud_commercial_proposal_guard();
