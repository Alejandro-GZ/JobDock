package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

type SeriesUpdate struct {
	Cursor     int64                  `json:"cursor"`
	AttemptID  string                 `json:"attempt_id"`
	Kind       string                 `json:"kind"`
	CapturedAt time.Time              `json:"captured_at"`
	Metrics    []domain.MetricSample  `json:"metrics,omitempty"`
	Resource   *domain.ResourceSample `json:"resource,omitempty"`
}

func insertSeriesUpdate(ctx context.Context, tx *sql.Tx, jobID, attemptID, kind string, capturedAt time.Time) (int64, error) {
	result, err := tx.ExecContext(ctx, `INSERT INTO job_series_updates(job_id,attempt_id,kind,captured_at) VALUES(?,?,?,?)`, jobID, attemptID, kind, capturedAt.UTC().UnixMilli())
	if err != nil {
		return 0, mapConstraint(err)
	}
	return result.LastInsertId()
}

func (s *Store) LatestSeriesCursor(ctx context.Context, jobID, attemptID string) (int64, error) {
	var cursor int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM job_series_updates WHERE job_id=? AND attempt_id=?`, jobID, attemptID).Scan(&cursor)
	return cursor, err
}

func (s *Store) SeriesUpdates(ctx context.Context, jobID, attemptID string, after int64, limit int) ([]SeriesUpdate, bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,kind,captured_at FROM job_series_updates WHERE job_id=? AND attempt_id=? AND id>? ORDER BY id LIMIT ?`, jobID, attemptID, after, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	updates := make([]SeriesUpdate, 0)
	for rows.Next() {
		var update SeriesUpdate
		var captured int64
		if err = rows.Scan(&update.Cursor, &update.Kind, &captured); err != nil {
			return nil, false, err
		}
		update.AttemptID = attemptID
		update.CapturedAt = time.UnixMilli(captured).UTC()
		updates = append(updates, update)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(updates) > limit
	if hasMore {
		updates = updates[:limit]
	}
	if len(updates) == 0 {
		return updates, hasMore, nil
	}
	byCursor := make(map[int64]*SeriesUpdate, len(updates))
	placeholders := make([]string, len(updates))
	arguments := make([]any, len(updates))
	for index := range updates {
		byCursor[updates[index].Cursor] = &updates[index]
		placeholders[index], arguments[index] = "?", updates[index].Cursor
	}
	inClause := strings.Join(placeholders, ",")
	metricRows, err := s.db.QueryContext(ctx, `SELECT s.series_cursor,s.name,s.step,s.value,s.captured_at,COALESCE(d.unit,''),COALESCE(d.metadata_json,'') FROM job_metric_samples s LEFT JOIN job_metric_descriptors d ON d.attempt_id=s.attempt_id AND d.name=s.name WHERE s.series_cursor IN (`+inClause+`) ORDER BY s.series_cursor,s.id`, arguments...)
	if err != nil {
		return nil, false, err
	}
	for metricRows.Next() {
		var sample domain.MetricSample
		var step sql.NullInt64
		var captured int64
		var metadata string
		if err = metricRows.Scan(&sample.Cursor, &sample.Name, &step, &sample.Value, &captured, &sample.Unit, &metadata); err != nil {
			metricRows.Close()
			return nil, false, err
		}
		sample.JobID, sample.AttemptID, sample.CapturedAt = jobID, attemptID, time.UnixMilli(captured).UTC()
		if step.Valid {
			value := step.Int64
			sample.Step = &value
		}
		if metadata != "" {
			_ = json.Unmarshal([]byte(metadata), &sample.Metadata)
		}
		if update := byCursor[sample.Cursor]; update != nil {
			update.Metrics = append(update.Metrics, sample)
		}
	}
	if err = metricRows.Close(); err != nil {
		return nil, false, err
	}
	resourceRows, err := s.db.QueryContext(ctx, `SELECT series_cursor,job_id,attempt_id,captured_at,resolution_seconds,sample_count,cpu_millis,memory_bytes,gpu_utilization_basis_points,gpu_memory_bytes FROM job_resource_samples WHERE series_cursor IN (`+inClause+`) ORDER BY series_cursor`, arguments...)
	if err != nil {
		return nil, false, err
	}
	defer resourceRows.Close()
	for resourceRows.Next() {
		var sample domain.ResourceSample
		var captured int64
		var gpuUtilization, gpuMemory sql.NullInt64
		if err = resourceRows.Scan(&sample.Cursor, &sample.JobID, &sample.AttemptID, &captured, &sample.ResolutionSeconds, &sample.SampleCount, &sample.CPUMillis, &sample.MemoryBytes, &gpuUtilization, &gpuMemory); err != nil {
			return nil, false, err
		}
		sample.CapturedAt = time.Unix(captured, 0).UTC()
		if gpuUtilization.Valid {
			value := gpuUtilization.Int64
			sample.GPUUtilizationBasisPoints = &value
		}
		if gpuMemory.Valid {
			value := gpuMemory.Int64
			sample.GPUMemoryBytes = &value
		}
		if update := byCursor[sample.Cursor]; update != nil {
			update.Resource = &sample
		}
	}
	return updates, hasMore, resourceRows.Err()
}
