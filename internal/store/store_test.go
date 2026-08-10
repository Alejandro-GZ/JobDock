package store_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/scheduler"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

func TestSQLiteSchedulingRoundTrip(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	user := domain.User{ID: ids.New(), Username: "admin", Role: domain.RoleAdmin, CreatedAt: time.Now().UTC()}
	if err = repository.CreateUser(ctx, user, "hash"); err != nil {
		t.Fatal(err)
	}
	node := domain.Node{ID: ids.New(), Name: "gpu-a", Status: domain.NodeOnline, AgentVersion: "test", ProtocolVersion: 1, Architecture: "amd64", DockerVersion: "29", CPUTotalMillis: 8000, MemoryTotalBytes: 32 << 30, WorkspaceFreeBytes: 100 << 30, Labels: map[string]string{"room": "a"}, GPUs: []domain.GPU{{UUID: "GPU-1", Model: "Test", VRAMBytes: 24 << 30}}, LastHeartbeat: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	if err = repository.UpsertNode(ctx, node, "credential"); err != nil {
		t.Fatal(err)
	}
	if err = repository.RotateNodeCredential(ctx, node.ID, "new-credential"); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.NodeByCredential(ctx, "credential"); err != nil {
		t.Fatalf("previous credential should remain valid during overlap: %v", err)
	}
	job := domain.Job{ID: ids.New(), OwnerID: user.ID, Spec: domain.JobSpec{Name: "gpu-job", Image: "alpine:3", Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 1000, MemoryBytes: 1 << 30, GPU: domain.GPURequest{Count: 1, MinVRAMBytes: 20 << 30}}, NodeSelector: map[string]string{"room": "a"}}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: time.Now().UTC()}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	box, _ := secretbox.New(bytes.Repeat([]byte{9}, 32))
	if err = scheduler.New(repository, box).Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	stored, err := repository.Job(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.JobAssigned || stored.AssignedNodeID != node.ID {
		t.Fatalf("unexpected placement %#v", stored)
	}
	assignment, err := repository.AssignmentForNode(ctx, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignment.GPUUUIDs) != 1 || assignment.GPUUUIDs[0] != "GPU-1" {
		t.Fatalf("unexpected GPUs %#v", assignment.GPUUUIDs)
	}
	if _, err = box.Decrypt(assignment.JobTokenEncrypted, []byte("assignment/"+assignment.ID)); err != nil {
		t.Fatal(err)
	}
}

func TestIdempotencyReplay(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	user := domain.User{ID: ids.New(), Username: "member", Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
	if err = repository.CreateUser(ctx, user, "hash"); err != nil {
		t.Fatal(err)
	}
	cached, _, _, err := repository.ClaimIdempotency(ctx, user.ID, "1234567890abcdef", "POST", "/jobs")
	if err != nil || cached {
		t.Fatalf("first claim: %v %v", cached, err)
	}
	if err = repository.CompleteIdempotency(ctx, user.ID, "1234567890abcdef", 201, []byte(`{"id":"one"}`)); err != nil {
		t.Fatal(err)
	}
	cached, status, data, err := repository.ClaimIdempotency(ctx, user.ID, "1234567890abcdef", "POST", "/jobs")
	if err != nil || !cached || status != 201 || string(data) != `{"id":"one"}` {
		t.Fatalf("replay: %v %d %s %v", cached, status, data, err)
	}
}
