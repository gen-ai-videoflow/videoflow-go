CREATE TABLE cloud_projects (
 id text PRIMARY KEY, workspace_id text NOT NULL REFERENCES cloud_workspaces(id),
 name text NOT NULL, description text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON cloud_projects(workspace_id,created_at);
CREATE TABLE cloud_credit_entries (
 id text PRIMARY KEY, workspace_id text NOT NULL REFERENCES cloud_credit_accounts(workspace_id),
 reference text NOT NULL, amount bigint NOT NULL, kind text NOT NULL, description text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(workspace_id,reference)
);
CREATE TABLE cloud_quotes (
 id text PRIMARY KEY, workspace_id text NOT NULL REFERENCES cloud_workspaces(id),
 project_id text NOT NULL REFERENCES cloud_projects(id), prompt text NOT NULL, duration integer NOT NULL CHECK(duration BETWEEN 5 AND 30),
 aspect_ratio text NOT NULL, credits bigint NOT NULL CHECK(credits>0), model text NOT NULL,
 expires_at timestamptz NOT NULL, consumed boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_jobs (
 id text PRIMARY KEY, workspace_id text NOT NULL REFERENCES cloud_workspaces(id), quote_id text NOT NULL UNIQUE REFERENCES cloud_quotes(id),
 idempotency_key text NOT NULL, status text NOT NULL DEFAULT 'queued', credits bigint NOT NULL,
 provider_id text NOT NULL DEFAULT '', output_key text NOT NULL DEFAULT '', error text NOT NULL DEFAULT '',
 attempt integer NOT NULL DEFAULT 0, lease_until timestamptz, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(workspace_id,idempotency_key)
);
CREATE TABLE cloud_outbox (
 id text PRIMARY KEY, kind text NOT NULL, aggregate_id text NOT NULL, payload jsonb NOT NULL DEFAULT '{}',
 sent_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON cloud_outbox(created_at) WHERE sent_at IS NULL;
CREATE TABLE cloud_job_events (
 id bigserial PRIMARY KEY, job_id text NOT NULL REFERENCES cloud_jobs(id), status text NOT NULL, message text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON cloud_job_events(job_id,id);
CREATE TABLE cloud_purchases (
 id text PRIMARY KEY, workspace_id text NOT NULL REFERENCES cloud_workspaces(id), pack_id text NOT NULL,
 credits bigint NOT NULL, price_cents bigint NOT NULL, currency text NOT NULL, status text NOT NULL DEFAULT 'pending',
 stripe_session text UNIQUE, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloud_webhook_receipts (id text PRIMARY KEY, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE cloud_videos (
 id text PRIMARY KEY, channel_id text NOT NULL REFERENCES cloud_channels(id), job_id text NOT NULL REFERENCES cloud_jobs(id),
 title text NOT NULL, description text NOT NULL DEFAULT '', visibility text NOT NULL CHECK(visibility IN ('public','unlisted','private')),
 audience text NOT NULL CHECK(audience IN ('free','members')), status text NOT NULL DEFAULT 'draft',
 manifest_key text NOT NULL DEFAULT '', encoding_id text NOT NULL DEFAULT '', thumbnail_key text NOT NULL DEFAULT '',
 views bigint NOT NULL DEFAULT 0, watch_seconds bigint NOT NULL DEFAULT 0, created_at timestamptz NOT NULL DEFAULT now(),
 published_at timestamptz, UNIQUE(channel_id,job_id)
);
CREATE INDEX ON cloud_videos(published_at DESC) WHERE status='published' AND visibility='public';
CREATE TABLE cloud_follows (user_id text REFERENCES cloud_users(id),channel_id text REFERENCES cloud_channels(id), created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(user_id,channel_id));
CREATE TABLE cloud_memberships (id text PRIMARY KEY,user_id text NOT NULL REFERENCES cloud_users(id), channel_id text NOT NULL REFERENCES cloud_channels(id), status text NOT NULL,paid_through timestamptz NOT NULL,provider_id text UNIQUE, UNIQUE(user_id,channel_id));
CREATE TABLE cloud_watch_events (id text PRIMARY KEY,user_id text REFERENCES cloud_users(id),video_id text NOT NULL REFERENCES cloud_videos(id),seconds integer NOT NULL CHECK(seconds BETWEEN 0 AND 15),created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX ON cloud_watch_events(video_id,created_at);
CREATE TABLE cloud_reports(id text PRIMARY KEY,user_id text NOT NULL REFERENCES cloud_users(id),video_id text NOT NULL REFERENCES cloud_videos(id),reason text NOT NULL,status text NOT NULL DEFAULT 'open',created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE cloud_earnings(id text PRIMARY KEY,user_id text NOT NULL REFERENCES cloud_users(id),reference text NOT NULL UNIQUE,amount_cents bigint NOT NULL,currency text NOT NULL,status text NOT NULL,description text NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
