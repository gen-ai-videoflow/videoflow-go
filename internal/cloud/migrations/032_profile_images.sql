CREATE TABLE IF NOT EXISTS cloud_profile_images (
 id text PRIMARY KEY,
 channel_id text NOT NULL REFERENCES cloud_channels(id) ON DELETE CASCADE,
 object_key text NOT NULL UNIQUE,
 kind text NOT NULL CHECK (kind IN ('avatar','banner')),
 bytes bigint NOT NULL CHECK (bytes > 0 AND bytes <= 3145728),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS cloud_profile_images_channel_idx ON cloud_profile_images(channel_id);
