-- A refund or dispute must freeze future allocations immediately, even between
-- worker runs. Holds never mutate the confirmed liability or its release clock.
CREATE FUNCTION cloud_reward_revenue_receipt_hold() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE receipt text; why text;
BEGIN
 IF TG_TABLE_NAME='cloud_purchases' THEN
  IF NEW.refunded_cents<=OLD.refunded_cents THEN RETURN NEW; END IF;
  receipt:=NEW.id; why:='Stripe refund increased; receipt backing requires recovery';
 ELSE
  IF NEW.status IN('won','warning_closed') THEN RETURN NEW; END IF;
  SELECT id INTO receipt FROM cloud_purchases WHERE payment_intent=NEW.payment_intent;
  why:='Stripe dispute; receipt backing requires independent recovery';
 END IF;
 INSERT INTO cloud_reward_revenue_source_holds(source_id,reason)
 SELECT DISTINCT d.source_id,why FROM cloud_reward_revenue_lots l
 JOIN cloud_reward_revenue_consumption c ON c.lot_id=l.id
 JOIN cloud_reward_revenue_recognition r ON r.job_id=c.job_id
 JOIN cloud_reward_revenue_days d ON d.day=r.day
 WHERE l.provider='stripe' AND l.receipt_id=receipt AND d.source_id IS NOT NULL
 ON CONFLICT DO NOTHING;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_reward_revenue_refund_hold AFTER UPDATE OF refunded_cents ON cloud_purchases FOR EACH ROW EXECUTE FUNCTION cloud_reward_revenue_receipt_hold();
CREATE TRIGGER cloud_reward_revenue_dispute_hold AFTER INSERT OR UPDATE ON cloud_payment_disputes FOR EACH ROW EXECUTE FUNCTION cloud_reward_revenue_receipt_hold();
CREATE TRIGGER cloud_reward_revenue_hold_immutable BEFORE UPDATE OR DELETE ON cloud_reward_revenue_source_holds FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
