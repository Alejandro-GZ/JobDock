CREATE TABLE job_dashboards (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    schema_version INTEGER NOT NULL CHECK (schema_version > 0),
    config_json TEXT NOT NULL CHECK (length(config_json) <= 65536),
    sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
    is_default INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1)),
    template_id TEXT,
    template_version INTEGER,
    template_schema_version INTEGER,
    template_applied_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (user_id, job_id, name COLLATE NOCASE)
);

CREATE INDEX job_dashboards_owner_job_order_idx
    ON job_dashboards(user_id, job_id, sort_order, id);

CREATE UNIQUE INDEX job_dashboards_default_idx
    ON job_dashboards(user_id, job_id)
    WHERE is_default = 1;

CREATE TABLE active_job_dashboards (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    dashboard_id TEXT NOT NULL REFERENCES job_dashboards(id) ON DELETE CASCADE,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, job_id)
);

INSERT INTO job_dashboards(
    id,user_id,job_id,name,schema_version,config_json,sort_order,is_default,
    template_id,template_version,template_schema_version,template_applied_at,
    created_at,updated_at
)
SELECT
    'legacy-' || user_id || '-' || job_id,user_id,job_id,'Dashboard',schema_version,
    config_json,0,1,template_id,template_version,template_schema_version,
    template_applied_at,updated_at,updated_at
FROM dashboard_preferences;

INSERT INTO active_job_dashboards(user_id,job_id,dashboard_id,updated_at)
SELECT user_id,job_id,id,updated_at FROM job_dashboards;
