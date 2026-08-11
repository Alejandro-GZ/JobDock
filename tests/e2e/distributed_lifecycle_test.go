//go:build e2e && linux

package e2e

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/secretbox"
	_ "modernc.org/sqlite"
)

const (
	bootstrapUsername = "admin"
	bootstrapPassword = "jobdock-e2e-password"
)

type job struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	AttemptID string `json:"attempt_id"`
}

type event struct {
	Type string `json:"type"`
}

type managedProcess struct {
	command *exec.Cmd
	logFile *os.File
}

type harness struct {
	t            *testing.T
	root         string
	serverBin    string
	agentBin     string
	dockerSocket string
	serverURL    string
	server       *managedProcess
	agent        *managedProcess
	client       *http.Client
	csrf         string
	enrollment   string
	jobs         []string
	processIndex atomic.Int64
}

func TestDistributedDockerLifecycle(t *testing.T) {
	if os.Getenv("JOBDOCK_E2E") != "1" {
		t.Skip("set JOBDOCK_E2E=1 to run real-Docker tests")
	}
	h := newHarness(t)
	defer h.close()
	h.startServer()
	h.waitHealthy(20 * time.Second)
	h.login()
	h.createEnrollmentToken()
	h.startAgent()
	h.waitNodeOnline(30 * time.Second)

	h.run("submit run logs output archive", h.testHappyPath)
	h.run("SDK metrics and resource series", h.testObservableSeries)
	h.run("cooperative stop", h.testStop)
	h.run("server restart", h.testServerRestart)
	h.run("agent restart", h.testAgentRestart)
	h.run("lost reconciliation", h.testLostReconciliation)
	h.run("image pull failure", h.testPullFailure)
	h.run("output upload failure", h.testUploadFailure)
}

func (h *harness) run(name string, test func(*testing.T)) {
	h.t.Run(name, func(t *testing.T) {
		parent := h.t
		h.t = t
		defer func() { h.t = parent }()
		test(t)
	})
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	serverBin, agentBin := os.Getenv("JOBDOCK_E2E_SERVER_BIN"), os.Getenv("JOBDOCK_E2E_AGENT_BIN")
	if serverBin == "" || agentBin == "" {
		t.Fatal("JOBDOCK_E2E_SERVER_BIN and JOBDOCK_E2E_AGENT_BIN are required")
	}
	for _, path := range []*string{&serverBin, &agentBin} {
		absolute, err := filepath.Abs(*path)
		if err != nil {
			t.Fatal(err)
		}
		*path = absolute
	}
	socket := os.Getenv("JOBDOCK_E2E_DOCKER_SOCKET")
	if socket == "" {
		socket = "/var/run/docker.sock"
	}
	if _, err := os.Stat(socket); err != nil {
		t.Fatalf("Docker socket unavailable: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	jar, _ := cookiejar.New(nil)
	return &harness{t: t, root: t.TempDir(), serverBin: serverBin, agentBin: agentBin, dockerSocket: socket, serverURL: fmt.Sprintf("http://127.0.0.1:%d", port), client: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}

func (h *harness) startServer() {
	h.t.Helper()
	if h.server != nil {
		h.t.Fatal("server is already running")
	}
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(h.serverURL, "http://"))
	env := []string{
		"JOBDOCK_LISTEN_ADDR=127.0.0.1:" + port,
		"JOBDOCK_PUBLIC_URL=" + h.serverURL,
		"JOBDOCK_ALLOW_INSECURE_HTTP=true",
		"JOBDOCK_BOOTSTRAP_ADMIN_USERNAME=" + bootstrapUsername,
		"JOBDOCK_BOOTSTRAP_ADMIN_PASSWORD=" + bootstrapPassword,
		"JOBDOCK_DATA_DIR=" + filepath.Join(h.root, "server"),
		"JOBDOCK_DATABASE_PATH=" + filepath.Join(h.root, "server", "jobdock.db"),
		"JOBDOCK_HEARTBEAT_OFFLINE_AFTER=2s",
		"JOBDOCK_JOB_LOST_AFTER=3s",
		"JOBDOCK_MAX_LOG_BYTES=1048576",
		"JOBDOCK_MAX_OUTPUT_BYTES=512",
	}
	h.server = h.startProcess("server", h.serverBin, env)
}

func (h *harness) startAgent() {
	h.t.Helper()
	if h.agent != nil {
		h.t.Fatal("agent is already running")
	}
	env := []string{
		"JOBDOCK_SERVER_URL=" + h.serverURL,
		"JOBDOCK_ENROLLMENT_TOKEN=" + h.enrollment,
		"JOBDOCK_NODE_NAME=e2e-node",
		"JOBDOCK_ALLOW_INSECURE_HTTP=true",
		"JOBDOCK_GPU_MODE=disabled",
		"JOBDOCK_AGENT_STATE_DIR=" + filepath.Join(h.root, "agent-state"),
		"JOBDOCK_WORKSPACE_DIR=" + filepath.Join(h.root, "agent-workspace"),
		"JOBDOCK_DOCKER_SOCKET=" + h.dockerSocket,
	}
	h.agent = h.startProcess("agent", h.agentBin, env)
}

func (h *harness) startProcess(name, binary string, environment []string) *managedProcess {
	h.t.Helper()
	logPath := filepath.Join(h.root, fmt.Sprintf("%s-%02d.log", name, h.processIndex.Add(1)))
	logFile, err := os.Create(logPath)
	if err != nil {
		h.t.Fatal(err)
	}
	command := exec.Command(binary)
	command.Env = append(os.Environ(), environment...)
	command.Stdout, command.Stderr = logFile, logFile
	if err = command.Start(); err != nil {
		_ = logFile.Close()
		h.t.Fatal(err)
	}
	h.t.Logf("started %s pid=%d log=%s", name, command.Process.Pid, logPath)
	return &managedProcess{command: command, logFile: logFile}
}

func (h *harness) kill(process **managedProcess) {
	h.t.Helper()
	if *process == nil {
		return
	}
	p := *process
	_ = p.command.Process.Kill()
	_ = p.command.Wait()
	_ = p.logFile.Close()
	*process = nil
}

func (h *harness) close() {
	h.kill(&h.agent)
	h.kill(&h.server)
	for _, id := range h.jobs {
		_ = exec.Command("docker", "rm", "-f", "jobdock-"+strings.ReplaceAll(id, "-", "")[:12]).Run()
	}
	if h.t.Failed() {
		entries, _ := filepath.Glob(filepath.Join(h.root, "*.log"))
		for _, path := range entries {
			data, _ := os.ReadFile(path)
			h.t.Logf("diagnostic %s:\n%s", filepath.Base(path), data)
		}
	}
}

func (h *harness) waitHealthy(timeout time.Duration) {
	h.t.Helper()
	h.eventually(timeout, func() bool {
		response, err := h.client.Get(h.serverURL + "/health/ready")
		if err != nil {
			return false
		}
		_ = response.Body.Close()
		return response.StatusCode == http.StatusOK
	}, "server readiness")
}

func (h *harness) login() {
	h.t.Helper()
	var response struct {
		CSRF string `json:"csrf_token"`
	}
	h.request(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": bootstrapUsername, "password": bootstrapPassword}, &response, http.StatusOK, false)
	h.csrf = response.CSRF
}

func (h *harness) createEnrollmentToken() {
	h.t.Helper()
	var response struct {
		Token string `json:"token"`
	}
	h.request(http.MethodPost, "/api/v1/nodes/enrollment-tokens", map[string]any{}, &response, http.StatusCreated, true)
	h.enrollment = response.Token
}

func (h *harness) waitNodeOnline(timeout time.Duration) {
	h.t.Helper()
	h.eventually(timeout, func() bool {
		var response struct {
			Items []struct {
				Status string `json:"status"`
			} `json:"items"`
		}
		if h.requestOptional(http.MethodGet, "/api/v1/nodes", nil, &response, false) != nil {
			return false
		}
		return len(response.Items) == 1 && response.Items[0].Status == "ONLINE"
	}, "online agent node")
}

func (h *harness) submit(name, image string, command []string) job {
	h.t.Helper()
	payload := map[string]any{
		"name": name, "image": image, "command": command,
		"resources": map[string]any{"cpu_millis": 100, "memory_bytes": 32 << 20, "gpu": map[string]any{"count": 0, "min_vram_bytes": 0}},
	}
	var created job
	h.request(http.MethodPost, "/api/v1/jobs", payload, &created, http.StatusCreated, true)
	h.jobs = append(h.jobs, created.ID)
	return created
}

func (h *harness) currentJob(id string) (job, error) {
	var current job
	err := h.requestOptional(http.MethodGet, "/api/v1/jobs/"+id, nil, &current, false)
	return current, err
}

func (h *harness) waitJob(id string, timeout time.Duration, wanted ...string) job {
	h.t.Helper()
	wants := map[string]bool{}
	for _, status := range wanted {
		wants[status] = true
	}
	var current job
	h.eventually(timeout, func() bool {
		value, err := h.currentJob(id)
		if err != nil {
			return false
		}
		current = value
		if wants[value.Status] {
			return true
		}
		if value.Status == "FAILED" && !wants["FAILED"] {
			h.t.Fatalf("job %s failed unexpectedly", id)
		}
		return false
	}, "job "+id+" status "+strings.Join(wanted, "/"))
	return current
}

func (h *harness) release(id string) {
	h.t.Helper()
	path := filepath.Join(h.root, "agent-workspace", id, "output", "release")
	if err := os.WriteFile(path, []byte("release"), 0o666); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) testHappyPath(t *testing.T) {
	created := h.submit("e2e-happy", "alpine:3.20", []string{"sh", "-c", "echo jobdock-e2e-log; printf artifact-body > /jobdock/output/result.txt"})
	h.waitJob(created.ID, 60*time.Second, "SUCCEEDED")
	logData := h.raw(http.MethodGet, "/api/v1/jobs/"+created.ID+"/logs/stdout", nil, http.StatusOK, false)
	if !bytes.Contains(logData, []byte("jobdock-e2e-log")) {
		t.Fatalf("stdout does not contain marker: %q", logData)
	}
	archiveData := h.raw(http.MethodGet, "/api/v1/jobs/"+created.ID+"/archive.zip", nil, http.StatusOK, false)
	archive, err := zip.NewReader(bytes.NewReader(archiveData), int64(len(archiveData)))
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]string{"logs/stdout.log": "jobdock-e2e-log", "output/result.txt": "artifact-body", "metadata/job.json": created.ID}
	for _, file := range archive.File {
		expected, ok := wanted[file.Name]
		if !ok {
			continue
		}
		reader, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		data, _ := io.ReadAll(reader)
		_ = reader.Close()
		if !bytes.Contains(data, []byte(expected)) {
			t.Errorf("archive %s does not contain %q", file.Name, expected)
		}
		delete(wanted, file.Name)
	}
	if len(wanted) != 0 {
		t.Fatalf("archive entries missing: %v", wanted)
	}
}

func (h *harness) testObservableSeries(t *testing.T) {
	created := h.submit("e2e-observable-series", "alpine:3.20", []string{"sh", "-c", "i=0; while [ $i -lt 12 ]; do i=$((i+1)); dd if=/dev/zero of=/dev/null bs=1M count=8 2>/dev/null; sleep 1; done"})
	h.waitJob(created.ID, 30*time.Second, "RUNNING")
	token := h.jobToken(created.ID)
	payload := []byte(`{"items":[{"name":"loss","value":0.75,"step":1},{"name":"accuracy","value":0.8,"step":1}]}`)
	request, _ := http.NewRequest(http.MethodPost, h.serverURL+"/api/v1/job-context/metrics", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := h.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("metric ingestion returned %d", response.StatusCode)
	}
	finished := h.waitJob(created.ID, 60*time.Second, "SUCCEEDED")
	var metrics struct {
		AttemptID string `json:"attempt_id"`
		Series    []struct {
			Name   string `json:"name"`
			Points []any  `json:"points"`
		} `json:"series"`
	}
	h.request(http.MethodGet, "/api/v1/jobs/"+created.ID+"/metrics?resolution=raw", nil, &metrics, http.StatusOK, false)
	if metrics.AttemptID != finished.AttemptID || len(metrics.Series) != 2 || len(metrics.Series[0].Points) == 0 {
		t.Fatalf("SDK metric series missing: %#v", metrics)
	}
	var resources struct {
		AttemptID string `json:"attempt_id"`
		Points    []struct {
			SampleCount int64 `json:"sample_count"`
			CPUMillis   int64 `json:"cpu_millis"`
		} `json:"points"`
	}
	h.request(http.MethodGet, "/api/v1/jobs/"+created.ID+"/resources?resolution=5s", nil, &resources, http.StatusOK, false)
	if resources.AttemptID != finished.AttemptID || len(resources.Points) == 0 || resources.Points[0].SampleCount < 1 {
		t.Fatalf("resource series missing: %#v", resources)
	}
	for _, endpoint := range []string{"metrics", "resources"} {
		data := h.raw(http.MethodGet, "/api/v1/jobs/"+created.ID+"/"+endpoint+"?format=csv", nil, http.StatusOK, false)
		if !bytes.Contains(data, []byte(finished.AttemptID)) {
			t.Fatalf("%s CSV is missing attempt identity: %s", endpoint, data)
		}
	}
}

func (h *harness) jobToken(jobID string) string {
	h.t.Helper()
	database, err := sql.Open("sqlite", filepath.Join(h.root, "server", "jobdock.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		h.t.Fatal(err)
	}
	defer database.Close()
	var assignmentID string
	var ciphertext []byte
	if err = database.QueryRow(`SELECT id,job_token_ciphertext FROM assignments WHERE job_id=? ORDER BY created_at DESC LIMIT 1`, jobID).Scan(&assignmentID, &ciphertext); err != nil {
		h.t.Fatal(err)
	}
	encodedKey, err := os.ReadFile(filepath.Join(h.root, "server", "master.key"))
	if err != nil {
		h.t.Fatal(err)
	}
	key, err := base64.StdEncoding.DecodeString(string(encodedKey))
	if err != nil {
		h.t.Fatal(err)
	}
	box, err := secretbox.New(key)
	if err != nil {
		h.t.Fatal(err)
	}
	plaintext, err := box.Decrypt(ciphertext, []byte("assignment/"+assignmentID))
	if err != nil {
		h.t.Fatal(err)
	}
	return string(plaintext)
}

func (h *harness) testStop(t *testing.T) {
	created := h.submit("e2e-stop", "alpine:3.20", []string{"sleep", "300"})
	h.waitJob(created.ID, 30*time.Second, "RUNNING")
	h.request(http.MethodPost, "/api/v1/jobs/"+created.ID+"/stop", map[string]any{}, nil, http.StatusAccepted, true)
	h.waitJob(created.ID, 45*time.Second, "CANCELLED")
}

func (h *harness) testServerRestart(t *testing.T) {
	created := h.submit("e2e-server-restart", "alpine:3.20", []string{"sh", "-c", "while [ ! -f /jobdock/output/release ]; do sleep 1; done; echo server-recovered"})
	running := h.waitJob(created.ID, 30*time.Second, "RUNNING")
	h.kill(&h.server)
	h.startServer()
	h.waitHealthy(20 * time.Second)
	h.release(created.ID)
	finished := h.waitJob(created.ID, 60*time.Second, "SUCCEEDED")
	if finished.AttemptID != running.AttemptID {
		t.Fatalf("server restart changed attempt: %s -> %s", running.AttemptID, finished.AttemptID)
	}
}

func (h *harness) testAgentRestart(t *testing.T) {
	created := h.submit("e2e-agent-restart", "alpine:3.20", []string{"sh", "-c", "while [ ! -f /jobdock/output/release ]; do sleep 1; done; echo agent-recovered"})
	running := h.waitJob(created.ID, 30*time.Second, "RUNNING")
	h.kill(&h.agent)
	h.startAgent()
	h.waitNodeOnline(20 * time.Second)
	h.release(created.ID)
	finished := h.waitJob(created.ID, 60*time.Second, "SUCCEEDED")
	if finished.AttemptID != running.AttemptID {
		t.Fatalf("agent restart changed attempt: %s -> %s", running.AttemptID, finished.AttemptID)
	}
}

func (h *harness) testLostReconciliation(t *testing.T) {
	created := h.submit("e2e-lost", "alpine:3.20", []string{"sh", "-c", "while [ ! -f /jobdock/output/release ]; do sleep 1; done; echo lost-reconciled"})
	running := h.waitJob(created.ID, 30*time.Second, "RUNNING")
	h.kill(&h.agent)
	lost := h.waitJob(created.ID, 18*time.Second, "LOST")
	if lost.AttemptID != running.AttemptID {
		t.Fatalf("LOST changed attempt: %s -> %s", running.AttemptID, lost.AttemptID)
	}
	h.startAgent()
	h.waitNodeOnline(20 * time.Second)
	reconciled := h.waitJob(created.ID, 30*time.Second, "RUNNING")
	if reconciled.AttemptID != running.AttemptID {
		t.Fatalf("reconciliation duplicated attempt: %s -> %s", running.AttemptID, reconciled.AttemptID)
	}
	h.release(created.ID)
	h.waitJob(created.ID, 60*time.Second, "SUCCEEDED")
}

func (h *harness) testPullFailure(t *testing.T) {
	created := h.submit("e2e-pull-failure", "alpine:jobdock-tag-does-not-exist", []string{"true"})
	h.waitJob(created.ID, 60*time.Second, "FAILED")
	events := h.events(created.ID)
	if !containsEvent(events, "image_pull_failed") {
		t.Fatalf("pull failure event missing: %#v", events)
	}
}

func (h *harness) testUploadFailure(t *testing.T) {
	created := h.submit("e2e-upload-failure", "alpine:3.20", []string{"sh", "-c", "head -c 2048 /dev/zero > /jobdock/output/too-large.bin; echo workload-survived"})
	h.waitJob(created.ID, 60*time.Second, "SUCCEEDED")
	events := h.events(created.ID)
	if !containsEvent(events, "output_upload_warning") {
		t.Fatalf("upload warning event missing: %#v", events)
	}
}

func (h *harness) events(id string) []event {
	h.t.Helper()
	var response struct {
		Items []event `json:"items"`
	}
	h.request(http.MethodGet, "/api/v1/jobs/"+id+"/events", nil, &response, http.StatusOK, false)
	return response.Items
}

func containsEvent(items []event, wanted string) bool {
	for _, item := range items {
		if item.Type == wanted {
			return true
		}
	}
	return false
}

func (h *harness) eventually(timeout time.Duration, check func() bool, description string) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	h.t.Fatalf("timed out waiting for %s", description)
}

func (h *harness) request(method, path string, body, destination any, status int, csrf bool) {
	h.t.Helper()
	responseBody := h.raw(method, path, body, status, csrf)
	if destination != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, destination); err != nil {
			h.t.Fatalf("decode %s %s: %v\n%s", method, path, err, responseBody)
		}
	}
}

func (h *harness) requestOptional(method, path string, body, destination any, csrf bool) error {
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, h.serverURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if csrf {
		request.Header.Set("X-CSRF-Token", h.csrf)
		request.Header.Set("Idempotency-Key", fmt.Sprintf("e2e-%d-%d", time.Now().UnixNano(), h.processIndex.Add(1)))
	}
	response, err := h.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s %s returned %d: %s", method, path, response.StatusCode, data)
	}
	if destination != nil && len(data) > 0 {
		return json.Unmarshal(data, destination)
	}
	return nil
}

func (h *harness) raw(method, path string, body any, status int, csrf bool) []byte {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequest(method, h.serverURL+path, reader)
	if err != nil {
		h.t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if csrf {
		request.Header.Set("X-CSRF-Token", h.csrf)
		request.Header.Set("Idempotency-Key", fmt.Sprintf("e2e-%d-%d", time.Now().UnixNano(), h.processIndex.Add(1)))
	}
	response, err := h.client.Do(request)
	if err != nil {
		h.t.Fatal(err)
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		h.t.Fatal(readErr)
	}
	if response.StatusCode != status {
		h.t.Fatalf("%s %s returned %d, want %d: %s", method, path, response.StatusCode, status, data)
	}
	return data
}
