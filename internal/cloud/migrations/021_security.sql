CREATE TABLE cloud_totp (
 user_id text PRIMARY KEY REFERENCES cloud_users(id),secret_ciphertext text NOT NULL,enabled boolean NOT NULL DEFAULT false,last_step bigint NOT NULL DEFAULT -1,pending_expires_at timestamptz NOT NULL DEFAULT now()+interval '10 minutes',created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_recovery_codes (
 user_id text NOT NULL REFERENCES cloud_users(id),code_hash text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(user_id,code_hash)
);
CREATE TABLE cloud_auth_challenges (
 token_hash text PRIMARY KEY,user_id text REFERENCES cloud_users(id),purpose text NOT NULL CHECK(purpose IN ('password_mfa','passkey_register','passkey_login')),session_data jsonb NOT NULL DEFAULT '{}',expires_at timestamptz NOT NULL DEFAULT now()+interval '5 minutes'
);
CREATE TABLE cloud_passkeys (
 id text PRIMARY KEY,user_id text NOT NULL REFERENCES cloud_users(id),credential jsonb NOT NULL,name text NOT NULL,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON cloud_passkeys(user_id);
CREATE INDEX ON cloud_auth_challenges(expires_at);
ALTER TABLE cloud_sessions ADD COLUMN passkey_verified_at timestamptz;
