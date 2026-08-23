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
	"strings"
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
	payload := `{"schema_version":1,"widgets":[{"id":"loss","type":"lineplot","title":"Training loss","size":{"columns":6,"rows":3},"position":{"x":0,"y":0},"sources":[{"kind":"metric","name":"loss"}],"x_axis":"step","time_range":"6h","grid_columns":12,"appearance":{"schema_version":1,"color_scheme":"cool","legend":"open","line_style":"dashed","show_points":true,"future_property":"ignored"}}]}`
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
	if saved.SchemaVersion != 1 || len(saved.Widgets) != 1 || saved.Widgets[0].Sources[0].Name != "loss" || saved.Widgets[0].Appearance == nil || saved.Widgets[0].Appearance.ColorScheme != "cool" || saved.Widgets[0].Appearance.LineStyle != "dashed" {
		t.Fatalf("saved dashboard: %#v", saved)
	}
	materializedPayload := `{"schema_version":1,"widgets":[{"id":"loss","type":"lineplot","size":{"columns":6,"rows":3},"position":{"x":0,"y":0},"sources":[{"kind":"metric","name":"loss"}]}],"materialized_from":{"template_id":"training-general","template_version":1,"schema_version":1}}`
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard", bytes.NewBufferString(materializedPayload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", session.CSRF)
	result, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	result.Body.Close()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("materialize dashboard status: %d", result.StatusCode)
	}
	var versioned struct {
		Compatibility    string                            `json:"compatibility"`
		MaterializedFrom *dashboardTemplateMaterialization `json:"materialized_from"`
	}
	getSeriesJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard", &versioned)
	if versioned.Compatibility != "compatible" || versioned.MaterializedFrom == nil || versioned.MaterializedFrom.TemplateID != "training-general" || versioned.MaterializedFrom.TemplateVersion != 1 || versioned.MaterializedFrom.AppliedAt.IsZero() {
		t.Fatalf("dashboard provenance: %#v", versioned)
	}
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard", bytes.NewBufferString(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", session.CSRF)
	result, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	result.Body.Close()
	getSeriesJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard", &versioned)
	if versioned.MaterializedFrom == nil || versioned.MaterializedFrom.TemplateID != "training-general" {
		t.Fatalf("ordinary edit discarded provenance: %#v", versioned)
	}
	detachedPayload := `{"schema_version":1,"widgets":[{"id":"loss","type":"lineplot","size":{"columns":6,"rows":3},"position":{"x":0,"y":0},"sources":[{"kind":"metric","name":"loss"}]}],"materialized_from":null}`
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard", bytes.NewBufferString(detachedPayload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", session.CSRF)
	result, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	result.Body.Close()
	getSeriesJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard", &versioned)
	if versioned.MaterializedFrom != nil {
		t.Fatalf("explicit provenance detach failed: %#v", versioned)
	}
	partialConfig := `{"widgets":[{"id":"loss","type":"lineplot","future_property":true,"size":{"columns":6,"rows":3},"position":{"x":0,"y":0},"sources":[{"kind":"metric","name":"loss"}]},{"id":"future","type":"starplot","size":{"columns":6,"rows":3},"position":{"x":6,"y":0},"sources":[]}]}`
	if _, err = repository.DB().ExecContext(ctx, `UPDATE job_dashboards SET config_json=? WHERE user_id=? AND job_id=?`, partialConfig, owner.ID, job.ID); err != nil {
		t.Fatal(err)
	}
	var partial struct {
		Widgets       []dashboardWidget `json:"widgets"`
		Compatibility string            `json:"compatibility"`
		Reason        string            `json:"fallback_reason"`
	}
	getSeriesJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard", &partial)
	if len(partial.Widgets) != 1 || partial.Widgets[0].ID != "loss" || partial.Compatibility != "partially_compatible" || partial.Reason != "unsupported_widgets_omitted" {
		t.Fatalf("partially restored dashboard: %#v", partial)
	}
	futureAppearance := `{"widgets":[{"id":"loss","type":"lineplot","size":{"columns":6,"rows":3},"position":{"x":0,"y":0},"sources":[{"kind":"metric","name":"loss"}],"appearance":{"schema_version":2,"future_style":true}}]}`
	if _, err = repository.DB().ExecContext(ctx, `UPDATE job_dashboards SET config_json=? WHERE user_id=? AND job_id=?`, futureAppearance, owner.ID, job.ID); err != nil {
		t.Fatal(err)
	}
	getSeriesJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard", &partial)
	if len(partial.Widgets) != 1 || partial.Widgets[0].Appearance != nil || partial.Compatibility != "partially_compatible" || partial.Reason != "unsupported_widget_appearance_omitted" {
		t.Fatalf("future appearance fallback: %#v", partial)
	}
	if _, err = repository.DB().ExecContext(ctx, `UPDATE job_dashboards SET schema_version=99 WHERE user_id=? AND job_id=?`, owner.ID, job.ID); err != nil {
		t.Fatal(err)
	}
	var fallback struct {
		Widgets       json.RawMessage `json:"widgets"`
		Compatibility string          `json:"compatibility"`
		Reason        string          `json:"fallback_reason"`
	}
	getSeriesJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard", &fallback)
	if string(fallback.Widgets) != "null" || fallback.Compatibility != "incompatible" || fallback.Reason != "unsupported_schema_version" {
		t.Fatalf("fallback: %#v", fallback)
	}
	audit, err := repository.ListAudit(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	foundApply := false
	for _, event := range audit {
		foundApply = foundApply || event.Action == "dashboard.template.apply" && event.Metadata["template_id"] == "training-general"
	}
	if !foundApply {
		t.Fatalf("dashboard template application was not audited: %#v", audit)
	}
}

func TestMultipleDashboardLifecycleIsIndependentAndDeterministic(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	owner := createSeriesUser(t, repository, "multiple-dashboard-owner")
	job := domain.Job{ID: ids.New(), OwnerID: owner.ID, Spec: domain.JobSpec{Name: "multi-dashboard", Image: "alpine", Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1024}}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: time.Now().UTC()}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	files, _ := filestore.New(root, 1<<20, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{8}, 32))
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
		Items []struct {
			ID, Name  string
			IsDefault bool `json:"is_default"`
		} `json:"items"`
		ActiveID  string `json:"active_dashboard_id"`
		DefaultID string `json:"default_dashboard_id"`
	}
	getSeriesJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboards", &initial)
	if len(initial.Items) != 1 || initial.ActiveID == "" || initial.ActiveID != initial.DefaultID || !initial.Items[0].IsDefault {
		t.Fatalf("initial dashboard set: %#v", initial)
	}
	firstID := initial.ActiveID
	putDashboardJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboards/"+firstID, session.CSRF, `{"schema_version":1,"widgets":[{"id":"loss","type":"lineplot","size":{"columns":6,"rows":3},"position":{"x":0,"y":0},"sources":[{"kind":"metric","name":"loss"}]}]}`, http.StatusOK)

	created := putDashboardJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboards", session.CSRF, `{"name":"Validation","source_dashboard_id":"`+firstID+`"}`, http.StatusCreated)
	var duplicate struct {
		ID, Name string
		Widgets  []dashboardWidget `json:"widgets"`
	}
	if err = json.Unmarshal(created, &duplicate); err != nil {
		t.Fatal(err)
	}
	if duplicate.ID == firstID || duplicate.Name != "Validation" || len(duplicate.Widgets) != 1 {
		t.Fatalf("duplicated dashboard: %#v", duplicate)
	}
	putDashboardJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboards/"+duplicate.ID, session.CSRF, `{"name":"Evaluation","is_default":true,"active":true}`, http.StatusOK)
	putDashboardJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboards/"+duplicate.ID, session.CSRF, `{"schema_version":1,"widgets":[]}`, http.StatusOK)
	var original struct {
		Widgets []dashboardWidget `json:"widgets"`
	}
	getSeriesJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboards/"+firstID, &original)
	if len(original.Widgets) != 1 {
		t.Fatalf("dashboard configurations were mixed: %#v", original)
	}

	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/jobs/"+job.ID+"/dashboards/"+duplicate.ID, nil)
	request.Header.Set("X-CSRF-Token", session.CSRF)
	result, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	result.Body.Close()
	if result.StatusCode != http.StatusNoContent {
		t.Fatalf("delete dashboard status: %d", result.StatusCode)
	}
	getSeriesJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboards", &initial)
	if len(initial.Items) != 1 || initial.ActiveID != firstID || initial.DefaultID != firstID {
		t.Fatalf("deterministic fallback: %#v", initial)
	}
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/v1/jobs/"+job.ID+"/dashboards/"+firstID, nil)
	request.Header.Set("X-CSRF-Token", session.CSRF)
	result, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	result.Body.Close()
	if result.StatusCode != http.StatusConflict {
		t.Fatalf("last dashboard delete status: %d", result.StatusCode)
	}
}

func putDashboardJSON(t *testing.T, client *http.Client, url, csrf, payload string, wantStatus int) []byte {
	t.Helper()
	request, _ := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(payload))
	if strings.Contains(url, "/dashboards/") {
		request.Method = http.MethodPut
		if strings.HasPrefix(payload, `{"name"`) {
			request.Method = http.MethodPatch
		}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s status=%d body=%s", request.Method, url, response.StatusCode, body)
	}
	return body
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
	var templateCatalog struct {
		Items []dashboardTemplate `json:"items"`
	}
	getSeriesJSON(t, client, server.URL+"/api/v1/dashboard/templates", &templateCatalog)
	if len(templateCatalog.Items) < 45 || templateCatalog.Items[0].ID != "training-general" || templateCatalog.Items[0].Category != "general" {
		t.Fatalf("official dashboard template catalog: %#v", templateCatalog)
	}
	var semanticCatalog observabilityCatalog
	getSeriesJSON(t, client, server.URL+"/api/v1/observability/catalog", &semanticCatalog)
	if semanticCatalog.Version != 1 || len(semanticCatalog.Phases) != 30 {
		t.Fatalf("observability catalog: %#v", semanticCatalog)
	}
	var matches struct {
		AttemptID string                   `json:"attempt_id"`
		Items     []dashboardTemplateMatch `json:"items"`
	}
	getSeriesJSON(t, client, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard/templates/matches?attempt_id="+attemptID, &matches)
	if matches.AttemptID != attemptID || len(matches.Items) != len(templateCatalog.Items) || matches.Items[0].TemplateID != "training-general" || !matches.Items[0].Applicable {
		t.Fatalf("template matches: %#v", matches)
	}
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
	ambiguous := semanticTemplate(templateSlot("loss", []string{"metric:loss"}, 1, 1))
	payload, _ = json.Marshal(map[string]any{
		"attempt_id": attemptID,
		"template":   ambiguous,
		"overrides": []dashboardTemplateOverride{{
			WidgetID: "loss",
			SlotID:   "loss",
			Sources:  []dashboardWidgetSource{{Kind: "metric", Name: "custom_validation_objective"}},
		}},
	})
	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard/templates/resolve", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	overriddenResponse, requestErr := client.Do(request)
	if requestErr != nil {
		t.Fatal(requestErr)
	}
	defer overriddenResponse.Body.Close()
	if overriddenResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(overriddenResponse.Body)
		t.Fatalf("manual template resolution status=%d body=%s", overriddenResponse.StatusCode, body)
	}
	if err = json.NewDecoder(overriddenResponse.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Widgets) != 1 || len(result.Widgets[0].Sources) != 1 || result.Widgets[0].Sources[0].Name != "custom_validation_objective" {
		t.Fatalf("manual template resolution: %#v", result)
	}
	payload, _ = json.Marshal(map[string]any{
		"attempt_id": attemptID,
		"template":   ambiguous,
		"overrides": []dashboardTemplateOverride{{
			WidgetID: "loss",
			SlotID:   "loss",
			Sources:  []dashboardWidgetSource{{Kind: "metric", Name: "not-a-candidate"}},
		}},
	})
	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard/templates/resolve", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	invalidResponse, requestErr := client.Do(request)
	if requestErr != nil {
		t.Fatal(requestErr)
	}
	defer invalidResponse.Body.Close()
	if invalidResponse.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(invalidResponse.Body)
		t.Fatalf("invalid manual template resolution status=%d body=%s", invalidResponse.StatusCode, body)
	}
	future := ambiguous
	future.SchemaVersion = 99
	payload, _ = json.Marshal(map[string]any{"attempt_id": attemptID, "template": future})
	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard/templates/resolve", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	fallbackResponse, requestErr := client.Do(request)
	if requestErr != nil {
		t.Fatal(requestErr)
	}
	defer fallbackResponse.Body.Close()
	if fallbackResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(fallbackResponse.Body)
		t.Fatalf("future template fallback status=%d body=%s", fallbackResponse.StatusCode, body)
	}
	if err = json.NewDecoder(fallbackResponse.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Compatibility != "incompatible" || result.FallbackReason != "unsupported_schema_version" || result.AttemptID != attemptID || len(result.Widgets) != 0 {
		t.Fatalf("future template fallback: %#v", result)
	}
	items, err := repository.MetricDescriptors(ctx, job.ID, attemptID, nil)
	if err != nil || len(items) != 2 {
		t.Fatalf("template resolution changed telemetry descriptors: %#v %v", items, err)
	}
}
