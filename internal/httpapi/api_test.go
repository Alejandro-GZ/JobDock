package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

func TestLoginAndIdempotentJobCreation(t *testing.T) {
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	files, _ := filestore.New(root, 1<<20, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{5}, 32))
	cfg := config.Server{AllowInsecureHTTP: true, BootstrapUsername: "admin", BootstrapPassword: "correct horse battery", SessionTTL: 24 * 60 * 60 * 1e9}
	api := New(cfg, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err = api.BootstrapAdmin(context.Background()); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	installerResponse, err := http.Get(server.URL + "/install-agent.sh")
	if err != nil {
		t.Fatal(err)
	}
	installer, _ := io.ReadAll(installerResponse.Body)
	installerResponse.Body.Close()
	if installerResponse.StatusCode != http.StatusOK || !bytes.Contains(installer, []byte("DEFAULT_VERSION=\"0.1.0\"")) {
		t.Fatalf("versioned installer endpoint: status=%d", installerResponse.StatusCode)
	}
	loginBody := bytes.NewBufferString(`{"username":"admin","password":"correct horse battery"}`)
	response, err := http.Post(server.URL+"/api/v1/auth/login", "application/json", loginBody)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("login %d: %s", response.StatusCode, data)
	}
	var session struct {
		CSRF string `json:"csrf_token"`
	}
	if err = json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	var cookie *http.Cookie
	for _, item := range response.Cookies() {
		if item.Name == "jobdock_session" {
			cookie = item
		}
	}
	if cookie == nil {
		t.Fatal("missing session cookie")
	}
	for _, path := range []string{"/jobs", "/nodes", "/secrets"} {
		request, _ := http.NewRequest("GET", server.URL+"/api/v1"+path, nil)
		request.AddCookie(cookie)
		result, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		var collection struct {
			Items json.RawMessage `json:"items"`
		}
		decodeErr := json.NewDecoder(result.Body).Decode(&collection)
		result.Body.Close()
		if result.StatusCode != http.StatusOK || decodeErr != nil || string(collection.Items) != "[]" {
			t.Fatalf("empty collection %s: status=%d items=%s error=%v", path, result.StatusCode, collection.Items, decodeErr)
		}
	}
	node := domain.Node{ID: "11111111-1111-4111-8111-111111111111", Name: "reported", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 1000, MemoryTotalBytes: 1024, WorkspaceFreeBytes: 1024, Labels: map[string]string{"source": "agent"}, LastHeartbeat: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	if err = repository.UpsertNode(context.Background(), node, "node-credential"); err != nil {
		t.Fatal(err)
	}
	metadataRequest, _ := http.NewRequest("PATCH", server.URL+"/api/v1/nodes/"+node.ID, bytes.NewBufferString(`{"name":"GPU worker","labels":{"zone":"lab"}}`))
	metadataRequest.Header.Set("Content-Type", "application/json")
	metadataRequest.Header.Set("X-CSRF-Token", session.CSRF)
	metadataRequest.AddCookie(cookie)
	metadataResponse, err := http.DefaultClient.Do(metadataRequest)
	if err != nil {
		t.Fatal(err)
	}
	metadataResponse.Body.Close()
	if metadataResponse.StatusCode != http.StatusOK {
		t.Fatalf("metadata update: %d", metadataResponse.StatusCode)
	}
	nodes, err := repository.ListNodes(context.Background())
	if err != nil || len(nodes) != 1 || nodes[0].Name != "GPU worker" || nodes[0].Labels["zone"] != "lab" {
		t.Fatalf("effective node metadata: %#v %v", nodes, err)
	}
	auditEvents, err := repository.ListAudit(context.Background(), 10)
	if err != nil || len(auditEvents) == 0 || auditEvents[0].Action != "node.metadata.update" {
		t.Fatalf("metadata audit event: %#v %v", auditEvents, err)
	}
	createMember, _ := http.NewRequest("POST", server.URL+"/api/v1/users", bytes.NewBufferString(`{"username":"member","password":"correct member battery","role":"member"}`))
	createMember.Header.Set("Content-Type", "application/json")
	createMember.Header.Set("X-CSRF-Token", session.CSRF)
	createMember.AddCookie(cookie)
	createdMember, err := http.DefaultClient.Do(createMember)
	if err != nil {
		t.Fatal(err)
	}
	createdMember.Body.Close()
	memberLogin, err := http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewBufferString(`{"username":"member","password":"correct member battery"}`))
	if err != nil {
		t.Fatal(err)
	}
	var memberSession struct {
		CSRF string `json:"csrf_token"`
	}
	if err = json.NewDecoder(memberLogin.Body).Decode(&memberSession); err != nil {
		t.Fatal(err)
	}
	memberLogin.Body.Close()
	memberPatch, _ := http.NewRequest("PATCH", server.URL+"/api/v1/nodes/"+node.ID, bytes.NewBufferString(`{"name":"forbidden","labels":{}}`))
	memberPatch.Header.Set("Content-Type", "application/json")
	memberPatch.Header.Set("X-CSRF-Token", memberSession.CSRF)
	for _, memberCookie := range memberLogin.Cookies() {
		memberPatch.AddCookie(memberCookie)
	}
	memberResult, err := http.DefaultClient.Do(memberPatch)
	if err != nil {
		t.Fatal(err)
	}
	memberResult.Body.Close()
	if memberResult.StatusCode != http.StatusForbidden {
		t.Fatalf("member metadata update status: %d", memberResult.StatusCode)
	}
	jobBody := `{"name":"test-job","image":"alpine:3","command":["echo","ok"],"resources":{"cpu_millis":100,"memory_bytes":1048576,"gpu":{"count":0,"min_vram_bytes":0}}}`
	create := func() (int, string) {
		request, _ := http.NewRequest("POST", server.URL+"/api/v1/jobs", bytes.NewBufferString(jobBody))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-CSRF-Token", session.CSRF)
		request.Header.Set("Idempotency-Key", "1234567890abcdef")
		request.AddCookie(cookie)
		result, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		defer result.Body.Close()
		data, _ := io.ReadAll(result.Body)
		return result.StatusCode, string(data)
	}
	status, first := create()
	if status != 201 {
		t.Fatalf("first create %d: %s", status, first)
	}
	status, second := create()
	if status != 201 || first != second {
		t.Fatalf("replay mismatch %d %q %q", status, first, second)
	}
	jobs, err := repository.ListJobs(context.Background(), false)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs: %d %v", len(jobs), err)
	}
	if _, err = repository.DB().Exec(`UPDATE jobs SET status='SUCCEEDED',desired_status='SUCCEEDED',observed_status='SUCCEEDED',finished_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339Nano), jobs[0].ID); err != nil {
		t.Fatal(err)
	}
	rerunRequest := func() (int, string, string) {
		request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/"+jobs[0].ID+"/rerun", nil)
		request.Header.Set("X-CSRF-Token", session.CSRF)
		request.Header.Set("Idempotency-Key", "rerun-1234567890abcdef")
		request.AddCookie(cookie)
		result, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		defer result.Body.Close()
		data, _ := io.ReadAll(result.Body)
		return result.StatusCode, string(data), result.Header.Get("Idempotency-Replayed")
	}
	firstStatus, firstRerun, _ := rerunRequest()
	secondStatus, secondRerun, replayed := rerunRequest()
	if firstStatus != http.StatusAccepted || secondStatus != http.StatusAccepted || firstRerun != secondRerun || replayed != "true" {
		t.Fatalf("rerun idempotency: first=%d second=%d replayed=%q bodies=%q/%q", firstStatus, secondStatus, replayed, firstRerun, secondRerun)
	}
	attempts, err := repository.Attempts(context.Background(), jobs[0].ID)
	if err != nil || len(attempts) != 0 {
		t.Fatalf("HTTP retry created attempts outside scheduling: %#v %v", attempts, err)
	}
	if _, err = files.AppendLog(jobs[0].ID, "stdout", 0, bytes.NewBufferString("hello")); err != nil {
		t.Fatal(err)
	}
	firstChunk := readLogSSE(t, server.Client(), server.URL+"/api/v1/jobs/"+jobs[0].ID+"/logs/stdout/tail?after=0", cookie, "")
	if firstChunk.StartOffset != 0 || firstChunk.NextOffset != 5 || string(firstChunk.Data) != "hello" {
		t.Fatalf("initial log chunk: %#v", firstChunk)
	}
	if _, err = files.AppendLog(jobs[0].ID, "stdout", 5, bytes.NewBufferString(" world")); err != nil {
		t.Fatal(err)
	}
	resumedChunk := readLogSSE(t, server.Client(), server.URL+"/api/v1/jobs/"+jobs[0].ID+"/logs/stdout/tail?after=0", cookie, "5")
	if resumedChunk.StartOffset != 5 || resumedChunk.NextOffset != 11 || string(resumedChunk.Data) != " world" {
		t.Fatalf("resumed log chunk downloaded old bytes: %#v", resumedChunk)
	}
}

func readLogSSE(t *testing.T, client *http.Client, url string, cookie *http.Cookie, lastEventID string) liveLogChunk {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	request.AddCookie(cookie)
	if lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("log stream status %d: %s", response.StatusCode, body)
	}
	scanner := bufio.NewScanner(response.Body)
	var eventType, data string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" && data != "" {
			break
		}
		if len(line) > 7 && line[:7] == "event: " {
			eventType = line[7:]
		}
		if len(line) > 6 && line[:6] == "data: " {
			data = line[6:]
		}
	}
	if err = scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if eventType != "log" || data == "" {
		t.Fatalf("unexpected SSE event type=%q data=%q", eventType, data)
	}
	var chunk liveLogChunk
	if err = json.Unmarshal([]byte(data), &chunk); err != nil {
		t.Fatal(err)
	}
	return chunk
}
