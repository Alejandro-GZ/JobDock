CREATE TABLE job_distribution_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    group_name TEXT NOT NULL CHECK (length(group_name) BETWEEN 1 AND 128),
    unit TEXT,
    step INTEGER,
    captured_at INTEGER NOT NULL,
    values_json TEXT NOT NULL,
    scores_json TEXT,
    tags_json TEXT,
    metadata_json TEXT
);
CREATE INDEX job_distribution_attempt_idx ON job_distribution_observations(job_id, attempt_id, name, group_name, captured_at DESC, id DESC);

ALTER TABLE job_rich_observable_descriptors RENAME TO job_rich_observable_descriptors_v24;
CREATE TABLE job_rich_observable_descriptors (
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('matrix', 'progress', 'distribution')),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (job_id, attempt_id, kind, name)
);
INSERT INTO job_rich_observable_descriptors SELECT * FROM job_rich_observable_descriptors_v24;
DROP TABLE job_rich_observable_descriptors_v24;
CREATE INDEX job_rich_observable_descriptors_attempt_idx ON job_rich_observable_descriptors(job_id, attempt_id, kind, name);

ALTER TABLE job_observation_updates RENAME TO job_observation_updates_v24;
CREATE TABLE job_observation_updates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('checkpoint', 'progress', 'matrix', 'distribution')),
    observed_at INTEGER NOT NULL
);
INSERT INTO job_observation_updates SELECT * FROM job_observation_updates_v24;
DROP TABLE job_observation_updates_v24;
CREATE INDEX job_observation_updates_tail_idx ON job_observation_updates(job_id, attempt_id, id);
