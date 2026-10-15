CREATE TABLE cloud_watch_clocks(user_id text PRIMARY KEY REFERENCES cloud_users(id),last_at timestamptz NOT NULL DEFAULT now());
INSERT INTO cloud_watch_clocks(user_id,last_at)SELECT user_id,min(started_at)FROM cloud_watch_sessions WHERE user_id IS NOT NULL GROUP BY user_id;
