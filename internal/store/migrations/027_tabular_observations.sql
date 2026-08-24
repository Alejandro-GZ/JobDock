ALTER TABLE job_rich_observable_descriptors RENAME TO job_rich_observable_descriptors_v26;
CREATE TABLE job_rich_observable_descriptors (
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('matrix', 'progress', 'distribution', 'table')),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    subtype TEXT,
    unit TEXT,
    tags_json TEXT,
    metadata_json TEXT,
    PRIMARY KEY (job_id, attempt_id, kind, name)
);
INSERT INTO job_rich_observable_descriptors(job_id, attempt_id, kind, name, created_at, updated_at, subtype, unit, tags_json, metadata_json)
SELECT job_id, attempt_id, kind, name, created_at, updated_at, subtype, unit, tags_json, metadata_json
FROM job_rich_observable_descriptors_v26;
DROP TABLE job_rich_observable_descriptors_v26;
CREATE INDEX job_rich_observable_descriptors_attempt_idx ON job_rich_observable_descriptors(job_id, attempt_id, kind, name);

ALTER TABLE job_observation_updates RENAME TO job_observation_updates_v26;
CREATE TABLE job_observation_updates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('checkpoint', 'progress', 'matrix', 'distribution', 'table')),
    observed_at INTEGER NOT NULL
);
INSERT INTO job_observation_updates(id, job_id, attempt_id, kind, observed_at)
SELECT id, job_id, attempt_id, kind, observed_at FROM job_observation_updates_v26;
DROP TABLE job_observation_updates_v26;
CREATE INDEX job_observation_updates_tail_idx ON job_observation_updates(job_id, attempt_id, id);

CREATE TABLE job_table_descriptors (
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    subtype TEXT NOT NULL DEFAULT 'table',
    columns_json TEXT NOT NULL,
    tags_json TEXT,
    metadata_json TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (attempt_id, name)
);

CREATE TABLE job_table_rows (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    step INTEGER,
    captured_at INTEGER NOT NULL,
    record_json TEXT NOT NULL
);

CREATE INDEX idx_job_table_rows_query
    ON job_table_rows(job_id, attempt_id, name, id);
CREATE INDEX idx_job_table_rows_captured
    ON job_table_rows(job_id, attempt_id, name, captured_at, id);
