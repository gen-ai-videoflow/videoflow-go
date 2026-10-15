CREATE TABLE cloud_prompt_templates (
 id text PRIMARY KEY,
 project_id text NOT NULL REFERENCES cloud_projects(id) ON DELETE CASCADE,
 revision bigint NOT NULL CHECK(revision>0),
 template jsonb NOT NULL CHECK(jsonb_typeof(template)='object'),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cloud_prompt_templates_project ON cloud_prompt_templates(project_id,updated_at DESC);
