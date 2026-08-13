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
)

func TestPollAssignmentsBoundsImmediateEmptyResponses(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pollResponse{})
	}))
	defer server.Close()

	agent := &Agent{
		config:     config.Agent{ServerURL: server.URL},
		log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		http:       server.Client(),
		credential: "node-token",
		running:    map[string]*runtimeAssignment{},
		syncing:    map[string]bool{},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	if err := agent.pollAssignments(ctx); err != nil {
		t.Fatal(err)
	}
	if count := requests.Load(); count < 2 || count > 4 {
		t.Fatalf("immediate empty responses produced %d requests in 700ms; want 2..4", count)
	}
}

func TestEmptyPollDelayDoesNotApplyToWork(t *testing.T) {
	response := pollResponse{StopJobIDs: []string{"job-1"}}
	if !response.hasWork() {
		t.Fatal("a response containing work was classified as empty")
	}
	if (pollResponse{}).hasWork() {
		t.Fatal("an empty response was classified as work")
	}
}
