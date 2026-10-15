-- Public IDs are independent of authentication tokens and their hashes.
ALTER TABLE cloud_sessions ADD COLUMN id text NOT NULL DEFAULT gen_random_uuid()::text;
CREATE UNIQUE INDEX cloud_sessions_public_id_idx ON cloud_sessions(id);
CREATE INDEX cloud_sessions_expiry_idx ON cloud_sessions(expires_at);
CREATE INDEX cloud_sessions_activity_idx ON cloud_sessions(last_seen_at);
