-- Historical earnings do not acquire invented revenue provenance. Finance may
-- reserve new real backing for an unpaid legacy obligation after two-person
-- review; the original earning remains intact.
CREATE TABLE cloud_reward_legacy_backing_proposals (
 id text PRIMARY KEY, earning_id text NOT NULL REFERENCES cloud_earnings(id),
 user_id text NOT NULL REFERENCES cloud_users(id), source_id text NOT NULL REFERENCES cloud_reward_sources(id),
 cents bigint NOT NULL CHECK(cents>0), reason text NOT NULL CHECK(length(reason)>0),
 evidence_reference text NOT NULL CHECK(length(evidence_reference)>0), proposed_by text NOT NULL,
 approved_by text CHECK(approved_by<>proposed_by AND approved_by<>user_id),
 created_at timestamptz NOT NULL DEFAULT now(), approved_at timestamptz
);
CREATE UNIQUE INDEX cloud_reward_legacy_one_approval ON cloud_reward_legacy_backing_proposals(earning_id) WHERE approved_by IS NOT NULL;
CREATE TABLE cloud_reward_legacy_backing (
 earning_id text PRIMARY KEY REFERENCES cloud_earnings(id), user_id text NOT NULL REFERENCES cloud_users(id),
 source_id text NOT NULL REFERENCES cloud_reward_sources(id), cents bigint NOT NULL CHECK(cents>0),
 proposal_id text UNIQUE NOT NULL REFERENCES cloud_reward_legacy_backing_proposals(id),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER cloud_reward_legacy_backing_immutable BEFORE UPDATE OR DELETE ON cloud_reward_legacy_backing FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE FUNCTION cloud_reward_legacy_proposal_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR OLD.approved_by IS NOT NULL OR ROW(NEW.id,NEW.earning_id,NEW.user_id,NEW.source_id,NEW.cents,NEW.reason,NEW.evidence_reference,NEW.proposed_by,NEW.created_at) IS DISTINCT FROM ROW(OLD.id,OLD.earning_id,OLD.user_id,OLD.source_id,OLD.cents,OLD.reason,OLD.evidence_reference,OLD.proposed_by,OLD.created_at) THEN RAISE EXCEPTION 'legacy backing evidence is immutable'; END IF;
 RETURN NEW; END $$;
CREATE TRIGGER cloud_reward_legacy_proposal_guard BEFORE UPDATE OR DELETE ON cloud_reward_legacy_backing_proposals FOR EACH ROW EXECUTE FUNCTION cloud_reward_legacy_proposal_guard();
