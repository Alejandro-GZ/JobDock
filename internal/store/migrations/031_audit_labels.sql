ALTER TABLE audit_events ADD COLUMN actor_label TEXT;
ALTER TABLE audit_events ADD COLUMN target_label TEXT;

UPDATE audit_events
SET actor_label = COALESCE(
    (SELECT username FROM users WHERE users.id = audit_events.actor_id),
    CASE WHEN actor_id IS NULL THEN 'System' END
);

UPDATE audit_events
SET target_label = CASE target_type
    WHEN 'user' THEN (SELECT username FROM users WHERE users.id = audit_events.target_id)
    WHEN 'node' THEN (SELECT COALESCE(name_override, name) FROM nodes WHERE nodes.id = audit_events.target_id)
    WHEN 'job' THEN (SELECT json_extract(spec_json, '$.name') FROM jobs WHERE jobs.id = audit_events.target_id)
    WHEN 'secret' THEN (SELECT name FROM secrets WHERE secrets.id = audit_events.target_id)
    WHEN 'build' THEN (SELECT name FROM builds WHERE builds.id = audit_events.target_id)
    WHEN 'dashboard' THEN (SELECT name FROM job_dashboards WHERE job_dashboards.id = audit_events.target_id)
    WHEN 'personal_access_token' THEN (SELECT name FROM personal_access_tokens WHERE personal_access_tokens.id = audit_events.target_id)
    ELSE NULL
END;

UPDATE audit_events
SET target_label = COALESCE(
    target_label,
    json_extract(metadata_json, '$.name'),
    json_extract(metadata_json, '$.username')
);

CREATE INDEX audit_events_actor_id_idx ON audit_events(actor_id, id DESC);
CREATE INDEX audit_events_target_type_idx ON audit_events(target_type, id DESC);
CREATE INDEX audit_events_created_at_idx ON audit_events(created_at, id DESC);
