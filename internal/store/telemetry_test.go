package store_test

import (
	"context"
	"errors"
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
	job := domain.Job{ID: ids.New(), OwnerID: user.ID, Spec: domain.JobSpec{Name: "telemetry", Image: "alpine:3", Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1024}}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: time.Now().UTC()}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)
	gpuLow, gpuHigh := int64(1000), int64(3000)
	gpuMemoryLow, gpuMemoryHigh := int64(1<<30), int64(3<<30)
	oldBucket := now.Add(-25 * time.Hour).Truncate(5 * time.Minute)
	samples := []domain.ResourceSample{
		{JobID: job.ID, CapturedAt: oldBucket.Add(10 * time.Second), CPUMillis: 1000, MemoryBytes: 100, GPUUtilizationBasisPoints: &gpuLow, GPUMemoryBytes: &gpuMemoryLow},
		{JobID: job.ID, CapturedAt: oldBucket.Add(20 * time.Second), CPUMillis: 3000, MemoryBytes: 300, GPUUtilizationBasisPoints: &gpuHigh, GPUMemoryBytes: &gpuMemoryHigh},
		{JobID: job.ID, CapturedAt: now.Add(-time.Hour), CPUMillis: 500, MemoryBytes: 50},
		{JobID: job.ID, CapturedAt: now.Add(-31 * 24 * time.Hour), CPUMillis: 9000, MemoryBytes: 900},
	}
	for _, sample := range samples {
		if err = repository.AppendResourceSample(ctx, sample); err != nil {
			t.Fatal(err)
		}
	}
	if err = repository.MaintainResourceTelemetry(ctx, now, 24*time.Hour, 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	stored, err := repository.ResourceSamples(ctx, job.ID, time.Unix(0, 0), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 {
		t.Fatalf("expected one compacted and one raw sample, got %#v", stored)
	}
	compacted := stored[0]
	if compacted.ResolutionSeconds != 300 || compacted.SampleCount != 2 || compacted.CPUMillis != 2000 || compacted.MemoryBytes != 200 || compacted.GPUUtilizationBasisPoints == nil || *compacted.GPUUtilizationBasisPoints != 2000 || compacted.GPUMemoryBytes == nil || *compacted.GPUMemoryBytes != 2<<30 {
		t.Fatalf("unexpected compacted sample: %#v", compacted)
	}
	if stored[1].ResolutionSeconds != 5 || stored[1].SampleCount != 1 {
		t.Fatalf("recent sample was downsampled: %#v", stored[1])
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
