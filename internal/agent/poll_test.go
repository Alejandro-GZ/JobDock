package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
)

func TestPollAssignmentsBoundsImmediateEmptyResponses(t *testing.T) {
	testBoundedImmediatePolling(t, pollResponse{}, nil)
}

func TestPollAssignmentsBoundsImmediateDuplicateAssignment(t *testing.T) {
	assignment := domain.Assignment{JobID: "job-1"}
	testBoundedImmediatePolling(t, pollResponse{Assignment: &assignment}, map[string]*runtimeAssignment{
		"job-1": {Assignment: assignment},
	})
}

func testBoundedImmediatePolling(t *testing.T, response pollResponse, running map[string]*runtimeAssignment) {
	t.Helper()
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	if running == nil {
		running = map[string]*runtimeAssignment{}
	}

	agent := &Agent{
		config:     config.Agent{ServerURL: server.URL},
		log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		http:       server.Client(),
		credential: "node-token",
		running:    running,
		syncing:    map[string]bool{},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	if err := agent.pollAssignments(ctx); err != nil {
		t.Fatal(err)
	}
	if count := requests.Load(); count < 2 || count > 4 {
		t.Fatalf("immediate responses produced %d requests in 700ms; want 2..4", count)
	}
}
