package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/store"
)

func TestBuildAssignmentsSurviveRestartAndCompleteByDigest(t *testing.T) {
	path := t.TempDir() + "/jobdock.db"
	repository, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	owner := domain.User{ID: ids.New(), Username: "build-operator", Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
	if err = repository.CreateUser(ctx, owner, "hash"); err != nil {
		t.Fatal(err)
	}
	build := domain.Build{ID: ids.New(), OwnerID: owner.ID, Name: "restart safe build", Mode: domain.BuildModeRailpack, Status: domain.BuildCreated, Source: domain.BuildSource{Filename: "project.zip", Size: 12, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}, CreatedAt: time.Now().UTC(), Version: 1}
	if err = repository.CreateBuild(ctx, build); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.UpdateBuildStatus(ctx, build.ID, domain.BuildAnalyzing, "", "Analyzing"); err != nil {
		t.Fatal(err)
	}
	plan := domain.BuildPlan{BuildID: build.ID, Provider: "python", Plan: json.RawMessage(`{"steps":{}}`), Info: json.RawMessage(`{"success":true}`), CreatedAt: time.Now().UTC()}
	if err = repository.SaveBuildPlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	queued, err := repository.QueueBuild(ctx, build.ID, ids.New())
	if err != nil || queued.Build.Status != domain.BuildBuilding || queued.Plan == nil || queued.Plan.ConfirmedAt == nil {
		t.Fatalf("queued work=%#v error=%v", queued, err)
	}
	claimed, err := repository.NextBuildWork(ctx, "builder-one", time.Minute)
	if err != nil || claimed.Assignment.Status != domain.BuildAssignmentRunning || claimed.Assignment.BuilderID != "builder-one" {
		t.Fatalf("claimed work=%#v error=%v", claimed, err)
	}
	if err = repository.Close(); err != nil {
		t.Fatal(err)
	}
	repository, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	recovered, err := repository.NextBuildWork(ctx, "builder-one", time.Minute)
	if err != nil || recovered.Assignment.ID != claimed.Assignment.ID {
		t.Fatalf("recovered work=%#v error=%v", recovered, err)
	}
	digest := "sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	now := time.Now().UTC()
	if err = repository.SaveManagedArtifact(ctx, domain.ManagedArtifact{BuildID: build.ID, OwnerID: owner.ID, Digest: digest, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Size: 42, MediaType: domain.ManagedImageMediaType, RuntimeImage: "jobdock.local/managed/" + build.ID + ":artifact", CreatedAt: now, LastReferencedAt: now}); err != nil {
		t.Fatal(err)
	}
	completed, err := repository.CompleteBuildAssignment(ctx, recovered.Assignment.ID, "builder-one", domain.BuildAssignmentSucceeded, digest, "BuildKit completed")
	if err != nil || completed.Status != domain.BuildSucceeded || completed.OCIDigest != digest {
		t.Fatalf("completed build=%#v error=%v", completed, err)
	}
	if _, err = repository.NextBuildWork(ctx, "builder-two", time.Minute); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("completed assignment was reclaimed: %v", err)
	}
}

func TestDockerfileAssignmentCancellationIsPersisted(t *testing.T) {
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	ctx := context.Background()
	owner := domain.User{ID: ids.New(), Username: "dockerfile-owner", Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
	if err = repository.CreateUser(ctx, owner, "hash"); err != nil {
		t.Fatal(err)
	}
	build := domain.Build{ID: ids.New(), OwnerID: owner.ID, Name: "dockerfile build", Mode: domain.BuildModeDockerfile, Status: domain.BuildCreated, Source: domain.BuildSource{Filename: "source.tgz", Size: 8, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}, CreatedAt: time.Now().UTC(), Version: 1}
	if err = repository.CreateBuild(ctx, build); err != nil {
		t.Fatal(err)
	}
	queued, err := repository.QueueBuild(ctx, build.ID, ids.New())
	if err != nil || queued.Plan != nil {
		t.Fatalf("queue Dockerfile work=%#v error=%v", queued, err)
	}
	claimed, err := repository.NextBuildWork(ctx, "builder-two", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := repository.RequestBuildCancellation(ctx, build.ID)
	if err != nil || cancelled.Status != domain.BuildCancelled {
		t.Fatalf("cancelled build=%#v error=%v", cancelled, err)
	}
	heartbeat, err := repository.RenewBuildAssignment(ctx, claimed.Assignment.ID, "builder-two", time.Minute)
	if err != nil || !heartbeat.CancelRequested {
		t.Fatalf("cancel heartbeat=%#v error=%v", heartbeat, err)
	}
	completed, err := repository.CompleteBuildAssignment(ctx, claimed.Assignment.ID, "builder-two", domain.BuildAssignmentSucceeded, "sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd", "late success")
	if err != nil || completed.Status != domain.BuildCancelled || completed.OCIDigest != "" {
		t.Fatalf("cancellation did not win: %#v %v", completed, err)
	}
}
