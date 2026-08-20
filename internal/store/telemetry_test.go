package store_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/store"
)

func TestResourceTelemetryDownsamplingAndRetention(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	user := domain.User{ID: ids.New(), Username: "telemetry-owner", Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
	if err = repository.CreateUser(ctx, user, "hash"); err != nil {
		t.Fatal(err)
	}
	node := domain.Node{ID: ids.New(), Name: "telemetry-node", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 4000, MemoryTotalBytes: 8 << 30, WorkspaceFreeBytes: 10 << 30, Labels: map[string]string{}, LastHeartbeat: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	if err = repository.UpsertNode(ctx, node, "telemetry-credential"); err != nil {
		t.Fatal(err)
	}
	job := domain.Job{ID: ids.New(), OwnerID: user.ID, Spec: domain.JobSpec{Name: "telemetry", Image: "alpine:3", Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1024}}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: time.Now().UTC()}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	attemptID := ids.New()
	if _, err = repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,1,?,?,?, ?,?); UPDATE jobs SET attempt_id=?,assigned_node_id=? WHERE id=?`, attemptID, job.ID, node.ID, ids.New(), "RUNNING", ids.New(), time.Now().UTC().Format(time.RFC3339Nano), attemptID, node.ID, job.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)
	gpuLow, gpuHigh := int64(1000), int64(3000)
	gpuMemoryLow, gpuMemoryHigh := int64(1<<30), int64(3<<30)
	oldBucket := now.Add(-25 * time.Hour).Truncate(5 * time.Minute)
	samples := []domain.ResourceSample{
		{JobID: job.ID, AttemptID: attemptID, CapturedAt: oldBucket.Add(10 * time.Second), CPUMillis: 1000, MemoryBytes: 100, GPUUtilizationBasisPoints: &gpuLow, GPUMemoryBytes: &gpuMemoryLow},
		{JobID: job.ID, AttemptID: attemptID, CapturedAt: oldBucket.Add(20 * time.Second), CPUMillis: 3000, MemoryBytes: 300, GPUUtilizationBasisPoints: &gpuHigh, GPUMemoryBytes: &gpuMemoryHigh},
		{JobID: job.ID, AttemptID: attemptID, CapturedAt: now.Add(-time.Hour), CPUMillis: 500, MemoryBytes: 50},
		{JobID: job.ID, AttemptID: attemptID, CapturedAt: now.Add(-31 * 24 * time.Hour), CPUMillis: 9000, MemoryBytes: 900},
	}
	for _, sample := range samples {
		if err = repository.AppendResourceSample(ctx, sample); err != nil {
			t.Fatal(err)
		}
	}
	if err = repository.MaintainResourceTelemetry(ctx, now, 24*time.Hour, 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	storedRaw, _, err := repository.ResourceSamples(ctx, job.ID, attemptID, time.Unix(0, 0), now, 5, 100)
	if err != nil {
		t.Fatal(err)
	}
	storedCompacted, _, err := repository.ResourceSamples(ctx, job.ID, attemptID, time.Unix(0, 0), now, 300, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedRaw) != 1 || len(storedCompacted) != 1 {
		t.Fatalf("expected one compacted and one raw sample, got raw=%#v compacted=%#v", storedRaw, storedCompacted)
	}
	compacted := storedCompacted[0]
	if compacted.ResolutionSeconds != 300 || compacted.SampleCount != 2 || compacted.CPUMillis != 2000 || compacted.MemoryBytes != 200 || compacted.GPUUtilizationBasisPoints == nil || *compacted.GPUUtilizationBasisPoints != 2000 || compacted.GPUMemoryBytes == nil || *compacted.GPUMemoryBytes != 2<<30 {
		t.Fatalf("unexpected compacted sample: %#v", compacted)
	}
	if storedRaw[0].ResolutionSeconds != 5 || storedRaw[0].SampleCount != 1 || storedRaw[0].AttemptID != attemptID {
		t.Fatalf("recent sample was downsampled or lost attempt identity: %#v", storedRaw[0])
	}
}

func TestMetricSeriesAreAttemptAwareBoundedAndAggregated(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	user := domain.User{ID: ids.New(), Username: "metric-owner", Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
	_ = repository.CreateUser(ctx, user, "hash")
	node := domain.Node{ID: ids.New(), Name: "metric-node", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 1000, MemoryTotalBytes: 1024, WorkspaceFreeBytes: 1024, Labels: map[string]string{}, LastHeartbeat: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	_ = repository.UpsertNode(ctx, node, "metric-credential")
	job := domain.Job{ID: ids.New(), OwnerID: user.ID, Spec: domain.JobSpec{Name: "metrics", Image: "alpine:3", Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1024}}, Status: domain.JobRunning, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobRunning, CreatedAt: time.Now().UTC()}
	_ = repository.CreateJob(ctx, job)
	attemptID := ids.New()
	_, err = repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,1,?,?,?, ?,?); UPDATE jobs SET attempt_id=?,assigned_node_id=? WHERE id=?`, attemptID, job.ID, node.ID, ids.New(), "RUNNING", ids.New(), time.Now().UTC().Format(time.RFC3339Nano), attemptID, node.ID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	step1, step2 := int64(1), int64(2)
	samples := []domain.MetricSample{
		{JobID: job.ID, AttemptID: attemptID, Name: "loss", Step: &step1, Value: 4, CapturedAt: base, Unit: "ratio", Metadata: map[string]any{"split": "train"}, Tags: []string{"phase:train", "metric:loss"}},
		{JobID: job.ID, AttemptID: attemptID, Name: "loss", Step: &step2, Value: 2, CapturedAt: base.Add(10 * time.Second)},
		{JobID: job.ID, AttemptID: attemptID, Name: "accuracy", Step: &step2, Value: .75, CapturedAt: base.Add(10 * time.Second)},
	}
	if err = repository.AppendMetricSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	series, truncated, err := repository.MetricSeries(ctx, job.ID, attemptID, []string{"loss"}, base.Add(-time.Second), base.Add(time.Minute), 60, 100)
	if err != nil || truncated || len(series) != 1 || len(series[0].Points) != 1 {
		t.Fatalf("metric series: %#v truncated=%v err=%v", series, truncated, err)
	}
	if series[0].Points[0].Value != 3 || series[0].Points[0].SampleCount != 2 || series[0].Last != 2 || series[0].Min != 2 || series[0].Max != 4 || series[0].SampleCount != 2 {
		t.Fatalf("metric aggregation or statistics are wrong: %#v", series[0])
	}
	if series[0].Unit != "ratio" || series[0].Metadata["split"] != "train" || !reflect.DeepEqual(series[0].Tags, []string{"metric:loss", "phase:train"}) {
		t.Fatalf("metric descriptor was not preserved: %#v", series[0])
	}
	if err = repository.AppendMetricSamples(ctx, []domain.MetricSample{{JobID: job.ID, AttemptID: attemptID, Name: "loss", Value: 99, CapturedAt: base.Add(15 * time.Second), Unit: "seconds"}}); !errors.Is(err, store.ErrMetricDescriptorConflict) {
		t.Fatalf("descriptor conflict: %v", err)
	}
	if err = repository.AppendMetricSamples(ctx, []domain.MetricSample{{JobID: job.ID, AttemptID: attemptID, Name: "loss", Value: 99, CapturedAt: base.Add(15 * time.Second), Tags: []string{"phase:validation", "metric:loss"}}}); !errors.Is(err, store.ErrMetricDescriptorConflict) {
		t.Fatalf("semantic tag conflict: %v", err)
	}
	if err = repository.AppendMetricSamples(ctx, []domain.MetricSample{
		{JobID: job.ID, AttemptID: attemptID, Name: "must-rollback", Value: 1, CapturedAt: base.Add(16 * time.Second), Tags: []string{"custom:temporary"}},
		{JobID: job.ID, AttemptID: attemptID, Name: "loss", Value: 1, CapturedAt: base.Add(16 * time.Second), Tags: []string{"phase:test"}},
	}); !errors.Is(err, store.ErrMetricDescriptorConflict) {
		t.Fatalf("atomic semantic tag conflict: %v", err)
	}
	var rolledBack int
	if err = repository.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM job_metric_samples WHERE attempt_id=? AND name='must-rollback'`, attemptID).Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatalf("conflicting batch was partially committed: count=%d err=%v", rolledBack, err)
	}
	_, truncated, err = repository.MetricSeries(ctx, job.ID, attemptID, nil, base.Add(-time.Second), base.Add(time.Minute), 0, 1)
	if err != nil || !truncated {
		t.Fatalf("bounded metric query: truncated=%v err=%v", truncated, err)
	}
	snapshotCursor, err := repository.LatestSeriesCursor(ctx, job.ID, attemptID)
	if err != nil || snapshotCursor == 0 {
		t.Fatalf("initial series cursor: %d %v", snapshotCursor, err)
	}
	step3 := int64(3)
	if err = repository.AppendMetricSamples(ctx, []domain.MetricSample{{JobID: job.ID, AttemptID: attemptID, Name: "loss", Step: &step3, Value: 1, CapturedAt: base.Add(20 * time.Second)}}); err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := repository.MetricSeriesAt(ctx, job.ID, attemptID, []string{"loss"}, base.Add(-time.Second), base.Add(time.Minute), 0, 100, snapshotCursor)
	if err != nil || len(snapshot) != 1 || snapshot[0].SampleCount != 2 || snapshot[0].Last != 2 {
		t.Fatalf("cursor-consistent snapshot: %#v %v", snapshot, err)
	}
	updates, hasMore, err := repository.SeriesUpdates(ctx, job.ID, attemptID, snapshotCursor, 10)
	if err != nil || hasMore || len(updates) != 1 || updates[0].Cursor <= snapshotCursor || len(updates[0].Metrics) != 1 || updates[0].Metrics[0].Value != 1 {
		t.Fatalf("incremental metric updates: %#v more=%v err=%v", updates, hasMore, err)
	}
	if err = repository.AppendMetricSamples(ctx, []domain.MetricSample{{JobID: job.ID, AttemptID: attemptID, Name: "loss", Value: 3, CapturedAt: base.Add(5 * time.Second)}}); err != nil {
		t.Fatal(err)
	}
	raw, _, err := repository.MetricSeries(ctx, job.ID, attemptID, []string{"loss"}, base.Add(-time.Second), base.Add(time.Minute), 0, 100)
	if err != nil || len(raw) != 1 || len(raw[0].Points) != 4 || !raw[0].Points[1].CapturedAt.Equal(base.Add(5*time.Second)) {
		t.Fatalf("out-of-arrival samples are not in temporal order: %#v %v", raw, err)
	}
	secondAttempt := ids.New()
	if _, err = repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,2,?,?,?,?,?)`, secondAttempt, job.ID, node.ID, ids.New(), "RUNNING", ids.New(), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err = repository.AppendMetricSamples(ctx, []domain.MetricSample{{JobID: job.ID, AttemptID: secondAttempt, Name: "loss", Value: 9, CapturedAt: base, Unit: "seconds"}}); err != nil {
		t.Fatalf("descriptor leaked between attempts: %v", err)
	}
}

func TestRawDockerStatsEventsAreRejected(t *testing.T) {
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	err = repository.AppendEvent(context.Background(), domain.Event{JobID: "job", Sequence: 1, Type: "resource_sample", Payload: map[string]any{"networks": map[string]any{"eth0": map[string]any{"rx_bytes": 42}}}})
	if !errors.Is(err, store.ErrRawTelemetry) {
		t.Fatalf("raw event was not rejected: %v", err)
	}
}
