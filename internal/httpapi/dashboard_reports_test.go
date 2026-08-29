package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

func TestDashboardReportFilename(t *testing.T) {
	for _, test := range []struct {
		name    string
		attempt int
		want    string
	}{
		{"Training run", 2, "Training-run-attempt-2-report.html"},
		{"../../unsafe <job>", 1, "unsafe-job-attempt-1-report.html"},
		{"  ", 4, "job-attempt-4-report.html"},
	} {
		if got := dashboardReportFilename(test.name, test.attempt); got != test.want {
			t.Errorf("dashboardReportFilename(%q) = %q, want %q", test.name, got, test.want)
		}
	}
}

func TestRenderDashboardReportIsSelfContainedAndEscapesJobData(t *testing.T) {
	malicious := `</script><script>globalThis.compromised=true</script>`
	manifest := dashboardReportManifest{
		SchemaVersion:  dashboardReportSchemaVersion,
		JobDockVersion: "v0.1.0",
		GeneratedAt:    time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC),
		Theme:          "dark",
		Job:            dashboardReportJob{ID: "job-1", Name: malicious},
		Attempt:        dashboardReportAttempt{ID: "attempt-1", AttemptNumber: 1},
		Dashboards:     []dashboardReportDashboard{{ID: "dashboard-1", Name: malicious, SchemaVersion: 1, Config: dashboardConfig{Widgets: []dashboardWidget{}}}},
		Sources:        dashboardReportSources{Metrics: []domain.MetricSeries{}, Resources: []domain.ResourceSample{}, Matrices: map[string]domain.MatrixObservation{}, Distributions: map[string][]distributionView{}, Tables: map[string]domain.TablePage{}, Logs: map[string]string{"stdout": malicious}, Checkpoints: []domain.CheckpointSync{}},
		Warnings:       []dashboardReportWarning{},
	}
	document, err := renderDashboardReport(manifest, `document.body.dataset.ready="true"`, `body{color:red}`)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(document, []byte(malicious)) {
		t.Fatal("untrusted job data was embedded as executable markup")
	}
	text := string(document)
	for _, expected := range []string{"connect-src &#39;none&#39;", "form-action &#39;none&#39;", "object-src &#39;none&#39;", "jobdock-report-data", "body{color:red}"} {
		if !strings.Contains(text, expected) {
			t.Errorf("report missing %q", expected)
		}
	}
	if !strings.Contains(text, `<html lang="en" class="dark">`) || !strings.Contains(text, `name="jobdock-attempt-id" content="attempt-1"`) {
		t.Fatal("report omitted the captured theme or trace metadata")
	}
	match := regexp.MustCompile(`<script id="jobdock-report-data" type="application/json">([^<]+)</script>`).FindStringSubmatch(text)
	if len(match) != 2 {
		t.Fatal("encoded report manifest not found")
	}
	payload, err := base64.StdEncoding.DecodeString(match[1])
	if err != nil {
		t.Fatal(err)
	}
	var decoded dashboardReportManifest
	if err = json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Job.Name != malicious || decoded.Sources.Logs["stdout"] != malicious {
		t.Fatal("encoded report manifest did not preserve source data")
	}
}

func TestCollectReportSourcesDeduplicatesWidgetQueries(t *testing.T) {
	set := reportSourceSet{metrics: map[string]bool{}, matrices: map[string]bool{}, distributions: map[string]bool{}, tables: map[string]bool{}, logs: map[string]bool{}}
	collectReportSources([]dashboardWidget{
		{Sources: []dashboardWidgetSource{{Kind: "metric", Name: "loss"}, {Kind: "resource", Name: "cpu"}, {Kind: "log", Name: "stdout"}}},
		{Sources: []dashboardWidgetSource{{Kind: "metric", Name: "loss"}, {Kind: "matrix", Name: "confusion"}, {Kind: "table", Name: "predictions"}, {Kind: "progress", Name: "progress"}, {Kind: "checkpoint", Name: "checkpoints"}}},
	}, &set)
	if len(set.metrics) != 1 || !set.metrics["loss"] || !set.resources || !set.logs["stdout"] || !set.matrices["confusion"] || !set.tables["predictions"] || !set.progress || !set.checkpoints {
		t.Fatalf("unexpected collected sources: %#v", set)
	}
}

func TestSanitizeReportRecordRedactsCredentialFields(t *testing.T) {
	result := sanitizeReportRecord(map[string]any{"score": .9, "api_key": "plain", "nested": map[string]any{"accessToken": "plain", "label": "safe"}})
	if result["api_key"] != "[REDACTED]" || result["nested"].(map[string]any)["accessToken"] != "[REDACTED]" || result["nested"].(map[string]any)["label"] != "safe" {
		t.Fatalf("unexpected sanitized record: %#v", result)
	}
}

func TestDashboardReportOrderedLogsPreservesCombinedStreamOrder(t *testing.T) {
	files, err := filestore.New(t.TempDir(), 1<<20, 1<<20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	jobID, attemptID := ids.New(), ids.New()
	stdout, stderr := "one\nthree\n", "two\n"
	if _, err = files.AppendAttemptLog(jobID, attemptID, "stdout", 0, strings.NewReader(stdout)); err != nil {
		t.Fatal(err)
	}
	if _, err = files.AppendAttemptLog(jobID, attemptID, "stderr", 0, strings.NewReader(stderr)); err != nil {
		t.Fatal(err)
	}
	orders := []combinedLogOrder{{Sequence: 1, Stream: "stdout", StartOffset: 0, NextOffset: 4}, {Sequence: 2, Stream: "stderr", StartOffset: 0, NextOffset: 4}, {Sequence: 3, Stream: "stdout", StartOffset: 4, NextOffset: int64(len(stdout))}}
	var encoded bytes.Buffer
	for _, order := range orders {
		line, _ := json.Marshal(order)
		encoded.Write(line)
		encoded.WriteByte('\n')
	}
	if _, err = files.AppendAttemptLog(jobID, attemptID, ".order", 0, &encoded); err != nil {
		t.Fatal(err)
	}
	api := &API{files: files}
	fragments := api.dashboardReportOrderedLogs(jobID, attemptID, map[string]bool{"stdout": true, "stderr": true}, map[string]string{"stdout": stdout, "stderr": stderr}, map[string]int64{"stdout": 0, "stderr": 0})
	if len(fragments) != 3 || fragments[0].Stream != "stdout" || fragments[0].Text != "one\n" || fragments[1].Stream != "stderr" || fragments[1].Text != "two\n" || fragments[2].Stream != "stdout" || fragments[2].Text != "three\n" {
		t.Fatalf("unexpected ordered log fragments: %#v", fragments)
	}
}

func TestDashboardReportEndpointGeneratesSelectedOfflineDashboard(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	repository, err := store.Open(filepath.Join(root, "jobdock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	owner := createSeriesUser(t, repository, "report-owner")
	now := time.Now().UTC().Add(-time.Minute)
	job := domain.Job{ID: ids.New(), OwnerID: owner.ID, Spec: domain.JobSpec{Name: "Offline report", Image: "alpine", Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1024}}, Status: domain.JobRunning, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobRunning, CreatedAt: now}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	node := domain.Node{ID: ids.New(), Name: "node", Status: domain.NodeOnline, Labels: map[string]string{}, CreatedAt: now, LastHeartbeat: now}
	if err = repository.UpsertNode(ctx, node, "credential"); err != nil {
		t.Fatal(err)
	}
	attemptID := ids.New()
	if _, err = repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,?,?,?,?,?,?)`, attemptID, job.ID, 1, node.ID, ids.New(), domain.JobRunning, "job-token-hash", now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.DB().ExecContext(ctx, `UPDATE jobs SET attempt_id=?,assigned_node_id=? WHERE id=?`, attemptID, node.ID, job.ID); err != nil {
		t.Fatal(err)
	}
	configJSON, _ := json.Marshal(dashboardConfig{Widgets: []dashboardWidget{{ID: "loss", Type: "lineplot", Size: dashboardWidgetSize{Columns: 6, Rows: 4}, Position: dashboardWidgetPosition{}, Sources: []dashboardWidgetSource{{Kind: "metric", Name: "loss"}}}}})
	dashboard := store.DashboardPreference{ID: ids.New(), UserID: owner.ID, JobID: job.ID, Name: "Training", SchemaVersion: 1, ConfigJSON: configJSON, CreatedAt: now, UpdatedAt: now, IsDefault: true}
	if err = repository.CreateDashboard(ctx, dashboard, true); err != nil {
		t.Fatal(err)
	}
	if err = repository.AppendMetricSamples(ctx, []domain.MetricSample{{JobID: job.ID, AttemptID: attemptID, Name: "loss", Value: .42, CapturedAt: now.Add(10 * time.Second), Unit: "ratio"}}); err != nil {
		t.Fatal(err)
	}
	files, _ := filestore.New(root, 1<<20, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{3}, 32))
	webDir := filepath.Join(root, "web")
	if err = os.MkdirAll(filepath.Join(webDir, "report"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(webDir, "report", "report.js"), []byte(`document.body.dataset.report="ready"`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(webDir, "report", "report.css"), []byte(`body{background:#000}`), 0o600); err != nil {
		t.Fatal(err)
	}
	api := New(config.Server{AllowInsecureHTTP: true, SessionTTL: time.Hour}, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil))).SetVersion("v-test")
	api.webDir = webDir
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	client := loginSeriesUser(t, server.URL, owner.Username)
	meResponse, err := client.Get(server.URL + "/api/v1/auth/me")
	if err != nil {
		t.Fatal(err)
	}
	var me struct {
		CSRF string `json:"csrf_token"`
	}
	if err = json.NewDecoder(meResponse.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	meResponse.Body.Close()
	body, _ := json.Marshal(dashboardReportRequest{AttemptID: attemptID, DashboardIDs: []string{dashboard.ID}})
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/"+job.ID+"/dashboard-reports", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", me.CSRF)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	document, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("report status %d: %s", response.StatusCode, document)
	}
	if response.Header.Get("Content-Type") != "text/html; charset=utf-8" || !strings.Contains(response.Header.Get("Content-Disposition"), "Offline-report-attempt-1-report.html") || !bytes.Contains(document, []byte("jobdock-report-data")) {
		t.Fatalf("unexpected report response: headers=%v body=%s", response.Header, document)
	}
}
