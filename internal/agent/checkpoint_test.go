package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
)

func TestCheckpointUploadResumesFromDurableServerOffset(t *testing.T) {
	content := []byte("already-stored-and-new-checkpoint-data")
	stored := append([]byte(nil), content[:14]...)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-JobDock-Protocol-Version") != "1" {
			t.Error("binary upload omitted protocol version")
		}
		requests++
		offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
		w.Header().Set("Content-Type", "application/json")
		if offset != int64(len(stored)) {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]int64{"next_offset": int64(len(stored))})
			return
		}
		chunk, _ := io.ReadAll(r.Body)
		stored = append(stored, chunk...)
		_ = json.NewEncoder(w).Encode(map[string]int64{"next_offset": int64(len(stored))})
	}))
	defer server.Close()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "epoch-10.pt"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	agent := &Agent{config: config.Agent{ServerURL: server.URL}, http: server.Client(), credential: "node-token"}
	manifest, err := agent.uploadCheckpoint(context.Background(), domain.CheckpointSync{ID: "sync-one"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || string(stored) != string(content) {
		t.Fatalf("upload did not resume: requests=%d stored=%q", requests, stored)
	}
	if len(manifest) != 1 || manifest[0].Path != "epoch-10.pt" || manifest[0].Size != int64(len(content)) {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestWriteReadOnlyFileCanReplaceMountedCredential(t *testing.T) {
	path := filepath.Join(t.TempDir(), "job-token")
	if err := writeReadOnlyFile(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := writeReadOnlyFile(path, []byte("rotated")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "rotated" {
		t.Fatalf("credential = %q, err=%v", data, err)
	}
}
