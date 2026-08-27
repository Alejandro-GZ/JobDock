ALTER TABLE nodes ADD COLUMN capabilities_json TEXT NOT NULL DEFAULT '[]';

CREATE TABLE cpu_packages (
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    package_id TEXT NOT NULL,
    model TEXT NOT NULL,
    physical_cores INTEGER NOT NULL,
    logical_cpus_json TEXT NOT NULL,
    total_millis INTEGER NOT NULL,
    PRIMARY KEY (node_id, package_id)
);

ALTER TABLE assignments ADD COLUMN cpu_package_id TEXT NOT NULL DEFAULT '';
ALTER TABLE assignments ADD COLUMN cpu_set TEXT NOT NULL DEFAULT '';
ALTER TABLE job_attempts ADD COLUMN cpu_package_id TEXT NOT NULL DEFAULT '';
ALTER TABLE job_attempts ADD COLUMN gpu_uuids_json TEXT NOT NULL DEFAULT '[]';
