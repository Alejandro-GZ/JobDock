CREATE TABLE job_observability_manifest_sources (
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL CHECK (length(source_type) BETWEEN 1 AND 64),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    unit TEXT,
    tags_json TEXT,
    metadata_json TEXT,
    phase TEXT,
    milestone TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (attempt_id, source_type, name),
    CHECK (phase IS NULL OR milestone IS NULL)
);

CREATE INDEX job_observability_manifest_sources_catalog_idx
    ON job_observability_manifest_sources(job_id, attempt_id, source_type, name);
