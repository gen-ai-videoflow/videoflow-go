CREATE TABLE cloud_analytics_archive_batches (
 id text PRIMARY KEY, object_key text NOT NULL UNIQUE, payload_sha256 text NOT NULL,
 event_count integer NOT NULL CHECK(event_count>0), day date NOT NULL,
 archived_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_account_deletion_requests (
 user_id text PRIMARY KEY REFERENCES cloud_users(id), requested_at timestamptz NOT NULL DEFAULT now(),
 status text NOT NULL DEFAULT 'requested' CHECK(status IN ('requested','reviewing','completed','cancelled'))
);
CREATE INDEX ON cloud_watch_events(created_at,id);
CREATE INDEX ON cloud_upload_reservations(expires_at) WHERE deleted_at IS NULL;
