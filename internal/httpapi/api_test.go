package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

func TestLoginAndIdempotentJobCreation(t *testing.T) {
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	files, _ := filestore.New(root, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{5}, 32))
	cfg := config.Server{AllowInsecureHTTP: true, BootstrapUsername: "admin", BootstrapPassword: "correct horse battery", SessionTTL: 24 * 60 * 60 * 1e9}
	api := New(cfg, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err = api.BootstrapAdmin(context.Background()); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	loginBody := bytes.NewBufferString(`{"username":"admin","password":"correct horse battery"}`)
	response, err := http.Post(server.URL+"/api/v1/auth/login", "application/json", loginBody)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("login %d: %s", response.StatusCode, data)
	}
	var session struct {
		CSRF string `json:"csrf_token"`
	}
	if err = json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	var cookie *http.Cookie
	for _, item := range response.Cookies() {
		if item.Name == "jobdock_session" {
			cookie = item
		}
	}
	if cookie == nil {
		t.Fatal("missing session cookie")
	}
	jobBody := `{"name":"test-job","image":"alpine:3","command":["echo","ok"],"resources":{"cpu_millis":100,"memory_bytes":1048576,"gpu":{"count":0,"min_vram_bytes":0}}}`
	create := func() (int, string) {
		request, _ := http.NewRequest("POST", server.URL+"/api/v1/jobs", bytes.NewBufferString(jobBody))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-CSRF-Token", session.CSRF)
		request.Header.Set("Idempotency-Key", "1234567890abcdef")
		request.AddCookie(cookie)
		result, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		defer result.Body.Close()
		data, _ := io.ReadAll(result.Body)
		return result.StatusCode, string(data)
	}
	status, first := create()
	if status != 201 {
		t.Fatalf("first create %d: %s", status, first)
	}
	status, second := create()
	if status != 201 || first != second {
		t.Fatalf("replay mismatch %d %q %q", status, first, second)
	}
	jobs, err := repository.ListJobs(context.Background(), false)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs: %d %v", len(jobs), err)
	}
}
