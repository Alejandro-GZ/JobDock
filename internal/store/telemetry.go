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
	_, err := s.db.ExecContext(ctx, `INSERT INTO job_resource_samples(job_id,captured_at,resolution_seconds,sample_count,cpu_millis,memory_bytes,gpu_utilization_basis_points,gpu_memory_bytes) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(job_id,captured_at,resolution_seconds) DO NOTHING`,
		sample.JobID, sample.CapturedAt.UTC().Unix(), int(TelemetryRawResolution/time.Second), 1, sample.CPUMillis, sample.MemoryBytes, sample.GPUUtilizationBasisPoints, sample.GPUMemoryBytes)
	return mapConstraint(err)
}

func (s *Store) ResourceSamples(ctx context.Context, jobID string, from, to time.Time) ([]domain.ResourceSample, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT job_id,captured_at,resolution_seconds,sample_count,cpu_millis,memory_bytes,gpu_utilization_basis_points,gpu_memory_bytes FROM job_resource_samples WHERE job_id=? AND captured_at>=? AND captured_at<=? ORDER BY captured_at,resolution_seconds`, jobID, from.UTC().Unix(), to.UTC().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.ResourceSample, 0)
	for rows.Next() {
		var sample domain.ResourceSample
		var captured int64
		var gpuUtilization, gpuMemory sql.NullInt64
		if err = rows.Scan(&sample.JobID, &captured, &sample.ResolutionSeconds, &sample.SampleCount, &sample.CPUMillis, &sample.MemoryBytes, &gpuUtilization, &gpuMemory); err != nil {
			return nil, err
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
	return result, rows.Err()
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
	_, err = tx.ExecContext(ctx, `INSERT INTO job_resource_samples(job_id,captured_at,resolution_seconds,sample_count,cpu_millis,memory_bytes,gpu_utilization_basis_points,gpu_memory_bytes)
		SELECT job_id,(captured_at/300)*300,300,SUM(sample_count),
		       SUM(cpu_millis*sample_count)/SUM(sample_count),
		       SUM(memory_bytes*sample_count)/SUM(sample_count),
		       CASE WHEN COUNT(gpu_utilization_basis_points)>0 THEN SUM(gpu_utilization_basis_points*sample_count)/SUM(CASE WHEN gpu_utilization_basis_points IS NOT NULL THEN sample_count ELSE 0 END) END,
		       CASE WHEN COUNT(gpu_memory_bytes)>0 THEN SUM(gpu_memory_bytes*sample_count)/SUM(CASE WHEN gpu_memory_bytes IS NOT NULL THEN sample_count ELSE 0 END) END
		FROM job_resource_samples
		WHERE resolution_seconds=5 AND captured_at<? AND captured_at>=?
		GROUP BY job_id,(captured_at/300)*300
		ON CONFLICT(job_id,captured_at,resolution_seconds) DO NOTHING`, rawCutoff, retentionCutoff)
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
