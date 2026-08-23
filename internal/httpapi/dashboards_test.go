package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/auth"
	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

func TestDashboardConfigurationPersistsAndFallsBackSafely(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	owner := createSeriesUser(t, repository, "dashboard-owner")
	job := domain.Job{ID: ids.New(), OwnerID: owner.ID, Spec: domain.JobSpec{Name: "dashboard", Image: "alpine", Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1024}}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: time.Now().UTC()}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	files, _ := filestore.New(root, 1<<20, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{7}, 32))
	server := httptest.NewServer(New(config.Server{AllowInsecureHTTP: true, SessionTTL: time.Hour}, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	login, _ := json.Marshal(map[string]string{"username": owner.Username, "password": "correct series password"})
	response, err := client.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(login))
	if err != nil {
		t.Fatal(err)
	}
	var session struct {
		CSRF string `json:"csrf_token"`
	}
	if err = json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	var initial struct {
		SchemaVersion int             `json:"schema_version"`
		Widgets       json.RawMessage `json:"widgets"`
	}
	getSeriesJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard", &initial)
	if initial.SchemaVersion != 1 || string(initial.Widgets) != "null" {
		t.Fatalf("initial dashboard: %#v", initial)
	}
	payload := `{"schema_version":1,"widgets":[{"id":"loss","type":"lineplot","title":"Training loss","size":{"columns":6,"rows":3},"position":{"x":0,"y":0},"sources":[{"kind":"metric","name":"loss"}],"x_axis":"step","time_range":"6h","grid_columns":12}]}`
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard", bytes.NewBufferString(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", session.CSRF)
	result, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	result.Body.Close()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("save dashboard status: %d", result.StatusCode)
	}
	var saved struct {
		SchemaVersion int               `json:"schema_version"`
		Widgets       []dashboardWidget `json:"widgets"`
	}
	getSeriesJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard", &saved)
	if saved.SchemaVersion != 1 || len(saved.Widgets) != 1 || saved.Widgets[0].Sources[0].Name != "loss" {
		t.Fatalf("saved dashboard: %#v", saved)
	}
	if _, err = repository.DB().ExecContext(ctx, `UPDATE dashboard_preferences SET schema_version=99 WHERE user_id=? AND job_id=?`, owner.ID, job.ID); err != nil {
		t.Fatal(err)
	}
	var fallback struct {
		Widgets json.RawMessage `json:"widgets"`
		Reason  string          `json:"fallback_reason"`
	}
	getSeriesJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard", &fallback)
	if string(fallback.Widgets) != "null" || fallback.Reason != "unsupported_schema_version" {
		t.Fatalf("fallback: %#v", fallback)
	}
}

func TestDashboardValidationRejectsUnsupportedWidgets(t *testing.T) {
	err := validateDashboardConfig(dashboardConfig{Widgets: []dashboardWidget{{ID: "one", Type: "unknown", Size: dashboardWidgetSize{Columns: 1, Rows: 1}}}})
	if err == nil {
		t.Fatal("unsupported widget was accepted")
	}
}

func TestDashboardValidationRequiresFixedGaugeMaximum(t *testing.T) {
	config := dashboardConfig{Widgets: []dashboardWidget{{ID: "gauge", Type: "gauge", Size: dashboardWidgetSize{Columns: 3, Rows: 3}, Sources: []dashboardWidgetSource{{Kind: "metric", Name: "loss"}}, GaugeMaxMode: "fixed"}}}
	if err := validateDashboardConfig(config); err == nil {
		t.Fatal("fixed gauge without a maximum was accepted")
	}
	maximum := 100.0
	config.Widgets[0].GaugeMaxValue = &maximum
	if err := validateDashboardConfig(config); err != nil {
		t.Fatalf("valid fixed gauge was rejected: %v", err)
	}
}

func TestDashboardTemplateResolutionUsesAttemptDescriptorCatalog(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	owner := createSeriesUser(t, repository, "template-owner")
	job := domain.Job{ID: ids.New(), OwnerID: owner.ID, Spec: domain.JobSpec{Name: "template", Image: "alpine", Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1024}}, Status: domain.JobRunning, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobRunning, CreatedAt: time.Now().UTC()}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	node := domain.Node{ID: ids.New(), Name: "template-node", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 1000, MemoryTotalBytes: 1 << 30, WorkspaceFreeBytes: 10 << 30, Labels: map[string]string{}, LastHeartbeat: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	if err = repository.UpsertNode(ctx, node, auth.TokenHash("template-node-token")); err != nil {
		t.Fatal(err)
	}
	attemptID := ids.New()
	if _, err = repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,1,?,?,?,?,?)`, attemptID, job.ID, node.ID, ids.New(), "RUNNING", auth.TokenHash("template-token"), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.DB().ExecContext(ctx, `UPDATE jobs SET attempt_id=? WHERE id=?`, attemptID, job.ID); err != nil {
		t.Fatal(err)
	}
	if err = repository.AppendMetricSamples(ctx, []domain.MetricSample{
		{JobID: job.ID, AttemptID: attemptID, Name: "custom_training_objective", Value: .5, CapturedAt: time.Now().UTC(), Tags: []string{"metric:loss", "phase:train"}},
		{JobID: job.ID, AttemptID: attemptID, Name: "custom_validation_objective", Value: .4, CapturedAt: time.Now().UTC(), Tags: []string{"metric:loss", "phase:validation"}},
	}); err != nil {
		t.Fatal(err)
	}
	otherAttemptID := ids.New()
	if _, err = repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,2,?,?,?,?,?)`, otherAttemptID, job.ID, node.ID, ids.New(), "SUCCEEDED", auth.TokenHash("other-template-token"), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err = repository.AppendMetricSamples(ctx, []domain.MetricSample{{JobID: job.ID, AttemptID: otherAttemptID, Name: "other_attempt_loss", Value: .1, CapturedAt: time.Now().UTC(), Tags: []string{"metric:loss", "phase:train"}}}); err != nil {
		t.Fatal(err)
	}
	files, _ := filestore.New(root, 1<<20, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{9}, 32))
	server := httptest.NewServer(New(config.Server{AllowInsecureHTTP: true, SessionTTL: time.Hour}, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer server.Close()
	client := loginSeriesUser(t, server.URL, owner.Username)
	template := semanticTemplate(
		templateSlot("train", []string{"metric:loss", "phase:train"}, 1, 1),
		templateSlot("validation", []string{"metric:loss", "phase:validation"}, 1, 1),
	)
	payload, _ := json.Marshal(map[string]any{"attempt_id": attemptID, "template": template})
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard/templates/resolve", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("template resolution status=%d body=%s", response.StatusCode, body)
	}
	var result dashboardTemplateResolution
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.AttemptID != attemptID || len(result.Widgets) != 1 || len(result.Widgets[0].Sources) != 2 || result.Widgets[0].Sources[0].Name != "custom_training_objective" || result.Widgets[0].Sources[1].Name != "custom_validation_objective" {
		t.Fatalf("resolved template: %#v", result)
	}
	items, err := repository.MetricDescriptors(ctx, job.ID, attemptID, nil)
	if err != nil || len(items) != 2 {
		t.Fatalf("template resolution changed telemetry descriptors: %#v %v", items, err)
	}
}
