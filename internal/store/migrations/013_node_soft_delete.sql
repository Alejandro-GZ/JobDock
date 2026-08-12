ALTER TABLE nodes ADD COLUMN deleted_at TEXT;

CREATE INDEX nodes_active_status_idx
ON nodes(deleted_at, status, last_heartbeat);
