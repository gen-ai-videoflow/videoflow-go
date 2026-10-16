ALTER TABLE cloud_purchases ADD COLUMN commercial_revision bigint NOT NULL DEFAULT 0 CHECK(commercial_revision>=0);
ALTER TABLE cloud_crypto_invoices ADD COLUMN commercial_revision bigint NOT NULL DEFAULT 0 CHECK(commercial_revision>=0);
CREATE FUNCTION cloud_commercial_purchase_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF (NEW.id,NEW.workspace_id,NEW.pack_id,NEW.credits,NEW.price_cents,NEW.commercial_revision) IS DISTINCT FROM (OLD.id,OLD.workspace_id,OLD.pack_id,OLD.credits,OLD.price_cents,OLD.commercial_revision) THEN RAISE EXCEPTION 'purchase price snapshot is immutable'; END IF;
 IF TG_TABLE_NAME='cloud_purchases' THEN IF NEW.currency<>OLD.currency THEN RAISE EXCEPTION 'purchase currency snapshot is immutable'; END IF; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_purchase_commercial_guard BEFORE UPDATE ON cloud_purchases FOR EACH ROW EXECUTE FUNCTION cloud_commercial_purchase_guard();
CREATE TRIGGER cloud_crypto_commercial_guard BEFORE UPDATE ON cloud_crypto_invoices FOR EACH ROW EXECUTE FUNCTION cloud_commercial_purchase_guard();
CREATE FUNCTION cloud_crypto_price_terms_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF (NEW.user_id,NEW.purpose,NEW.reference,NEW.network,NEW.vm_type,NEW.token,NEW.token_address,NEW.treasury_address,NEW.amount,NEW.consolidate_amount,NEW.amount_net,NEW.fee_amount) IS DISTINCT FROM (OLD.user_id,OLD.purpose,OLD.reference,OLD.network,OLD.vm_type,OLD.token,OLD.token_address,OLD.treasury_address,OLD.amount,OLD.consolidate_amount,OLD.amount_net,OLD.fee_amount) THEN RAISE EXCEPTION 'crypto invoice terms are immutable'; END IF; RETURN NEW;
END $$;
CREATE TRIGGER cloud_crypto_price_terms_guard BEFORE UPDATE ON cloud_crypto_invoices FOR EACH ROW EXECUTE FUNCTION cloud_crypto_price_terms_guard();
