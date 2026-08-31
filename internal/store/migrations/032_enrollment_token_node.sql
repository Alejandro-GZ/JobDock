-- The enrollment token is consumed immediately before the node row is inserted,
-- so this intentionally remains a logical reference rather than an immediate FK.
ALTER TABLE enrollment_tokens ADD COLUMN node_id TEXT;
