package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/auth"
	"github.com/jobdock/jobdock/internal/buildanalysis"
	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

type fakeBuildAnalyzer struct {
	result buildanalysis.Result
	err    error
}

func (f fakeBuildAnalyzer) Analyze(context.Context, string) (buildanalysis.Result, error) {
	return f.result, f.err
}

func TestSourceBuildAPIIsReproducibleAuthorizedAndIndependentFromJobs(t *testing.T) {
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	files, err := filestore.New(root, 1<<20, 1<<20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	box, _ := secretbox.New(bytes.Repeat([]byte{7}, 32))
	analyzer := fakeBuildAnalyzer{result: buildanalysis.Result{Provider: "node", Runtime: "node", PackageManager: "npm", Entrypoint: "npm run start", RailpackVersion: "0.36.0", Plan: json.RawMessage(`{"deploy":{"startCommand":"npm run start"}}`), Info: json.RawMessage(`{"success":true,"detectedProviders":["node"]}`), Logs: []byte("Railpack detected Node with npm")}}
	api := NewWithBuildAnalyzer(config.Server{AllowInsecureHTTP: true, SessionTTL: time.Hour, MaxInputBytes: 1 << 20}, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil)), analyzer)
	server := httptest.NewServer(api.Handler())
	defer server.Close()

	createUser := func(username string) domain.User {
		hash, hashErr := auth.HashPassword("correct member battery")
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		user := domain.User{ID: ids.New(), Username: username, Role: domain.RoleMember, CreatedAt: time.Now().UTC()}
		if createErr := repository.CreateUser(context.Background(), user, hash); createErr != nil {
			t.Fatal(createErr)
		}
		return user
	}
	owner, other := createUser("build-owner"), createUser("other-user")
	login := func(username string) (*http.Cookie, string) {
		response, loginErr := http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewBufferString(`{"username":"`+username+`","password":"correct member battery"}`))
		if loginErr != nil {
			t.Fatal(loginErr)
		}
		defer response.Body.Close()
		var session struct {
			CSRF string `json:"csrf_token"`
		}
		if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&session) != nil {
			t.Fatalf("login %s returned %d", username, response.StatusCode)
		}
		for _, cookie := range response.Cookies() {
			if cookie.Name == "jobdock_session" {
				return cookie, session.CSRF
			}
		}
		t.Fatal("missing session cookie")
		return nil, ""
	}
	ownerCookie, ownerCSRF := login(owner.Username)
	otherCookie, _ := login(other.Username)

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	metadata, _ := writer.CreateFormField("metadata")
	_, _ = metadata.Write([]byte(`{"name":"training source","mode":"RAILPACK"}`))
	var project bytes.Buffer
	projectArchive := zip.NewWriter(&project)
	manifest, _ := projectArchive.Create("project/package.json")
	_, _ = manifest.Write([]byte(`{"name":"example","scripts":{"start":"node index.js"}}`))
	_ = projectArchive.Close()
	source, _ := writer.CreateFormFile("source", "project.zip")
	_, _ = source.Write(project.Bytes())
	_ = writer.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/builds", &requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-CSRF-Token", ownerCSRF)
	request.Header.Set("Idempotency-Key", "build-create-1234567890")
	request.AddCookie(ownerCookie)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var build domain.Build
	decodeErr := json.NewDecoder(response.Body).Decode(&build)
	response.Body.Close()
	if response.StatusCode != http.StatusCreated || decodeErr != nil || build.Status != domain.BuildAnalyzing || build.Source.Size != int64(project.Len()) || len(build.Source.SHA256) != 64 {
		t.Fatalf("create build status=%d build=%#v error=%v", response.StatusCode, build, decodeErr)
	}
	planRequest, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/builds/"+build.ID+"/plan", nil)
	planRequest.AddCookie(ownerCookie)
	planResponse, err := server.Client().Do(planRequest)
	if err != nil {
		t.Fatal(err)
	}
	var plan domain.BuildPlan
	_ = json.NewDecoder(planResponse.Body).Decode(&plan)
	planResponse.Body.Close()
	if planResponse.StatusCode != http.StatusOK || plan.Provider != "node" || plan.PackageManager != "npm" || plan.Entrypoint != "npm run start" || plan.ConfirmedAt != nil {
		t.Fatalf("detected plan status=%d plan=%#v", planResponse.StatusCode, plan)
	}
	confirm, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/builds/"+build.ID+"/confirm", nil)
	confirm.Header.Set("X-CSRF-Token", ownerCSRF)
	confirm.Header.Set("Idempotency-Key", "build-confirm-123456789")
	confirm.AddCookie(ownerCookie)
	confirmResponse, err := server.Client().Do(confirm)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(confirmResponse.Body).Decode(&plan)
	confirmResponse.Body.Close()
	if confirmResponse.StatusCode != http.StatusOK || plan.ConfirmedAt == nil {
		t.Fatalf("confirm status=%d plan=%#v", confirmResponse.StatusCode, plan)
	}

	forbidden, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/builds/"+build.ID, nil)
	forbidden.AddCookie(otherCookie)
	forbiddenResponse, err := server.Client().Do(forbidden)
	if err != nil {
		t.Fatal(err)
	}
	forbiddenResponse.Body.Close()
	if forbiddenResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-owner build status=%d", forbiddenResponse.StatusCode)
	}
	logs, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/builds/"+build.ID+"/logs?offset=0&limit=5", nil)
	logs.AddCookie(ownerCookie)
	logResponse, err := server.Client().Do(logs)
	if err != nil {
		t.Fatal(err)
	}
	logData, _ := io.ReadAll(logResponse.Body)
	logResponse.Body.Close()
	if logResponse.StatusCode != http.StatusOK || string(logData) != "Railp" || logResponse.Header.Get("X-JobDock-Next-Offset") != "5" {
		t.Fatalf("build logs status=%d body=%q headers=%v", logResponse.StatusCode, logData, logResponse.Header)
	}

	cancel, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/builds/"+build.ID+"/cancel", nil)
	cancel.Header.Set("X-CSRF-Token", ownerCSRF)
	cancel.Header.Set("Idempotency-Key", "build-cancel-123456789")
	cancel.AddCookie(ownerCookie)
	cancelResponse, err := server.Client().Do(cancel)
	if err != nil {
		t.Fatal(err)
	}
	var cancelled domain.Build
	_ = json.NewDecoder(cancelResponse.Body).Decode(&cancelled)
	cancelResponse.Body.Close()
	if cancelResponse.StatusCode != http.StatusAccepted || cancelled.Status != domain.BuildCancelled || cancelled.FinishedAt == nil {
		t.Fatalf("cancel build status=%d build=%#v", cancelResponse.StatusCode, cancelled)
	}
	events, err := repository.BuildEvents(context.Background(), build.ID)
	if err != nil || len(events) != 3 || events[0].Status != domain.BuildCreated || events[1].Status != domain.BuildAnalyzing || events[2].Status != domain.BuildCancelled {
		t.Fatalf("build events=%#v error=%v", events, err)
	}
	jobs, err := repository.ListJobs(context.Background(), false)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("source build created a job: %#v %v", jobs, err)
	}
}
