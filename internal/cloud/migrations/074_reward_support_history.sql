CREATE TABLE cloud_promotional_credit_events (
 id bigserial PRIMARY KEY, grant_id text NOT NULL REFERENCES cloud_promotional_credits(id),
 job_id text REFERENCES cloud_jobs(id), kind text NOT NULL CHECK(kind IN('grant','reserve','consume','release','expire')),
 credits bigint NOT NULL CHECK(credits>0), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX cloud_promo_event_once ON cloud_promotional_credit_events(grant_id,coalesce(job_id,''),kind);
CREATE FUNCTION cloud_reward_support_append_only() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'support history is append only'; END $$;
CREATE TRIGGER cloud_promo_history_immutable BEFORE UPDATE OR DELETE ON cloud_promotional_credit_events FOR EACH ROW EXECUTE FUNCTION cloud_reward_support_append_only();
CREATE FUNCTION cloud_reward_support_review_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='cloud_reward_support_claims' THEN
  IF (NEW.id,NEW.program_id,NEW.user_id,NEW.workspace_id,NEW.evidence,NEW.created_at) IS DISTINCT FROM (OLD.id,OLD.program_id,OLD.user_id,OLD.workspace_id,OLD.evidence,OLD.created_at) OR OLD.status<>'pending' THEN RAISE EXCEPTION 'support evidence and decisions are immutable'; END IF;
 ELSIF TG_TABLE_NAME='cloud_reward_appeals' THEN
  IF (NEW.id,NEW.user_id,NEW.allocation_id,NEW.subject,NEW.evidence,NEW.created_at) IS DISTINCT FROM (OLD.id,OLD.user_id,OLD.allocation_id,OLD.subject,OLD.evidence,OLD.created_at) OR OLD.status<>'open' THEN RAISE EXCEPTION 'appeal evidence and decisions are immutable'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_support_review_guard BEFORE UPDATE ON cloud_reward_support_claims FOR EACH ROW EXECUTE FUNCTION cloud_reward_support_review_guard();
CREATE TRIGGER cloud_appeal_review_guard BEFORE UPDATE ON cloud_reward_appeals FOR EACH ROW EXECUTE FUNCTION cloud_reward_support_review_guard();
