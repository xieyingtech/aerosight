CREATE TABLE media_index_jobs (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id integer NOT NULL,
    asset_id integer NOT NULL,
    source_version integer NOT NULL,
    source_checksum text NOT NULL,
    space text NOT NULL,
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','running','indexed','failed','obsolete')),
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    error_code text,
    artifact_key text,
    artifact_checksum text,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (asset_id, source_version, source_checksum, space),
    FOREIGN KEY (asset_id, project_id) REFERENCES assets(id, project_id) ON DELETE CASCADE
);
CREATE INDEX media_index_jobs_pending ON media_index_jobs (next_attempt_at,id) WHERE state IN ('queued','running','failed');

CREATE TABLE media_index_segments (
    id uuid PRIMARY KEY,
    job_id bigint NOT NULL REFERENCES media_index_jobs(id) ON DELETE CASCADE,
    project_id integer NOT NULL,
    asset_id integer NOT NULL,
    start_ms bigint NOT NULL CHECK (start_ms >= 0),
    end_ms bigint NOT NULL CHECK (end_ms > start_ms),
    description text NOT NULL,
    time_quality text NOT NULL,
    captured_start timestamptz,
    captured_end timestamptz,
    active boolean NOT NULL DEFAULT true,
    index_deleted boolean NOT NULL DEFAULT false,
    FOREIGN KEY (asset_id, project_id) REFERENCES assets(id, project_id) ON DELETE CASCADE,
    UNIQUE(job_id,start_ms)
);
CREATE INDEX media_index_segments_project ON media_index_segments(project_id,asset_id) WHERE active;

CREATE TABLE media_index_retired_points (id uuid PRIMARY KEY, space text NOT NULL);
CREATE FUNCTION retire_media_index_points() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO media_index_retired_points(id,space)
  SELECT s.id,j.space FROM media_index_segments s JOIN media_index_jobs j ON s.job_id=j.id
  WHERE j.asset_id=OLD.id ON CONFLICT(id) DO NOTHING;
  RETURN OLD;
END;
$$;
CREATE TRIGGER assets_retire_media_index BEFORE DELETE ON assets
FOR EACH ROW EXECUTE FUNCTION retire_media_index_points();
