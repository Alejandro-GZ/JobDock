ALTER TABLE nodes ADD COLUMN gpu_discovery_status TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE nodes ADD COLUMN gpu_error_code TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN gpu_error_message TEXT NOT NULL DEFAULT '';
ALTER TABLE job_events ADD COLUMN status TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS job_events_owner_stream_idx ON job_events(id, job_id);
