package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
)

func TestMaterializeInputsVerifiesManifestAndCleansFailures(t *testing.T) {
	content := []byte("immutable data")
	digest := sha256.Sum256(content)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer node-token" || r.Header.Get("X-JobDock-Protocol-Version") != "1" {
			t.Error("input download omitted scoped agent headers")
		}
		w.Header().Set("X-JobDock-Content-SHA256", hex.EncodeToString(digest[:]))
		_, _ = w.Write(content)
	}))
	defer server.Close()
	workspace := t.TempDir()
	agent := &Agent{config: config.Agent{ServerURL: server.URL, WorkspaceDir: workspace}, http: server.Client(), credential: "node-token"}
	root := filepath.Join(workspace, "job", "input")
	manifest := []domain.InputFile{{Path: "dataset/value.txt", Size: int64(len(content)), SHA256: hex.EncodeToString(digest[:])}}
	if err := agent.materializeInputs(context.Background(), "job", manifest, root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "dataset", "value.txt"))
	if err != nil || string(data) != string(content) {
		t.Fatalf("materialized content = %q, err=%v", data, err)
	}
	if err = removeMaterializedInputs(root); err != nil {
		t.Fatalf("remove read-only inputs: %v", err)
	}
	if _, err = os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("successful materialization was not cleaned: %v", err)
	}
	if err = agent.materializeInputs(context.Background(), "job", manifest, root); err != nil {
		t.Fatal(err)
	}
	manifest[0].SHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err = agent.materializeInputs(context.Background(), "job", manifest, root); err == nil {
		t.Fatal("digest mismatch was accepted")
	}
	if _, err = os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("failed materialization was not cleaned: %v", err)
	}
}

func TestMaterializeInputsRejectsIncompleteServerCommitment(t *testing.T) {
	content := []byte("immutable data")
	digest := sha256.Sum256(content)
	includeDigest := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if includeDigest {
			w.Header().Set("X-JobDock-Content-SHA256", hex.EncodeToString(digest[:]))
			w.Header().Set("Content-Length", "1")
		}
		_, _ = w.Write(content)
	}))
	defer server.Close()
	workspace := t.TempDir()
	agent := &Agent{config: config.Agent{ServerURL: server.URL, WorkspaceDir: workspace}, http: server.Client(), credential: "node-token"}
	root := filepath.Join(workspace, "job", "input")
	manifest := []domain.InputFile{{Path: "dataset/value.txt", Size: int64(len(content)), SHA256: hex.EncodeToString(digest[:])}}

	if err := agent.materializeInputs(context.Background(), "job", manifest, root); err == nil {
		t.Fatal("a response without a committed digest was accepted")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("digest-header failure left materialized inputs: %v", err)
	}
	includeDigest = true
	if err := agent.materializeInputs(context.Background(), "job", manifest, root); err == nil {
		t.Fatal("a response with an incorrect committed size was accepted")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("size-header failure left materialized inputs: %v", err)
	}
}
