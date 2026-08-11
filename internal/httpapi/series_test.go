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
	"net/url"
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
	files, _ := filestore.New(root, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{8}, 32))
	api := New(config.Server{AllowInsecureHTTP: true, SessionTTL: time.Hour}, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(api.Handler())
	defer server.Close()

	metricRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/job-context/metrics", strings.NewReader(`{"items":[{"name":"loss","value":0.5,"step":1},{"name":"accuracy","value":0.8,"step":1}]}`))
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
	from, to := url.QueryEscape(base.Add(-time.Second).Format(time.RFC3339)), url.QueryEscape(base.Add(2*time.Minute).Format(time.RFC3339))
	metricURL := server.URL + "/api/v1/jobs/" + job.ID + "/metrics?attempt_id=" + attemptID + "&from=" + from + "&to=" + to + "&resolution=raw"
	var metrics metricSeriesResponse
	getSeriesJSON(t, ownerClient, metricURL, &metrics)
	if metrics.AttemptID != attemptID || len(metrics.Series) != 2 || metrics.Truncated {
		t.Fatalf("metric response: %#v", metrics)
	}
	resourceURL := server.URL + "/api/v1/jobs/" + job.ID + "/resources?attempt_id=" + attemptID + "&from=" + from + "&to=" + to + "&resolution=5s"
	var resources resourceSeriesResponse
	getSeriesJSON(t, ownerClient, resourceURL, &resources)
	if len(resources.Points) != 1 || resources.Points[0].AttemptID != attemptID || resources.Points[0].GPUUtilizationBasisPoints == nil {
		t.Fatalf("resource response: %#v", resources)
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
	}
	otherClient := loginSeriesUser(t, server.URL, other.Username)
	for _, endpoint := range []string{metricURL, resourceURL} {
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
