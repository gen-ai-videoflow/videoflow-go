CREATE TABLE cloud_video_preferences (
 user_id text PRIMARY KEY REFERENCES cloud_users(id) ON DELETE CASCADE,
 settings jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(settings)='object'),
 updated_at timestamptz NOT NULL DEFAULT now()
);
