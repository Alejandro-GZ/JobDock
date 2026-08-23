CREATE TABLE job_observability_phases (
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    phase_id TEXT NOT NULL CHECK (length(phase_id) BETWEEN 1 AND 128),
    name TEXT,
    position INTEGER,
    metadata_json TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (attempt_id, phase_id)
);

CREATE INDEX job_observability_phases_catalog_idx
    ON job_observability_phases(job_id, attempt_id, position, phase_id);
