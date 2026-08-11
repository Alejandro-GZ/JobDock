CREATE TABLE build_plans (
    build_id TEXT PRIMARY KEY REFERENCES builds(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    runtime TEXT NOT NULL DEFAULT '',
    package_manager TEXT NOT NULL DEFAULT '',
    entrypoint TEXT NOT NULL DEFAULT '',
    railpack_version TEXT NOT NULL DEFAULT '',
    plan_json TEXT NOT NULL CHECK(json_valid(plan_json)),
    info_json TEXT NOT NULL CHECK(json_valid(info_json)),
    created_at TEXT NOT NULL,
    confirmed_at TEXT
);
