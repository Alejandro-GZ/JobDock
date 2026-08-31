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
	"time"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

func TestDisabledBuilderCapabilityRejectsSourceBuildsButKeepsOCIJobs(t *testing.T) {
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	files, _ := filestore.New(root, 1<<20, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{9}, 32))
	cfg := config.Server{AllowInsecureHTTP: true, BootstrapUsername: "admin", BootstrapPassword: "correct horse battery", SessionTTL: time.Hour, BuilderToken: "builder-token-with-at-least-32-characters", BuilderDisabled: true}
	api := New(cfg, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err = api.BootstrapAdmin(context.Background()); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	defer server.Close()

	login, err := http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewBufferString(`{"username":"admin","password":"correct horse battery"}`))
	if err != nil {
		t.Fatal(err)
	}
	var session struct {
		CSRF string `json:"csrf_token"`
	}
	if err = json.NewDecoder(login.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	login.Body.Close()
	cookie := login.Cookies()[0]

	capabilityRequest, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/capabilities", nil)
	capabilityRequest.AddCookie(cookie)
	capabilityResponse, err := server.Client().Do(capabilityRequest)
	if err != nil {
		t.Fatal(err)
	}
	var capability struct {
		SourceBuilds struct {
			Enabled bool   `json:"enabled"`
			Reason  string `json:"reason"`
		} `json:"source_builds"`
	}
	if err = json.NewDecoder(capabilityResponse.Body).Decode(&capability); err != nil {
		t.Fatal(err)
	}
	capabilityResponse.Body.Close()
	if capabilityResponse.StatusCode != http.StatusOK || capability.SourceBuilds.Enabled || capability.SourceBuilds.Reason == "" {
		t.Fatalf("unexpected capability: status=%d body=%+v", capabilityResponse.StatusCode, capability)
	}

	buildRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/builds", bytes.NewBufferString("ignored"))
	buildRequest.AddCookie(cookie)
	buildRequest.Header.Set("X-CSRF-Token", session.CSRF)
	buildResponse, err := server.Client().Do(buildRequest)
	if err != nil {
		t.Fatal(err)
	}
	var problem struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(buildResponse.Body).Decode(&problem)
	buildResponse.Body.Close()
	if buildResponse.StatusCode != http.StatusServiceUnavailable || problem.Code != "source_builds_disabled" {
		t.Fatalf("build response status=%d code=%s", buildResponse.StatusCode, problem.Code)
	}

	jobRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs", bytes.NewBufferString(`{"name":"oci-only","image":"alpine:3.20","command":["true"],"resources":{"cpu_millis":100,"memory_bytes":1048576,"gpu":{"count":0,"min_vram_bytes":0}}}`))
	jobRequest.AddCookie(cookie)
	jobRequest.Header.Set("Content-Type", "application/json")
	jobRequest.Header.Set("X-CSRF-Token", session.CSRF)
	jobRequest.Header.Set("Idempotency-Key", "oci-only-capability-test")
	jobResponse, err := server.Client().Do(jobRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer jobResponse.Body.Close()
	if jobResponse.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(jobResponse.Body)
		t.Fatalf("OCI job status=%d body=%s", jobResponse.StatusCode, body)
	}
}
