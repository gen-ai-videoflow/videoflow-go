CREATE TABLE cloud_reward_correction_debts (
 correction_id text PRIMARY KEY REFERENCES cloud_reward_corrections(id), user_id text NOT NULL REFERENCES cloud_users(id),
 earning_id text NOT NULL UNIQUE REFERENCES cloud_earnings(id), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER cloud_reward_correction_debt_immutable BEFORE UPDATE OR DELETE ON cloud_reward_correction_debts FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE FUNCTION cloud_reward_correction_hold_release_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF EXISTS(SELECT 1 FROM cloud_reward_correction_debts WHERE user_id=OLD.user_id) AND
 (SELECT coalesce(sum(amount_cents),0) FROM cloud_earnings WHERE user_id=OLD.user_id AND currency='usd' AND cloud_reward_earning_backed(id) AND (status='available' OR (status='pending' AND amount_cents<0)))<0 THEN RAISE EXCEPTION 'correction debt remains outstanding'; END IF;
 IF EXISTS(SELECT 1 FROM cloud_reward_corrections c WHERE c.user_id=OLD.user_id AND c.approved_by IS NOT NULL AND c.cents>0 AND NOT cloud_reward_revenue_covered(c.source_id)) THEN RAISE EXCEPTION 'correction backing remains held'; END IF;
 RETURN OLD;
END $$;
CREATE TRIGGER cloud_reward_correction_hold_release_guard BEFORE DELETE ON cloud_reward_holds FOR EACH ROW EXECUTE FUNCTION cloud_reward_correction_hold_release_guard();
CREATE FUNCTION cloud_reward_correction_coverage_hold() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE affected_user text;
BEGIN
 FOR affected_user IN
  WITH RECURSIVE affected(id) AS (SELECT NEW.source_id UNION SELECT r.held_source_id FROM affected a JOIN cloud_reward_revenue_recoveries r ON r.replacement_source_id=a.id)
  SELECT DISTINCT c.user_id FROM cloud_reward_corrections c WHERE c.approved_by IS NOT NULL AND c.cents>0 AND c.source_id IN(SELECT id FROM affected) ORDER BY c.user_id
 LOOP
  PERFORM pg_advisory_xact_lock(hashtextextended(affected_user,814037226));
  INSERT INTO cloud_reward_holds(user_id,reason,actor) VALUES(affected_user,'Correction backing coverage review: '||NEW.source_id,'system:correction-coverage') ON CONFLICT(user_id) DO NOTHING;
 END LOOP;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_reward_correction_coverage_hold AFTER INSERT ON cloud_reward_revenue_source_holds FOR EACH ROW EXECUTE FUNCTION cloud_reward_correction_coverage_hold();
