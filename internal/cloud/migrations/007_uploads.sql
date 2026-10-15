CREATE TABLE cloud_upload_reservations (
 object_key text PRIMARY KEY, workspace_id text NOT NULL REFERENCES cloud_workspaces(id),
 bytes bigint NOT NULL CHECK(bytes>0 AND bytes<=104857600),
 created_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL DEFAULT now()+interval '1 day',
 deleted_at timestamptz
);
CREATE INDEX ON cloud_upload_reservations(workspace_id) WHERE deleted_at IS NULL;
