CREATE TABLE cloud_editor_documents (
 project_id text PRIMARY KEY REFERENCES cloud_projects(id) ON DELETE CASCADE,
 revision bigint NOT NULL CHECK (revision > 0),
 document jsonb NOT NULL CHECK (jsonb_typeof(document) = 'object'),
 updated_at timestamptz NOT NULL DEFAULT now()
);
