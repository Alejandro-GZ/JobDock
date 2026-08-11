package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/auth"
	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

func TestMultipartJobInputsAreImmutableAuthorizedAndBounded(t *testing.T) {
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	files, _ := filestore.New(root, 1<<20, 1<<20, 16)
	box, _ := secretbox.New(bytes.Repeat([]byte{4}, 32))
	api := New(config.Server{AllowInsecureHTTP: true, BootstrapUsername: "admin", BootstrapPassword: "correct input password", SessionTTL: time.Hour, MaxInputBytes: 16}, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err = api.BootstrapAdmin(context.Background()); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	login, err := client.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewBufferString(`{"username":"admin","password":"correct input password"}`))
	if err != nil {
		t.Fatal(err)
	}
	var session struct {
		CSRF string `json:"csrf_token"`
	}
	_ = json.NewDecoder(login.Body).Decode(&session)
	login.Body.Close()

	create := func(content string) (*http.Response, domain.Job) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		spec, _ := writer.CreateFormField("spec")
		_, _ = io.WriteString(spec, `{"name":"input job","image":"alpine:3","command":["true"],"resources":{"cpu_millis":100,"memory_bytes":1024,"gpu":{"count":0,"min_vram_bytes":0}}}`)
		input, _ := writer.CreateFormFile("input:dataset/value.txt", "value.txt")
		_, _ = io.WriteString(input, content)
		_ = writer.Close()
		request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		request.Header.Set("X-CSRF-Token", session.CSRF)
		response, requestErr := client.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		var job domain.Job
		if response.StatusCode == http.StatusCreated {
			_ = json.NewDecoder(response.Body).Decode(&job)
		}
		response.Body.Close()
		return response, job
	}
	response, job := create("immutable input")
	if response.StatusCode != http.StatusCreated || len(job.Spec.Inputs) != 1 || job.Spec.Inputs[0].Path != "dataset/value.txt" || job.Spec.Inputs[0].Size != 15 {
		t.Fatalf("multipart job status=%d job=%#v", response.StatusCode, job)
	}
	node := domain.Node{ID: "11111111-1111-4111-8111-111111111111", Name: "input-node", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 1000, MemoryTotalBytes: 1024, WorkspaceFreeBytes: 1024, Labels: map[string]string{}, LastHeartbeat: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	if err = repository.UpsertNode(context.Background(), node, auth.TokenHash("node-token")); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.DB().Exec(`UPDATE jobs SET assigned_node_id=? WHERE id=?`, node.ID, job.ID); err != nil {
		t.Fatal(err)
	}
	download, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/agent/jobs/"+job.ID+"/inputs/dataset/value.txt", nil)
	download.Header.Set("Authorization", "Bearer node-token")
	download.Header.Set("X-JobDock-Protocol-Version", "1")
	downloadResponse, err := client.Do(download)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(downloadResponse.Body)
	downloadResponse.Body.Close()
	if downloadResponse.StatusCode != http.StatusOK || string(data) != "immutable input" || downloadResponse.Header.Get("X-JobDock-Content-SHA256") != job.Spec.Inputs[0].SHA256 {
		t.Fatalf("agent input download status=%d body=%q headers=%v", downloadResponse.StatusCode, data, downloadResponse.Header)
	}
	tooLarge, _ := create("this input is larger than sixteen bytes")
	if tooLarge.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized input status=%d", tooLarge.StatusCode)
	}
	jobs, _ := repository.ListJobs(context.Background(), false)
	if len(jobs) != 1 {
		t.Fatalf("failed input upload created a job: %#v", jobs)
	}
}
