CREATE TABLE job_metric_descriptors (
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    unit TEXT CHECK (unit IS NULL OR length(unit) BETWEEN 1 AND 64),
    metadata_json TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (attempt_id, name)
);

CREATE INDEX job_metric_descriptors_job_attempt_idx
    ON job_metric_descriptors(job_id, attempt_id, name);

INSERT INTO job_metric_descriptors(job_id, attempt_id, name, created_at, updated_at)
SELECT job_id, attempt_id, name, MIN(captured_at), MAX(captured_at)
FROM job_metric_samples
GROUP BY job_id, attempt_id, name;
