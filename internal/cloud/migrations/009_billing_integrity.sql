ALTER TABLE cloud_purchases ADD COLUMN idempotency_key text;
UPDATE cloud_purchases SET idempotency_key=id;
ALTER TABLE cloud_purchases ALTER COLUMN idempotency_key SET NOT NULL;
CREATE UNIQUE INDEX cloud_purchase_idempotency ON cloud_purchases(workspace_id,idempotency_key);
ALTER TABLE cloud_purchases ADD COLUMN checkout_url text NOT NULL DEFAULT '';
CREATE TABLE cloud_money_postings(id bigserial PRIMARY KEY,entry_id text NOT NULL REFERENCES cloud_earnings(id),account text NOT NULL,currency text NOT NULL,amount_cents bigint NOT NULL,UNIQUE(entry_id,account));
INSERT INTO cloud_money_postings(entry_id,account,currency,amount_cents)SELECT id,'creator:'||user_id,currency,amount_cents FROM cloud_earnings;
INSERT INTO cloud_money_postings(entry_id,account,currency,amount_cents)SELECT id,'platform:creator_payable',currency,-amount_cents FROM cloud_earnings;
CREATE FUNCTION cloud_money_entry_postings() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO cloud_money_postings(entry_id,account,currency,amount_cents)VALUES(NEW.id,'creator:'||NEW.user_id,NEW.currency,NEW.amount_cents),(NEW.id,'platform:creator_payable',NEW.currency,-NEW.amount_cents);
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_money_auto_post AFTER INSERT ON cloud_earnings FOR EACH ROW EXECUTE FUNCTION cloud_money_entry_postings();
CREATE FUNCTION cloud_balance_money_entry() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT currency FROM cloud_money_postings WHERE entry_id=NEW.entry_id GROUP BY currency HAVING sum(amount_cents)<>0) THEN
 RAISE EXCEPTION 'money journal is not balanced';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER cloud_money_balanced AFTER INSERT OR UPDATE ON cloud_money_postings DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION cloud_balance_money_entry();
