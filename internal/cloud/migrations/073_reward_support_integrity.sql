CREATE FUNCTION cloud_reward_support_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='cloud_reward_support_programs' THEN
 IF
 (NEW.title,NEW.criteria,NEW.source_id,NEW.budget_cents,NEW.grant_credits,NEW.service_cost_cents,NEW.creator_cap,NEW.grant_cap,NEW.expiry_days,NEW.minimum_publications,NEW.proposed_by) IS DISTINCT FROM
 (OLD.title,OLD.criteria,OLD.source_id,OLD.budget_cents,OLD.grant_credits,OLD.service_cost_cents,OLD.creator_cap,OLD.grant_cap,OLD.expiry_days,OLD.minimum_publications,OLD.proposed_by) THEN
 RAISE EXCEPTION 'support program versions are immutable';
 END IF; END IF;
 IF TG_TABLE_NAME='cloud_promotional_credits' THEN
 IF (NEW.id,NEW.workspace_id,NEW.credits,NEW.service_cost_cents,NEW.expires_at) IS DISTINCT FROM (OLD.id,OLD.workspace_id,OLD.credits,OLD.service_cost_cents,OLD.expires_at) THEN
 RAISE EXCEPTION 'promotional grant terms are immutable';
 END IF; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_support_program_immutable BEFORE UPDATE ON cloud_reward_support_programs FOR EACH ROW EXECUTE FUNCTION cloud_reward_support_immutable();
CREATE TRIGGER cloud_promo_terms_immutable BEFORE UPDATE ON cloud_promotional_credits FOR EACH ROW EXECUTE FUNCTION cloud_reward_support_immutable();
