package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/store"
)

func TestResourceSummariesAreBoundedAndIncremental(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	user := domain.User{ID: ids.New(), Username: "summary-owner", Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
	if err = repository.CreateUser(ctx, user, "hash"); err != nil {
		t.Fatal(err)
	}
	node := domain.Node{ID: ids.New(), Name: "summary-node", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 4000, MemoryTotalBytes: 8 << 30, WorkspaceFreeBytes: 10 << 30, Labels: map[string]string{}, LastHeartbeat: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	if err = repository.UpsertNode(ctx, node, "credential-hash"); err != nil {
		t.Fatal(err)
	}
	job := domain.Job{ID: ids.New(), OwnerID: user.ID, Spec: domain.JobSpec{Name: "summary", Resources: domain.Resources{CPUMillis: 1000, MemoryBytes: 1024}}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: time.Now().UTC()}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	attemptID := ids.New()
	if _, err = repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,1,?,?,'RUNNING',?,?)`, attemptID, job.ID, node.ID, ids.New(), ids.New(), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	for index := 0; index < 4; index++ {
		if err = repository.AppendResourceSample(ctx, domain.ResourceSample{JobID: job.ID, AttemptID: attemptID, CapturedAt: base.Add(time.Duration(index) * 5 * time.Second), CPUMillis: int64(100 + index), MemoryBytes: int64(1000 + index)}); err != nil {
			t.Fatal(err)
		}
	}
	refs := []store.JobAttemptRef{{JobID: job.ID, AttemptID: attemptID}}
	initial, cursor, err := repository.ResourceSummaries(ctx, refs, 3, nil)
	if err != nil || len(initial[job.ID]) != 3 || initial[job.ID][0].CPUMillis != 101 || cursor == 0 {
		t.Fatalf("initial summary: %#v cursor=%d error=%v", initial, cursor, err)
	}
	if err = repository.AppendResourceSample(ctx, domain.ResourceSample{JobID: job.ID, AttemptID: attemptID, CapturedAt: base.Add(25 * time.Second), CPUMillis: 999, MemoryBytes: 9999}); err != nil {
		t.Fatal(err)
	}
	incremental, next, err := repository.ResourceSummaries(ctx, refs, 3, &cursor)
	if err != nil || len(incremental[job.ID]) != 1 || incremental[job.ID][0].CPUMillis != 999 || next <= cursor {
		t.Fatalf("incremental summary: %#v cursor=%d error=%v", incremental, next, err)
	}
}
