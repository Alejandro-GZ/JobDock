PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'member')),
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions(user_id);

CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    agent_version TEXT NOT NULL,
    protocol_version INTEGER NOT NULL,
    architecture TEXT NOT NULL,
    docker_version TEXT NOT NULL,
    cpu_total_millis INTEGER NOT NULL,
    memory_total_bytes INTEGER NOT NULL,
    workspace_free_bytes INTEGER NOT NULL,
    labels_json TEXT NOT NULL,
    credential_hash TEXT NOT NULL UNIQUE,
    credential_created_at TEXT NOT NULL,
    previous_credential_hash TEXT,
    previous_credential_expires_at TEXT,
    last_heartbeat TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS gpus (
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    uuid TEXT NOT NULL,
    model TEXT NOT NULL,
    vram_bytes INTEGER NOT NULL,
    PRIMARY KEY (node_id, uuid)
);

CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    owner_id TEXT NOT NULL REFERENCES users(id),
    spec_json TEXT NOT NULL,
    status TEXT NOT NULL,
    desired_status TEXT NOT NULL,
    observed_status TEXT NOT NULL,
    assigned_node_id TEXT REFERENCES nodes(id),
    attempt_id TEXT,
    image_digest TEXT NOT NULL DEFAULT '',
    exit_code INTEGER,
    queue_reason_code TEXT NOT NULL DEFAULT '',
    queue_reason TEXT NOT NULL DEFAULT '',
    failure_reason TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    deleted_at TEXT,
    version INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS jobs_status_created_idx ON jobs(status, created_at);
CREATE INDEX IF NOT EXISTS jobs_owner_idx ON jobs(owner_id, created_at DESC);

CREATE TABLE IF NOT EXISTS job_attempts (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jobs(id),
    attempt_number INTEGER NOT NULL,
    node_id TEXT NOT NULL REFERENCES nodes(id),
    assignment_id TEXT NOT NULL UNIQUE,
    container_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    job_token_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    UNIQUE(job_id, attempt_number)
);

CREATE TABLE IF NOT EXISTS assignments (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jobs(id),
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id),
    node_id TEXT NOT NULL REFERENCES nodes(id),
    gpu_uuids_json TEXT NOT NULL,
    job_token_ciphertext BLOB NOT NULL,
    delivered_at TEXT,
    accepted_at TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS job_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL REFERENCES jobs(id),
    sequence INTEGER NOT NULL,
    type TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(job_id, sequence)
);

CREATE TABLE IF NOT EXISTS enrollment_tokens (
    token_hash TEXT PRIMARY KEY,
    created_by TEXT NOT NULL REFERENCES users(id),
    expires_at TEXT NOT NULL,
    used_at TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS secrets (
    id TEXT PRIMARY KEY,
    owner_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    ciphertext BLOB NOT NULL,
    kind TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(owner_id, name)
);

CREATE TABLE IF NOT EXISTS audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id TEXT,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    metadata_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    response_status INTEGER,
    response_json BLOB,
    created_at TEXT NOT NULL,
    PRIMARY KEY (user_id, key)
);

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
