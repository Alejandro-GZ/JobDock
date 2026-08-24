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
	declarations := []domain.ObservableSourceDeclaration{
		{Name: "loss", Type: "metric", Unit: "ratio", Tags: []string{"metric:loss"}, Phase: "train"},
		{Name: "future_accuracy", Type: "metric", Unit: "ratio", Tags: []string{"metric:accuracy"}, Phase: "validation"},
		{Name: "validation", Type: "matrix", Phase: "validation"},
	}
	trainOrder, validationOrder := 10, 20
	phases := []domain.ObservabilityPhaseDeclaration{{ID: "validation", Name: "Validation", Order: &validationOrder, Metadata: map[string]any{"dataset": "holdout"}}, {ID: "train", Name: "Training", Order: &trainOrder}}
	update, err := repository.ApplyObservabilityManifest(ctx, job.ID, attemptID, declarations, phases)
	if err != nil || len(update.SourcesAdded) != 3 || len(update.PhasesAdded) != 2 {
		t.Fatal(err)
	}
	update, err = repository.ApplyObservabilityManifest(ctx, job.ID, attemptID, declarations, phases)
	if err != nil || len(update.SourcesAdded) != 0 || len(update.PhasesAdded) != 0 {
		t.Fatalf("identical manifest must be idempotent: %#v %v", update, err)
	}
	storedPhases, err := repository.ObservabilityPhases(ctx, job.ID, attemptID)
	if err != nil || len(storedPhases) != 2 || storedPhases[0].ID != "train" || storedPhases[1].ID != "validation" || storedPhases[1].Metadata["dataset"] != "holdout" {
		t.Fatalf("ordered attempt phases: %#v %v", storedPhases, err)
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
	if _, err = repository.AppendMatrix(ctx, domain.MatrixObservation{JobID: job.ID, AttemptID: attemptID, Name: "validation", Values: nullableTestMatrix([][]float64{{1, 0}, {0, 1}})}); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.AppendMatrix(ctx, domain.MatrixObservation{JobID: job.ID, AttemptID: attemptID, Name: "validation", Values: nullableTestMatrix([][]float64{{2, 0}, {0, 2}})}); err != nil {
		t.Fatal(err)
	}
	descriptors, err := repository.ObservableDescriptors(ctx, job.ID, attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if len(descriptors) != 4 || descriptors[0].Name != "future_accuracy" || !descriptors[0].Declared || descriptors[0].Observed || descriptors[0].Phase != "validation" || descriptors[1].Name != "loss" || !descriptors[1].Declared || !descriptors[1].Observed || descriptors[1].Unit != "ratio" || descriptors[2].Type != "matrix" || descriptors[2].Subtype != "confusion_matrix" || !descriptors[2].Declared || !descriptors[2].Observed || descriptors[3].Type != "progress" || descriptors[3].Declared || !descriptors[3].Observed {
		t.Fatalf("observable descriptors: %#v", descriptors)
	}
	var metricSamples int
	if err = repository.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM job_metric_samples WHERE attempt_id=?`, attemptID).Scan(&metricSamples); err != nil || metricSamples != 1 {
		t.Fatalf("manifest created synthetic metric samples: count=%d err=%v", metricSamples, err)
	}
	if err = repository.DeclareObservableSources(ctx, job.ID, attemptID, []domain.ObservableSourceDeclaration{{Name: "loss", Type: "metric", Unit: "seconds"}}); !errors.Is(err, store.ErrObservableDeclarationConflict) {
		t.Fatalf("conflicting declaration: %v", err)
	}
	if err = repository.DeclareObservableSources(ctx, job.ID, attemptID, []domain.ObservableSourceDeclaration{{Name: "loss", Type: "matrix"}}); !errors.Is(err, store.ErrObservableDeclarationConflict) {
		t.Fatalf("incompatible source type change: %v", err)
	}
	changedOrder := 30
	if _, err = repository.ApplyObservabilityManifest(ctx, job.ID, attemptID, nil, []domain.ObservabilityPhaseDeclaration{{ID: "train", Name: "Training", Order: &changedOrder}}); !errors.Is(err, store.ErrObservableDeclarationConflict) {
		t.Fatalf("incompatible phase change: %v", err)
	}
	otherAttemptID := ids.New()
	if _, err = repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,2,?,?,?,?,?)`, otherAttemptID, job.ID, node.ID, ids.New(), "RUNNING", ids.New(), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	other, err := repository.ObservableDescriptors(ctx, job.ID, otherAttemptID)
	if err != nil || len(other) != 0 {
		t.Fatalf("descriptors leaked across attempts: %#v %v", other, err)
	}
	otherPhases, err := repository.ObservabilityPhases(ctx, job.ID, otherAttemptID)
	if err != nil || len(otherPhases) != 0 {
		t.Fatalf("phases leaked across attempts: %#v %v", otherPhases, err)
	}
}

func nullableTestMatrix(values [][]float64) [][]*float64 {
	result := make([][]*float64, len(values))
	for row := range values {
		result[row] = make([]*float64, len(values[row]))
		for column := range values[row] {
			value := values[row][column]
			result[row][column] = &value
		}
	}
	return result
}
