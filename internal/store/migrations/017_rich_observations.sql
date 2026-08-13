ALTER TABLE checkpoint_syncs ADD COLUMN label TEXT;
ALTER TABLE checkpoint_syncs ADD COLUMN step INTEGER;
ALTER TABLE checkpoint_syncs ADD COLUMN observed_at INTEGER;
ALTER TABLE checkpoint_syncs ADD COLUMN metadata_json TEXT;

CREATE TABLE job_milestone_definitions (
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    weight REAL CHECK (weight IS NULL OR weight > 0),
    metadata_json TEXT,
    PRIMARY KEY (attempt_id, position),
    UNIQUE (attempt_id, name)
);

CREATE TABLE job_progress_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('simple', 'segment', 'milestone')),
    value REAL CHECK (value IS NULL OR (value >= 0 AND value <= 1)),
    milestone TEXT,
    step INTEGER,
    captured_at INTEGER NOT NULL,
    metadata_json TEXT
);
CREATE INDEX job_progress_attempt_idx ON job_progress_observations(job_id, attempt_id, captured_at, id);

CREATE TABLE job_matrix_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    step INTEGER,
    captured_at INTEGER NOT NULL,
    labels_json TEXT NOT NULL,
    values_json TEXT NOT NULL,
    metadata_json TEXT
);
CREATE INDEX job_matrix_attempt_idx ON job_matrix_observations(job_id, attempt_id, name, captured_at DESC, id DESC);

CREATE TABLE job_observation_updates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('checkpoint', 'progress', 'matrix')),
    observed_at INTEGER NOT NULL
);
CREATE INDEX job_observation_updates_tail_idx ON job_observation_updates(job_id, attempt_id, id);
