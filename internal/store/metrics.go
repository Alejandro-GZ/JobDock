package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

type MetricDescriptor struct {
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Unit      string         `json:"unit,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
	Phase     string         `json:"phase,omitempty"`
	Milestone string         `json:"milestone,omitempty"`
	Declared  bool           `json:"declared"`
	Observed  bool           `json:"observed"`
}

func (s *Store) MetricDescriptors(ctx context.Context, jobID, attemptID string, requiredTags []string) ([]MetricDescriptor, error) {
	rows, err := s.db.QueryContext(ctx, metricDescriptorQuery(0), jobID, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]MetricDescriptor, 0)
	for rows.Next() {
		item := MetricDescriptor{Type: "metric", Observed: true}
		var metadata string
		var tags string
		if err = rows.Scan(&item.Name, &item.Unit, &metadata, &tags); err != nil {
			return nil, err
		}
		if metadata != "" {
			_ = json.Unmarshal([]byte(metadata), &item.Metadata)
		}
		if tags != "" {
			_ = json.Unmarshal([]byte(tags), &item.Tags)
		}
		result = append(result, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	declared, err := s.DeclaredObservableSources(ctx, jobID, attemptID)
	if err != nil {
		return nil, err
	}
	metrics := make([]MetricDescriptor, 0, len(declared))
	for _, item := range declared {
		if item.Type == "metric" {
			metrics = append(metrics, item)
		}
	}
	merged := mergeObservableDescriptors(result, metrics)
	filtered := merged[:0]
	for _, item := range merged {
		if descriptorHasTags(item, requiredTags) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func metricDescriptorQuery(tagCount int) string {
	query := `SELECT d.name,COALESCE(d.unit,''),COALESCE(d.metadata_json,''),COALESCE(d.tags_json,'') FROM job_metric_descriptors d WHERE d.job_id=? AND d.attempt_id=?`
	for range tagCount {
		query += ` AND EXISTS (SELECT 1 FROM json_each(COALESCE(d.tags_json,'[]')) WHERE json_each.value=?)`
	}
	return query + ` ORDER BY d.name LIMIT 256`
}

func (s *Store) AppendMetricSamples(ctx context.Context, samples []domain.MetricSample) error {
	if len(samples) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cursor, err := insertSeriesUpdate(ctx, tx, samples[0].JobID, samples[0].AttemptID, "metrics", time.Now().UTC())
	if err != nil {
		return err
	}
	statement, err := tx.PrepareContext(ctx, `INSERT INTO job_metric_samples(job_id,attempt_id,name,step,value,captured_at,series_cursor) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, sample := range samples {
		if sample.JobID != samples[0].JobID || sample.AttemptID != samples[0].AttemptID {
			return fmt.Errorf("metric batch spans multiple job attempts")
		}
		if err = ensureMetricDescriptor(ctx, tx, sample); err != nil {
			return err
		}
		if _, err = statement.ExecContext(ctx, sample.JobID, sample.AttemptID, sample.Name, sample.Step, sample.Value, sample.CapturedAt.UTC().UnixMilli(), cursor); err != nil {
			return mapConstraint(err)
		}
	}
	return tx.Commit()
}

func ensureMetricDescriptor(ctx context.Context, tx *sql.Tx, sample domain.MetricSample) error {
	var currentUnit, currentMetadata, currentTags sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT unit,metadata_json,tags_json FROM job_metric_descriptors WHERE attempt_id=? AND name=?`, sample.AttemptID, sample.Name).Scan(&currentUnit, &currentMetadata, &currentTags)
	metadata := ""
	if len(sample.Metadata) > 0 {
		encoded, marshalErr := json.Marshal(sample.Metadata)
		if marshalErr != nil {
			return marshalErr
		}
		metadata = string(encoded)
	}
	tags, encodeErr := encodeMetricTags(sample.Tags)
	if encodeErr != nil {
		return encodeErr
	}
	var declaredUnit, declaredMetadata, declaredTags sql.NullString
	declarationErr := tx.QueryRowContext(ctx, `SELECT unit,metadata_json,tags_json FROM job_observability_manifest_sources WHERE attempt_id=? AND source_type='metric' AND name=?`, sample.AttemptID, sample.Name).Scan(&declaredUnit, &declaredMetadata, &declaredTags)
	if declarationErr != nil && !errors.Is(declarationErr, sql.ErrNoRows) {
		return declarationErr
	}
	if declarationErr == nil {
		if sample.Unit != "" && declaredUnit.Valid && sample.Unit != declaredUnit.String || metadata != "" && declaredMetadata.Valid && metadata != declaredMetadata.String || tags != "" && declaredTags.Valid && tags != declaredTags.String {
			return ErrMetricDescriptorConflict
		}
		if sample.Unit == "" && declaredUnit.Valid {
			sample.Unit = declaredUnit.String
		}
		if metadata == "" && declaredMetadata.Valid {
			metadata = declaredMetadata.String
		}
		if tags == "" && declaredTags.Valid {
			tags = declaredTags.String
		}
	}
	now := time.Now().UTC().UnixMilli()
	if errors.Is(err, sql.ErrNoRows) {
		var unitValue, metadataValue any
		if sample.Unit != "" {
			unitValue = sample.Unit
		}
		if metadata != "" {
			metadataValue = metadata
		}
		var tagsValue any
		if tags != "" {
			tagsValue = tags
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO job_metric_descriptors(job_id,attempt_id,name,unit,metadata_json,tags_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, sample.JobID, sample.AttemptID, sample.Name, unitValue, metadataValue, tagsValue, now, now)
		return mapConstraint(err)
	}
	if err != nil {
		return err
	}
	if sample.Unit != "" && currentUnit.Valid && currentUnit.String != sample.Unit {
		return ErrMetricDescriptorConflict
	}
	if metadata != "" && currentMetadata.Valid && currentMetadata.String != metadata {
		return ErrMetricDescriptorConflict
	}
	if tags != "" && currentTags.Valid && currentTags.String != tags {
		return ErrMetricDescriptorConflict
	}
	if (sample.Unit != "" && !currentUnit.Valid) || (metadata != "" && !currentMetadata.Valid) || (tags != "" && !currentTags.Valid) {
		_, err = tx.ExecContext(ctx, `UPDATE job_metric_descriptors SET unit=COALESCE(unit,?),metadata_json=COALESCE(metadata_json,?),tags_json=COALESCE(tags_json,?),updated_at=? WHERE attempt_id=? AND name=?`, nullableString(sample.Unit), nullableString(metadata), nullableString(tags), now, sample.AttemptID, sample.Name)
	}
	return err
}

func encodeMetricTags(tags []string) (string, error) {
	if tags == nil {
		return "", nil
	}
	unique := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		unique[tag] = struct{}{}
	}
	normalized := make([]string, 0, len(unique))
	for tag := range unique {
		normalized = append(normalized, tag)
	}
	sort.Strings(normalized)
	encoded, err := json.Marshal(normalized)
	return string(encoded), err
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) MetricSeries(ctx context.Context, jobID, attemptID string, names []string, from, to time.Time, resolutionSeconds, limit int) ([]domain.MetricSeries, bool, error) {
	return s.metricSeries(ctx, jobID, attemptID, names, from, to, resolutionSeconds, limit, nil)
}

func (s *Store) MetricSeriesAt(ctx context.Context, jobID, attemptID string, names []string, from, to time.Time, resolutionSeconds, limit int, maxCursor int64) ([]domain.MetricSeries, bool, error) {
	return s.metricSeries(ctx, jobID, attemptID, names, from, to, resolutionSeconds, limit, &maxCursor)
}

func (s *Store) metricSeries(ctx context.Context, jobID, attemptID string, names []string, from, to time.Time, resolutionSeconds, limit int, maxCursor *int64) ([]domain.MetricSeries, bool, error) {
	arguments := []any{jobID, attemptID, from.UTC().UnixMilli(), to.UTC().UnixMilli()}
	nameClause := ""
	if len(names) > 0 {
		placeholders := make([]string, len(names))
		for index, name := range names {
			placeholders[index] = "?"
			arguments = append(arguments, name)
		}
		nameClause = " AND name IN (" + strings.Join(placeholders, ",") + ")"
	}
	cursorClause := ""
	if maxCursor != nil {
		cursorClause = " AND (series_cursor IS NULL OR series_cursor<=?)"
		arguments = append(arguments, *maxCursor)
	}
	var query string
	if resolutionSeconds <= 0 {
		query = `SELECT s.name,s.step,s.value,s.captured_at,1,COALESCE(s.series_cursor,0),COALESCE(d.unit,''),COALESCE(d.metadata_json,''),COALESCE(d.tags_json,'') FROM job_metric_samples s LEFT JOIN job_metric_descriptors d ON d.attempt_id=s.attempt_id AND d.name=s.name WHERE s.job_id=? AND s.attempt_id=? AND s.captured_at>=? AND s.captured_at<=?` + strings.ReplaceAll(nameClause, " name ", " s.name ") + strings.ReplaceAll(cursorClause, "series_cursor", "s.series_cursor") + ` ORDER BY s.name,s.captured_at,s.id LIMIT ?`
	} else {
		bucketMilliseconds := int64(resolutionSeconds) * 1000
		query = `SELECT s.name,MAX(s.step),AVG(s.value),(s.captured_at/?)*?,COUNT(*),MAX(COALESCE(s.series_cursor,0)),COALESCE(d.unit,''),COALESCE(d.metadata_json,''),COALESCE(d.tags_json,'') FROM job_metric_samples s LEFT JOIN job_metric_descriptors d ON d.attempt_id=s.attempt_id AND d.name=s.name WHERE s.job_id=? AND s.attempt_id=? AND s.captured_at>=? AND s.captured_at<=?` + strings.ReplaceAll(nameClause, " name ", " s.name ") + strings.ReplaceAll(cursorClause, "series_cursor", "s.series_cursor") + ` GROUP BY s.name,(s.captured_at/?) ORDER BY s.name,(s.captured_at/?) LIMIT ?`
		arguments = append([]any{bucketMilliseconds, bucketMilliseconds}, arguments...)
		arguments = append(arguments, bucketMilliseconds, bucketMilliseconds)
	}
	arguments = append(arguments, limit+1)
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	seriesByName := map[string]*domain.MetricSeries{}
	order := make([]string, 0)
	pointCount := 0
	for rows.Next() {
		var name string
		var step sql.NullInt64
		var value float64
		var captured, sampleCount, cursor int64
		var unit, metadata, tags string
		if err = rows.Scan(&name, &step, &value, &captured, &sampleCount, &cursor, &unit, &metadata, &tags); err != nil {
			return nil, false, err
		}
		item := seriesByName[name]
		if item == nil {
			item = &domain.MetricSeries{Name: name, Unit: unit, Points: []domain.MetricPoint{}}
			if metadata != "" {
				_ = json.Unmarshal([]byte(metadata), &item.Metadata)
			}
			if tags != "" {
				_ = json.Unmarshal([]byte(tags), &item.Tags)
			}
			seriesByName[name] = item
			order = append(order, name)
		}
		var stepValue *int64
		if step.Valid {
			value := step.Int64
			stepValue = &value
		}
		item.Points = append(item.Points, domain.MetricPoint{Cursor: cursor, CapturedAt: time.UnixMilli(captured).UTC(), Step: stepValue, Value: value, SampleCount: sampleCount})
		pointCount++
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := pointCount > limit
	if truncated {
		for index := len(order) - 1; index >= 0 && pointCount > limit; index-- {
			item := seriesByName[order[index]]
			for len(item.Points) > 0 && pointCount > limit {
				item.Points = item.Points[:len(item.Points)-1]
				pointCount--
			}
		}
	}
	result := make([]domain.MetricSeries, 0, len(order))
	for _, name := range order {
		item := seriesByName[name]
		statsCursorClause := ""
		statsArguments := []any{jobID, attemptID, name, from.UTC().UnixMilli(), to.UTC().UnixMilli()}
		if maxCursor != nil {
			statsCursorClause = " AND (series_cursor IS NULL OR series_cursor<=?)"
			statsArguments = append(statsArguments, *maxCursor)
		}
		statsArguments = append(statsArguments, jobID, attemptID, name, from.UTC().UnixMilli(), to.UTC().UnixMilli())
		if maxCursor != nil {
			statsArguments = append(statsArguments, *maxCursor)
		}
		statsQuery := `SELECT MIN(value),MAX(value),COUNT(*),(SELECT value FROM job_metric_samples latest WHERE latest.job_id=? AND latest.attempt_id=? AND latest.name=? AND latest.captured_at>=? AND latest.captured_at<=?` + statsCursorClause + ` ORDER BY latest.captured_at DESC,latest.id DESC LIMIT 1) FROM job_metric_samples WHERE job_id=? AND attempt_id=? AND name=? AND captured_at>=? AND captured_at<=?` + statsCursorClause
		if err = s.db.QueryRowContext(ctx, statsQuery, statsArguments...).Scan(&item.Min, &item.Max, &item.SampleCount, &item.Last); err != nil {
			return nil, false, fmt.Errorf("metric statistics: %w", err)
		}
		result = append(result, *item)
	}
	return result, truncated, nil
}

func (s *Store) AttemptBelongsToJob(ctx context.Context, jobID, attemptID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_attempts WHERE id=? AND job_id=?`, attemptID, jobID).Scan(&count)
	return count == 1, err
}
