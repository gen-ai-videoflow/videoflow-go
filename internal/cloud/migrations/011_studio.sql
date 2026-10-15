CREATE TABLE cloud_studio_entities(
 id text PRIMARY KEY,project_id text NOT NULL REFERENCES cloud_projects(id),
 kind text NOT NULL CHECK(kind IN ('scripts','characters','scenes','shots','assets')),
 scene_id text,title text NOT NULL,content text NOT NULL DEFAULT '',description text NOT NULL DEFAULT '',prompt text NOT NULL DEFAULT '',object_key text NOT NULL DEFAULT '',
 duration double precision NOT NULL DEFAULT 0 CHECK(duration>=0 AND duration<=3600),position integer NOT NULL DEFAULT 0 CHECK(position>=0 AND position<=10000),
 metadata jsonb NOT NULL DEFAULT '{}',created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(project_id,id),FOREIGN KEY(project_id,scene_id)REFERENCES cloud_studio_entities(project_id,id) ON DELETE CASCADE,
 CHECK((kind='shots' AND scene_id IS NOT NULL)OR(kind<>'shots' AND scene_id IS NULL))
);
CREATE INDEX ON cloud_studio_entities(project_id,kind,position);
CREATE TABLE cloud_job_captions(job_id text PRIMARY KEY REFERENCES cloud_jobs(id),segments jsonb NOT NULL DEFAULT '[]',version bigint NOT NULL DEFAULT 1,updated_at timestamptz NOT NULL DEFAULT now());
