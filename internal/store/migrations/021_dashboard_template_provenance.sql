ALTER TABLE dashboard_preferences ADD COLUMN template_id TEXT;
ALTER TABLE dashboard_preferences ADD COLUMN template_version INTEGER;
ALTER TABLE dashboard_preferences ADD COLUMN template_schema_version INTEGER;
ALTER TABLE dashboard_preferences ADD COLUMN template_applied_at TEXT;

