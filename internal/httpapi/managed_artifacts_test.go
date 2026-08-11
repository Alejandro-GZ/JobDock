package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/auth"
	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

func TestManagedArtifactPublicationAndScopedAgentDownload(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := store.Open(filepath.Join(root, "jobdock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	files, _ := filestore.New(root, 1<<20, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{3}, 32))
	api := New(config.Server{AllowInsecureHTTP: true, SessionTTL: time.Hour, BuilderToken: "builder-token-with-at-least-32-characters", BuilderLease: time.Minute, MaxBuildArtifactBytes: 1 << 20}, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	now := time.Now().UTC()
	owner := domain.User{ID: ids.New(), Username: "managed-owner", Role: domain.RoleMember, CreatedAt: now}
	if err = repository.CreateUser(ctx, owner, "hash"); err != nil {
		t.Fatal(err)
	}
	build := domain.Build{ID: ids.New(), OwnerID: owner.ID, Name: "managed", Mode: domain.BuildModeDockerfile, Status: domain.BuildCreated, Source: domain.BuildSource{Filename: "source.zip", Size: 1, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}, CreatedAt: now, Version: 1}
	if err = repository.CreateBuild(ctx, build); err != nil {
		t.Fatal(err)
	}
	queued, err := repository.QueueBuild(ctx, build.ID, ids.New())
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.NextBuildWork(ctx, "builder-test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	archive := []byte("docker-compatible-image-archive")
	upload, _ := http.NewRequest(http.MethodPut, server.URL+"/api/v1/builder/assignments/"+claimed.Assignment.ID+"/artifact", bytes.NewReader(archive))
	upload.Header.Set("Authorization", "Bearer builder-token-with-at-least-32-characters")
	upload.Header.Set("X-JobDock-Protocol-Version", "1")
	upload.Header.Set("X-JobDock-Builder-ID", "builder-test")
	upload.Header.Set("X-JobDock-OCI-Digest", digest)
	upload.Header.Set("X-JobDock-Runtime-Image", "jobdock.local/managed/"+build.ID+":artifact")
	uploadResponse, err := server.Client().Do(upload)
	if err != nil {
		t.Fatal(err)
	}
	var artifact domain.ManagedArtifact
	_ = json.NewDecoder(uploadResponse.Body).Decode(&artifact)
	uploadResponse.Body.Close()
	if uploadResponse.StatusCode != http.StatusCreated || artifact.Size != int64(len(archive)) || artifact.Digest != digest {
		t.Fatalf("upload status=%d artifact=%#v", uploadResponse.StatusCode, artifact)
	}
	completed, err := repository.CompleteBuildAssignment(ctx, queued.Assignment.ID, "builder-test", domain.BuildAssignmentSucceeded, digest, "published")
	if err != nil || !completed.ArtifactAvailable || completed.ArtifactReference != domain.ManagedArtifactReference(build.ID, digest) {
		t.Fatalf("completed=%#v error=%v", completed, err)
	}
	job := domain.Job{ID: ids.New(), OwnerID: owner.ID, Spec: domain.JobSpec{Name: "managed job", Image: completed.ArtifactReference, Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1 << 20}}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: now}
	if err = repository.CreateJobWithManagedArtifact(ctx, job, build.ID, digest); err != nil {
		t.Fatal(err)
	}
	nodeToken := "node-token"
	node := domain.Node{ID: ids.New(), Name: "worker", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 1000, MemoryTotalBytes: 1 << 30, WorkspaceFreeBytes: 20 << 30, Labels: map[string]string{}, LastHeartbeat: now, CreatedAt: now}
	if err = repository.UpsertNode(ctx, node, auth.TokenHash(nodeToken)); err != nil {
		t.Fatal(err)
	}
	assignmentID := ids.New()
	if err = repository.ReserveJob(ctx, job.ID, node.ID, ids.New(), assignmentID, "job-token", []byte("ciphertext"), nil); err != nil {
		t.Fatal(err)
	}
	download, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/agent/assignments/"+assignmentID+"/artifact", nil)
	download.Header.Set("Authorization", "Bearer "+nodeToken)
	download.Header.Set("X-JobDock-Protocol-Version", "1")
	downloadResponse, err := server.Client().Do(download)
	if err != nil {
		t.Fatal(err)
	}
	received, _ := io.ReadAll(downloadResponse.Body)
	downloadResponse.Body.Close()
	if downloadResponse.StatusCode != http.StatusOK || !bytes.Equal(received, archive) || downloadResponse.Header.Get("X-JobDock-OCI-Digest") != digest {
		t.Fatalf("download status=%d body=%q headers=%v", downloadResponse.StatusCode, received, downloadResponse.Header)
	}
	otherToken := "other-node-token"
	otherNode := node
	otherNode.ID, otherNode.Name = ids.New(), "other-worker"
	if err = repository.UpsertNode(ctx, otherNode, auth.TokenHash(otherToken)); err != nil {
		t.Fatal(err)
	}
	forbidden, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/agent/assignments/"+assignmentID+"/artifact", nil)
	forbidden.Header.Set("Authorization", "Bearer "+otherToken)
	forbidden.Header.Set("X-JobDock-Protocol-Version", "1")
	forbiddenResponse, err := server.Client().Do(forbidden)
	if err != nil {
		t.Fatal(err)
	}
	forbiddenResponse.Body.Close()
	if forbiddenResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-node artifact download status=%d", forbiddenResponse.StatusCode)
	}
}
