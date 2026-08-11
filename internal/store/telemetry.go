package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

const (
	TelemetryRawResolution        = 5 * time.Second
	TelemetryDownsampleResolution = 5 * time.Minute
)

func (s *Store) AppendResourceSample(ctx context.Context, sample domain.ResourceSample) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cursor, err := insertSeriesUpdate(ctx, tx, sample.JobID, sample.AttemptID, "resources", sample.CapturedAt)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO job_resource_samples(job_id,attempt_id,captured_at,resolution_seconds,sample_count,cpu_millis,memory_bytes,gpu_utilization_basis_points,gpu_memory_bytes,series_cursor) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(attempt_id,captured_at,resolution_seconds) DO NOTHING`,
		sample.JobID, sample.AttemptID, sample.CapturedAt.UTC().Unix(), int(TelemetryRawResolution/time.Second), 1, sample.CPUMillis, sample.MemoryBytes, sample.GPUUtilizationBasisPoints, sample.GPUMemoryBytes, cursor)
	if err != nil {
		return mapConstraint(err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 0 {
		if _, err = tx.ExecContext(ctx, `DELETE FROM job_series_updates WHERE id=?`, cursor); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ResourceSamples(ctx context.Context, jobID, attemptID string, from, to time.Time, resolution, limit int) ([]domain.ResourceSample, bool, error) {
	return s.resourceSamples(ctx, jobID, attemptID, from, to, resolution, limit, nil)
}

func (s *Store) ResourceSamplesAt(ctx context.Context, jobID, attemptID string, from, to time.Time, resolution, limit int, maxCursor int64) ([]domain.ResourceSample, bool, error) {
	return s.resourceSamples(ctx, jobID, attemptID, from, to, resolution, limit, &maxCursor)
}

func (s *Store) resourceSamples(ctx context.Context, jobID, attemptID string, from, to time.Time, resolution, limit int, maxCursor *int64) ([]domain.ResourceSample, bool, error) {
	query := `SELECT COALESCE(series_cursor,0),job_id,attempt_id,captured_at,resolution_seconds,sample_count,cpu_millis,memory_bytes,gpu_utilization_basis_points,gpu_memory_bytes FROM job_resource_samples WHERE job_id=? AND attempt_id=? AND captured_at>=? AND captured_at<=? AND resolution_seconds=?`
	arguments := []any{jobID, attemptID, from.UTC().Unix(), to.UTC().Unix(), resolution}
	if maxCursor != nil {
		query += ` AND (series_cursor IS NULL OR series_cursor<=?)`
		arguments = append(arguments, *maxCursor)
	}
	query += ` ORDER BY captured_at LIMIT ?`
	arguments = append(arguments, limit+1)
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	result := make([]domain.ResourceSample, 0)
	for rows.Next() {
		var sample domain.ResourceSample
		var captured int64
		var gpuUtilization, gpuMemory sql.NullInt64
		if err = rows.Scan(&sample.Cursor, &sample.JobID, &sample.AttemptID, &captured, &sample.ResolutionSeconds, &sample.SampleCount, &sample.CPUMillis, &sample.MemoryBytes, &gpuUtilization, &gpuMemory); err != nil {
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
		result = append(result, sample)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(result) > limit
	if truncated {
		result = result[:limit]
	}
	return result, truncated, nil
}

// MaintainResourceTelemetry keeps recent five-second samples, compacts older
// samples into five-minute averages, and deletes data past the final retention.
func (s *Store) MaintainResourceTelemetry(ctx context.Context, now time.Time, rawRetention, retention time.Duration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rawCutoff := now.UTC().Add(-rawRetention).Unix()
	retentionCutoff := now.UTC().Add(-retention).Unix()
	_, err = tx.ExecContext(ctx, `INSERT INTO job_resource_samples(job_id,attempt_id,captured_at,resolution_seconds,sample_count,cpu_millis,memory_bytes,gpu_utilization_basis_points,gpu_memory_bytes)
		SELECT job_id,attempt_id,(captured_at/300)*300,300,SUM(sample_count),
		       SUM(cpu_millis*sample_count)/SUM(sample_count),
		       SUM(memory_bytes*sample_count)/SUM(sample_count),
		       CASE WHEN COUNT(gpu_utilization_basis_points)>0 THEN SUM(gpu_utilization_basis_points*sample_count)/SUM(CASE WHEN gpu_utilization_basis_points IS NOT NULL THEN sample_count ELSE 0 END) END,
		       CASE WHEN COUNT(gpu_memory_bytes)>0 THEN SUM(gpu_memory_bytes*sample_count)/SUM(CASE WHEN gpu_memory_bytes IS NOT NULL THEN sample_count ELSE 0 END) END
		FROM job_resource_samples
		WHERE resolution_seconds=5 AND captured_at<? AND captured_at>=?
		GROUP BY job_id,attempt_id,(captured_at/300)*300
		ON CONFLICT(attempt_id,captured_at,resolution_seconds) DO NOTHING`, rawCutoff, retentionCutoff)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM job_resource_samples WHERE resolution_seconds=5 AND captured_at<?`, rawCutoff); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM job_resource_samples WHERE captured_at<?`, retentionCutoff); err != nil {
		return err
	}
	return tx.Commit()
}
