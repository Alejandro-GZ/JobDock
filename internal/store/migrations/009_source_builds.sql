CREATE TABLE builds (
    id TEXT PRIMARY KEY,
    owner_id TEXT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    mode TEXT NOT NULL CHECK(mode IN ('RAILPACK','DOCKERFILE')),
    status TEXT NOT NULL CHECK(status IN ('CREATED','ANALYZING','BUILDING','SUCCEEDED','FAILED','CANCELLED')),
    source_filename TEXT NOT NULL,
    source_size_bytes INTEGER NOT NULL CHECK(source_size_bytes >= 0),
    source_sha256 TEXT NOT NULL CHECK(length(source_sha256) = 64),
    oci_digest TEXT NOT NULL DEFAULT '',
    failure_reason TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    version INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX builds_owner_created_idx ON builds(owner_id, created_at DESC);
CREATE INDEX builds_status_created_idx ON builds(status, created_at);

CREATE TABLE build_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    build_id TEXT NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK(status IN ('CREATED','ANALYZING','BUILDING','SUCCEEDED','FAILED','CANCELLED')),
    message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX build_events_build_id_idx ON build_events(build_id, id);
