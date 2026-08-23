package store

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMultiDashboardMigrationPreservesLegacyPreference(t *testing.T) {
	path := t.TempDir() + "/jobdock.db"
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for version := 1; version <= 21; version++ {
		matches, globErr := migrations.ReadDir("migrations")
		if globErr != nil {
			t.Fatal(globErr)
		}
		prefix := fmt.Sprintf("%03d_", version)
		name := ""
		for _, entry := range matches {
			if len(entry.Name()) >= len(prefix) && entry.Name()[:len(prefix)] == prefix {
				name = entry.Name()
				break
			}
		}
		if name == "" {
			t.Fatalf("migration %d not found", version)
		}
		contents, readErr := migrations.ReadFile("migrations/" + name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = database.Exec(string(contents)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		if _, err = database.Exec(`INSERT OR IGNORE INTO schema_migrations(version,applied_at) VALUES(?,?)`, version, "2026-08-12T10:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = database.Exec(`INSERT INTO users(id,username,password_hash,role,created_at) VALUES('user','owner','hash','member','2026-08-12T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec(`INSERT INTO jobs(id,owner_id,spec_json,status,desired_status,observed_status,created_at) VALUES('job','user','{"name":"legacy","image":"alpine","command":["true"],"resources":{"cpu_millis":1,"memory_bytes":1,"gpu":{"count":0,"min_vram_bytes":0}}}','QUEUED','RUNNING','QUEUED','2026-08-12T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec(`INSERT INTO dashboard_preferences(user_id,job_id,schema_version,config_json,template_id,template_version,template_schema_version,template_applied_at,updated_at) VALUES('user','job',1,'{"widgets":[]}','training-general',2,1,'2026-08-12T10:01:00Z','2026-08-12T10:01:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}

	repository, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	item, err := repository.DashboardPreference(context.Background(), "user", "job")
	if err != nil {
		t.Fatal(err)
	}
	if item.Name != "Dashboard" || !item.IsDefault || string(item.ConfigJSON) != `{"widgets":[]}` || item.TemplateID != "training-general" || item.TemplateVersion != 2 {
		t.Fatalf("migrated dashboard: %#v", item)
	}
	items, activeID, err := repository.ListDashboards(context.Background(), "user", "job")
	if err != nil || len(items) != 1 || activeID != item.ID {
		t.Fatalf("migrated dashboard list: %#v active=%q err=%v", items, activeID, err)
	}
}
