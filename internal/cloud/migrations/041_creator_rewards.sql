-- New reward accounting is isolated from legacy monthly/cliff-held earnings.
CREATE TABLE cloud_reward_policies (
 version text PRIMARY KEY, terms_version text NOT NULL, active boolean NOT NULL DEFAULT false,
 window_seconds bigint NOT NULL DEFAULT 2592000 CHECK(window_seconds=2592000),
 vesting_seconds bigint NOT NULL DEFAULT 2592000 CHECK(vesting_seconds=2592000),
 created_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO cloud_reward_policies(version,terms_version,created_by) VALUES('pilot-v1','pilot-v1','migration');
CREATE UNIQUE INDEX cloud_reward_one_active_policy ON cloud_reward_policies(active) WHERE active;
CREATE TABLE cloud_reward_enrollments (
 user_id text NOT NULL REFERENCES cloud_users(id), policy_version text NOT NULL REFERENCES cloud_reward_policies(version),
 accepted_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(user_id,policy_version)
);
ALTER TABLE cloud_videos ADD COLUMN first_published_at timestamptz;
UPDATE cloud_videos SET first_published_at=published_at WHERE published_at IS NOT NULL;
CREATE FUNCTION cloud_reward_publication_clock() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND OLD.first_published_at IS NOT NULL THEN NEW.first_published_at=OLD.first_published_at;
 ELSIF NEW.status='published' THEN NEW.first_published_at=COALESCE(NEW.first_published_at,NEW.published_at,now());
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_reward_publication_clock BEFORE INSERT OR UPDATE ON cloud_videos FOR EACH ROW EXECUTE FUNCTION cloud_reward_publication_clock();
CREATE INDEX cloud_reward_video_window ON cloud_videos(first_published_at) WHERE first_published_at IS NOT NULL;
CREATE TABLE cloud_reward_sources (
 id text PRIMARY KEY, reference text NOT NULL UNIQUE, kind text NOT NULL CHECK(kind IN('generation','advertising','support')),
 currency text NOT NULL DEFAULT 'usd' CHECK(currency='usd'), cents bigint NOT NULL CHECK(cents>0),
 reserved_cents bigint NOT NULL DEFAULT 0 CHECK(reserved_cents>=0 AND reserved_cents<=cents),
 evidence_reference text NOT NULL CHECK(length(evidence_reference)>0), proposed_by text NOT NULL,
 reconciled_by text NOT NULL CHECK(reconciled_by<>proposed_by), reconciled_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_reward_batches (
 id text PRIMARY KEY, day date NOT NULL, source_id text NOT NULL REFERENCES cloud_reward_sources(id),
 policy_version text NOT NULL REFERENCES cloud_reward_policies(version), input_hash text NOT NULL,
 evidence_reference text NOT NULL, total_cents bigint NOT NULL CHECK(total_cents>0),
 proposed_by text NOT NULL, confirmed_by text, confirmed_at timestamptz,
 status text NOT NULL DEFAULT 'draft' CHECK(status IN('draft','confirmed')),
 UNIQUE(day,source_id,policy_version),
 CHECK((status='draft' AND confirmed_at IS NULL AND confirmed_by IS NULL) OR (status='confirmed' AND confirmed_at IS NOT NULL AND confirmed_by IS NOT NULL AND confirmed_by<>proposed_by))
);
CREATE TABLE cloud_reward_allocations (
 id text PRIMARY KEY, batch_id text NOT NULL REFERENCES cloud_reward_batches(id), video_id text NOT NULL REFERENCES cloud_videos(id),
 user_id text NOT NULL REFERENCES cloud_users(id), cents bigint NOT NULL CHECK(cents>0),
 UNIQUE(batch_id,video_id)
);
CREATE INDEX cloud_reward_allocations_owner ON cloud_reward_allocations(user_id,batch_id);
CREATE TABLE cloud_reward_releases (
 allocation_id text PRIMARY KEY REFERENCES cloud_reward_allocations(id), released_cents bigint NOT NULL DEFAULT 0 CHECK(released_cents>=0),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_reward_release_events (
 id text PRIMARY KEY, allocation_id text NOT NULL REFERENCES cloud_reward_allocations(id),
 through_cents bigint NOT NULL CHECK(through_cents>0), earning_id text NOT NULL UNIQUE REFERENCES cloud_earnings(id),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(allocation_id,through_cents)
);
CREATE TABLE cloud_reward_holds (
 user_id text PRIMARY KEY REFERENCES cloud_users(id), reason text NOT NULL, actor text NOT NULL, updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE FUNCTION cloud_reward_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'reward financial evidence is immutable; use an audited adjustment'; END $$;
CREATE TRIGGER cloud_reward_allocation_immutable BEFORE UPDATE OR DELETE ON cloud_reward_allocations FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE TRIGGER cloud_reward_release_event_immutable BEFORE UPDATE OR DELETE ON cloud_reward_release_events FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
CREATE FUNCTION cloud_reward_source_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'funding source is immutable'; END IF;
 IF ROW(NEW.id,NEW.reference,NEW.kind,NEW.currency,NEW.cents,NEW.evidence_reference,NEW.proposed_by,NEW.reconciled_by,NEW.reconciled_at) IS DISTINCT FROM ROW(OLD.id,OLD.reference,OLD.kind,OLD.currency,OLD.cents,OLD.evidence_reference,OLD.proposed_by,OLD.reconciled_by,OLD.reconciled_at) OR NEW.reserved_cents<OLD.reserved_cents THEN RAISE EXCEPTION 'funding evidence is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_reward_source_guard BEFORE UPDATE OR DELETE ON cloud_reward_sources FOR EACH ROW EXECUTE FUNCTION cloud_reward_source_guard();
CREATE FUNCTION cloud_reward_batch_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR OLD.status='confirmed' THEN RAISE EXCEPTION 'confirmed reward batch is immutable'; END IF;
 IF ROW(NEW.id,NEW.day,NEW.source_id,NEW.policy_version,NEW.input_hash,NEW.evidence_reference,NEW.total_cents,NEW.proposed_by) IS DISTINCT FROM ROW(OLD.id,OLD.day,OLD.source_id,OLD.policy_version,OLD.input_hash,OLD.evidence_reference,OLD.total_cents,OLD.proposed_by) THEN RAISE EXCEPTION 'batch inputs are immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_reward_batch_guard BEFORE UPDATE OR DELETE ON cloud_reward_batches FOR EACH ROW EXECUTE FUNCTION cloud_reward_batch_guard();
