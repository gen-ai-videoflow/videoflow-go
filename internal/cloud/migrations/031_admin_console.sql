CREATE TABLE cloud_admin_roles (
 user_id text PRIMARY KEY REFERENCES cloud_users(id) ON DELETE CASCADE,
 role text NOT NULL CHECK(role IN ('viewer','moderator','owner')),
 granted_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_admin_settings (
 key text PRIMARY KEY CHECK(key IN ('generation_enabled','publishing_enabled','announcement')),
 value jsonb NOT NULL, revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 updated_by text NOT NULL, updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO cloud_admin_settings(key,value,updated_by) VALUES
 ('generation_enabled','true','migration'),('publishing_enabled','true','migration'),('announcement','""','migration');
CREATE INDEX cloud_admin_audit_created_idx ON cloud_admin_audit(created_at DESC,id DESC);
