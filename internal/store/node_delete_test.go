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

func testDeleteNode(id string) domain.Node {
	now := time.Now().UTC()
	return domain.Node{
		ID: id, Name: "worker", Status: domain.NodeOnline,
		AgentVersion: "test", ProtocolVersion: 1, Architecture: "amd64", DockerVersion: "test",
		CPUTotalMillis: 4000, MemoryTotalBytes: 8 << 30, WorkspaceFreeBytes: 20 << 30,
		Labels: map[string]string{"zone": "test"},
		GPUDiscovery: domain.GPUDiscovery{Status: "unknown"},
		LastHeartbeat: now, CreatedAt: now,
	}
}

func TestDeleteNodeHidesNodeAndRevokesCredential(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	node := testDeleteNode(ids.New())
	if err = repository.UpsertNode(ctx, node, "node-delete-credential"); err != nil {
		t.Fatal(err)
	}
	if err = repository.DeleteNode(ctx, node.ID); err != nil {
		t.Fatal(err)
	}
	nodes, err := repository.ListNodes(ctx)
	if err != nil || len(nodes) != 0 {
		t.Fatalf("deleted node remained visible: %#v %v", nodes, err)
	}
	if _, err = repository.NodeByCredential(ctx, "node-delete-credential"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted credential lookup returned %v", err)
	}
	node.LastHeartbeat = time.Now().UTC()
	if err = repository.Heartbeat(ctx, node); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted node heartbeat returned %v", err)
	}
}

func TestDeleteNodeRejectsActiveJobButPreservesTerminalHistory(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	owner := domain.User{ID: ids.New(), Username: "node-delete-owner", Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
	if err = repository.CreateUser(ctx, owner, "hash"); err != nil {
		t.Fatal(err)
	}
	node := testDeleteNode(ids.New())
	if err = repository.UpsertNode(ctx, node, "active-node-credential"); err != nil {
		t.Fatal(err)
	}
	job := domain.Job{
		ID: ids.New(), OwnerID: owner.ID,
		Spec: domain.JobSpec{Name: "active node job", Image: "alpine:latest", Command: []string{"true"}},
		Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued,
		CreatedAt: time.Now().UTC(), Version: 1,
	}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	attemptID, assignmentID := ids.New(), ids.New()
	if err = repository.ReserveJob(ctx, job.ID, node.ID, attemptID, assignmentID, "job-token-hash", []byte("encrypted-token"), nil); err != nil {
		t.Fatal(err)
	}
	if err = repository.DeleteNode(ctx, node.ID); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("active node deletion returned %v", err)
	}
	exitCode := 1
	if err = repository.UpdateJobStatus(ctx, job.ID, domain.JobFailed, &exitCode, "", "test terminal state"); err != nil {
		t.Fatal(err)
	}
	if err = repository.DeleteNode(ctx, node.ID); err != nil {
		t.Fatalf("terminal node deletion: %v", err)
	}
	attempt, err := repository.Attempt(ctx, job.ID, attemptID)
	if err != nil || attempt.NodeID != node.ID || attempt.Status != domain.JobFailed {
		t.Fatalf("historical attempt was not preserved: %#v %v", attempt, err)
	}
}
