ALTER TABLE cloud_media_assets ADD COLUMN poster_key text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX cloud_media_probe_once ON cloud_outbox(kind,aggregate_id) WHERE kind='media_probe';
