package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/dockerengine"
	"github.com/jobdock/jobdock/internal/domain"
)

var version = "dev"

type Agent struct {
	config              config.Agent
	log                 *slog.Logger
	docker              *dockerengine.Client
	http                *http.Client
	credential          string
	nodeID              string
	credentialRotatedAt time.Time
	mu                  sync.Mutex
	running             map[string]*runtimeAssignment
	gpu                 GPUDiscoverer
	gpuLogOnce          sync.Once
}

type runtimeAssignment struct {
	domain.Assignment
	ContainerID   string     `json:"container_id"`
	Sequence      int64      `json:"sequence"`
	StdoutOffset  int64      `json:"stdout_offset"`
	StderrOffset  int64      `json:"stderr_offset"`
	Completed     bool       `json:"completed"`
	StopRequested bool       `json:"stop_requested"`
	mu            sync.Mutex `json:"-"`
	eventMu       sync.Mutex `json:"-"`
}

type pollResponse struct {
	Assignment *domain.Assignment `json:"assignment"`
	StopJobIDs []string           `json:"stop_job_ids"`
}

type credentialState struct {
	NodeID     string    `json:"node_id"`
	Credential string    `json:"credential"`
	RotatedAt  time.Time `json:"rotated_at"`
}

func New(cfg config.Agent, logger *slog.Logger) *Agent {
	return NewWithGPUDiscoverer(cfg, logger, newGPUDiscoverer())
}

func NewWithGPUDiscoverer(cfg config.Agent, logger *slog.Logger, gpu GPUDiscoverer) *Agent {
	return &Agent{config: cfg, log: logger, docker: dockerengine.New(cfg.DockerSocket), http: &http.Client{Timeout: 30 * time.Second}, running: map[string]*runtimeAssignment{}, gpu: gpu}
}

func (a *Agent) Run(ctx context.Context) error {
	if err := os.MkdirAll(a.config.StateDir, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(a.config.WorkspaceDir, 0o700); err != nil {
		return err
	}
	if err := a.docker.Ping(ctx); err != nil {
		return fmt.Errorf("connect to Docker Engine: %w", err)
	}
	if err := a.authenticate(ctx); err != nil {
		return err
	}
	go a.heartbeatLoop(ctx)
	if err := a.reconcile(ctx); err != nil {
		a.log.Warn("reconciliation_failed", "error", err)
	}
	for ctx.Err() == nil {
		var response pollResponse
		if err := a.apiJSON(ctx, "GET", "/api/v1/agent/assignments/next", nil, &response); err != nil {
			a.log.Warn("assignment_poll_failed", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(3 * time.Second):
				continue
			}
		}
		for _, jobID := range response.StopJobIDs {
			a.stop(ctx, jobID)
		}
		if response.Assignment != nil {
			a.startAssignment(ctx, *response.Assignment)
		}
	}
	return nil
}

func (a *Agent) authenticate(ctx context.Context) error {
	credentialPath := filepath.Join(a.config.StateDir, "credential.json")
	if a.config.Token != "" {
		a.credential = a.config.Token
	}
	if a.credential == "" {
		var saved credentialState
		if data, err := os.ReadFile(credentialPath); err == nil && json.Unmarshal(data, &saved) == nil {
			a.nodeID = saved.NodeID
			a.credential = saved.Credential
			a.credentialRotatedAt = saved.RotatedAt
		}
	}
	if a.credential != "" && a.nodeID != "" {
		return nil
	}
	if a.config.EnrollmentToken == "" {
		return errors.New("agent needs JOBDOCK_ENROLLMENT_TOKEN for first enrollment")
	}
	node, err := a.inventory(ctx)
	if err != nil {
		return err
	}
	request := map[string]any{"enrollment_token": a.config.EnrollmentToken, "node": node}
	var response struct {
		NodeID          string `json:"node_id"`
		Credential      string `json:"credential"`
		ProtocolVersion int    `json:"protocol_version"`
	}
	if err = a.apiJSONUnauthenticated(ctx, "POST", "/api/v1/agent/enroll", request, &response); err != nil {
		return fmt.Errorf("enroll agent: %w", err)
	}
	a.nodeID, a.credential = response.NodeID, response.Credential
	a.credentialRotatedAt = time.Now().UTC()
	if err = a.saveCredential(); err != nil {
		return err
	}
	a.log.Info("agent_enrolled", "node_id", a.nodeID, "name", a.config.Name)
	return nil
}

func (a *Agent) inventory(ctx context.Context) (domain.Node, error) {
	info, err := a.docker.Info(ctx)
	if err != nil {
		return domain.Node{}, err
	}
	free, total := diskSpace(a.config.WorkspaceDir)
	status := domain.NodeOnline
	minimum := int64(10 << 30)
	if tenPercent := total / 10; tenPercent > minimum {
		minimum = tenPercent
	}
	if free < minimum {
		status = domain.NodeDegraded
	}
	gpus, discovery, degraded := resolveGPUInventory(ctx, a.config.GPUMode, a.gpu)
	a.gpuLogOnce.Do(func() {
		if discovery.Status == "available" {
			a.log.Info("gpu_discovery_ready", "count", len(gpus))
		} else if discovery.Status == "unavailable" {
			a.log.Warn("gpu_discovery_unavailable", "mode", a.config.GPUMode, "code", discovery.ErrorCode, "message", discovery.Message)
		} else {
			a.log.Info("gpu_discovery_skipped", "status", discovery.Status)
		}
	})
	if degraded {
		status = domain.NodeDegraded
	}
	return domain.Node{Name: a.config.Name, Status: status, AgentVersion: version, ProtocolVersion: 1, Architecture: info.Architecture, DockerVersion: info.ServerVersion, CPUTotalMillis: int64(info.NCPU) * 1000, MemoryTotalBytes: info.MemTotal, WorkspaceFreeBytes: free, Labels: a.config.Labels, GPUs: gpus, GPUDiscovery: discovery}, nil
}

func (a *Agent) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		if time.Since(a.credentialRotatedAt) > 30*24*time.Hour {
			if err := a.rotateCredential(ctx); err != nil && ctx.Err() == nil {
				a.log.Warn("credential_rotation_failed", "error", err)
			}
		}
		node, err := a.inventory(ctx)
		if err == nil {
			node.ID = a.nodeID
			err = a.apiJSON(ctx, "POST", "/api/v1/agent/heartbeat", node, nil)
		}
		if err != nil && ctx.Err() == nil {
			a.log.Warn("heartbeat_failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *Agent) rotateCredential(ctx context.Context) error {
	var response credentialState
	if err := a.apiJSON(ctx, "POST", "/api/v1/agent/credential/rotate", map[string]any{}, &response); err != nil {
		return err
	}
	if response.RotatedAt.IsZero() {
		response.RotatedAt = time.Now().UTC()
	}
	if err := a.saveCredentialState(response); err != nil {
		return err
	}
	a.credential = response.Credential
	a.credentialRotatedAt = response.RotatedAt
	return nil
}
func (a *Agent) saveCredential() error {
	return a.saveCredentialState(credentialState{NodeID: a.nodeID, Credential: a.credential, RotatedAt: a.credentialRotatedAt})
}
func (a *Agent) saveCredentialState(state credentialState) error {
	data, _ := json.MarshalIndent(state, "", "  ")
	return os.WriteFile(filepath.Join(a.config.StateDir, "credential.json"), data, 0o600)
}

func (a *Agent) startAssignment(ctx context.Context, assignment domain.Assignment) {
	a.mu.Lock()
	if _, exists := a.running[assignment.JobID]; exists {
		a.mu.Unlock()
		return
	}
	record := &runtimeAssignment{Assignment: assignment}
	a.running[assignment.JobID] = record
	a.mu.Unlock()
	if err := a.save(record); err != nil {
		a.log.Error("persist_assignment_failed", "error", err, "job_id", assignment.JobID)
		a.removeRunning(assignment.JobID)
		return
	}
	go a.execute(ctx, record)
}

func (a *Agent) execute(ctx context.Context, record *runtimeAssignment) {
	jobDir := filepath.Join(a.config.WorkspaceDir, record.JobID)
	outputDir := filepath.Join(jobDir, "output")
	logsDir := filepath.Join(jobDir, "logs")
	secretsDir := filepath.Join(jobDir, "secrets")
	for _, dir := range []string{jobDir, logsDir, secretsDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			a.fail(record, "workspace_create_failed", err)
			return
		}
	}
	if err := os.MkdirAll(outputDir, 0o777); err != nil {
		a.fail(record, "workspace_create_failed", err)
		return
	}
	if err := os.Chmod(outputDir, 0o777); err != nil {
		a.fail(record, "workspace_permission_failed", err)
		return
	}
	tokenFile := filepath.Join(secretsDir, "job-token")
	if err := os.WriteFile(tokenFile, []byte(record.JobToken), 0o444); err != nil {
		a.fail(record, "token_write_failed", err)
		return
	}
	environment := make([]string, 0, len(record.Spec.Environment)+6)
	for key, value := range record.Spec.Environment {
		environment = append(environment, key+"="+value)
	}
	environment = append(environment, "JOBDOCK_JOB_ID="+record.JobID, "JOBDOCK_ATTEMPT_ID="+record.AttemptID, "JOBDOCK_API_URL="+a.config.ServerURL, "JOBDOCK_OUTPUT_DIR=/jobdock/output", "JOBDOCK_JOB_TOKEN_FILE=/run/secrets/jobdock/token")
	binds := []string{outputDir + ":/jobdock/output", tokenFile + ":/run/secrets/jobdock/token:ro"}
	for _, ref := range record.Spec.SecretRefs {
		value, ok := record.Secrets[ref.Name]
		if !ok {
			a.fail(record, "secret_missing", fmt.Errorf("secret %s is absent", ref.Name))
			return
		}
		if ref.Mode == "env" {
			environment = append(environment, ref.Target+"="+value)
			continue
		}
		target := safeFilename(ref.Target)
		path := filepath.Join(secretsDir, target)
		if err := os.WriteFile(path, []byte(value), 0o444); err != nil {
			a.fail(record, "secret_write_failed", err)
			return
		}
		binds = append(binds, path+":/run/secrets/jobdock/"+target+":ro")
	}
	if record.ContainerID == "" {
		if !a.sendEvent(record, "image_pull_started", domain.JobPullingImage, nil, "", "", nil) {
			return
		}
		if err := a.docker.Pull(ctx, record.Spec.Image, record.RegistryAuth); err != nil {
			a.fail(record, "image_pull_failed", err)
			return
		}
		digest := a.docker.ImageDigest(ctx, record.Spec.Image)
		if !a.sendEvent(record, "image_pull_finished", domain.JobStarting, nil, "", digest, nil) {
			return
		}
		containerID, err := a.docker.Create(ctx, dockerengine.CreateOptions{Name: "jobdock-" + strings.ReplaceAll(record.JobID, "-", "")[:12], JobID: record.JobID, AttemptID: record.AttemptID, Image: record.Spec.Image, Command: record.Spec.Command, WorkingDirectory: record.Spec.WorkingDirectory, Environment: environment, Binds: binds, CPUMillis: record.Spec.Resources.CPUMillis, MemoryBytes: record.Spec.Resources.MemoryBytes, GPUUUIDs: record.GPUUUIDs})
		if err != nil {
			a.fail(record, "container_create_failed", err)
			return
		}
		record.ContainerID = containerID
		if err = a.save(record); err != nil {
			a.fail(record, "assignment_persist_failed", err)
			return
		}
		if err = a.apiJSON(ctx, "POST", "/api/v1/agent/assignments/"+record.ID+"/accept", map[string]string{"container_id": containerID}, nil); err != nil {
			a.fail(record, "assignment_accept_failed", err)
			return
		}
		if err = a.docker.Start(ctx, containerID); err != nil {
			a.fail(record, "container_start_failed", err)
			return
		}
	}
	if !a.sendEvent(record, "container_started", domain.JobRunning, nil, "", "", nil) {
		return
	}
	logDone := make(chan struct{})
	go func() { defer close(logDone); a.streamLogs(ctx, record, logsDir) }()
	statsDone := make(chan struct{})
	go func() { defer close(statsDone); a.telemetry(ctx, record) }()
	exitCode, waitErr := a.docker.Wait(ctx, record.ContainerID)
	<-logDone
	if waitErr != nil {
		a.fail(record, "container_wait_failed", waitErr)
		return
	}
	if err := a.uploadOutputs(ctx, record, outputDir); err != nil {
		a.log.Warn("output_upload_incomplete", "error", err, "job_id", record.JobID)
		_ = a.sendEvent(record, "output_upload_warning", "", nil, err.Error(), "", map[string]any{})
	}
	status := domain.JobSucceeded
	eventType := "completed"
	reason := ""
	record.mu.Lock()
	stopRequested := record.StopRequested
	record.mu.Unlock()
	if stopRequested {
		status = domain.JobCancelled
		eventType = "cancelled"
		reason = "job was stopped by request"
	} else if exitCode != 0 {
		status = domain.JobFailed
		eventType = "failed"
		reason = fmt.Sprintf("container exited with code %d", exitCode)
	}
	_ = a.sendEvent(record, eventType, status, &exitCode, reason, "", nil)
	record.Completed = true
	_ = a.save(record)
	_ = a.docker.Remove(context.Background(), record.ContainerID)
	a.removeRunning(record.JobID)
}

func (a *Agent) streamLogs(ctx context.Context, record *runtimeAssignment, logsDir string) {
	stdoutRedactor, stderrRedactor := newRedactor(record.Secrets), newRedactor(record.Secrets)
	files := map[string]*os.File{}
	for _, stream := range []string{"stdout", "stderr"} {
		file, err := os.OpenFile(filepath.Join(logsDir, stream+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		files[stream] = file
		defer file.Close()
	}
	_ = a.docker.Logs(ctx, record.ContainerID, func(stream string, payload []byte) error {
		redactor := stdoutRedactor
		if stream == "stderr" {
			redactor = stderrRedactor
		}
		safe := redactor.Push(payload)
		if len(safe) == 0 {
			return nil
		}
		if _, err := files[stream].Write(safe); err != nil {
			return err
		}
		return a.syncLog(ctx, record, stream, filepath.Join(logsDir, stream+".log"))
	})
	for stream, redactor := range map[string]*streamRedactor{"stdout": stdoutRedactor, "stderr": stderrRedactor} {
		if tail := redactor.Flush(); len(tail) > 0 {
			_, _ = files[stream].Write(tail)
		}
		_ = a.syncLog(ctx, record, stream, filepath.Join(logsDir, stream+".log"))
	}
}

func (a *Agent) syncLog(ctx context.Context, record *runtimeAssignment, stream, path string) error {
	record.mu.Lock()
	offset := record.StdoutOffset
	if stream == "stderr" {
		offset = record.StderrOffset
	}
	record.mu.Unlock()
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	buffer := make([]byte, 256<<10)
	for {
		count, readErr := file.Read(buffer)
		if count > 0 {
			next, uploadErr := a.uploadBinary(ctx, "PUT", fmt.Sprintf("/api/v1/agent/jobs/%s/logs/%s?offset=%d", record.JobID, stream, offset), bytes.NewReader(buffer[:count]))
			if uploadErr != nil {
				return uploadErr
			}
			offset = next
			record.mu.Lock()
			if stream == "stdout" {
				record.StdoutOffset = offset
			} else {
				record.StderrOffset = offset
			}
			record.mu.Unlock()
			_ = a.save(record)
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (a *Agent) telemetry(ctx context.Context, record *runtimeAssignment) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats, err := a.docker.Stats(ctx, record.ContainerID)
			if err != nil {
				a.log.Warn("resource_sample_failed", "error", err, "job_id", record.JobID)
				continue
			}
			sample := domain.ResourceSample{CPUMillis: stats.CPUMillis, MemoryBytes: stats.MemoryBytes}
			if len(record.GPUUUIDs) > 0 {
				usage, sampleErr := a.gpu.Sample(ctx, record.GPUUUIDs)
				if sampleErr != nil {
					a.log.Warn("gpu_sample_failed", "error", sampleErr, "job_id", record.JobID)
				} else {
					sample.GPUUtilizationBasisPoints = &usage.UtilizationBasisPoints
					sample.GPUMemoryBytes = &usage.MemoryBytes
				}
			}
			uploadCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = a.apiJSON(uploadCtx, "POST", "/api/v1/agent/jobs/"+record.JobID+"/telemetry", sample, nil)
			cancel()
			if err != nil {
				a.log.Warn("resource_sample_upload_failed", "error", err, "job_id", record.JobID)
			}
		}
	}
}

func (a *Agent) uploadOutputs(ctx context.Context, record *runtimeAssignment, root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		offset := int64(0)
		buffer := make([]byte, 1<<20)
		for {
			count, readErr := file.Read(buffer)
			if count > 0 {
				endpoint := "/api/v1/agent/jobs/" + record.JobID + "/outputs/" + strings.ReplaceAll(url.PathEscape(filepath.ToSlash(relative)), "%2F", "/") + "?offset=" + strconv.FormatInt(offset, 10)
				next, uploadErr := a.uploadBinary(ctx, "PUT", endpoint, bytes.NewReader(buffer[:count]))
				if uploadErr != nil {
					return uploadErr
				}
				offset = next
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				return readErr
			}
		}
		return nil
	})
}

func (a *Agent) stop(ctx context.Context, jobID string) {
	a.mu.Lock()
	record := a.running[jobID]
	a.mu.Unlock()
	if record == nil || record.ContainerID == "" {
		return
	}
	record.mu.Lock()
	record.StopRequested = true
	record.mu.Unlock()
	_ = a.save(record)
	if err := a.docker.Stop(ctx, record.ContainerID, 30); err != nil {
		a.log.Warn("container_stop_failed", "error", err, "job_id", jobID)
	}
}

func (a *Agent) reconcile(ctx context.Context) error {
	entries, err := os.ReadDir(filepath.Join(a.config.StateDir, "assignments"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	containers, _ := a.docker.ManagedContainers(ctx)
	byJob := map[string]dockerengine.Container{}
	for _, container := range containers {
		byJob[container.Labels["jobdock.job_id"]] = container
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(a.config.StateDir, "assignments", entry.Name()))
		if readErr != nil {
			continue
		}
		record := &runtimeAssignment{}
		if json.Unmarshal(data, record) != nil || record.Completed {
			continue
		}
		if container, ok := byJob[record.JobID]; ok {
			record.ContainerID = container.ID
		}
		a.mu.Lock()
		a.running[record.JobID] = record
		a.mu.Unlock()
		go a.execute(ctx, record)
	}
	return nil
}

func (a *Agent) sendEvent(record *runtimeAssignment, eventType string, status domain.JobStatus, exitCode *int, reason, imageDigest string, payload map[string]any) bool {
	record.eventMu.Lock()
	defer record.eventMu.Unlock()
	record.mu.Lock()
	sequence := record.Sequence + 1
	record.mu.Unlock()
	body := map[string]any{"sequence": sequence, "type": eventType, "status": status, "exit_code": exitCode, "reason": reason, "image_digest": imageDigest, "payload": payload}
	var err error
	for _, delay := range []time.Duration{0, time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second} {
		if delay > 0 {
			time.Sleep(delay)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err = a.apiJSON(ctx, "POST", "/api/v1/agent/jobs/"+record.JobID+"/events", body, nil)
		cancel()
		if err == nil {
			break
		}
	}
	if err != nil {
		a.log.Warn("event_upload_failed", "error", err, "job_id", record.JobID, "event", eventType)
		return false
	}
	record.mu.Lock()
	record.Sequence = sequence
	record.mu.Unlock()
	_ = a.save(record)
	return true
}
func (a *Agent) fail(record *runtimeAssignment, eventType string, err error) {
	a.log.Error(eventType, "error", err, "job_id", record.JobID)
	_ = a.sendEvent(record, eventType, domain.JobFailed, nil, err.Error(), "", nil)
	if record.ContainerID != "" {
		_ = a.docker.Remove(context.Background(), record.ContainerID)
	}
	a.removeRunning(record.JobID)
}
func (a *Agent) save(record *runtimeAssignment) error {
	dir := filepath.Join(a.config.StateDir, "assignments")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	record.mu.Lock()
	data, err := json.MarshalIndent(record, "", "  ")
	record.mu.Unlock()
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, "assignment-*.tmp")
	if err != nil {
		return err
	}
	if err = temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(data)
	}
	closeErr := temporary.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(temporary.Name())
		return err
	}
	return os.Rename(temporary.Name(), filepath.Join(dir, record.JobID+".json"))
}
func (a *Agent) removeRunning(jobID string) { a.mu.Lock(); delete(a.running, jobID); a.mu.Unlock() }

func (a *Agent) apiJSON(ctx context.Context, method, path string, body, response any) error {
	return a.doJSON(ctx, method, path, body, response, a.credential)
}
func (a *Agent) apiJSONUnauthenticated(ctx context.Context, method, path string, body, response any) error {
	return a.doJSON(ctx, method, path, body, response, "")
}
func (a *Agent) doJSON(ctx context.Context, method, path string, body, response any, credential string) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, a.config.ServerURL+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-JobDock-Protocol-Version", "1")
	if credential != "" {
		request.Header.Set("Authorization", "Bearer "+credential)
	}
	result, err := a.http.Do(request)
	if err != nil {
		return err
	}
	defer result.Body.Close()
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return readAPIError(result)
	}
	if response != nil {
		return json.NewDecoder(result.Body).Decode(response)
	}
	return nil
}
func (a *Agent) uploadBinary(ctx context.Context, method, path string, body io.Reader) (int64, error) {
	request, err := http.NewRequestWithContext(ctx, method, a.config.ServerURL+path, body)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+a.credential)
	request.Header.Set("Content-Type", "application/octet-stream")
	result, err := a.http.Do(request)
	if err != nil {
		return 0, err
	}
	defer result.Body.Close()
	var response struct {
		NextOffset int64 `json:"next_offset"`
	}
	if decodeErr := json.NewDecoder(result.Body).Decode(&response); decodeErr != nil {
		return 0, decodeErr
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return response.NextOffset, fmt.Errorf("upload returned %s", result.Status)
	}
	return response.NextOffset, nil
}
func readAPIError(response *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	return fmt.Errorf("JobDock API %s: %s", response.Status, strings.TrimSpace(string(data)))
}
func safeFilename(value string) string {
	value = filepath.Base(value)
	value = strings.ReplaceAll(value, "..", "")
	if value == "" || value == "." {
		return "secret"
	}
	return value
}
