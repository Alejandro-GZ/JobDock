CREATE TABLE job_rich_observable_descriptors (
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('matrix', 'progress')),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (job_id, attempt_id, kind, name)
);

CREATE INDEX job_rich_observable_descriptors_attempt_idx
    ON job_rich_observable_descriptors(job_id, attempt_id, kind, name);

INSERT INTO job_rich_observable_descriptors(job_id, attempt_id, kind, name, created_at, updated_at)
SELECT job_id, attempt_id, 'matrix', name, MIN(captured_at), MAX(captured_at)
FROM job_matrix_observations
GROUP BY job_id, attempt_id, name;

INSERT INTO job_rich_observable_descriptors(job_id, attempt_id, kind, name, created_at, updated_at)
SELECT job_id, attempt_id, 'progress', 'progress', MIN(captured_at), MAX(captured_at)
FROM job_progress_observations
GROUP BY job_id, attempt_id;
