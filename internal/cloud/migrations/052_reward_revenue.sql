-- USD evidence is independently reviewed; a checkout price is not settlement proof.
CREATE TABLE cloud_reward_revenue_evidence (
 id text PRIMARY KEY, kind text NOT NULL CHECK(kind IN('receipt','cost','reversal')),
 resource_id text NOT NULL, payload jsonb NOT NULL, reason text NOT NULL CHECK(length(reason)>0),
 proposed_by text NOT NULL, approved_by text CHECK(approved_by<>proposed_by),
 created_at timestamptz NOT NULL DEFAULT now(), approved_at timestamptz
);
CREATE UNIQUE INDEX cloud_reward_revenue_one_approved ON cloud_reward_revenue_evidence(kind,resource_id) WHERE approved_by IS NOT NULL AND kind<>'reversal';
CREATE TABLE cloud_reward_revenue_lots (
 id text PRIMARY KEY REFERENCES cloud_reward_revenue_evidence(id), provider text NOT NULL,
 receipt_id text NOT NULL, workspace_id text NOT NULL REFERENCES cloud_workspaces(id), credits bigint NOT NULL CHECK(credits>0),
 gross_cents bigint NOT NULL CHECK(gross_cents>=0), fee_cents bigint NOT NULL CHECK(fee_cents>=0),
 tax_cents bigint NOT NULL CHECK(tax_cents>=0), refund_cents bigint NOT NULL CHECK(refund_cents>=0),
 acquired_at timestamptz NOT NULL, settled_at timestamptz NOT NULL, evidence_reference text NOT NULL,
 UNIQUE(provider,receipt_id), CHECK(fee_cents+tax_cents+refund_cents<=gross_cents)
);
CREATE TABLE cloud_reward_revenue_consumption (
 job_id text NOT NULL REFERENCES cloud_jobs(id), lot_id text NOT NULL REFERENCES cloud_reward_revenue_lots(id),
 credits bigint NOT NULL CHECK(credits>0), net_cents bigint NOT NULL CHECK(net_cents>=0),
 PRIMARY KEY(job_id,lot_id)
);
CREATE TABLE cloud_reward_revenue_recognition (
 job_id text PRIMARY KEY REFERENCES cloud_jobs(id), day date NOT NULL,
 cost_evidence_id text NOT NULL REFERENCES cloud_reward_revenue_evidence(id),
 purchased_credits bigint NOT NULL CHECK(purchased_credits>0), net_receipts_cents bigint NOT NULL,
 provider_cost_cents bigint NOT NULL CHECK(provider_cost_cents>=0), media_cost_cents bigint NOT NULL CHECK(media_cost_cents>=0),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_reward_revenue_days (
 day date PRIMARY KEY, policy_revision bigint NOT NULL, funding_bps bigint NOT NULL, reserve_bps bigint NOT NULL, daily_cap_cents bigint NOT NULL,
 net_cents bigint NOT NULL, funded_cents bigint NOT NULL CHECK(funded_cents>=0),
 source_id text UNIQUE REFERENCES cloud_reward_sources(id), actor text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_reward_revenue_source_holds (
 source_id text PRIMARY KEY REFERENCES cloud_reward_sources(id), reason text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE FUNCTION cloud_reward_revenue_evidence_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR OLD.approved_by IS NOT NULL OR NEW.id<>OLD.id OR NEW.kind<>OLD.kind OR NEW.resource_id<>OLD.resource_id OR NEW.payload<>OLD.payload OR NEW.reason<>OLD.reason OR NEW.proposed_by<>OLD.proposed_by OR NEW.created_at<>OLD.created_at THEN RAISE EXCEPTION 'revenue evidence is immutable'; END IF; RETURN NEW; END $$;
CREATE TRIGGER cloud_reward_revenue_evidence_guard BEFORE UPDATE OR DELETE ON cloud_reward_revenue_evidence FOR EACH ROW EXECUTE FUNCTION cloud_reward_revenue_evidence_guard();
CREATE TRIGGER cloud_reward_revenue_lot_immutable BEFORE UPDATE OR DELETE ON cloud_reward_revenue_lots FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE TRIGGER cloud_reward_revenue_consumption_immutable BEFORE UPDATE OR DELETE ON cloud_reward_revenue_consumption FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE TRIGGER cloud_reward_revenue_recognition_immutable BEFORE UPDATE OR DELETE ON cloud_reward_revenue_recognition FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE TRIGGER cloud_reward_revenue_day_immutable BEFORE UPDATE OR DELETE ON cloud_reward_revenue_days FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE FUNCTION cloud_reward_revenue_hold_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.reserved_cents>OLD.reserved_cents AND EXISTS(SELECT 1 FROM cloud_reward_revenue_source_holds WHERE source_id=NEW.id) THEN RAISE EXCEPTION 'revenue backing held for refund or shortfall review'; END IF; RETURN NEW; END $$;
CREATE TRIGGER cloud_reward_revenue_hold_guard BEFORE UPDATE ON cloud_reward_sources FOR EACH ROW EXECUTE FUNCTION cloud_reward_revenue_hold_guard();
ALTER TABLE cloud_reward_revenue_days ADD COLUMN carry_loss_cents bigint NOT NULL DEFAULT 0 CHECK(carry_loss_cents>=0);
