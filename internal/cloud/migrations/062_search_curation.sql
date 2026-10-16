ALTER TABLE cloud_discovery_overrides ADD COLUMN search_query text NOT NULL DEFAULT '' CHECK(length(search_query)<=200);
-- Expression matches the live FTS predicate; built once by migration, not an unbounded admin job.
CREATE INDEX cloud_discovery_search_fts ON cloud_videos USING gin(to_tsvector('simple',title||' '||description));
