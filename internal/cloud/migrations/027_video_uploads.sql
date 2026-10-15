CREATE TABLE cloud_video_uploads(
 id text PRIMARY KEY,asset_id text UNIQUE NOT NULL REFERENCES cloud_media_assets(id),workspace_id text NOT NULL REFERENCES cloud_workspaces(id),
 object_key text UNIQUE NOT NULL,storage_upload_id text NOT NULL DEFAULT '',bytes bigint NOT NULL CHECK(bytes BETWEEN 1 AND 104857600),
 checksum text NOT NULL,filename text NOT NULL,content_type text NOT NULL,state text NOT NULL CHECK(state IN ('creating','uploading','completing','validating','ready','rejected','aborting','aborted')),
 idempotency_key text NOT NULL,fingerprint text NOT NULL,quota_released boolean NOT NULL DEFAULT false,error text NOT NULL DEFAULT '',
 expires_at timestamptz NOT NULL DEFAULT now()+interval '24 hours',created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(workspace_id,idempotency_key)
);
CREATE INDEX ON cloud_video_uploads(state,expires_at);
