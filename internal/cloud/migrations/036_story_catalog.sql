CREATE TABLE cloud_story_catalog(id boolean PRIMARY KEY DEFAULT true CHECK(id),catalog jsonb NOT NULL,refreshed_at timestamptz NOT NULL DEFAULT now());
