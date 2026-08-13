package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
)

func TestStopCompletesPersistedAssignmentWithoutContainer(t *testing.T) {
	var received struct {
		AttemptID string           `json:"attempt_id"`
		Status    domain.JobStatus `json:"status"`
		Type      string           `json:"type"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/agent/jobs/job-one/events" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	stateDir := t.TempDir()
	agent := New(config.Agent{ServerURL: server.URL, StateDir: stateDir}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	agent.credential = "agent-credential"
	record := &runtimeAssignment{Assignment: domain.Assignment{ID: "assignment-one", JobID: "job-one", AttemptID: "attempt-one"}, Sequence: 4}
	if err := agent.save(record); err != nil {
		t.Fatal(err)
	}

	agent.stop(context.Background(), record.JobID)

	if received.AttemptID != "attempt-one" || received.Status != domain.JobCancelled || received.Type != "cancelled" {
		t.Fatalf("unexpected cancellation event: %#v", received)
	}
	data, err := os.ReadFile(filepath.Join(stateDir, "assignments", "job-one.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved runtimeAssignment
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.StopRequested || !saved.Completed {
		t.Fatalf("persisted assignment state: stop_requested=%t completed=%t", saved.StopRequested, saved.Completed)
	}
}

func TestLoadAssignmentRejectsPathTraversal(t *testing.T) {
	agent := New(config.Agent{StateDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if record := agent.loadAssignment("../credential"); record != nil {
		t.Fatalf("path traversal loaded an assignment: %#v", record)
	}
}
