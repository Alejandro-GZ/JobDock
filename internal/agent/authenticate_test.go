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
	"time"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
)

func TestAuthenticateReenrollsWhenPersistedCredentialWasRevoked(t *testing.T) {
	stateDir := t.TempDir()
	saved := credentialState{NodeID: "deleted-node", Credential: "revoked-credential", RotatedAt: time.Now().UTC()}
	data, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(stateDir, "credential.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	var heartbeatCalls, enrollCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/heartbeat":
			heartbeatCalls++
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "invalid_agent_credential", "detail": "Agent credential is invalid"})
		case "/api/v1/agent/enroll":
			enrollCalls++
			var request struct {
				EnrollmentToken string `json:"enrollment_token"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode enrollment: %v", err)
			}
			if request.EnrollmentToken != "fresh-enrollment-token" {
				t.Errorf("enrollment token = %q", request.EnrollmentToken)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"node_id": "replacement-node", "credential": "replacement-credential", "protocol_version": 1})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	agent := &Agent{
		config: config.Agent{ServerURL: server.URL, StateDir: stateDir, WorkspaceDir: stateDir, EnrollmentToken: "fresh-enrollment-token"},
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		http:   server.Client(),
		inventoryOverride: func(context.Context) (domain.Node, error) {
			return domain.Node{Name: "replacement"}, nil
		},
	}
	if err := agent.authenticate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if heartbeatCalls != 1 || enrollCalls != 1 {
		t.Fatalf("API calls: heartbeat=%d enroll=%d; want 1 each", heartbeatCalls, enrollCalls)
	}
	if agent.nodeID != "replacement-node" || agent.credential != "replacement-credential" {
		t.Fatalf("replacement credential not activated: node=%q credential=%q", agent.nodeID, agent.credential)
	}
	var persisted credentialState
	persistedData, err := os.ReadFile(filepath.Join(stateDir, "credential.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(persistedData, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.NodeID != "replacement-node" || persisted.Credential != "replacement-credential" {
		t.Fatalf("replacement credential not persisted: %+v", persisted)
	}
}
