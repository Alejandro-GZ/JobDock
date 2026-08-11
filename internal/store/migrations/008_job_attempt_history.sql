ALTER TABLE job_attempts ADD COLUMN image_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE job_attempts ADD COLUMN exit_code INTEGER;
ALTER TABLE job_attempts ADD COLUMN failure_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE job_attempts ADD COLUMN outputs_json TEXT NOT NULL DEFAULT '[]';

ALTER TABLE job_events ADD COLUMN attempt_id TEXT;
UPDATE job_events
SET attempt_id = (SELECT jobs.attempt_id FROM jobs WHERE jobs.id = job_events.job_id)
WHERE attempt_id IS NULL;
CREATE INDEX job_events_job_attempt_idx ON job_events(job_id, attempt_id, id);
