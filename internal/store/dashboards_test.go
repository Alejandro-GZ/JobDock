package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/store"
)

func TestDashboardPreferenceIsVersionedAndIsolatedByUserAndJob(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir() + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	now := time.Now().UTC()
	first := domain.User{ID: ids.New(), Username: "first", Role: domain.RoleMember, CreatedAt: now}
	second := domain.User{ID: ids.New(), Username: "second", Role: domain.RoleMember, CreatedAt: now}
	for _, user := range []domain.User{first, second} {
		if err = repository.CreateUser(ctx, user, "hash"); err != nil {
			t.Fatal(err)
		}
	}
	job := domain.Job{ID: ids.New(), OwnerID: first.ID, Spec: domain.JobSpec{Name: "job", Image: "alpine", Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1024}}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: now}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	preference := store.DashboardPreference{UserID: first.ID, JobID: job.ID, SchemaVersion: 1, ConfigJSON: []byte(`{"widgets":[]}`), UpdatedAt: now}
	if err = repository.PutDashboardPreference(ctx, preference); err != nil {
		t.Fatal(err)
	}
	loaded, err := repository.DashboardPreference(ctx, first.ID, job.ID)
	if err != nil || loaded.SchemaVersion != 1 || string(loaded.ConfigJSON) != `{"widgets":[]}` {
		t.Fatalf("loaded preference: %#v %v", loaded, err)
	}
	if _, err = repository.DashboardPreference(ctx, second.ID, job.ID); err != store.ErrNotFound {
		t.Fatalf("preference leaked across users: %v", err)
	}
	preference.SchemaVersion = 2
	preference.ConfigJSON = []byte(`{"widgets":[{"id":"one"}]}`)
	preference.UpdatedAt = now.Add(time.Second)
	if err = repository.PutDashboardPreference(ctx, preference); err != nil {
		t.Fatal(err)
	}
	loaded, err = repository.DashboardPreference(ctx, first.ID, job.ID)
	if err != nil || loaded.SchemaVersion != 2 {
		t.Fatalf("updated preference: %#v %v", loaded, err)
	}
}
