ALTER TABLE cloud_videos ADD COLUMN caption_segments jsonb NOT NULL DEFAULT '[]';
