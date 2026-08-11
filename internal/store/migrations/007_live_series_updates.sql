CREATE TABLE job_series_updates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('metrics', 'resources')),
    captured_at INTEGER NOT NULL
);

CREATE INDEX job_series_updates_tail_idx
    ON job_series_updates(job_id, attempt_id, id);

ALTER TABLE job_metric_samples
    ADD COLUMN series_cursor INTEGER REFERENCES job_series_updates(id) ON DELETE CASCADE;

CREATE INDEX job_metric_samples_cursor_idx
    ON job_metric_samples(job_id, attempt_id, series_cursor);

ALTER TABLE job_resource_samples
    ADD COLUMN series_cursor INTEGER REFERENCES job_series_updates(id) ON DELETE CASCADE;

CREATE INDEX job_resource_samples_cursor_idx
    ON job_resource_samples(job_id, attempt_id, series_cursor);
