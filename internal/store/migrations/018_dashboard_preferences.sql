CREATE TABLE dashboard_preferences (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    schema_version INTEGER NOT NULL CHECK (schema_version > 0),
    config_json TEXT NOT NULL CHECK (length(config_json) <= 65536),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, job_id)
);

CREATE INDEX dashboard_preferences_job_idx ON dashboard_preferences(job_id);
