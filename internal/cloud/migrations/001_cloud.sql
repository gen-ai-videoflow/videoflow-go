CREATE TABLE IF NOT EXISTS cloud_users (
 id text PRIMARY KEY, email text NOT NULL UNIQUE, password_hash text NOT NULL,
 name text NOT NULL, verified boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS cloud_workspaces (
 id text PRIMARY KEY, owner_id text NOT NULL UNIQUE REFERENCES cloud_users(id) ON DELETE CASCADE,
 name text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS cloud_channels (
 id text PRIMARY KEY, user_id text NOT NULL UNIQUE REFERENCES cloud_users(id) ON DELETE CASCADE,
 handle text NOT NULL UNIQUE, name text NOT NULL, description text NOT NULL DEFAULT '',
 avatar_url text NOT NULL DEFAULT '', banner_url text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS cloud_credit_accounts (
 workspace_id text PRIMARY KEY REFERENCES cloud_workspaces(id) ON DELETE CASCADE,
 balance bigint NOT NULL DEFAULT 0, reserved bigint NOT NULL DEFAULT 0,
 CHECK(balance >= 0 AND reserved >= 0 AND reserved <= balance)
);
CREATE TABLE IF NOT EXISTS cloud_sessions (
 token_hash text PRIMARY KEY, user_id text NOT NULL REFERENCES cloud_users(id) ON DELETE CASCADE,
 created_at timestamptz NOT NULL DEFAULT now(), last_seen_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL DEFAULT now() + interval '30 days'
);
CREATE INDEX IF NOT EXISTS cloud_sessions_user_idx ON cloud_sessions(user_id);
CREATE TABLE IF NOT EXISTS cloud_tokens (
 token_hash text PRIMARY KEY, user_id text NOT NULL REFERENCES cloud_users(id) ON DELETE CASCADE,
 purpose text NOT NULL CHECK(purpose IN ('verify_email','reset_password')),
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
