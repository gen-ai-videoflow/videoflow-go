CREATE TABLE cloud_playback_grants(token_hash text PRIMARY KEY,video_id text NOT NULL REFERENCES cloud_videos(id),user_id text REFERENCES cloud_users(id),expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX ON cloud_playback_grants(expires_at);
CREATE TABLE cloud_credit_postings(id bigserial PRIMARY KEY,entry_id text NOT NULL REFERENCES cloud_credit_entries(id),account text NOT NULL,amount bigint NOT NULL,UNIQUE(entry_id,account));
INSERT INTO cloud_credit_postings(entry_id,account,amount)SELECT id,'workspace:'||workspace_id,amount FROM cloud_credit_entries;
INSERT INTO cloud_credit_postings(entry_id,account,amount)SELECT id,'platform:credits',-amount FROM cloud_credit_entries;
CREATE FUNCTION cloud_balance_credit_entry() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (SELECT coalesce(sum(amount),0) FROM cloud_credit_postings WHERE entry_id=NEW.entry_id)<>0 THEN
  RAISE EXCEPTION 'credit journal is not balanced';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER cloud_credit_balanced AFTER INSERT OR UPDATE ON cloud_credit_postings DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION cloud_balance_credit_entry();
CREATE FUNCTION cloud_credit_entry_postings() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO cloud_credit_postings(entry_id,account,amount)VALUES(NEW.id,'workspace:'||NEW.workspace_id,NEW.amount),(NEW.id,'platform:credits',-NEW.amount);
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_credit_auto_post AFTER INSERT ON cloud_credit_entries FOR EACH ROW EXECUTE FUNCTION cloud_credit_entry_postings();
CREATE TABLE cloud_watch_sessions(id text PRIMARY KEY,video_id text NOT NULL REFERENCES cloud_videos(id),user_id text REFERENCES cloud_users(id),viewer_hash text NOT NULL,started_at timestamptz NOT NULL DEFAULT now(),last_event_at timestamptz NOT NULL DEFAULT now(),seconds integer NOT NULL DEFAULT 0,view_counted boolean NOT NULL DEFAULT false);
CREATE TABLE cloud_daily_viewers(video_id text NOT NULL REFERENCES cloud_videos(id),viewer_hash text NOT NULL,day date NOT NULL,PRIMARY KEY(video_id,viewer_hash,day));
ALTER TABLE cloud_watch_events ADD COLUMN session_id text REFERENCES cloud_watch_sessions(id);
CREATE TABLE cloud_viewer_preferences(user_id text NOT NULL REFERENCES cloud_users(id),channel_id text NOT NULL REFERENCES cloud_channels(id),hidden boolean NOT NULL DEFAULT true,PRIMARY KEY(user_id,channel_id));
