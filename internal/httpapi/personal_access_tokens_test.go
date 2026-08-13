package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/auth"
	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

func TestPersonalAccessTokenLifecycleAndScopes(t *testing.T) {
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	files, _ := filestore.New(root, 1<<20, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{7}, 32))
	api := New(config.Server{AllowInsecureHTTP: true, BootstrapUsername: "admin", BootstrapPassword: "correct horse battery", SessionTTL: time.Hour}, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err = api.BootstrapAdmin(context.Background()); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	defer server.Close()

	login, err := http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewBufferString(`{"username":"admin","password":"correct horse battery"}`))
	if err != nil {
		t.Fatal(err)
	}
	var session struct {
		CSRF string `json:"csrf_token"`
	}
	_ = json.NewDecoder(login.Body).Decode(&session)
	login.Body.Close()
	cookie := login.Cookies()[0]
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/auth/tokens", bytes.NewBufferString(`{"name":"CLI","scopes":["nodes:read"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", session.CSRF)
	request.AddCookie(cookie)
	created, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Token               string                    `json:"token"`
		PersonalAccessToken store.PersonalAccessToken `json:"personal_access_token"`
	}
	_ = json.NewDecoder(created.Body).Decode(&result)
	created.Body.Close()
	if created.StatusCode != http.StatusCreated || result.Token == "" || result.PersonalAccessToken.Prefix == result.Token {
		t.Fatalf("create response: %d %#v", created.StatusCode, result)
	}

	call := func(path string) int {
		req, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+result.Token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if status := call("/api/v1/nodes"); status != http.StatusOK {
		t.Fatalf("nodes with PAT: %d", status)
	}
	if status := call("/api/v1/jobs"); status != http.StatusForbidden {
		t.Fatalf("scope isolation: %d", status)
	}

	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/auth/tokens", nil)
	listReq.AddCookie(cookie)
	listed, _ := http.DefaultClient.Do(listReq)
	data, _ := io.ReadAll(listed.Body)
	listed.Body.Close()
	if bytes.Contains(data, []byte(result.Token)) {
		t.Fatal("token secret was returned by list endpoint")
	}
	revoke, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/auth/tokens/"+result.PersonalAccessToken.ID, nil)
	revoke.Header.Set("X-CSRF-Token", session.CSRF)
	revoke.AddCookie(cookie)
	revoked, _ := http.DefaultClient.Do(revoke)
	revoked.Body.Close()
	if revoked.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke: %d", revoked.StatusCode)
	}
	if status := call("/api/v1/nodes"); status != http.StatusUnauthorized {
		t.Fatalf("revoked token: %d", status)
	}

	expiredSecret := "jdp_" + ids.Token(32)
	expired := store.PersonalAccessToken{ID: ids.New(), UserID: result.PersonalAccessToken.UserID, Name: "expired", Prefix: expiredSecret[:12], Scopes: []string{scopeNodesRead}, CreatedAt: time.Now().Add(-2 * time.Hour)}
	expiry := time.Now().Add(-time.Hour)
	expired.ExpiresAt = &expiry
	if err = repository.CreatePersonalAccessToken(context.Background(), expired, auth.TokenHash(expiredSecret)); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+expiredSecret)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired token: %d", resp.StatusCode)
	}
}
