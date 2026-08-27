package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

// JobAttemptRef identifies the current attempt whose bounded telemetry is
// needed by a list view. Callers are expected to authorize these references.
type JobAttemptRef struct {
	JobID     string
	AttemptID string
}

type NodeAssignmentRecord struct {
	Job      domain.Job
	CPUSet   string
	GPUUUIDs []string
}

func (s *Store) ActiveNodeAssignments(ctx context.Context, nodeID string) ([]NodeAssignmentRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT j.id,j.owner_id,j.spec_json,j.status,j.attempt_id,COALESCE(a.cpu_set,''),COALESCE(a.gpu_uuids_json,'[]')
		FROM jobs j LEFT JOIN assignments a ON a.job_id=j.id AND a.attempt_id=j.attempt_id
		WHERE j.assigned_node_id=? AND j.status IN ('ASSIGNED','PULLING_IMAGE','STARTING','RUNNING','STOPPING','LOST')
		ORDER BY j.created_at`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]NodeAssignmentRecord, 0)
	for rows.Next() {
		var item NodeAssignmentRecord
		var specJSON, gpuJSON string
		if err = rows.Scan(&item.Job.ID, &item.Job.OwnerID, &specJSON, &item.Job.Status, &item.Job.AttemptID, &item.CPUSet, &gpuJSON); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(specJSON), &item.Job.Spec); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(gpuJSON), &item.GPUUUIDs)
		items = append(items, item)
	}
	return items, rows.Err()
}

// ResourceSummaries returns at most limit recent points per attempt. When
// after is non-nil it returns only points added after the shared series cursor.
func (s *Store) ResourceSummaries(ctx context.Context, refs []JobAttemptRef, limit int, after *int64) (map[string][]domain.ResourceSample, int64, error) {
	result := make(map[string][]domain.ResourceSample, len(refs))
	if len(refs) == 0 {
		return result, 0, nil
	}
	conditions := make([]string, 0, len(refs))
	arguments := make([]any, 0, len(refs)*2+2)
	for _, ref := range refs {
		conditions = append(conditions, "(job_id=? AND attempt_id=?)")
		arguments = append(arguments, ref.JobID, ref.AttemptID)
	}
	filter := strings.Join(conditions, " OR ")
	var cursor int64
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM job_series_updates WHERE `+filter, arguments...).Scan(&cursor); err != nil {
		return nil, 0, err
	}
	query := `SELECT series_cursor,job_id,attempt_id,captured_at,resolution_seconds,sample_count,cpu_millis,memory_bytes,gpu_utilization_basis_points,gpu_memory_bytes FROM (
		SELECT COALESCE(series_cursor,0) AS series_cursor,job_id,attempt_id,captured_at,resolution_seconds,sample_count,cpu_millis,memory_bytes,gpu_utilization_basis_points,gpu_memory_bytes,
		ROW_NUMBER() OVER (PARTITION BY job_id,attempt_id ORDER BY captured_at DESC,COALESCE(series_cursor,0) DESC) AS position
		FROM job_resource_samples WHERE (` + filter + `)`
	if after != nil {
		query += ` AND series_cursor>?`
		arguments = append(arguments, *after)
	}
	query += `) WHERE position<=? ORDER BY job_id,attempt_id,captured_at,series_cursor`
	arguments = append(arguments, limit)
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var sample domain.ResourceSample
		var captured int64
		var gpuUtilization, gpuMemory sql.NullInt64
		if err = rows.Scan(&sample.Cursor, &sample.JobID, &sample.AttemptID, &captured, &sample.ResolutionSeconds, &sample.SampleCount, &sample.CPUMillis, &sample.MemoryBytes, &gpuUtilization, &gpuMemory); err != nil {
			return nil, 0, err
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
		result[sample.JobID] = append(result[sample.JobID], sample)
	}
	return result, cursor, rows.Err()
}
