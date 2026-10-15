CREATE TABLE cloud_media_assets (
 id text PRIMARY KEY,workspace_id text NOT NULL REFERENCES cloud_workspaces(id),source_job_id text UNIQUE REFERENCES cloud_jobs(id),
 origin text NOT NULL CHECK(origin IN ('generated','upload')),object_key text NOT NULL,status text NOT NULL CHECK(status IN ('ready','validating','rejected')),
 filename text NOT NULL DEFAULT 'video.mp4',content_type text NOT NULL DEFAULT 'video/mp4',bytes bigint NOT NULL DEFAULT 0,
 width integer NOT NULL DEFAULT 0,height integer NOT NULL DEFAULT 0,duration_ms bigint NOT NULL DEFAULT 0,has_audio boolean NOT NULL DEFAULT false,
 measured boolean NOT NULL DEFAULT false,checksum text NOT NULL DEFAULT '',error text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON cloud_media_assets(workspace_id,created_at);
ALTER TABLE cloud_videos ADD COLUMN source_asset_id text REFERENCES cloud_media_assets(id);
ALTER TABLE cloud_videos ALTER COLUMN job_id DROP NOT NULL;
ALTER TABLE cloud_videos ADD COLUMN format text NOT NULL DEFAULT 'standard' CHECK(format IN ('standard','short'));
ALTER TABLE cloud_videos ADD COLUMN origin text NOT NULL DEFAULT 'generated' CHECK(origin IN ('generated','upload'));
ALTER TABLE cloud_videos ADD COLUMN ai_generated boolean NOT NULL DEFAULT true;
CREATE UNIQUE INDEX cloud_videos_source_unique ON cloud_videos(channel_id,source_asset_id) WHERE source_asset_id IS NOT NULL;
