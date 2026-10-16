CREATE TABLE cloud_user_restrictions (
 user_id text PRIMARY KEY REFERENCES cloud_users(id) ON DELETE CASCADE,
 suspended boolean NOT NULL, reason text NOT NULL, updated_by text NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cloud_admin_jobs_created_idx ON cloud_jobs(created_at DESC,id DESC);
CREATE INDEX cloud_admin_users_created_idx ON cloud_users(created_at DESC,id DESC);
CREATE INDEX cloud_admin_videos_created_idx ON cloud_videos(created_at DESC,id DESC);
ALTER TABLE cloud_sessions ADD COLUMN mfa_verified_at timestamptz;
-- Existing sessions must authenticate again; enrollment alone is not proof.
CREATE INDEX cloud_admin_purchases_created_idx ON cloud_purchases(created_at DESC,id DESC);
CREATE INDEX cloud_admin_follow_channel_idx ON cloud_follows(channel_id);
CREATE INDEX cloud_admin_published_views_idx ON cloud_videos(views DESC,id) WHERE status='published';
