package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

func TestFirstRunSetupCreatesOnePermanentAdministrator(t *testing.T) {
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	files, _ := filestore.New(root, 1<<20, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{9}, 32))
	cfg := config.Server{AllowInsecureHTTP: true, BootstrapUsername: "admin", SetupToken: strings.Repeat("setup-token-", 4), SessionTTL: time.Hour}
	api := New(cfg, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err = api.BootstrapAdmin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if count, _ := repository.UserCount(context.Background()); count != 0 {
		t.Fatalf("setup mode unexpectedly created %d users", count)
	}
	server := httptest.NewServer(api.Handler())
	defer server.Close()

	statusResponse, err := http.Get(server.URL + "/api/v1/auth/setup")
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		Required          bool   `json:"required"`
		Enabled           bool   `json:"enabled"`
		SuggestedUsername string `json:"suggested_username"`
	}
	if err = json.NewDecoder(statusResponse.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	statusResponse.Body.Close()
	if !status.Required || !status.Enabled || status.SuggestedUsername != "admin" {
		t.Fatalf("unexpected setup status: %#v", status)
	}

	invalid, err := http.Post(server.URL+"/api/v1/auth/setup", "application/json", bytes.NewBufferString(`{"token":"wrong","username":"owner","password":"correct horse battery"}`))
	if err != nil {
		t.Fatal(err)
	}
	invalid.Body.Close()
	if invalid.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid setup token returned %d", invalid.StatusCode)
	}

	body := `{"token":"` + cfg.SetupToken + `","username":"owner","password":"correct horse battery"}`
	created, err := http.Post(server.URL+"/api/v1/auth/setup", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	var session struct {
		User struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
		CSRF string `json:"csrf_token"`
	}
	if err = json.NewDecoder(created.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	if created.StatusCode != http.StatusOK || session.User.Username != "owner" || session.User.Role != "admin" || session.CSRF == "" || len(created.Cookies()) != 1 {
		t.Fatalf("unexpected setup response: status=%d session=%#v cookies=%v", created.StatusCode, session, created.Cookies())
	}

	repeated, err := http.Post(server.URL+"/api/v1/auth/setup", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	repeated.Body.Close()
	if repeated.StatusCode != http.StatusConflict {
		t.Fatalf("reused setup token returned %d", repeated.StatusCode)
	}
	if count, _ := repository.UserCount(context.Background()); count != 1 {
		t.Fatalf("setup created %d users", count)
	}
	completedStatus, err := http.Get(server.URL + "/api/v1/auth/setup")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.NewDecoder(completedStatus.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	completedStatus.Body.Close()
	if status.Required || status.Enabled {
		t.Fatalf("completed setup remained enabled: %#v", status)
	}
	events, err := repository.ListAudit(context.Background(), 10)
	if err != nil || len(events) != 1 || events[0].Action != "user.setup" || events[0].ActorLabel != "owner" {
		t.Fatalf("setup audit event: %#v %v", events, err)
	}
}
