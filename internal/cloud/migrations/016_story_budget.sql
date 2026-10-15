ALTER TABLE cloud_pipeline_plans ADD COLUMN story_max_cost_micros bigint NOT NULL DEFAULT 0 CHECK(story_max_cost_micros>=0);
