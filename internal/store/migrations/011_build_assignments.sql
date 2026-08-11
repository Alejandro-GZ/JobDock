CREATE TABLE build_assignments (
    id TEXT PRIMARY KEY,
    build_id TEXT NOT NULL UNIQUE REFERENCES builds(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK(status IN ('PENDING','RUNNING','SUCCEEDED','FAILED','CANCELLED')),
    cancel_requested INTEGER NOT NULL DEFAULT 0 CHECK(cancel_requested IN (0,1)),
    builder_id TEXT,
    lease_expires_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX build_assignments_claim_idx ON build_assignments(status, cancel_requested, lease_expires_at, created_at);
