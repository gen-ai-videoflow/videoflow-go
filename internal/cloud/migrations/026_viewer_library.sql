CREATE TABLE cloud_saved_videos(user_id text NOT NULL REFERENCES cloud_users(id) ON DELETE CASCADE,video_id text NOT NULL REFERENCES cloud_videos(id),created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(user_id,video_id));
CREATE TABLE cloud_video_likes(user_id text NOT NULL REFERENCES cloud_users(id) ON DELETE CASCADE,video_id text NOT NULL REFERENCES cloud_videos(id),created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(user_id,video_id));
CREATE TABLE cloud_video_progress(user_id text NOT NULL REFERENCES cloud_users(id) ON DELETE CASCADE,video_id text NOT NULL REFERENCES cloud_videos(id),position_ms bigint NOT NULL CHECK(position_ms>=0 AND position_ms<=86400000),updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(user_id,video_id));
CREATE INDEX cloud_video_progress_user_recent ON cloud_video_progress(user_id,updated_at DESC);
CREATE INDEX cloud_videos_discovery_page ON cloud_videos(format,published_at DESC,id DESC) WHERE status='published' AND visibility='public';
