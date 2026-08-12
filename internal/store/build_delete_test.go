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

func TestDeleteBuildRejectsActiveAndReferencedBuilds(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	owner := domain.User{ID: ids.New(), Username: "delete-builder", Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
	if err = repository.CreateUser(ctx, owner, "hash"); err != nil {
		t.Fatal(err)
	}
	newBuild := func(name string) domain.Build {
		return domain.Build{
			ID: ids.New(), OwnerID: owner.ID, Name: name,
			Mode: domain.BuildModeRailpack, Status: domain.BuildCreated,
			Source: domain.BuildSource{
				Filename: "project.zip",
				Size:     16,
				SHA256:   "fc17afe4af56fca9d2943b7901e7517611b37a36db7a7775b3e341e7d20a6ba0",
			},
			CreatedAt: time.Now().UTC(), Version: 1,
		}
	}

	active := newBuild("active build")
	if err = repository.CreateBuild(ctx, active); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.UpdateBuildStatus(ctx, active.ID, domain.BuildAnalyzing, "", "analyzing"); err != nil {
		t.Fatal(err)
	}
	if err = repository.DeleteBuild(ctx, active.ID); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("active build deletion returned %v", err)
	}
	if _, err = repository.UpdateBuildStatus(ctx, active.ID, domain.BuildCancelled, "", "cancelled"); err != nil {
		t.Fatal(err)
	}
	if err = repository.DeleteBuild(ctx, active.ID); err != nil {
		t.Fatalf("cancelled build deletion: %v", err)
	}

	succeeded := newBuild("referenced build")
	if err = repository.CreateBuild(ctx, succeeded); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.UpdateBuildStatus(ctx, succeeded.ID, domain.BuildAnalyzing, "", "analyzing"); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.UpdateBuildStatus(ctx, succeeded.ID, domain.BuildBuilding, "", "building"); err != nil {
		t.Fatal(err)
	}
	digest := "sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	if _, err = repository.UpdateBuildStatus(ctx, succeeded.ID, domain.BuildSucceeded, digest, "done"); err != nil {
		t.Fatal(err)
	}
	job := domain.Job{
		ID: ids.New(), OwnerID: owner.ID,
		Spec: domain.JobSpec{Name: "uses managed build", Image: domain.ManagedArtifactReference(succeeded.ID, digest), Command: []string{"true"}},
		Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued,
		CreatedAt: time.Now().UTC(), Version: 1,
	}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err = repository.DeleteBuild(ctx, succeeded.ID); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("referenced build deletion returned %v", err)
	}
	if err = repository.MarkDeleted(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if err = repository.DeleteBuild(ctx, succeeded.ID); err != nil {
		t.Fatalf("unreferenced terminal build deletion: %v", err)
	}
	if _, err = repository.Build(ctx, succeeded.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted build lookup returned %v", err)
	}
}
