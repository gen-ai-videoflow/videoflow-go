CREATE TABLE cloud_reward_legacy_transfer_exceptions (
 earning_id text PRIMARY KEY REFERENCES cloud_earnings(id), batch_id text NOT NULL REFERENCES cloud_payout_batches(id),
 transfer_id text NOT NULL, actor text NOT NULL, reason text NOT NULL CHECK(length(reason)>0),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER cloud_reward_legacy_transfer_exception_immutable BEFORE UPDATE OR DELETE ON cloud_reward_legacy_transfer_exceptions FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE FUNCTION cloud_reward_earning_backed(earning text) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM cloud_earnings e WHERE e.id=earning AND (
 e.amount_cents<=0 OR e.reference LIKE 'vesting:%' OR e.reference LIKE 'correction:%'
 OR EXISTS(SELECT 1 FROM cloud_reward_legacy_backing b WHERE b.earning_id=e.id AND b.user_id=e.user_id AND b.cents>=e.amount_cents)
 ));
$$;
-- Defense in depth: privileged application paths cannot silently transition an
-- unfunded legacy obligation into a payment reservation or completed payment.
-- Already-paid history stays readable; reconciliation of already-submitted
-- legacy transfers uses an explicit historical exception, not a new POST.
CREATE FUNCTION cloud_reward_legacy_payment_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.amount_cents>0 AND NEW.reference NOT LIKE 'vesting:%' AND NEW.reference NOT LIKE 'correction:%' AND NEW.status IN('held','paid') AND OLD.status<>NEW.status AND NOT cloud_reward_earning_backed(OLD.id) AND NOT (NEW.status='paid' AND EXISTS(SELECT 1 FROM cloud_reward_legacy_transfer_exceptions x WHERE x.earning_id=OLD.id)) THEN
  RAISE EXCEPTION 'legacy earning requires independently approved fresh backing before payment';
 END IF;
 RETURN NEW; END $$;
CREATE TRIGGER cloud_reward_legacy_payment_guard BEFORE UPDATE ON cloud_earnings FOR EACH ROW EXECUTE FUNCTION cloud_reward_legacy_payment_guard();
