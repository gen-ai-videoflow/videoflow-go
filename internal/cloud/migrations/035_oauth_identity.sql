CREATE TABLE cloud_oauth_identities (
 provider text NOT NULL CHECK(provider IN ('google','apple')),
 subject text NOT NULL,
 user_id text NOT NULL REFERENCES cloud_users(id) ON DELETE CASCADE,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(provider,subject), UNIQUE(user_id,provider)
);
CREATE TABLE cloud_oauth_states (
 token_hash text PRIMARY KEY,
 provider text NOT NULL,
 browser_hash text NOT NULL,
 nonce_hash text NOT NULL,
 verifier_ciphertext text NOT NULL,
 link_user_id text REFERENCES cloud_users(id) ON DELETE CASCADE,
 link_session_hash text,
 expires_at timestamptz NOT NULL DEFAULT now()+interval '5 minutes'
);
CREATE INDEX cloud_oauth_states_expiry ON cloud_oauth_states(expires_at);
