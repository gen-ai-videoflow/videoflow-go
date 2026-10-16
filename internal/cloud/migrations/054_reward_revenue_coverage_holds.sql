ALTER TABLE cloud_reward_revenue_evidence DROP CONSTRAINT cloud_reward_revenue_evidence_kind_check;
ALTER TABLE cloud_reward_revenue_evidence ADD CONSTRAINT cloud_reward_revenue_evidence_kind_check CHECK(kind IN('receipt','cost','reversal','recovery'));
CREATE TABLE cloud_reward_revenue_recoveries (
 held_source_id text PRIMARY KEY REFERENCES cloud_reward_revenue_source_holds(source_id),
 replacement_source_id text NOT NULL REFERENCES cloud_reward_sources(id),
 cents bigint NOT NULL CHECK(cents>=0), evidence_id text UNIQUE NOT NULL REFERENCES cloud_reward_revenue_evidence(id),
 created_at timestamptz NOT NULL DEFAULT now(), CHECK(held_source_id<>replacement_source_id)
);
CREATE TRIGGER cloud_reward_revenue_recovery_immutable BEFORE UPDATE OR DELETE ON cloud_reward_revenue_recoveries FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE FUNCTION cloud_reward_revenue_covered(source text) RETURNS boolean LANGUAGE sql STABLE AS $$
 WITH RECURSIVE backing(id) AS (
 SELECT source UNION SELECT r.replacement_source_id FROM backing b JOIN cloud_reward_revenue_recoveries r ON r.held_source_id=b.id
 ) SELECT NOT EXISTS(SELECT 1 FROM backing b JOIN cloud_reward_revenue_source_holds h ON h.source_id=b.id WHERE NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_recoveries r WHERE r.held_source_id=b.id));
$$;
-- Existing liabilities survive a source reversal. Accounts with awards backed
-- by that source enter the established reward-hold workflow pending recovery.
CREATE FUNCTION cloud_reward_revenue_coverage_hold() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE affected_user text;
BEGIN
 -- Serialize with a concurrent new allocation/backing reservation before
 -- discovering affected owners; otherwise its uncommitted rows could be missed.
 PERFORM id FROM cloud_reward_sources WHERE id=NEW.source_id FOR UPDATE;
 -- Match payout execution's creator lock. A source refund cannot commit a
 -- new hold between its final hold check and provider submission.
 FOR affected_user IN
  WITH RECURSIVE affected(id) AS (SELECT NEW.source_id UNION SELECT r.held_source_id FROM affected x JOIN cloud_reward_revenue_recoveries r ON r.replacement_source_id=x.id),
  owners AS (
   SELECT a.user_id FROM cloud_reward_allocations a JOIN cloud_reward_batches b ON b.id=a.batch_id WHERE b.status='confirmed' AND b.source_id IN(SELECT id FROM affected)
   UNION SELECT legacy.user_id FROM cloud_reward_legacy_backing legacy JOIN cloud_earnings e ON e.id=legacy.earning_id WHERE e.status<>'paid' AND legacy.source_id IN(SELECT id FROM affected)
  ) SELECT user_id FROM owners ORDER BY user_id
 LOOP
  PERFORM pg_advisory_xact_lock(hashtextextended(affected_user,814037226));
  INSERT INTO cloud_reward_holds(user_id,reason,actor)
  VALUES(affected_user,'Backing coverage review: '||NEW.source_id||' · '||NEW.reason,'system:revenue-coverage')
  ON CONFLICT(user_id) DO NOTHING;
 END LOOP;
 INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)
 VALUES('system:revenue-coverage','hold_reward_source',NEW.source_id,NEW.reason);
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_reward_revenue_coverage_hold AFTER INSERT ON cloud_reward_revenue_source_holds FOR EACH ROW EXECUTE FUNCTION cloud_reward_revenue_coverage_hold();
CREATE FUNCTION cloud_reward_revenue_hold_release_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF EXISTS(SELECT 1 FROM cloud_reward_allocations a JOIN cloud_reward_batches b ON b.id=a.batch_id JOIN cloud_reward_revenue_source_holds h ON h.source_id=b.source_id WHERE a.user_id=OLD.user_id AND b.status='confirmed' AND NOT cloud_reward_revenue_covered(b.source_id)) OR EXISTS(SELECT 1 FROM cloud_reward_legacy_backing l JOIN cloud_earnings e ON e.id=l.earning_id WHERE l.user_id=OLD.user_id AND e.status<>'paid' AND NOT cloud_reward_revenue_covered(l.source_id)) THEN
  RAISE EXCEPTION 'source backing remains held; independent funded coverage recovery required before release';
 END IF;
 RETURN OLD; END $$;
CREATE TRIGGER cloud_reward_revenue_hold_release_guard BEFORE DELETE ON cloud_reward_holds FOR EACH ROW EXECUTE FUNCTION cloud_reward_revenue_hold_release_guard();
