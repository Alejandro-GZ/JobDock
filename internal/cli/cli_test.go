package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNodesJSONUsesBearerAndStableOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("authorization=%q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"items":[{"id":"node-1","name":"worker","status":"ONLINE","cpu_total_millis":1000,"cpu_allocated_millis":0,"memory_total_bytes":1024,"memory_allocated_bytes":0,"workspace_free_bytes":1,"labels":{},"gpus":[],"gpu_discovery":{"status":"AVAILABLE"},"last_heartbeat":"2026-01-01T00:00:00Z","created_at":"2026-01-01T00:00:00Z"}]}`)
	}))
	defer server.Close()
	var out, stderr bytes.Buffer
	app := New(&out, &stderr)
	app.getenv = func(key string) string {
		if key == "JOBDOCK_TOKEN" {
			return "test-secret"
		}
		return ""
	}
	if code := app.Run(context.Background(), []string{"--server", server.URL, "--format", "json", "nodes"}); code != ExitOK {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(out.String(), `"id":"node-1"`) {
		t.Fatalf("output=%s", out.String())
	}
}

func TestLogsFollowUsesAcknowledgedOffsets(t *testing.T) {
	requests := 0
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			if !strings.Contains(r.URL.RawQuery, "offset=0") {
				t.Errorf("first query=%s", r.URL.RawQuery)
			}
			w.Header().Set("X-JobDock-Next-Offset", "3")
			io.WriteString(w, "one")
			return
		}
		if !strings.Contains(r.URL.RawQuery, "offset=3") {
			t.Errorf("second query=%s", r.URL.RawQuery)
		}
		w.Header().Set("X-JobDock-Next-Offset", "6")
		io.WriteString(w, "two")
		close(done)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	var out, stderr bytes.Buffer
	app := New(&out, &stderr)
	app.getenv = func(key string) string {
		if key == "JOBDOCK_TOKEN" {
			return "secret"
		}
		return ""
	}
	go func() { <-done; time.Sleep(50 * time.Millisecond); cancel() }()
	code := app.Run(ctx, []string{"--server", server.URL, "logs", "-f", "job-1"})
	if code != ExitOK {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if out.String() != "onetwo" {
		t.Fatalf("logs=%q", out.String())
	}
}

func TestErrorsNeverContainToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"code":"unauthorized","message":"denied"}`)
	}))
	defer server.Close()
	var out, stderr bytes.Buffer
	app := New(&out, &stderr)
	app.getenv = func(key string) string {
		if key == "JOBDOCK_TOKEN" {
			return "very-sensitive-token"
		}
		return ""
	}
	code := app.Run(context.Background(), []string{"--server", server.URL, "nodes"})
	if code != ExitAuth {
		t.Fatalf("exit=%d", code)
	}
	if strings.Contains(stderr.String(), "very-sensitive-token") {
		t.Fatal("token leaked in error")
	}
}

func TestJobCommandsAndAtomicDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/jobs":
			io.WriteString(w, `{"items":[]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			resources := body["resources"].(map[string]any)
			if resources["memory_bytes"] != float64(134217728) {
				t.Errorf("memory=%v", resources["memory_bytes"])
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"id":"job-1","owner_id":"user-1","spec":{"name":"smoke","image":"alpine:3","command":["echo","ok"],"resources":{"cpu_millis":250,"memory_bytes":134217728,"gpu":{"count":0,"min_vram_bytes":0}}},"status":"QUEUED","desired_status":"RUNNING","observed_status":"QUEUED","created_at":"2026-01-01T00:00:00Z","version":1}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs/job-1/stop":
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"status":"STOPPING"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/jobs/job-1/archive.zip":
			io.WriteString(w, "zip-bytes")
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	run := func(args ...string) (int, string, string) {
		var out, stderr bytes.Buffer
		app := New(&out, &stderr)
		app.getenv = func(key string) string {
			if key == "JOBDOCK_TOKEN" {
				return "secret"
			}
			return ""
		}
		all := append([]string{"--server", server.URL}, args...)
		code := app.Run(context.Background(), all)
		return code, out.String(), stderr.String()
	}
	if code, _, errout := run("jobs"); code != ExitOK {
		t.Fatalf("jobs exit=%d %s", code, errout)
	}
	if code, out, errout := run("run", "--name", "smoke", "--image", "alpine:3", "--cpu", "250", "--memory", "134217728", "--", "echo", "ok"); code != ExitOK || strings.TrimSpace(out) != "job-1" {
		t.Fatalf("run exit=%d out=%q err=%s", code, out, errout)
	}
	if code, out, errout := run("stop", "job-1"); code != ExitOK || strings.TrimSpace(out) != "STOPPING" {
		t.Fatalf("stop exit=%d out=%q err=%s", code, out, errout)
	}
	destination := filepath.Join(t.TempDir(), "result.zip")
	if code, _, errout := run("download", "--output", destination, "job-1"); code != ExitOK {
		t.Fatalf("download exit=%d %s", code, errout)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "zip-bytes" {
		t.Fatalf("download=%q err=%v", data, err)
	}
}
