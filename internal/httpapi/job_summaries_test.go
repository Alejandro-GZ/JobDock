package httpapi

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

func TestJobTelemetrySummariesAreBatchedAndOwnerScoped(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	owner := createSeriesUser(t, repository, "summary-api-owner")
	other := createSeriesUser(t, repository, "summary-api-other")
	node := domain.Node{ID: ids.New(), Name: "summary-api-node", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 4000, MemoryTotalBytes: 8 << 30, WorkspaceFreeBytes: 10 << 30, Labels: map[string]string{}, LastHeartbeat: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	if err = repository.UpsertNode(ctx, node, "credential-hash"); err != nil {
		t.Fatal(err)
	}
	create := func(user domain.User, cpu int64) (domain.Job, string) {
		job := domain.Job{ID: ids.New(), OwnerID: user.ID, Spec: domain.JobSpec{Name: "summary", Resources: domain.Resources{CPUMillis: 1000, MemoryBytes: 2048}}, Status: domain.JobRunning, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobRunning, CreatedAt: time.Now().UTC()}
		if createErr := repository.CreateJob(ctx, job); createErr != nil {
			t.Fatal(createErr)
		}
		attempt := ids.New()
		if _, createErr := repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,1,?,?,'RUNNING',?,?)`, attempt, job.ID, node.ID, ids.New(), ids.New(), time.Now().UTC().Format(time.RFC3339Nano)); createErr != nil {
			t.Fatal(createErr)
		}
		if _, createErr := repository.DB().ExecContext(ctx, `UPDATE jobs SET attempt_id=? WHERE id=?`, attempt, job.ID); createErr != nil {
			t.Fatal(createErr)
		}
		if createErr := repository.AppendResourceSample(ctx, domain.ResourceSample{JobID: job.ID, AttemptID: attempt, CapturedAt: time.Now().UTC().Truncate(time.Second), CPUMillis: cpu, MemoryBytes: 1024}); createErr != nil {
			t.Fatal(createErr)
		}
		return job, attempt
	}
	owned, attempt := create(owner, 500)
	foreign, _ := create(other, 900)
	if err = repository.AppendProgress(ctx, owned.ID, attempt, "simple", domain.ProgressObservation{Value: .65}); err != nil {
		t.Fatal(err)
	}
	files, _ := filestore.New(root, 1<<20, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{7}, 32))
	api := New(config.Server{AllowInsecureHTTP: true, SessionTTL: time.Hour}, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	client := loginSeriesUser(t, server.URL, owner.Username)
	endpoint := server.URL + "/api/v1/jobs/telemetry-summaries?points=12&job_id=" + url.QueryEscape(owned.ID) + "&job_id=" + url.QueryEscape(foreign.ID)
	var response struct {
		Cursor int64                 `json:"cursor"`
		Items  []jobTelemetrySummary `json:"items"`
	}
	getSeriesJSON(t, client, endpoint, &response)
	if response.Cursor == 0 || len(response.Items) != 1 || response.Items[0].JobID != owned.ID || len(response.Items[0].Resources) != 1 || response.Items[0].Progress == nil || *response.Items[0].Progress != .65 {
		t.Fatalf("owner-scoped summaries: %#v", response)
	}
}
