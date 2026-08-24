ALTER TABLE job_matrix_observations ADD COLUMN matrix_type TEXT NOT NULL DEFAULT 'confusion_matrix'
    CHECK (matrix_type IN ('confusion_matrix', 'heatmap', 'correlation'));
ALTER TABLE job_matrix_observations ADD COLUMN row_labels_json TEXT;
ALTER TABLE job_matrix_observations ADD COLUMN column_labels_json TEXT;
ALTER TABLE job_matrix_observations ADD COLUMN unit TEXT;
ALTER TABLE job_matrix_observations ADD COLUMN tags_json TEXT;

UPDATE job_matrix_observations
SET row_labels_json = labels_json,
    column_labels_json = labels_json
WHERE row_labels_json IS NULL OR column_labels_json IS NULL;

ALTER TABLE job_rich_observable_descriptors ADD COLUMN subtype TEXT;
ALTER TABLE job_rich_observable_descriptors ADD COLUMN unit TEXT;
ALTER TABLE job_rich_observable_descriptors ADD COLUMN tags_json TEXT;
ALTER TABLE job_rich_observable_descriptors ADD COLUMN metadata_json TEXT;

UPDATE job_rich_observable_descriptors
SET subtype = 'confusion_matrix'
WHERE kind = 'matrix' AND subtype IS NULL;
