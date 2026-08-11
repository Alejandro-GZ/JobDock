package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/store"
)

func TestManagedArtifactOwnershipRerunAndSafeGarbageCollection(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(filepath.Join(t.TempDir(), "jobdock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	now := time.Now().UTC()
	owner := domain.User{ID: ids.New(), Username: "artifact-owner", Role: domain.RoleMember, CreatedAt: now}
	other := domain.User{ID: ids.New(), Username: "other-owner", Role: domain.RoleMember, CreatedAt: now}
	for _, user := range []domain.User{owner, other} {
		if err = repository.CreateUser(ctx, user, "hash"); err != nil {
			t.Fatal(err)
		}
	}
	digest := "sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	newArtifact := func(name string) (domain.Build, domain.ManagedArtifact) {
		build := domain.Build{ID: ids.New(), OwnerID: owner.ID, Name: name, Mode: domain.BuildModeDockerfile, Status: domain.BuildCreated, Source: domain.BuildSource{Filename: "source.zip", Size: 1, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}, CreatedAt: now, Version: 1}
		if err = repository.CreateBuild(ctx, build); err != nil {
			t.Fatal(err)
		}
		if _, err = repository.UpdateBuildStatus(ctx, build.ID, domain.BuildBuilding, "", "Building"); err != nil {
			t.Fatal(err)
		}
		artifact := domain.ManagedArtifact{BuildID: build.ID, OwnerID: owner.ID, Digest: digest, SHA256: "1123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Size: 42, MediaType: domain.ManagedImageMediaType, RuntimeImage: "jobdock.local/managed/" + build.ID + ":artifact", CreatedAt: now.Add(-48 * time.Hour), LastReferencedAt: now.Add(-48 * time.Hour)}
		if err = repository.SaveManagedArtifact(ctx, artifact); err != nil {
			t.Fatal(err)
		}
		if _, err = repository.UpdateBuildStatus(ctx, build.ID, domain.BuildSucceeded, digest, "Published"); err != nil {
			t.Fatal(err)
		}
		return build, artifact
	}
	referencedBuild, referenced := newArtifact("referenced")
	reference := domain.ManagedArtifactReference(referencedBuild.ID, digest)
	job := domain.Job{ID: ids.New(), OwnerID: owner.ID, Spec: domain.JobSpec{Name: "managed job", Image: reference, Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1 << 20}}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: now}
	if err = repository.CreateJobWithManagedArtifact(ctx, job, referencedBuild.ID, digest); err != nil {
		t.Fatal(err)
	}
	node := domain.Node{ID: ids.New(), Name: "managed-worker", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 1000, MemoryTotalBytes: 1 << 30, WorkspaceFreeBytes: 20 << 30, Labels: map[string]string{}, LastHeartbeat: now, CreatedAt: now}
	if err = repository.UpsertNode(ctx, node, "node-credential"); err != nil {
		t.Fatal(err)
	}
	if err = repository.ReserveJob(ctx, job.ID, node.ID, ids.New(), ids.New(), "job-token", []byte("cipher"), nil); err != nil {
		t.Fatal(err)
	}
	if err = repository.UpdateJobStatus(ctx, job.ID, domain.JobRunning, nil, digest, ""); err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	if err = repository.UpdateJobStatus(ctx, job.ID, domain.JobSucceeded, &exitCode, "", ""); err != nil {
		t.Fatal(err)
	}
	if err = repository.RerunJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	rerun, err := repository.Job(ctx, job.ID)
	if err != nil || rerun.Spec.Image != reference || rerun.Status != domain.JobQueued {
		t.Fatalf("rerun changed managed digest: %#v error=%v", rerun, err)
	}
	foreign := job
	foreign.ID, foreign.OwnerID = ids.New(), other.ID
	if err = repository.CreateJobWithManagedArtifact(ctx, foreign, referencedBuild.ID, digest); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-owner managed artifact create = %v", err)
	}
	unreferencedBuild, _ := newArtifact("unreferenced")
	collected, err := repository.GarbageCollectManagedArtifacts(ctx, now.Add(-24*time.Hour))
	if err != nil || len(collected) != 1 || collected[0].BuildID != unreferencedBuild.ID {
		t.Fatalf("collected=%#v error=%v", collected, err)
	}
	if _, err = repository.ManagedArtifact(ctx, referenced.BuildID); err != nil {
		t.Fatalf("referenced artifact was collected: %v", err)
	}
	if _, err = repository.ManagedArtifact(ctx, unreferencedBuild.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unreferenced artifact remains: %v", err)
	}
}
