package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/store"
)

func TestRichObservableDescriptorsAreStableAndAttemptScoped(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	user := domain.User{ID: ids.New(), Username: "observable-owner", Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
	if err = repository.CreateUser(ctx, user, "hash"); err != nil {
		t.Fatal(err)
	}
	node := domain.Node{ID: ids.New(), Name: "observable-node", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 1000, MemoryTotalBytes: 1024, WorkspaceFreeBytes: 1024, Labels: map[string]string{}, LastHeartbeat: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	if err = repository.UpsertNode(ctx, node, "observable-credential"); err != nil {
		t.Fatal(err)
	}
	job := domain.Job{ID: ids.New(), OwnerID: user.ID, Spec: domain.JobSpec{Name: "observables", Image: "alpine", Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1024}}, Status: domain.JobRunning, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobRunning, CreatedAt: time.Now().UTC()}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	attemptID := ids.New()
	if _, err = repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,1,?,?,?,?,?)`, attemptID, job.ID, node.ID, ids.New(), "RUNNING", ids.New(), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err = repository.AppendMetricSamples(ctx, []domain.MetricSample{{JobID: job.ID, AttemptID: attemptID, Name: "loss", Value: .5, CapturedAt: time.Now().UTC(), Tags: []string{"metric:loss"}}}); err != nil {
		t.Fatal(err)
	}
	if err = repository.AppendProgress(ctx, job.ID, attemptID, "simple", domain.ProgressObservation{Value: .5}); err != nil {
		t.Fatal(err)
	}
	if err = repository.AppendProgress(ctx, job.ID, attemptID, "simple", domain.ProgressObservation{Value: .75}); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.AppendMatrix(ctx, domain.MatrixObservation{JobID: job.ID, AttemptID: attemptID, Name: "validation", Values: [][]float64{{1, 0}, {0, 1}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.AppendMatrix(ctx, domain.MatrixObservation{JobID: job.ID, AttemptID: attemptID, Name: "validation", Values: [][]float64{{2, 0}, {0, 2}}}); err != nil {
		t.Fatal(err)
	}
	descriptors, err := repository.ObservableDescriptors(ctx, job.ID, attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if len(descriptors) != 3 || descriptors[0].Type != "metric" || descriptors[0].Name != "loss" || descriptors[1].Type != "matrix" || descriptors[1].Name != "validation" || descriptors[2].Type != "progress" || descriptors[2].Name != "progress" {
		t.Fatalf("observable descriptors: %#v", descriptors)
	}
	otherAttemptID := ids.New()
	if _, err = repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,2,?,?,?,?,?)`, otherAttemptID, job.ID, node.ID, ids.New(), "RUNNING", ids.New(), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	other, err := repository.ObservableDescriptors(ctx, job.ID, otherAttemptID)
	if err != nil || len(other) != 0 {
		t.Fatalf("descriptors leaked across attempts: %#v %v", other, err)
	}
}
