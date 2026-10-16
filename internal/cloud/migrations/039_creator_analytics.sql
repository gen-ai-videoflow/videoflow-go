CREATE INDEX IF NOT EXISTS cloud_daily_viewers_video_day_idx ON cloud_daily_viewers(video_id,day);
CREATE INDEX IF NOT EXISTS cloud_video_likes_user_created_idx ON cloud_video_likes(user_id,created_at DESC);
