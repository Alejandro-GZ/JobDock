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
	if err = repository.AppendServerEvent(ctx, job.ID, "queued", map[string]any{}); err != nil {
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
	if assignment.EventSequence != 2 {
		t.Fatalf("assignment must continue after queued and assigned events, got sequence %d", assignment.EventSequence)
	}
	if _, err = box.Decrypt(assignment.JobTokenEncrypted, []byte("assignment/"+assignment.ID)); err != nil {
		t.Fatal(err)
	}
	if err = repository.UpdateJobStatus(ctx, job.ID, domain.JobFailed, nil, "", "pull failed"); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.AssignmentForNode(ctx, node.ID); err != store.ErrNotFound {
		t.Fatalf("terminal assignment was redelivered: %v", err)
	}
}

func TestHardwareInventoryAndAssignmentRoundTrip(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/hardware.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	now := time.Now().UTC()
	user := domain.User{ID: ids.New(), Username: "hardware", Role: domain.RoleMember, CreatedAt: now}
	if err = repository.CreateUser(ctx, user, "hash"); err != nil {
		t.Fatal(err)
	}
	utilization, average, peak, memoryUsed, temperature := int64(5100), int64(4300), int64(7800), int64(512), int64(48)
	node := domain.Node{ID: ids.New(), Name: "dual-socket", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 4000, MemoryTotalBytes: 4096, WorkspaceTotalBytes: 40 << 30, WorkspaceFreeBytes: 20 << 30, Labels: map[string]string{}, Capabilities: []string{"cpu_package_affinity", "gpu_window_telemetry_v1"}, CPUPackages: []domain.CPUPackage{{ID: "1", Vendor: "Test Vendor", Model: "Test CPU", PhysicalCores: 1, LogicalCPUs: []int{2, 6}, TotalMillis: 2000}}, GPUs: []domain.GPU{{UUID: "GPU-1", Model: "Test GPU", VRAMBytes: 1024, PCIBusID: "0000:01:00.0", DriverVersion: "580", ComputeCapability: "8.9", UtilizationBasisPoints: &utilization, UtilizationAverageBasisPoints: &average, UtilizationPeakBasisPoints: &peak, UtilizationSampledAt: &now, UtilizationWindowSeconds: 10, UtilizationSampleCount: 10, MemoryUsedBytes: &memoryUsed, TemperatureCelsius: &temperature}}, GPUDiscovery: domain.GPUDiscovery{Status: "available"}, System: domain.NodeSystemInfo{Hostname: "worker", OperatingSystem: "Linux", KernelVersion: "6.8", Architecture: "amd64"}, Runtime: domain.NodeRuntimeInfo{DockerVersion: "29", StorageDriver: "overlay2", CgroupVersion: "2"}, LastHeartbeat: now, CreatedAt: now}
	if err = repository.UpsertNode(ctx, node, "hardware-credential"); err != nil {
		t.Fatal(err)
	}
	nodes, err := repository.ListNodes(ctx)
	if err != nil || len(nodes) != 1 || len(nodes[0].CPUPackages) != 1 || nodes[0].CPUPackages[0].LogicalCPUs[1] != 6 || nodes[0].CPUPackages[0].Vendor != "Test Vendor" || len(nodes[0].Capabilities) != 2 || nodes[0].WorkspaceTotalBytes != 40<<30 || nodes[0].System.Hostname != "worker" || nodes[0].Runtime.StorageDriver != "overlay2" || nodes[0].GPUs[0].PCIBusID != "0000:01:00.0" || nodes[0].GPUs[0].UtilizationBasisPoints == nil || *nodes[0].GPUs[0].UtilizationBasisPoints != utilization || nodes[0].GPUs[0].UtilizationAverageBasisPoints == nil || *nodes[0].GPUs[0].UtilizationAverageBasisPoints != average || nodes[0].GPUs[0].UtilizationPeakBasisPoints == nil || *nodes[0].GPUs[0].UtilizationPeakBasisPoints != peak || nodes[0].GPUs[0].UtilizationSampledAt == nil || nodes[0].GPUs[0].UtilizationSampleCount != 10 {
		t.Fatalf("inventory did not round trip: %#v %v", nodes, err)
	}
	job := domain.Job{ID: ids.New(), OwnerID: user.ID, Spec: domain.JobSpec{Name: "pinned-job", Image: "alpine", TargetNodeID: node.ID, Resources: domain.Resources{CPUMillis: 1000, CPUPackageID: "1", MemoryBytes: 1024, GPU: domain.GPURequest{Count: 1, UUIDs: []string{"GPU-1"}}}}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: now}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err = repository.ReserveJobWithAffinity(ctx, job.ID, node.ID, "attempt", "assignment", "token", []byte("cipher"), []string{"GPU-1"}, "1", "2,6"); err != nil {
		t.Fatal(err)
	}
	assignment, err := repository.AssignmentForNode(ctx, node.ID)
	if err != nil || assignment.CPUPackageID != "1" || assignment.CPUSet != "2,6" || assignment.GPUUUIDs[0] != "GPU-1" {
		t.Fatalf("assignment did not round trip: %#v %v", assignment, err)
	}
	attempt, err := repository.Attempt(ctx, job.ID, "attempt")
	if err != nil || attempt.CPUPackageID != "1" || attempt.GPUUUIDs[0] != "GPU-1" {
		t.Fatalf("attempt did not retain hardware: %#v %v", attempt, err)
	}
}

func TestLegacyNodeCollectionsAreNormalized(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/legacy-node.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	now := time.Now().UTC()
	node := domain.Node{ID: ids.New(), Name: "legacy", Status: domain.NodeOnline, ProtocolVersion: 1, LastHeartbeat: now, CreatedAt: now}
	if err = repository.UpsertNode(ctx, node, "legacy-credential"); err != nil {
		t.Fatal(err)
	}
	nodes, err := repository.ListNodes(ctx)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("list legacy node: %#v %v", nodes, err)
	}
	if nodes[0].Capabilities == nil || nodes[0].Labels == nil || nodes[0].GPUs == nil || nodes[0].CPUPackages == nil {
		t.Fatalf("legacy collections must be empty rather than null: %#v", nodes[0])
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

func TestRerunCreatesNumberedTraceableAttempts(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	now := time.Now().UTC()
	user := domain.User{ID: ids.New(), Username: "owner", Role: domain.RoleMember, CreatedAt: now}
	node := domain.Node{ID: ids.New(), Name: "worker", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 2000, MemoryTotalBytes: 2 << 30, WorkspaceFreeBytes: 20 << 30, Labels: map[string]string{}, LastHeartbeat: now, CreatedAt: now}
	if err = repository.CreateUser(ctx, user, "hash"); err != nil {
		t.Fatal(err)
	}
	if err = repository.UpsertNode(ctx, node, "credential"); err != nil {
		t.Fatal(err)
	}
	job := domain.Job{ID: ids.New(), OwnerID: user.ID, Spec: domain.JobSpec{Name: "repeatable", Image: "alpine:3", Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1 << 20}}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: now}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	attemptOne := ids.New()
	if err = repository.ReserveJob(ctx, job.ID, node.ID, attemptOne, ids.New(), "token-one", []byte("cipher"), nil); err != nil {
		t.Fatal(err)
	}
	if err = repository.UpdateJobStatus(ctx, job.ID, domain.JobRunning, nil, "sha256:first", ""); err != nil {
		t.Fatal(err)
	}
	outputs := []domain.OutputFile{{Path: "result.txt", Size: 6, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	if err = repository.SetAttemptOutputs(ctx, job.ID, attemptOne, outputs); err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	if err = repository.UpdateJobStatus(ctx, job.ID, domain.JobSucceeded, &exitCode, "", ""); err != nil {
		t.Fatal(err)
	}
	if err = repository.RerunJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	queued, err := repository.Job(ctx, job.ID)
	if err != nil || queued.Status != domain.JobQueued || queued.AttemptID != "" || queued.Spec.Image != job.Spec.Image || queued.StartedAt != nil || queued.FinishedAt != nil {
		t.Fatalf("rerun did not reset only execution state: %#v %v", queued, err)
	}
	attemptTwo := ids.New()
	if err = repository.ReserveJob(ctx, job.ID, node.ID, attemptTwo, ids.New(), "token-two", []byte("cipher"), nil); err != nil {
		t.Fatal(err)
	}
	currentAssignment, err := repository.AssignmentForNode(ctx, node.ID)
	if err != nil || currentAssignment.AttemptID != attemptTwo {
		t.Fatalf("rerun delivered stale assignment: %#v %v", currentAssignment, err)
	}
	if err = repository.RerunJob(ctx, job.ID); err != store.ErrConflict {
		t.Fatalf("active job rerun = %v, want conflict", err)
	}
	attempts, err := repository.Attempts(ctx, job.ID)
	if err != nil || len(attempts) != 2 || attempts[0].AttemptNumber != 2 || attempts[1].AttemptNumber != 1 {
		t.Fatalf("numbered attempts: %#v %v", attempts, err)
	}
	first := attempts[1]
	if first.NodeID != node.ID || first.Status != domain.JobSucceeded || first.StartedAt == nil || first.FinishedAt == nil || first.ExitCode == nil || *first.ExitCode != 0 || len(first.Outputs) != 1 || first.Outputs[0].Path != "result.txt" {
		t.Fatalf("first attempt trace was not retained: %#v", first)
	}
}

func TestEmptyCollectionsAreJSONArrays(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	users, err := repository.ListUsers(ctx)
	if err != nil || users == nil {
		t.Fatalf("users must be an empty slice: %#v %v", users, err)
	}
	jobs, err := repository.ListJobs(ctx, false)
	if err != nil || jobs == nil {
		t.Fatalf("jobs must be an empty slice: %#v %v", jobs, err)
	}
	nodes, err := repository.ListNodes(ctx)
	if err != nil || nodes == nil {
		t.Fatalf("nodes must be an empty slice: %#v %v", nodes, err)
	}
	events, err := repository.Events(ctx, "missing-job", 0)
	if err != nil || events == nil {
		t.Fatalf("events must be an empty slice: %#v %v", events, err)
	}
	secrets, err := repository.ListSecrets(ctx, "missing-owner")
	if err != nil || secrets == nil {
		t.Fatalf("secrets must be an empty slice: %#v %v", secrets, err)
	}
	audit, err := repository.ListAudit(ctx, 10)
	if err != nil || audit == nil {
		t.Fatalf("audit must be an empty slice: %#v %v", audit, err)
	}
}

func TestJobUpdatesAreIsolatedByOwner(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	for _, owner := range []string{"owner-a", "owner-b"} {
		user := domain.User{ID: owner, Username: owner, Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
		if err = repository.CreateUser(ctx, user, "hash"); err != nil {
			t.Fatal(err)
		}
		job := domain.Job{ID: "job-" + owner, OwnerID: owner, Spec: domain.JobSpec{Name: owner, Image: "alpine"}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: time.Now().UTC()}
		if err = repository.CreateJob(ctx, job); err != nil {
			t.Fatal(err)
		}
		if err = repository.AppendServerEvent(ctx, job.ID, "queued", map[string]any{}); err != nil {
			t.Fatal(err)
		}
	}
	updates, err := repository.JobUpdatesForOwner(ctx, "owner-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 || updates[0].JobID != "job-owner-a" || updates[0].Status != domain.JobQueued {
		t.Fatalf("unexpected private updates: %#v", updates)
	}
}

func TestNodeMetadataOverridesSurviveHeartbeats(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	node := domain.Node{ID: ids.New(), Name: "reported", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 1000, MemoryTotalBytes: 1024, WorkspaceFreeBytes: 1024, Labels: map[string]string{"source": "agent"}, LastHeartbeat: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	if err = repository.UpsertNode(ctx, node, "credential"); err != nil {
		t.Fatal(err)
	}
	if err = repository.UpdateNodeMetadata(ctx, node.ID, "effective", map[string]string{"zone": "lab"}); err != nil {
		t.Fatal(err)
	}
	node.Name = "reported-again"
	node.Labels = map[string]string{"source": "new-agent-value"}
	node.LastHeartbeat = time.Now().UTC().Add(time.Second)
	if err = repository.Heartbeat(ctx, node); err != nil {
		t.Fatal(err)
	}
	nodes, err := repository.ListNodes(ctx)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("nodes: %#v %v", nodes, err)
	}
	if nodes[0].Name != "effective" || nodes[0].Labels["zone"] != "lab" || len(nodes[0].Labels) != 1 {
		t.Fatalf("heartbeat replaced effective metadata: %#v", nodes[0])
	}
}

func TestLostJobRetainsLatestConfirmedCheckpoint(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	user := domain.User{ID: "checkpoint-owner", Username: "checkpoint-owner", Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
	if err = repository.CreateUser(ctx, user, "hash"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-10 * time.Minute)
	node := domain.Node{ID: "checkpoint-node", Name: "node", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 2000, MemoryTotalBytes: 2 << 30, WorkspaceFreeBytes: 20 << 30, Labels: map[string]string{}, LastHeartbeat: old, CreatedAt: old}
	if err = repository.UpsertNode(ctx, node, "credential"); err != nil {
		t.Fatal(err)
	}
	job := domain.Job{ID: "checkpoint-job", OwnerID: user.ID, Spec: domain.JobSpec{Name: "train", Image: "train:latest", Command: []string{"train"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1024}}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: time.Now().UTC()}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err = repository.ReserveJob(ctx, job.ID, node.ID, "attempt-one", "assignment-one", "hash", []byte("cipher"), nil); err != nil {
		t.Fatal(err)
	}
	checkpoint := domain.CheckpointSync{ID: "sync-confirmed", JobID: job.ID, AttemptID: "attempt-one", RequestedAt: time.Now().UTC()}
	if err = repository.CreateCheckpointSync(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	if err = repository.ConfirmCheckpointSync(ctx, checkpoint.ID, []domain.CheckpointFile{{Path: "epoch-10.pt", Size: 42}}); err != nil {
		t.Fatal(err)
	}
	if err = repository.MarkStaleNodes(ctx, time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	stored, err := repository.Job(ctx, job.ID)
	if err != nil || stored.Status != domain.JobLost {
		t.Fatalf("job was not lost: %#v %v", stored, err)
	}
	latest, err := repository.LatestConfirmedCheckpoint(ctx, job.ID)
	if err != nil || latest.ID != checkpoint.ID || latest.FileCount != 1 || latest.ByteCount != 42 {
		t.Fatalf("latest checkpoint lost: %#v %v", latest, err)
	}
}
