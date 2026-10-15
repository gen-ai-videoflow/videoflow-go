ALTER TABLE cloud_channels ADD COLUMN bio text NOT NULL DEFAULT '';
ALTER TABLE cloud_channels ADD COLUMN location text NOT NULL DEFAULT '';
ALTER TABLE cloud_channels ADD COLUMN website text NOT NULL DEFAULT '';
ALTER TABLE cloud_channels ADD COLUMN social_links jsonb NOT NULL DEFAULT '[]';
ALTER TABLE cloud_channels ADD COLUMN featured_video_id text NOT NULL DEFAULT '';
UPDATE cloud_channels SET bio=description WHERE description<>'';
