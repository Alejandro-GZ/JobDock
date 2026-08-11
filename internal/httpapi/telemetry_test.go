package httpapi

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

func TestAgentTelemetryPersistsOnlyNormalizedScalars(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	user := domain.User{ID: ids.New(), Username: "owner", Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
	if err = repository.CreateUser(ctx, user, "hash"); err != nil {
		t.Fatal(err)
	}
	credential := "agent-credential"
	node := domain.Node{ID: ids.New(), Name: "node", Status: domain.NodeOnline, AgentVersion: "test", ProtocolVersion: 1, Architecture: "amd64", DockerVersion: "test", CPUTotalMillis: 4000, MemoryTotalBytes: 8 << 30, WorkspaceFreeBytes: 10 << 30, Labels: map[string]string{}, LastHeartbeat: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	if err = repository.UpsertNode(ctx, node, auth.TokenHash(credential)); err != nil {
		t.Fatal(err)
	}
	job := domain.Job{ID: ids.New(), OwnerID: user.ID, Spec: domain.JobSpec{Name: "sampled", Image: "alpine:3", Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1024}}, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: time.Now().UTC()}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	attemptID := ids.New()
	if _, err = repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,1,?,?,?,?,?)`, attemptID, job.ID, node.ID, ids.New(), "RUNNING", ids.New(), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.DB().ExecContext(ctx, `UPDATE jobs SET assigned_node_id=?,attempt_id=?,status='RUNNING',observed_status='RUNNING' WHERE id=?`, node.ID, attemptID, job.ID); err != nil {
		t.Fatal(err)
	}
	files, _ := filestore.New(root, 1024, 1024, 1024)
	box, _ := secretbox.New(bytes.Repeat([]byte{3}, 32))
	api := New(config.Server{AllowInsecureHTTP: true}, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(api.Handler())
	defer server.Close()

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/agent/jobs/"+job.ID+"/telemetry", bytes.NewBufferString(`{"cpu_millis":1250,"memory_bytes":536870912,"gpu_utilization_basis_points":4200,"gpu_memory_bytes":1073741824}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("X-JobDock-Protocol-Version", "1")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("telemetry status: %d", response.StatusCode)
	}
	samples, _, err := repository.ResourceSamples(ctx, job.ID, attemptID, time.Now().Add(-time.Minute), time.Now().Add(time.Minute), 5, 100)
	if err != nil || len(samples) != 1 || samples[0].CPUMillis != 1250 || samples[0].GPUUtilizationBasisPoints == nil || *samples[0].GPUUtilizationBasisPoints != 4200 {
		t.Fatalf("normalized samples: %#v %v", samples, err)
	}

	rawRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/agent/jobs/"+job.ID+"/events", bytes.NewBufferString(`{"sequence":1,"type":"resource_sample","payload":{"networks":{"eth0":{"rx_bytes":42}}}}`))
	rawRequest.Header = request.Header.Clone()
	rawResponse, err := http.DefaultClient.Do(rawRequest)
	if err != nil {
		t.Fatal(err)
	}
	rawResponse.Body.Close()
	if rawResponse.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("raw telemetry status: %d", rawResponse.StatusCode)
	}
	events, err := repository.Events(ctx, job.ID, 0)
	if err != nil || len(events) != 0 {
		t.Fatalf("raw Docker Stats reached job events: %#v %v", events, err)
	}
}
