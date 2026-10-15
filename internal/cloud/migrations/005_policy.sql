CREATE TABLE cloud_video_catalog (id boolean PRIMARY KEY DEFAULT true CHECK(id),catalog jsonb NOT NULL,refreshed_at timestamptz NOT NULL);
CREATE TABLE cloud_video_policies (model text PRIMARY KEY,credits_per_second bigint NOT NULL CHECK(credits_per_second>0),max_cost_micros bigint NOT NULL CHECK(max_cost_micros>0),version text NOT NULL,enabled boolean NOT NULL DEFAULT true,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE cloud_provider_budget_days (day date PRIMARY KEY,reserved_micros bigint NOT NULL DEFAULT 0 CHECK(reserved_micros>=0));
CREATE TABLE cloud_provider_admissions(job_id text PRIMARY KEY REFERENCES cloud_jobs(id),day date NOT NULL REFERENCES cloud_provider_budget_days(day),max_cost_micros bigint NOT NULL CHECK(max_cost_micros>0),created_at timestamptz NOT NULL DEFAULT now());
ALTER TABLE cloud_quotes ADD COLUMN policy_version text NOT NULL DEFAULT '';
ALTER TABLE cloud_quotes ADD COLUMN provider_max_cost_micros bigint NOT NULL DEFAULT 0;
ALTER TABLE cloud_quotes ADD COLUMN resolution text NOT NULL DEFAULT '720p';
