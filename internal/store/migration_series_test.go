package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
	_ "modernc.org/sqlite"
)

func TestAttemptAwareSeriesMigrationPreservesLegacyData(t *testing.T) {
	path := t.TempDir() + "/jobdock.db"
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_initial.sql", "002_gpu_diagnostics_and_event_status.sql", "003_node_metadata_overrides.sql", "004_compact_resource_telemetry.sql", "005_checkpoint_syncs.sql"} {
		contents, readErr := migrations.ReadFile("migrations/" + name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = database.Exec(string(contents)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	if _, err = database.Exec(`INSERT OR IGNORE INTO schema_migrations(version,applied_at) VALUES(2,datetime('now')),(3,datetime('now')),(4,datetime('now')),(5,datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	created := "2026-08-12T10:00:00Z"
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users(id,username,password_hash,role,created_at) VALUES('user','owner','hash','member',?)`, []any{created}},
		{`INSERT INTO nodes(id,name,status,agent_version,protocol_version,architecture,docker_version,cpu_total_millis,memory_total_bytes,workspace_free_bytes,labels_json,credential_hash,credential_created_at,last_heartbeat,created_at,gpu_discovery_status) VALUES('node','node','ONLINE','test',1,'amd64','test',1000,1024,1024,'{}','credential',?,?,?,'available')`, []any{created, created, created}},
		{`INSERT INTO jobs(id,owner_id,spec_json,status,desired_status,observed_status,assigned_node_id,attempt_id,created_at) VALUES('job','user','{"name":"legacy","image":"alpine","command":["true"],"resources":{"cpu_millis":1,"memory_bytes":1,"gpu":{"count":0,"min_vram_bytes":0}}}','SUCCEEDED','SUCCEEDED','SUCCEEDED','node','attempt',?)`, []any{created}},
		{`INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES('attempt','job',1,'node','assignment','SUCCEEDED','job-token',?)`, []any{created}},
		{`INSERT INTO job_resource_samples(job_id,captured_at,resolution_seconds,sample_count,cpu_millis,memory_bytes) VALUES('job',?,5,1,750,1024)`, []any{time.Date(2026, 8, 12, 10, 0, 5, 0, time.UTC).Unix()}},
		{`INSERT INTO job_events(job_id,sequence,type,status,payload_json,created_at) VALUES('job',1,'metrics','SUCCEEDED','{"items":[{"name":"loss","value":0.25,"step":4}]}',?)`, []any{created}},
	}
	for _, statement := range statements {
		if _, err = database.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed legacy database: %v\n%s", err, statement.query)
		}
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}
	repository, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	ctx := context.Background()
	resources, _, err := repository.ResourceSamples(ctx, "job", "attempt", time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC), time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC), 5, 10)
	if err != nil || len(resources) != 1 || resources[0].AttemptID != "attempt" {
		t.Fatalf("migrated resources: %#v %v", resources, err)
	}
	metrics, _, err := repository.MetricSeries(ctx, "job", "attempt", nil, time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC), time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC), 0, 10)
	if err != nil || len(metrics) != 1 || metrics[0].Name != "loss" || metrics[0].Last != .25 {
		t.Fatalf("migrated metrics: %#v %v", metrics, err)
	}
	var descriptorCount int
	if err = repository.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM job_metric_descriptors WHERE attempt_id='attempt' AND name='loss' AND unit IS NULL AND metadata_json IS NULL AND tags_json IS NULL`).Scan(&descriptorCount); err != nil || descriptorCount != 1 {
		t.Fatalf("migrated metric descriptor: count=%d err=%v", descriptorCount, err)
	}
	if err = repository.AppendMetricSamples(ctx, []domain.MetricSample{{JobID: "job", AttemptID: "attempt", Name: "loss", Value: .2, CapturedAt: time.Date(2026, 8, 12, 10, 30, 0, 0, time.UTC), Unit: "ratio", Metadata: map[string]any{"split": "train"}, Tags: []string{"phase:train", "metric:loss"}}}); err != nil {
		t.Fatalf("enrich migrated descriptor: %v", err)
	}
	var unit, metadata, tags string
	if err = repository.DB().QueryRowContext(ctx, `SELECT unit,metadata_json,tags_json FROM job_metric_descriptors WHERE attempt_id='attempt' AND name='loss'`).Scan(&unit, &metadata, &tags); err != nil || unit != "ratio" || metadata != `{"split":"train"}` || tags != `["metric:loss","phase:train"]` {
		t.Fatalf("enriched migrated descriptor: unit=%q metadata=%q tags=%q err=%v", unit, metadata, tags, err)
	}
	events, err := repository.Events(ctx, "job", 0)
	if err != nil || len(events) != 0 {
		t.Fatalf("legacy metric event was not replaced: %#v %v", events, err)
	}
}
