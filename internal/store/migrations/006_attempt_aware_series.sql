ALTER TABLE job_resource_samples RENAME TO job_resource_samples_legacy;

CREATE TABLE job_resource_samples (
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    captured_at INTEGER NOT NULL,
    resolution_seconds INTEGER NOT NULL CHECK (resolution_seconds IN (5, 300)),
    sample_count INTEGER NOT NULL CHECK (sample_count > 0),
    cpu_millis INTEGER NOT NULL CHECK (cpu_millis >= 0),
    memory_bytes INTEGER NOT NULL CHECK (memory_bytes >= 0),
    gpu_utilization_basis_points INTEGER CHECK (gpu_utilization_basis_points BETWEEN 0 AND 10000),
    gpu_memory_bytes INTEGER CHECK (gpu_memory_bytes >= 0),
    PRIMARY KEY (attempt_id, captured_at, resolution_seconds)
);

INSERT INTO job_resource_samples (
    job_id, attempt_id, captured_at, resolution_seconds, sample_count,
    cpu_millis, memory_bytes, gpu_utilization_basis_points, gpu_memory_bytes
)
SELECT legacy.job_id, jobs.attempt_id, legacy.captured_at, legacy.resolution_seconds,
       legacy.sample_count, legacy.cpu_millis, legacy.memory_bytes,
       legacy.gpu_utilization_basis_points, legacy.gpu_memory_bytes
FROM job_resource_samples_legacy AS legacy
JOIN jobs ON jobs.id = legacy.job_id
WHERE jobs.attempt_id IS NOT NULL AND jobs.attempt_id <> '';

DROP TABLE job_resource_samples_legacy;
CREATE INDEX job_resource_samples_job_attempt_idx ON job_resource_samples(job_id, attempt_id, captured_at);
CREATE INDEX job_resource_samples_retention_idx ON job_resource_samples(resolution_seconds, captured_at);

CREATE TABLE job_metric_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    step INTEGER,
    value REAL NOT NULL,
    captured_at INTEGER NOT NULL
);

CREATE INDEX job_metric_samples_query_idx
    ON job_metric_samples(job_id, attempt_id, name, captured_at, id);
CREATE INDEX job_metric_samples_retention_idx
    ON job_metric_samples(job_id, attempt_id, captured_at);

INSERT INTO job_metric_samples(job_id, attempt_id, name, step, value, captured_at)
SELECT events.job_id,
       jobs.attempt_id,
       json_extract(item.value, '$.name'),
       json_extract(item.value, '$.step'),
       json_extract(item.value, '$.value'),
       CAST((julianday(events.created_at) - 2440587.5) * 86400000 AS INTEGER)
FROM job_events AS events
JOIN jobs ON jobs.id = events.job_id
JOIN json_each(events.payload_json, '$.items') AS item
WHERE events.type = 'metrics'
  AND jobs.attempt_id IS NOT NULL
  AND jobs.attempt_id <> ''
  AND json_type(item.value, '$.name') = 'text'
  AND json_type(item.value, '$.value') IN ('integer', 'real');

DELETE FROM job_events WHERE type = 'metrics';
