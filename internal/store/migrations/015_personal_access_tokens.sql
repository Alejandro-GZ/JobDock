CREATE TABLE personal_access_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    scopes_json TEXT NOT NULL,
    expires_at TEXT,
    last_used_at TEXT,
    revoked_at TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX personal_access_tokens_user_created_idx
    ON personal_access_tokens(user_id, created_at DESC);
