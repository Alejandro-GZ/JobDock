CREATE TABLE checkpoint_syncs (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    requested_at TEXT NOT NULL,
    confirmed_at TEXT,
    file_count INTEGER NOT NULL DEFAULT 0,
    byte_count INTEGER NOT NULL DEFAULT 0,
    manifest_json TEXT NOT NULL DEFAULT '[]'
);

CREATE INDEX checkpoint_syncs_pending_idx ON checkpoint_syncs(job_id, confirmed_at, requested_at);
CREATE INDEX checkpoint_syncs_latest_idx ON checkpoint_syncs(job_id, confirmed_at DESC);
