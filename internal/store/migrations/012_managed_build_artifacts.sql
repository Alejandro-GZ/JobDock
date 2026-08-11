CREATE TABLE managed_build_artifacts (
    build_id TEXT PRIMARY KEY REFERENCES builds(id) ON DELETE CASCADE,
    owner_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    digest TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    size_bytes INTEGER NOT NULL CHECK(size_bytes > 0),
    media_type TEXT NOT NULL,
    runtime_image TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_referenced_at TEXT NOT NULL
);

CREATE INDEX managed_build_artifacts_gc_idx
ON managed_build_artifacts(last_referenced_at);
