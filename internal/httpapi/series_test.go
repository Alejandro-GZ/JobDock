package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
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

func TestAuthorizedMetricAndResourceSeriesJSONAndCSV(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	owner := createSeriesUser(t, repository, "series-owner")
	other := createSeriesUser(t, repository, "series-other")
	node := domain.Node{ID: ids.New(), Name: "series-node", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 4000, MemoryTotalBytes: 8 << 30, WorkspaceFreeBytes: 10 << 30, Labels: map[string]string{}, LastHeartbeat: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	if err = repository.UpsertNode(ctx, node, auth.TokenHash("node-token")); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	job := domain.Job{ID: ids.New(), OwnerID: owner.ID, Spec: domain.JobSpec{Name: "series job", Image: "alpine:3", Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1024}}, Status: domain.JobRunning, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobRunning, CreatedAt: base}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	attemptID, jobToken := ids.New(), "series-job-token"
	if _, err = repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,1,?,?,?,?,?)`, attemptID, job.ID, node.ID, ids.New(), "RUNNING", auth.TokenHash(jobToken), base.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.DB().ExecContext(ctx, `UPDATE jobs SET assigned_node_id=?,attempt_id=? WHERE id=?`, node.ID, attemptID, job.ID); err != nil {
		t.Fatal(err)
	}
	gpuUtilization, gpuMemory := int64(4200), int64(1<<30)
	if err = repository.AppendResourceSample(ctx, domain.ResourceSample{JobID: job.ID, AttemptID: attemptID, CapturedAt: base.Add(10 * time.Second), CPUMillis: 1250, MemoryBytes: 512 << 20, GPUUtilizationBasisPoints: &gpuUtilization, GPUMemoryBytes: &gpuMemory}); err != nil {
		t.Fatal(err)
	}
	files, _ := filestore.New(root, 1<<20, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{8}, 32))
	api := New(config.Server{AllowInsecureHTTP: true, SessionTTL: time.Hour}, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(api.Handler())
	defer server.Close()

	manifestPayload := `{"version":1,"sources":[{"name":"loss","type":"metric","unit":"ratio","tags":["metric:loss","phase:train"],"metadata":{"split":"train"},"phase":"train"},{"name":"future_accuracy","type":"metric","unit":"ratio","tags":["metric:accuracy"],"phase":"validation"},{"name":"validation/confusion","type":"matrix","milestone":"validated"}]}`
	manifestRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/job-context/observability/manifest", strings.NewReader(manifestPayload))
	manifestRequest.Header.Set("Content-Type", "application/json")
	manifestRequest.Header.Set("Authorization", "Bearer "+jobToken)
	manifestResponse, err := http.DefaultClient.Do(manifestRequest)
	if err != nil {
		t.Fatal(err)
	}
	manifestResponse.Body.Close()
	if manifestResponse.StatusCode != http.StatusAccepted {
		t.Fatalf("manifest ingestion status: %d", manifestResponse.StatusCode)
	}
	var syntheticSamples int
	if err = repository.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM job_metric_samples WHERE attempt_id=?`, attemptID).Scan(&syntheticSamples); err != nil || syntheticSamples != 0 {
		t.Fatalf("manifest created metric points: count=%d err=%v", syntheticSamples, err)
	}

	metricPayload, _ := json.Marshal(map[string]any{"items": []any{
		map[string]any{"name": "loss", "value": .5, "step": 1, "timestamp": base.Add(20 * time.Second), "unit": "ratio", "metadata": map[string]any{"split": "train"}, "tags": []string{"phase:Train", "metric:loss", "phase:train"}},
		map[string]any{"name": "accuracy", "value": .8, "step": 1, "timestamp": base.Add(20 * time.Second)},
	}})
	metricRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/job-context/metrics", bytes.NewReader(metricPayload))
	metricRequest.Header.Set("Content-Type", "application/json")
	metricRequest.Header.Set("Authorization", "Bearer "+jobToken)
	metricResponse, err := http.DefaultClient.Do(metricRequest)
	if err != nil {
		t.Fatal(err)
	}
	metricResponse.Body.Close()
	if metricResponse.StatusCode != http.StatusAccepted {
		t.Fatalf("metric ingestion status: %d", metricResponse.StatusCode)
	}
	events, _ := repository.Events(ctx, job.ID, 0)
	for _, event := range events {
		if event.Type == "metrics" {
			t.Fatal("structured metrics were also persisted as generic job events")
		}
	}

	ownerClient := loginSeriesUser(t, server.URL, owner.Username)
	var catalog struct {
		AttemptID string                   `json:"attempt_id"`
		Items     []store.MetricDescriptor `json:"items"`
	}
	getSeriesJSON(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/metrics/catalog?attempt_id="+attemptID, &catalog)
	if catalog.AttemptID != attemptID || len(catalog.Items) != 3 {
		t.Fatalf("semantic metric catalog: %#v", catalog)
	}
	byName := map[string]store.MetricDescriptor{}
	for _, item := range catalog.Items {
		byName[item.Name] = item
	}
	if !byName["future_accuracy"].Declared || byName["future_accuracy"].Observed || !byName["loss"].Declared || !byName["loss"].Observed || strings.Join(byName["loss"].Tags, ",") != "metric:loss,phase:train" {
		t.Fatalf("declared metric catalog state: %#v", catalog)
	}
	var observableCatalog struct {
		AttemptID string                   `json:"attempt_id"`
		Items     []store.MetricDescriptor `json:"items"`
	}
	getSeriesJSON(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/observability/catalog?attempt_id="+attemptID, &observableCatalog)
	if observableCatalog.AttemptID != attemptID || len(observableCatalog.Items) != 4 || observableCatalog.Items[3].Name != "validation/confusion" || !observableCatalog.Items[3].Declared || observableCatalog.Items[3].Observed {
		t.Fatalf("observable catalog: %#v", observableCatalog)
	}
	var filteredCatalog struct {
		AttemptID string                   `json:"attempt_id"`
		Items     []store.MetricDescriptor `json:"items"`
	}
	getSeriesJSON(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/metrics/catalog?attempt_id="+attemptID+"&tag=phase:TRAIN&tag=metric:loss", &filteredCatalog)
	if filteredCatalog.AttemptID != attemptID || len(filteredCatalog.Items) != 1 || filteredCatalog.Items[0].Name != "loss" {
		t.Fatalf("AND-filtered semantic catalog: %#v", filteredCatalog)
	}
	getSeriesJSON(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/metrics/catalog?attempt_id="+attemptID+"&tag=phase:validation&tag=metric:loss", &filteredCatalog)
	if len(filteredCatalog.Items) != 0 {
		t.Fatalf("non-matching semantic catalog: %#v", filteredCatalog)
	}
	from, to := url.QueryEscape(base.Add(-time.Second).Format(time.RFC3339)), url.QueryEscape(base.Add(2*time.Minute).Format(time.RFC3339))
	metricURL := server.URL + "/api/v1/jobs/" + job.ID + "/metrics?attempt_id=" + attemptID + "&from=" + from + "&to=" + to + "&resolution=raw"
	var metrics metricSeriesResponse
	getSeriesJSON(t, ownerClient, metricURL, &metrics)
	if metrics.AttemptID != attemptID || len(metrics.Series) != 2 || metrics.Truncated {
		t.Fatalf("metric response: %#v", metrics)
	}
	for _, series := range metrics.Series {
		if series.Name == "loss" && (series.Unit != "ratio" || series.Metadata["split"] != "train" || strings.Join(series.Tags, ",") != "metric:loss,phase:train") {
			t.Fatalf("enriched metric descriptor: %#v", series)
		}
		if series.Name == "loss" && (len(series.Points) != 1 || !series.Points[0].CapturedAt.Equal(base.Add(20*time.Second))) {
			t.Fatalf("client metric timestamp was not preserved: %#v", series.Points)
		}
	}
	resourceURL := server.URL + "/api/v1/jobs/" + job.ID + "/resources?attempt_id=" + attemptID + "&from=" + from + "&to=" + to + "&resolution=5s"
	var resources resourceSeriesResponse
	getSeriesJSON(t, ownerClient, resourceURL, &resources)
	if len(resources.Points) != 1 || resources.Points[0].AttemptID != attemptID || resources.Points[0].GPUUtilizationBasisPoints == nil {
		t.Fatalf("resource response: %#v", resources)
	}
	if metrics.Cursor == 0 || resources.Cursor == 0 || metrics.Cursor != resources.Cursor {
		t.Fatalf("series responses do not share a consistent cursor: metrics=%d resources=%d", metrics.Cursor, resources.Cursor)
	}
	if err = repository.AppendMetricSamples(ctx, []domain.MetricSample{{JobID: job.ID, AttemptID: attemptID, Name: "loss", Value: .25, CapturedAt: base.Add(20 * time.Second)}}); err != nil {
		t.Fatal(err)
	}
	metricUpdate := readSeriesSSE(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/series/stream?attempt_id="+attemptID+"&after="+strconv.FormatInt(metrics.Cursor, 10), "")
	if metricUpdate.Cursor <= metrics.Cursor || metricUpdate.Kind != "metrics" || len(metricUpdate.Metrics) != 1 || metricUpdate.Metrics[0].Name != "loss" || metricUpdate.Metrics[0].Unit != "ratio" || strings.Join(metricUpdate.Metrics[0].Tags, ",") != "metric:loss,phase:train" {
		t.Fatalf("metric live update: %#v", metricUpdate)
	}
	conflict, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/job-context/metrics", strings.NewReader(`{"items":[{"name":"loss","value":0.1,"unit":"seconds"}]}`))
	conflict.Header.Set("Content-Type", "application/json")
	conflict.Header.Set("Authorization", "Bearer "+jobToken)
	conflictResponse, err := http.DefaultClient.Do(conflict)
	if err != nil {
		t.Fatal(err)
	}
	var problem map[string]any
	_ = json.NewDecoder(conflictResponse.Body).Decode(&problem)
	conflictResponse.Body.Close()
	if conflictResponse.StatusCode != http.StatusConflict || problem["code"] != "metric_descriptor_conflict" {
		t.Fatalf("descriptor conflict response: %d %#v", conflictResponse.StatusCode, problem)
	}
	manifestConflict, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/job-context/observability/manifest", strings.NewReader(`{"version":1,"sources":[{"name":"loss","type":"metric","unit":"seconds"}]}`))
	manifestConflict.Header.Set("Content-Type", "application/json")
	manifestConflict.Header.Set("Authorization", "Bearer "+jobToken)
	manifestConflictResponse, err := http.DefaultClient.Do(manifestConflict)
	if err != nil {
		t.Fatal(err)
	}
	problem = map[string]any{}
	_ = json.NewDecoder(manifestConflictResponse.Body).Decode(&problem)
	manifestConflictResponse.Body.Close()
	if manifestConflictResponse.StatusCode != http.StatusConflict || problem["code"] != "observable_declaration_conflict" {
		t.Fatalf("manifest conflict response: %d %#v", manifestConflictResponse.StatusCode, problem)
	}
	invalidCases := []struct {
		payload string
		status  int
		code    string
	}{
		{`{"items":[{"name":"future","value":1,"timestamp":"` + time.Now().UTC().Add(10*time.Minute).Format(time.RFC3339) + `"}]}`, http.StatusUnprocessableEntity, "invalid_metric_timestamp"},
		{`{"items":[{"name":"unsafe","value":1,"metadata":{"a":{"b":{"c":{"d":"too deep"}}}}}]}`, http.StatusUnprocessableEntity, "invalid_metric_metadata"},
		{`{"items":[{"name":"unsafe","value":1,"tags":["train"]}]}`, http.StatusUnprocessableEntity, "invalid_metric_tags"},
		{`{"items":[{"name":"forged","value":1,"attempt_id":"other"}]}`, http.StatusBadRequest, "invalid_json"},
	}
	for _, item := range invalidCases {
		request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/job-context/metrics", strings.NewReader(item.payload))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+jobToken)
		response, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		problem = map[string]any{}
		_ = json.NewDecoder(response.Body).Decode(&problem)
		response.Body.Close()
		if response.StatusCode != item.status || problem["code"] != item.code {
			t.Fatalf("invalid metric status=%d problem=%#v", response.StatusCode, problem)
		}
	}
	invalidCatalog, _ := ownerClient.Get(server.URL + "/api/v1/jobs/" + job.ID + "/metrics/catalog?attempt_id=" + attemptID + "&tag=train")
	if invalidCatalog.StatusCode != http.StatusUnprocessableEntity {
		invalidCatalog.Body.Close()
		t.Fatalf("invalid catalog tag status: %d", invalidCatalog.StatusCode)
	}
	invalidCatalog.Body.Close()
	if err = repository.AppendResourceSample(ctx, domain.ResourceSample{JobID: job.ID, AttemptID: attemptID, CapturedAt: base.Add(25 * time.Second), CPUMillis: 900, MemoryBytes: 256 << 20}); err != nil {
		t.Fatal(err)
	}
	resourceUpdate := readSeriesSSE(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/series/stream?attempt_id="+attemptID+"&after="+strconv.FormatInt(metrics.Cursor, 10), strconv.FormatInt(metricUpdate.Cursor, 10))
	if resourceUpdate.Cursor <= metricUpdate.Cursor || resourceUpdate.Kind != "resources" || resourceUpdate.Resource == nil || resourceUpdate.Resource.CPUMillis != 900 {
		t.Fatalf("resumed resource update: %#v", resourceUpdate)
	}
	for _, endpoint := range []string{metricURL + "&format=csv", resourceURL + "&format=csv"} {
		response, requestErr := ownerClient.Get(endpoint)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/csv") || !bytes.Contains(data, []byte(attemptID)) {
			t.Fatalf("CSV response status=%d headers=%v body=%s", response.StatusCode, response.Header, data)
		}
		if strings.Contains(endpoint, "/metrics?") && (!bytes.Contains(data, []byte("ratio")) || !bytes.Contains(data, []byte("metric:loss")) || !bytes.Contains(data, []byte(`{""split"":""train""}`))) {
			t.Fatalf("metric CSV is missing descriptor columns: %s", data)
		}
	}
	otherClient := loginSeriesUser(t, server.URL, other.Username)
	for _, endpoint := range []string{metricURL, resourceURL, server.URL + "/api/v1/jobs/" + job.ID + "/metrics/catalog?attempt_id=" + attemptID, server.URL + "/api/v1/jobs/" + job.ID + "/observability/catalog?attempt_id=" + attemptID} {
		response, requestErr := otherClient.Get(endpoint)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("cross-owner series status: %d", response.StatusCode)
		}
	}
}

func TestNormalizeMetricTagsForCatalog(t *testing.T) {
	normalized, err := normalizeMetricTags([]string{" phase:TRAIN ", "metric:loss", "phase:train"})
	if err != nil || strings.Join(normalized, ",") != "metric:loss,phase:train" {
		t.Fatalf("normalized tags: %v err=%v", normalized, err)
	}
	tooMany := make([]string, 33)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("custom:value-%d", index)
	}
	if _, err = normalizeMetricTags(tooMany); err == nil {
		t.Fatal("expected a bounded catalog tag filter")
	}
}

func readSeriesSSE(t *testing.T, client *http.Client, endpoint, lastEventID string) store.SeriesUpdate {
	t.Helper()
	request, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	if lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("series stream status %d: %s", response.StatusCode, data)
	}
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			var update store.SeriesUpdate
			if err = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &update); err != nil {
				t.Fatal(err)
			}
			return update
		}
	}
	t.Fatalf("series stream ended without data: %v", scanner.Err())
	return store.SeriesUpdate{}
}

func createSeriesUser(t *testing.T, repository *store.Store, username string) domain.User {
	t.Helper()
	hash, err := auth.HashPassword("correct series password")
	if err != nil {
		t.Fatal(err)
	}
	user := domain.User{ID: ids.New(), Username: username, Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
	if err = repository.CreateUser(context.Background(), user, hash); err != nil {
		t.Fatal(err)
	}
	return user
}

func loginSeriesUser(t *testing.T, serverURL, username string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	body, _ := json.Marshal(map[string]string{"username": username, "password": "correct series password"})
	response, err := client.Post(serverURL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status: %d", response.StatusCode)
	}
	return client
}

func getSeriesJSON(t *testing.T, client *http.Client, endpoint string, destination any) {
	t.Helper()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("GET %s returned %d: %s", endpoint, response.StatusCode, data)
	}
	if err = json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatal(err)
	}
}
