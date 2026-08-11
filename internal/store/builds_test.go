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

func TestBuildLifecyclePersistsSourceResultAndFailureHistory(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	owner := domain.User{ID: ids.New(), Username: "builder", Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
	if err = repository.CreateUser(ctx, owner, "hash"); err != nil {
		t.Fatal(err)
	}
	newBuild := func(name string) domain.Build {
		return domain.Build{ID: ids.New(), OwnerID: owner.ID, Name: name, Mode: domain.BuildModeRailpack, Status: domain.BuildCreated, Source: domain.BuildSource{Filename: "project.tar.gz", Size: 16, SHA256: "fc17afe4af56fca9d2943b7901e7517611b37a36db7a7775b3e341e7d20a6ba0"}, CreatedAt: time.Now().UTC(), Version: 1}
	}
	success := newBuild("successful build")
	if err = repository.CreateBuild(ctx, success); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.UpdateBuildStatus(ctx, success.ID, domain.BuildSucceeded, "sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd", ""); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("skipped lifecycle returned %v", err)
	}
	if _, err = repository.UpdateBuildStatus(ctx, success.ID, domain.BuildAnalyzing, "", "Source accepted"); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.UpdateBuildStatus(ctx, success.ID, domain.BuildBuilding, "", "Build started"); err != nil {
		t.Fatal(err)
	}
	completed, err := repository.UpdateBuildStatus(ctx, success.ID, domain.BuildSucceeded, "sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd", "Build completed")
	if err != nil || completed.OCIDigest == "" || completed.StartedAt == nil || completed.FinishedAt == nil || completed.Version != 4 {
		t.Fatalf("completed build: %#v %v", completed, err)
	}
	events, err := repository.BuildEvents(ctx, success.ID)
	if err != nil || len(events) != 4 || events[0].Status != domain.BuildCreated || events[3].Status != domain.BuildSucceeded {
		t.Fatalf("build events: %#v %v", events, err)
	}

	failed := newBuild("failed analysis")
	if err = repository.CreateBuild(ctx, failed); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.UpdateBuildStatus(ctx, failed.ID, domain.BuildAnalyzing, "", "Analyzing"); err != nil {
		t.Fatal(err)
	}
	failed, err = repository.UpdateBuildStatus(ctx, failed.ID, domain.BuildFailed, "", "Railpack could not detect a supported project")
	if err != nil || failed.FailureReason == "" || failed.OCIDigest != "" {
		t.Fatalf("failed build: %#v %v", failed, err)
	}
	jobs, err := repository.ListJobs(ctx, false)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("build failure created an invalid job: %#v %v", jobs, err)
	}
}
