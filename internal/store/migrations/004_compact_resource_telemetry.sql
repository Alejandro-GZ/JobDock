CREATE TABLE job_resource_samples (
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    captured_at INTEGER NOT NULL,
    resolution_seconds INTEGER NOT NULL CHECK (resolution_seconds IN (5, 300)),
    sample_count INTEGER NOT NULL CHECK (sample_count > 0),
    cpu_millis INTEGER NOT NULL CHECK (cpu_millis >= 0),
    memory_bytes INTEGER NOT NULL CHECK (memory_bytes >= 0),
    gpu_utilization_basis_points INTEGER CHECK (gpu_utilization_basis_points BETWEEN 0 AND 10000),
    gpu_memory_bytes INTEGER CHECK (gpu_memory_bytes >= 0),
    PRIMARY KEY (job_id, captured_at, resolution_seconds)
);
CREATE INDEX job_resource_samples_retention_idx ON job_resource_samples(resolution_seconds, captured_at);

-- Resource samples produced before this migration contain complete Docker Stats
-- documents. They are intentionally removed instead of being retained as JSON.
DELETE FROM job_events WHERE type = 'resource_sample';
