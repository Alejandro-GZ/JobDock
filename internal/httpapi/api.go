package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	deployassets "github.com/jobdock/jobdock/deploy"
	"github.com/jobdock/jobdock/internal/auth"
	"github.com/jobdock/jobdock/internal/buildanalysis"
	"github.com/jobdock/jobdock/internal/capacity"
	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

const protocolVersion = 1

type API struct {
	config        config.Server
	store         *store.Store
	files         *filestore.Store
	box           *secretbox.Box
	log           *slog.Logger
	webDir        string
	loginMu       sync.Mutex
	loginAttempts map[string][]time.Time
	buildAnalyzer buildanalysis.Analyzer
}

type userContextKey struct{}

func New(cfg config.Server, repository *store.Store, files *filestore.Store, box *secretbox.Box, logger *slog.Logger) *API {
	return NewWithBuildAnalyzer(cfg, repository, files, box, logger, buildanalysis.NewRailpack(cfg.RailpackBinary, cfg.BuildAnalysisTimeout).WithHome(filepath.Join(cfg.DataDir, "railpack")))
}

func NewWithBuildAnalyzer(cfg config.Server, repository *store.Store, files *filestore.Store, box *secretbox.Box, logger *slog.Logger, analyzer buildanalysis.Analyzer) *API {
	return &API{config: cfg, store: repository, files: files, box: box, log: logger, webDir: os.Getenv("JOBDOCK_WEB_DIR"), loginAttempts: map[string][]time.Time{}, buildAnalyzer: analyzer}
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := a.store.DB().PingContext(r.Context()); err != nil {
			writeProblem(w, http.StatusServiceUnavailable, "database_unavailable", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /metrics", a.metrics)
	mux.HandleFunc("GET /install-agent.sh", a.agentInstaller)
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.HandleFunc("POST /api/v1/auth/logout", a.withSession(false, true, a.logout))
	mux.HandleFunc("GET /api/v1/auth/me", a.withSession(false, false, a.me))
	mux.HandleFunc("GET /api/v1/auth/tokens", a.withSession(false, false, a.listPersonalAccessTokens))
	mux.HandleFunc("POST /api/v1/auth/tokens", a.withSession(false, true, a.createPersonalAccessToken))
	mux.HandleFunc("DELETE /api/v1/auth/tokens/{id}", a.withSession(false, true, a.revokePersonalAccessToken))
	mux.HandleFunc("GET /api/v1/users", a.withSession(true, false, a.listUsers))
	mux.HandleFunc("POST /api/v1/users", a.withSession(true, true, a.createUser))
	mux.HandleFunc("GET /api/v1/audit", a.withSession(true, false, a.listAudit))
	mux.HandleFunc("GET /api/v1/builds", a.withSession(false, false, a.listBuilds))
	mux.HandleFunc("POST /api/v1/builds", a.withSession(false, true, a.createBuild))
	mux.HandleFunc("GET /api/v1/builds/{id}", a.withSession(false, false, a.getBuild))
	mux.HandleFunc("DELETE /api/v1/builds/{id}", a.withSession(false, true, a.deleteBuild))
	mux.HandleFunc("GET /api/v1/builds/{id}/events", a.withSession(false, false, a.getBuildEvents))
	mux.HandleFunc("GET /api/v1/builds/{id}/plan", a.withSession(false, false, a.getBuildPlan))
	mux.HandleFunc("GET /api/v1/builds/{id}/logs", a.withSession(false, false, a.getBuildLogs))
	mux.HandleFunc("POST /api/v1/builds/{id}/confirm", a.withSession(false, true, a.confirmBuild))
	mux.HandleFunc("POST /api/v1/builds/{id}/cancel", a.withSession(false, true, a.cancelBuild))
	mux.HandleFunc("GET /api/v1/builder/assignments/next", a.withBuilder(a.nextBuildAssignment))
	mux.HandleFunc("GET /api/v1/builder/assignments/{id}", a.withBuilder(a.getBuildAssignment))
	mux.HandleFunc("POST /api/v1/builder/assignments/{id}/heartbeat", a.withBuilder(a.heartbeatBuildAssignment))
	mux.HandleFunc("GET /api/v1/builder/assignments/{id}/source", a.withBuilder(a.getBuildSource))
	mux.HandleFunc("PUT /api/v1/builder/assignments/{id}/logs", a.withBuilder(a.putBuildLog))
	mux.HandleFunc("PUT /api/v1/builder/assignments/{id}/artifact", a.withBuilder(a.putBuildArtifact))
	mux.HandleFunc("POST /api/v1/builder/assignments/{id}/complete", a.withBuilder(a.completeBuildAssignment))
	mux.HandleFunc("GET /api/v1/jobs", a.withAccess(scopeJobsRead, false, a.listJobs))
	mux.HandleFunc("POST /api/v1/jobs", a.withAccess(scopeJobsWrite, true, a.createJob))
	mux.HandleFunc("GET /api/v1/jobs/stream", a.withSession(false, false, a.jobsStream))
	mux.HandleFunc("GET /api/v1/jobs/{id}", a.withAccess(scopeJobsRead, false, a.getJob))
	mux.HandleFunc("GET /api/v1/jobs/{id}/attempts", a.withSession(false, false, a.listJobAttempts))
	mux.HandleFunc("POST /api/v1/jobs/{id}/rerun", a.withSession(false, true, a.rerunJob))
	mux.HandleFunc("GET /api/v1/jobs/{id}/attempts/{attempt}/archive.zip", a.withSession(false, false, a.attemptArchive))
	mux.HandleFunc("POST /api/v1/jobs/{id}/stop", a.withAccess(scopeJobsWrite, true, a.stopJob))
	mux.HandleFunc("DELETE /api/v1/jobs/{id}", a.withSession(false, true, a.deleteJob))
	mux.HandleFunc("GET /api/v1/jobs/{id}/events", a.withSession(false, false, a.jobEvents))
	mux.HandleFunc("GET /api/v1/jobs/{id}/metrics", a.withSession(false, false, a.jobMetrics))
	mux.HandleFunc("GET /api/v1/jobs/{id}/metrics/catalog", a.withSession(false, false, a.metricCatalog))
	mux.HandleFunc("GET /api/v1/jobs/{id}/dashboard", a.withSession(false, false, a.getDashboard))
	mux.HandleFunc("PUT /api/v1/jobs/{id}/dashboard", a.withSession(false, true, a.putDashboard))
	mux.HandleFunc("GET /api/v1/jobs/{id}/checkpoints", a.withSession(false, false, a.jobCheckpoints))
	mux.HandleFunc("GET /api/v1/jobs/{id}/progress", a.withSession(false, false, a.jobProgress))
	mux.HandleFunc("GET /api/v1/jobs/{id}/matrices", a.withSession(false, false, a.jobMatrices))
	mux.HandleFunc("GET /api/v1/jobs/{id}/observations/stream", a.withSession(false, false, a.jobObservationsStream))
	mux.HandleFunc("GET /api/v1/jobs/{id}/attempts/{attempt}/checkpoints/{sync}/archive.zip", a.withSession(false, false, a.checkpointArchive))
	mux.HandleFunc("GET /api/v1/jobs/{id}/resources", a.withSession(false, false, a.jobResources))
	mux.HandleFunc("GET /api/v1/jobs/{id}/series/stream", a.withSession(false, false, a.jobSeriesStream))
	mux.HandleFunc("GET /api/v1/jobs/{id}/stream", a.withSession(false, false, a.jobStream))
	mux.HandleFunc("GET /api/v1/jobs/{id}/logs/{stream}", a.withAccess(scopeLogsRead, false, a.getLogs))
	mux.HandleFunc("GET /api/v1/jobs/{id}/logs/{stream}/tail", a.withSession(false, false, a.tailLogs))
	mux.HandleFunc("GET /api/v1/jobs/{id}/archive.zip", a.withAccess(scopeArtifactsRead, false, a.archive))
	mux.HandleFunc("GET /api/v1/jobs/{id}/checkpoints/latest.zip", a.withSession(false, false, a.latestCheckpointArchive))
	mux.HandleFunc("GET /api/v1/nodes", a.withAccess(scopeNodesRead, false, a.listNodes))
	mux.HandleFunc("PATCH /api/v1/nodes/{id}", a.withSession(true, true, a.updateNodeMetadata))
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", a.withSession(true, true, a.deleteNode))
	mux.HandleFunc("POST /api/v1/nodes/enrollment-tokens", a.withSession(true, true, a.createEnrollmentToken))
	mux.HandleFunc("POST /api/v1/nodes/{id}/drain", a.withSession(true, true, a.drainNode))
	mux.HandleFunc("POST /api/v1/nodes/{id}/resume", a.withSession(true, true, a.resumeNode))
	mux.HandleFunc("GET /api/v1/secrets", a.withSession(false, false, a.listSecrets))
	mux.HandleFunc("POST /api/v1/secrets", a.withSession(false, true, a.createSecret))
	mux.HandleFunc("DELETE /api/v1/secrets/{id}", a.withSession(false, true, a.deleteSecret))
	mux.HandleFunc("POST /api/v1/agent/enroll", a.enrollAgent)
	mux.HandleFunc("POST /api/v1/agent/heartbeat", a.withAgent(a.agentHeartbeat))
	mux.HandleFunc("POST /api/v1/agent/credential/rotate", a.withAgent(a.rotateAgentCredential))
	mux.HandleFunc("GET /api/v1/agent/assignments/next", a.withAgent(a.nextAssignment))
	mux.HandleFunc("GET /api/v1/agent/assignments/{id}/artifact", a.withAgent(a.getManagedArtifact))
	mux.HandleFunc("POST /api/v1/agent/assignments/{id}/accept", a.withAgent(a.acceptAssignment))
	mux.HandleFunc("POST /api/v1/agent/jobs/{id}/events", a.withAgent(a.agentEvent))
	mux.HandleFunc("POST /api/v1/agent/jobs/{id}/telemetry", a.withAgent(a.agentTelemetry))
	mux.HandleFunc("PUT /api/v1/agent/jobs/{id}/logs/{stream}", a.withAgent(a.putLog))
	mux.HandleFunc("PUT /api/v1/agent/jobs/{id}/outputs/{path...}", a.withAgent(a.putOutput))
	mux.HandleFunc("GET /api/v1/agent/jobs/{id}/inputs/{path...}", a.withAgent(a.getInput))
	mux.HandleFunc("PUT /api/v1/agent/checkpoint-syncs/{sync}/files/{path...}", a.withAgent(a.putCheckpoint))
	mux.HandleFunc("POST /api/v1/agent/checkpoint-syncs/{sync}/complete", a.withAgent(a.completeCheckpoint))
	mux.HandleFunc("POST /api/v1/job-context/progress", a.withJob(a.sdkProgress))
	mux.HandleFunc("POST /api/v1/job-context/milestones", a.withJob(a.sdkMilestones))
	mux.HandleFunc("POST /api/v1/job-context/milestones/reached", a.withJob(a.sdkMilestoneReached))
	mux.HandleFunc("POST /api/v1/job-context/matrices", a.withJob(a.sdkMatrix))
	mux.HandleFunc("POST /api/v1/job-context/metrics", a.withJob(a.sdkMetrics))
	mux.HandleFunc("POST /api/v1/job-context/params", a.withJob(a.sdkParams))
	mux.HandleFunc("POST /api/v1/job-context/events", a.withJob(a.sdkEvents))
	mux.HandleFunc("GET /api/v1/job-context/stop", a.withJob(a.sdkStop))
	mux.HandleFunc("POST /api/v1/job-context/checkpoints", a.withJob(a.sdkCheckpoint))
	mux.HandleFunc("GET /api/v1/job-context/checkpoints/{sync}", a.withJob(a.sdkCheckpointStatus))
	mux.HandleFunc("/", a.serveWeb)
	return a.securityHeaders(a.requestLog(mux))
}

func (a *API) agentInstaller(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="install-agent.sh"`)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(deployassets.AgentInstaller)
}

func (a *API) BootstrapAdmin(ctx context.Context) error {
	count, err := a.store.UserCount(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if a.config.BootstrapPassword == "" {
		return errors.New("initial admin requires JOBDOCK_BOOTSTRAP_ADMIN_PASSWORD or password file")
	}
	hash, err := auth.HashPassword(a.config.BootstrapPassword)
	if err != nil {
		return err
	}
	user := domain.User{ID: ids.New(), Username: a.config.BootstrapUsername, Role: domain.RoleAdmin, CreatedAt: time.Now().UTC()}
	if err := a.store.CreateUser(ctx, user, hash); err != nil {
		return err
	}
	return a.store.Audit(ctx, user.ID, "user.bootstrap", "user", user.ID, map[string]any{"username": user.Username})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	client := clientAddress(r.RemoteAddr)
	if !a.allowLogin(client) {
		writeProblem(w, http.StatusTooManyRequests, "rate_limited", "Too many login attempts")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	user, hash, err := a.store.UserByUsername(r.Context(), body.Username)
	if err != nil || !auth.VerifyPassword(hash, body.Password) {
		a.recordLogin(client)
		writeProblem(w, http.StatusUnauthorized, "invalid_credentials", "Invalid username or password")
		return
	}
	token, csrf := ids.Token(32), ids.Token(24)
	expires := time.Now().UTC().Add(a.config.SessionTTL)
	if err := a.store.CreateSession(r.Context(), auth.TokenHash(token), csrf, user.ID, expires); err != nil {
		writeProblem(w, 500, "session_failed", "Unable to create session")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "jobdock_session", Value: token, Path: "/", Expires: expires, HttpOnly: true, Secure: !a.config.AllowInsecureHTTP, SameSite: http.SameSiteStrictMode})
	_ = a.store.Audit(r.Context(), user.ID, "auth.login", "user", user.ID, map[string]any{})
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "csrf_token": csrf})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("jobdock_session"); err == nil {
		_ = a.store.DeleteSession(r.Context(), auth.TokenHash(cookie.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: "jobdock_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: !a.config.AllowInsecureHTTP, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	session := r.Context().Value(userContextKey{}).(store.Session)
	writeJSON(w, 200, map[string]any{"user": session.User, "csrf_token": session.CSRFToken})
}
func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.ListUsers(r.Context())
	if err != nil {
		writeProblem(w, 500, "database_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": users})
}
func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	actor := currentUser(r)
	var body struct {
		Username, Password string
		Role               domain.Role
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Role != domain.RoleAdmin && body.Role != domain.RoleMember {
		writeProblem(w, 422, "invalid_role", "Role must be admin or member")
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	if len(body.Username) < 3 || len(body.Username) > 64 {
		writeProblem(w, 422, "invalid_username", "Username must contain between 3 and 64 characters")
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeProblem(w, 422, "invalid_password", err.Error())
		return
	}
	user := domain.User{ID: ids.New(), Username: strings.TrimSpace(body.Username), Role: body.Role, CreatedAt: time.Now().UTC()}
	if err = a.store.CreateUser(r.Context(), user, hash); err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), actor.ID, "user.create", "user", user.ID, map[string]any{"username": user.Username, "role": user.Role})
	writeJSON(w, 201, user)
}
func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListAudit(r.Context(), 200)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (a *API) createJob(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	idem, proceed := a.beginIdempotency(w, r, user.ID)
	if !proceed {
		return
	}
	jobID := ids.New()
	spec, ok := a.decodeJobRequest(w, r, jobID)
	if !ok {
		idem.abort(r.Context())
		_ = a.files.DeleteJob(jobID)
		return
	}
	if err := domain.ValidateJobSpec(spec); err != nil {
		idem.abort(r.Context())
		_ = a.files.DeleteJob(jobID)
		writeProblem(w, 422, "invalid_job_spec", err.Error())
		return
	}
	buildID, digest, managed, referenceErr := domain.ParseManagedArtifactReference(spec.Image)
	if referenceErr != nil {
		idem.abort(r.Context())
		_ = a.files.DeleteJob(jobID)
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_managed_artifact", referenceErr.Error())
		return
	}
	if managed && spec.RegistrySecret != "" {
		idem.abort(r.Context())
		_ = a.files.DeleteJob(jobID)
		writeProblem(w, http.StatusUnprocessableEntity, "managed_artifact_registry_forbidden", "Managed artifacts do not use user registry credentials")
		return
	}
	for _, ref := range spec.SecretRefs {
		if _, _, err := a.store.SecretCiphertext(r.Context(), user.ID, ref.Name); err != nil {
			idem.abort(r.Context())
			_ = a.files.DeleteJob(jobID)
			writeProblem(w, 422, "secret_not_found", fmt.Sprintf("Secret %q is not available", ref.Name))
			return
		}
	}
	if !managed && spec.RegistrySecret != "" {
		if _, kind, err := a.store.SecretCiphertext(r.Context(), user.ID, spec.RegistrySecret); err != nil || kind != "registry" {
			idem.abort(r.Context())
			_ = a.files.DeleteJob(jobID)
			writeProblem(w, 422, "registry_secret_not_found", "Registry secret is unavailable or has the wrong kind")
			return
		}
	}
	job := domain.Job{ID: jobID, OwnerID: user.ID, Spec: spec, Status: domain.JobQueued, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobQueued, CreatedAt: time.Now().UTC(), Version: 1}
	var createErr error
	if managed {
		createErr = a.store.CreateJobWithManagedArtifact(r.Context(), job, buildID, digest)
	} else {
		createErr = a.store.CreateJob(r.Context(), job)
	}
	if createErr != nil {
		idem.abort(r.Context())
		_ = a.files.DeleteJob(jobID)
		if managed && errors.Is(createErr, store.ErrNotFound) {
			writeProblem(w, http.StatusUnprocessableEntity, "managed_artifact_unavailable", "Managed artifact is unavailable or belongs to another user")
		} else {
			writeStoreError(w, createErr)
		}
		return
	}
	_ = a.store.AppendServerEvent(r.Context(), job.ID, "queued", map[string]any{})
	_ = a.store.Audit(r.Context(), user.ID, "job.create", "job", job.ID, map[string]any{"name": spec.Name})
	idem.write(w, r.Context(), 201, job)
}
func (a *API) listJobs(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	jobs, err := a.store.ListJobs(r.Context(), false)
	if err != nil {
		writeProblem(w, 500, "database_error", err.Error())
		return
	}
	statusFilter, nodeFilter, ownerFilter, nameFilter := r.URL.Query().Get("status"), r.URL.Query().Get("node"), r.URL.Query().Get("owner"), strings.ToLower(r.URL.Query().Get("name"))
	filtered := jobs[:0]
	for _, job := range jobs {
		if statusFilter != "" && string(job.Status) != statusFilter {
			continue
		}
		if nodeFilter != "" && job.AssignedNodeID != nodeFilter {
			continue
		}
		if ownerFilter != "" && job.OwnerID != ownerFilter {
			continue
		}
		if nameFilter != "" && !strings.Contains(strings.ToLower(job.Spec.Name), nameFilter) {
			continue
		}
		if user.Role != domain.RoleAdmin && job.OwnerID != user.ID {
			job.Spec = domain.JobSpec{Name: job.Spec.Name}
			job.FailureReason = ""
		}
		filtered = append(filtered, job)
	}
	writeJSON(w, 200, map[string]any{"items": filtered})
}
func (a *API) getJob(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, job)
}
func (a *API) stopJob(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	user := currentUser(r)
	idem, proceed := a.beginIdempotency(w, r, user.ID)
	if !proceed {
		return
	}
	if err := a.store.RequestStop(r.Context(), job.ID); err != nil {
		idem.abort(r.Context())
		writeStoreError(w, err)
		return
	}
	_ = a.store.AppendServerEvent(r.Context(), job.ID, "stop_requested", map[string]any{})
	_ = a.store.Audit(r.Context(), user.ID, "job.stop", "job", job.ID, map[string]any{})
	idem.write(w, r.Context(), 202, map[string]string{"status": "STOPPING"})
}
func (a *API) deleteJob(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	user := currentUser(r)
	idem, proceed := a.beginIdempotency(w, r, user.ID)
	if !proceed {
		return
	}
	if err := a.store.MarkDeleting(r.Context(), job.ID); err != nil {
		idem.abort(r.Context())
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "job.delete", "job", job.ID, map[string]any{})
	go func(id string) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := a.files.DeleteJob(id); err == nil {
			_ = a.store.MarkDeleted(ctx, id)
		}
	}(job.ID)
	idem.write(w, r.Context(), 202, map[string]string{"status": "DELETING"})
}
func (a *API) jobEvents(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	attemptID := r.URL.Query().Get("attempt_id")
	var events []domain.Event
	var err error
	if attemptID != "" {
		if _, err = a.store.Attempt(r.Context(), job.ID, attemptID); err != nil {
			writeStoreError(w, err)
			return
		}
		events, err = a.store.EventsForAttempt(r.Context(), job.ID, attemptID, after)
	} else {
		events, err = a.store.Events(r.Context(), job.ID, after)
	}
	if err != nil {
		writeProblem(w, 500, "database_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": events})
}
func (a *API) getLogs(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	stream := r.PathValue("stream")
	attemptID, ok := a.attemptIDForRequest(w, r, job)
	if !ok {
		return
	}
	offsetText := r.URL.Query().Get("offset")
	offset, _ := strconv.ParseInt(offsetText, 10, 64)
	size, err := a.files.AttemptLogSize(job.ID, attemptID, stream)
	if err != nil {
		writeProblem(w, 422, "invalid_log_stream", err.Error())
		return
	}
	if offsetText == "" && size > 256<<10 {
		offset = size - (256 << 10)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-JobDock-Start-Offset", strconv.FormatInt(offset, 10))
	w.Header().Set("X-JobDock-Next-Offset", strconv.FormatInt(size, 10))
	_, err = a.files.ReadAttemptLog(job.ID, attemptID, stream, offset, w)
	if err != nil {
		a.log.Error("read log", "error", err, "job_id", job.ID)
		return
	}
}
func (a *API) archive(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	attemptID, ok := a.attemptIDForRequest(w, r, job)
	if !ok {
		return
	}
	attempt, err := a.store.Attempt(r.Context(), job.ID, attemptID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.files.WriteAttemptMetadata(job.ID, attemptID, map[string]any{"job": job, "attempt": attempt})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="job-%s.zip"`, job.ID))
	if err := a.files.ArchiveAttempt(job.ID, attemptID, w); err != nil {
		a.log.Error("archive failed", "error", err, "job_id", job.ID)
	}
}

func (a *API) latestCheckpointArchive(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	checkpoint, err := a.store.LatestConfirmedCheckpoint(r.Context(), job.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="job-%s-checkpoint-%s.zip"`, job.ID, checkpoint.ID))
	if err = a.files.ArchiveCheckpoint(job.ID, checkpoint.ID, w); err != nil {
		a.log.Error("checkpoint archive failed", "error", err, "job_id", job.ID, "sync_id", checkpoint.ID)
	}
}

func (a *API) jobStream(w http.ResponseWriter, r *http.Request) {
	job, ok := a.authorizeJob(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, 500, "stream_unsupported", "Streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		events, err := a.store.Events(r.Context(), job.ID, after)
		if err != nil {
			return
		}
		for _, event := range events {
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "id: %d\nevent: job\ndata: %s\n\n", event.ID, data)
			after = event.ID
		}
		if len(events) == 0 {
			fmt.Fprint(w, ": keepalive\n\n")
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *API) jobsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, 500, "stream_unsupported", "Streaming is unavailable")
		return
	}
	user := currentUser(r)
	afterText := r.URL.Query().Get("after")
	after, _ := strconv.ParseInt(afterText, 10, 64)
	if afterText == "latest" {
		cursor, err := a.store.LatestJobUpdateCursorForOwner(r.Context(), user.ID)
		if err != nil {
			writeProblem(w, 500, "database_error", err.Error())
			return
		}
		after = cursor
	}
	if lastEventID := r.Header.Get("Last-Event-ID"); lastEventID != "" {
		if parsed, err := strconv.ParseInt(lastEventID, 10, 64); err == nil && parsed > after {
			after = parsed
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		updates, err := a.store.JobUpdatesForOwner(r.Context(), user.ID, after)
		if err != nil {
			return
		}
		for _, update := range updates {
			data, _ := json.Marshal(update)
			fmt.Fprintf(w, "id: %d\nevent: job-status\ndata: %s\n\n", update.Cursor, data)
			after = update.Cursor
		}
		if len(updates) == 0 {
			fmt.Fprint(w, ": keepalive\n\n")
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *API) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := capacity.Snapshot(r.Context(), a.store)
	if err != nil {
		writeProblem(w, 500, "database_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": nodes})
}
func (a *API) updateNodeMetadata(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 128 {
		writeProblem(w, 422, "invalid_node_name", "Node name must contain between 1 and 128 characters")
		return
	}
	if body.Labels == nil {
		body.Labels = map[string]string{}
	}
	if len(body.Labels) > 64 {
		writeProblem(w, 422, "invalid_node_labels", "A node can have at most 64 labels")
		return
	}
	for key, value := range body.Labels {
		if strings.TrimSpace(key) == "" || len(key) > 128 || len(value) > 256 {
			writeProblem(w, 422, "invalid_node_labels", "Label keys must contain 1 to 128 characters and values at most 256 characters")
			return
		}
	}
	id := r.PathValue("id")
	if err := a.store.UpdateNodeMetadata(r.Context(), id, body.Name, body.Labels); err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), currentUser(r).ID, "node.metadata.update", "node", id, map[string]any{"name": body.Name, "labels": body.Labels})
	writeJSON(w, 200, map[string]any{"id": id, "name": body.Name, "labels": body.Labels})
}
func (a *API) deleteNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.store.DeleteNode(r.Context(), id); err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), currentUser(r).ID, "node.delete", "node", id, map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}
func (a *API) createEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	token := ids.Token(32)
	expires := time.Now().UTC().Add(15 * time.Minute)
	if err := a.store.CreateEnrollmentToken(r.Context(), auth.TokenHash(token), user.ID, expires); err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "node.enrollment_token.create", "enrollment_token", "one_time", map[string]any{"expires_at": expires})
	writeJSON(w, 201, map[string]any{"token": token, "expires_at": expires})
}
func (a *API) drainNode(w http.ResponseWriter, r *http.Request) {
	a.setNodeStatus(w, r, domain.NodeDraining, "node.drain")
}
func (a *API) resumeNode(w http.ResponseWriter, r *http.Request) {
	a.setNodeStatus(w, r, domain.NodeOnline, "node.resume")
}
func (a *API) setNodeStatus(w http.ResponseWriter, r *http.Request, status domain.NodeStatus, action string) {
	id := r.PathValue("id")
	if err := a.store.SetNodeStatus(r.Context(), id, status); err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), currentUser(r).ID, action, "node", id, map[string]any{})
	writeJSON(w, 200, map[string]any{"status": status})
}

func (a *API) listSecrets(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListSecrets(r.Context(), currentUser(r).ID)
	if err != nil {
		writeProblem(w, 500, "database_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (a *API) createSecret(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var body struct{ Name, Value, Kind string }
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || body.Value == "" {
		writeProblem(w, 422, "invalid_secret", "Name and value are required")
		return
	}
	if body.Kind == "" {
		body.Kind = "generic"
	}
	if body.Kind != "generic" && body.Kind != "registry" {
		writeProblem(w, 422, "invalid_secret_kind", "Secret kind must be generic or registry")
		return
	}
	if body.Kind == "registry" {
		var registry struct {
			Username      string `json:"username"`
			Password      string `json:"password"`
			Auth          string `json:"auth"`
			ServerAddress string `json:"serveraddress"`
		}
		if json.Unmarshal([]byte(body.Value), &registry) != nil || registry.ServerAddress == "" || (registry.Auth == "" && (registry.Username == "" || registry.Password == "")) {
			writeProblem(w, 422, "invalid_registry_secret", "Registry secret must be Docker AuthConfig JSON with serveraddress and credentials")
			return
		}
	}
	ciphertext, err := a.box.Encrypt([]byte(body.Value), []byte(user.ID+"/"+body.Name))
	if err != nil {
		writeProblem(w, 500, "encryption_failed", "Unable to encrypt secret")
		return
	}
	now := time.Now().UTC()
	metadata := store.SecretMetadata{ID: ids.New(), OwnerID: user.ID, Name: body.Name, Kind: body.Kind, CreatedAt: now, UpdatedAt: now}
	if err = a.store.CreateSecret(r.Context(), metadata, ciphertext); err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "secret.create", "secret", metadata.ID, map[string]any{"name": body.Name, "kind": body.Kind})
	writeJSON(w, 201, metadata)
}
func (a *API) deleteSecret(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	if err := a.store.DeleteSecret(r.Context(), user.ID, id); err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "secret.delete", "secret", id, map[string]any{})
	w.WriteHeader(204)
}

func (a *API) enrollAgent(w http.ResponseWriter, r *http.Request) {
	if !compatibleProtocol(r) {
		writeProblem(w, http.StatusUpgradeRequired, "incompatible_protocol", "Agent protocol version is not supported")
		return
	}
	var body struct {
		EnrollmentToken string      `json:"enrollment_token"`
		Node            domain.Node `json:"node"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := a.store.ConsumeEnrollmentToken(r.Context(), auth.TokenHash(body.EnrollmentToken)); err != nil {
		writeProblem(w, 401, "invalid_enrollment_token", "Enrollment token is invalid, expired, or already used")
		return
	}
	credential := ids.Token(32)
	body.Node.ID = ids.New()
	body.Node.Status = domain.NodeOnline
	body.Node.ProtocolVersion = protocolVersion
	body.Node.LastHeartbeat = time.Now().UTC()
	body.Node.CreatedAt = body.Node.LastHeartbeat
	if err := a.store.UpsertNode(r.Context(), body.Node, auth.TokenHash(credential)); err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), "", "node.enroll", "node", body.Node.ID, map[string]any{"name": body.Node.Name})
	writeJSON(w, 201, map[string]any{"node_id": body.Node.ID, "credential": credential, "protocol_version": protocolVersion})
}
func (a *API) agentHeartbeat(w http.ResponseWriter, r *http.Request) {
	node := agentNode(r)
	var reported domain.Node
	if !decodeJSON(w, r, &reported) {
		return
	}
	reported.ID = node.ID
	reported.LastHeartbeat = time.Now().UTC()
	if reported.Status == "" {
		reported.Status = domain.NodeOnline
	}
	if err := a.store.Heartbeat(r.Context(), reported); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (a *API) rotateAgentCredential(w http.ResponseWriter, r *http.Request) {
	node := agentNode(r)
	credential := ids.Token(32)
	if err := a.store.RotateNodeCredential(r.Context(), node.ID, auth.TokenHash(credential)); err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.store.Audit(r.Context(), "", "node.credential.rotate", "node", node.ID, map[string]any{})
	writeJSON(w, 200, map[string]any{"node_id": node.ID, "credential": credential, "rotated_at": time.Now().UTC()})
}
func (a *API) nextAssignment(w http.ResponseWriter, r *http.Request) {
	node := agentNode(r)
	deadline := time.Now().Add(20 * time.Second)
	for {
		assignment, err := a.store.AssignmentForNode(r.Context(), node.ID)
		stops, _ := a.store.StopRequestsForNode(r.Context(), node.ID)
		checkpoints, _ := a.store.PendingCheckpointSyncsForNode(r.Context(), node.ID)
		if err == nil {
			plain, decryptErr := a.box.Decrypt(assignment.JobTokenEncrypted, []byte("assignment/"+assignment.ID))
			if decryptErr != nil {
				writeProblem(w, 500, "assignment_decryption_failed", "Unable to decrypt job credential")
				return
			}
			assignment.JobToken = string(plain)
			job, _ := a.store.Job(r.Context(), assignment.JobID)
			if _, _, managed, _ := domain.ParseManagedArtifactReference(assignment.Spec.Image); managed {
				artifact, artifactErr := a.store.ManagedArtifactForAssignment(r.Context(), assignment.ID, node.ID)
				if artifactErr != nil {
					writeProblem(w, http.StatusInternalServerError, "managed_artifact_unavailable", "Assigned managed artifact is unavailable")
					return
				}
				assignment.ManagedArtifact = &artifact
			}
			assignment.Secrets = map[string]string{}
			for _, ref := range assignment.Spec.SecretRefs {
				ciphertext, _, secretErr := a.store.SecretCiphertext(r.Context(), job.OwnerID, ref.Name)
				if secretErr != nil {
					writeProblem(w, 500, "secret_unavailable", "An assigned secret is unavailable")
					return
				}
				value, secretErr := a.box.Decrypt(ciphertext, []byte(job.OwnerID+"/"+ref.Name))
				if secretErr != nil {
					writeProblem(w, 500, "secret_decryption_failed", "Unable to decrypt assigned secret")
					return
				}
				assignment.Secrets[ref.Name] = string(value)
			}
			if assignment.Spec.RegistrySecret != "" {
				ciphertext, _, secretErr := a.store.SecretCiphertext(r.Context(), job.OwnerID, assignment.Spec.RegistrySecret)
				if secretErr != nil {
					writeProblem(w, 500, "registry_secret_unavailable", "Registry secret is unavailable")
					return
				}
				value, secretErr := a.box.Decrypt(ciphertext, []byte(job.OwnerID+"/"+assignment.Spec.RegistrySecret))
				if secretErr != nil {
					writeProblem(w, 500, "secret_decryption_failed", "Unable to decrypt registry secret")
					return
				}
				assignment.RegistryAuth = base64.RawURLEncoding.EncodeToString(value)
			}
			writeJSON(w, 200, map[string]any{"assignment": assignment, "stop_job_ids": stops, "checkpoint_syncs": checkpoints})
			return
		}
		if err != store.ErrNotFound {
			writeStoreError(w, err)
			return
		}
		if len(stops) > 0 || len(checkpoints) > 0 || time.Now().After(deadline) {
			writeJSON(w, 200, map[string]any{"assignment": nil, "stop_job_ids": stops, "checkpoint_syncs": checkpoints})
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}
func (a *API) acceptAssignment(w http.ResponseWriter, r *http.Request) {
	node := agentNode(r)
	var body struct {
		ContainerID string `json:"container_id"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := a.store.AcceptAssignment(r.Context(), node.ID, r.PathValue("id"), body.ContainerID); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (a *API) agentEvent(w http.ResponseWriter, r *http.Request) {
	node := agentNode(r)
	job, err := a.store.Job(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if job.AssignedNodeID != node.ID {
		writeProblem(w, 403, "forbidden", "Job is not assigned to this node")
		return
	}
	var body struct {
		AttemptID   string           `json:"attempt_id"`
		Sequence    int64            `json:"sequence"`
		Type        string           `json:"type"`
		Status      domain.JobStatus `json:"status"`
		ImageDigest string           `json:"image_digest"`
		ExitCode    *int             `json:"exit_code"`
		Reason      string           `json:"reason"`
		Payload     map[string]any   `json:"payload"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.AttemptID == "" || body.AttemptID != job.AttemptID {
		writeProblem(w, http.StatusConflict, "stale_attempt", "The event does not belong to the current job attempt")
		return
	}
	if body.Type == "resource_sample" {
		writeProblem(w, http.StatusUnprocessableEntity, "raw_telemetry_rejected", "Raw Docker Stats events are not accepted; upgrade the agent")
		return
	}
	event := domain.Event{JobID: job.ID, AttemptID: body.AttemptID, Sequence: body.Sequence, Type: body.Type, Status: body.Status, Payload: body.Payload, CreatedAt: time.Now().UTC()}
	if err = a.store.AppendEvent(r.Context(), event); err != nil {
		writeStoreError(w, err)
		return
	}
	if body.Status != "" {
		if err = a.store.UpdateJobStatus(r.Context(), job.ID, body.Status, body.ExitCode, body.ImageDigest, body.Reason); err != nil {
			writeStoreError(w, err)
			return
		}
		if body.Status == domain.JobSucceeded || body.Status == domain.JobFailed || body.Status == domain.JobCancelled {
			var outputs []domain.OutputFile
			if encoded, marshalErr := json.Marshal(body.Payload["outputs"]); marshalErr == nil {
				_ = json.Unmarshal(encoded, &outputs)
			}
			if err = a.store.SetAttemptOutputs(r.Context(), job.ID, body.AttemptID, outputs); err != nil {
				writeStoreError(w, err)
				return
			}
		}
	}
	w.WriteHeader(204)
}

func (a *API) agentTelemetry(w http.ResponseWriter, r *http.Request) {
	node := agentNode(r)
	job, err := a.store.Job(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if job.AssignedNodeID != node.ID {
		writeProblem(w, http.StatusForbidden, "forbidden", "Job is not assigned to this node")
		return
	}
	var sample domain.ResourceSample
	if !decodeJSON(w, r, &sample) {
		return
	}
	if sample.AttemptID == "" || sample.AttemptID != job.AttemptID {
		writeProblem(w, http.StatusConflict, "stale_attempt", "Telemetry does not belong to the current job attempt")
		return
	}
	if sample.CPUMillis < 0 || sample.MemoryBytes < 0 || (sample.GPUUtilizationBasisPoints != nil && (*sample.GPUUtilizationBasisPoints < 0 || *sample.GPUUtilizationBasisPoints > 10000)) || (sample.GPUMemoryBytes != nil && *sample.GPUMemoryBytes < 0) {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_resource_sample", "Resource telemetry values are outside their valid range")
		return
	}
	sample.JobID = job.ID
	sample.CapturedAt = time.Now().UTC()
	if err = a.store.AppendResourceSample(r.Context(), sample); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a *API) putLog(w http.ResponseWriter, r *http.Request) {
	node := agentNode(r)
	job, err := a.store.Job(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if job.AssignedNodeID != node.ID {
		writeProblem(w, 403, "forbidden", "Job is not assigned to this node")
		return
	}
	attemptID := r.URL.Query().Get("attempt_id")
	if attemptID == "" || attemptID != job.AttemptID {
		writeProblem(w, http.StatusConflict, "stale_attempt", "The log chunk does not belong to the current job attempt")
		return
	}
	offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	next, err := a.files.AppendAttemptLog(job.ID, attemptID, r.PathValue("stream"), offset, http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil && err != filestore.ErrOffsetMismatch && err != filestore.ErrLimitExceeded {
		writeProblem(w, 500, "log_write_failed", err.Error())
		return
	}
	status := 200
	if err == filestore.ErrOffsetMismatch {
		status = 409
	}
	if err == filestore.ErrLimitExceeded {
		status = 413
		_ = a.store.AppendServerEvent(r.Context(), job.ID, "log_limit_exceeded", map[string]any{})
	}
	writeJSON(w, status, map[string]any{"next_offset": next})
}
func (a *API) putOutput(w http.ResponseWriter, r *http.Request) {
	node := agentNode(r)
	job, err := a.store.Job(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if job.AssignedNodeID != node.ID {
		writeProblem(w, 403, "forbidden", "Job is not assigned to this node")
		return
	}
	attemptID := r.URL.Query().Get("attempt_id")
	if attemptID == "" || attemptID != job.AttemptID {
		writeProblem(w, http.StatusConflict, "stale_attempt", "The output chunk does not belong to the current job attempt")
		return
	}
	offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	next, err := a.files.AppendAttemptOutput(job.ID, attemptID, r.PathValue("path"), offset, http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		if err == filestore.ErrOffsetMismatch {
			writeJSON(w, 409, map[string]any{"next_offset": next})
			return
		}
		if err == filestore.ErrLimitExceeded {
			writeProblem(w, 413, "output_limit_exceeded", err.Error())
			return
		}
		writeProblem(w, 422, "invalid_output", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"next_offset": next})
}

func (a *API) checkpointForAgent(w http.ResponseWriter, r *http.Request) (domain.CheckpointSync, domain.Job, bool) {
	item, err := a.store.CheckpointSync(r.Context(), r.PathValue("sync"))
	if err != nil {
		writeStoreError(w, err)
		return item, domain.Job{}, false
	}
	job, err := a.store.Job(r.Context(), item.JobID)
	if err != nil {
		writeStoreError(w, err)
		return item, job, false
	}
	if job.AssignedNodeID != agentNode(r).ID || job.AttemptID != item.AttemptID {
		writeProblem(w, http.StatusForbidden, "forbidden", "Checkpoint is not assigned to this node")
		return item, job, false
	}
	return item, job, true
}

func (a *API) putCheckpoint(w http.ResponseWriter, r *http.Request) {
	item, job, ok := a.checkpointForAgent(w, r)
	if !ok {
		return
	}
	if item.ConfirmedAt != nil {
		writeProblem(w, http.StatusConflict, "checkpoint_confirmed", "Checkpoint is already confirmed")
		return
	}
	offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	next, err := a.files.AppendCheckpoint(job.ID, item.ID, r.PathValue("path"), offset, http.MaxBytesReader(w, r.Body, 8<<20))
	if err == filestore.ErrOffsetMismatch {
		writeJSON(w, http.StatusConflict, map[string]any{"next_offset": next})
		return
	}
	if err == filestore.ErrLimitExceeded {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"next_offset": next, "error": "checkpoint_limit_exceeded"})
		return
	}
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_checkpoint", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"next_offset": next})
}

func (a *API) completeCheckpoint(w http.ResponseWriter, r *http.Request) {
	item, job, ok := a.checkpointForAgent(w, r)
	if !ok {
		return
	}
	var body struct {
		Files []domain.CheckpointFile `json:"files"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	manifest := make(map[string]int64, len(body.Files))
	for _, file := range body.Files {
		if file.Path == "" || file.Size < 0 {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_checkpoint_manifest", "Checkpoint manifest contains an invalid file")
			return
		}
		if _, duplicate := manifest[file.Path]; duplicate {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_checkpoint_manifest", "Checkpoint manifest contains duplicate paths")
			return
		}
		manifest[file.Path] = file.Size
	}
	if item.ConfirmedAt == nil {
		if err := a.files.ConfirmCheckpoint(job.ID, item.ID, manifest); err != nil {
			writeProblem(w, http.StatusConflict, "checkpoint_incomplete", err.Error())
			return
		}
		if err := a.store.ConfirmCheckpointSync(r.Context(), item.ID, body.Files); err != nil {
			writeStoreError(w, err)
			return
		}
		observedAt := item.RequestedAt
		if item.ObservedAt != nil {
			observedAt = *item.ObservedAt
		}
		_ = a.store.AppendObservationUpdate(r.Context(), job.ID, item.AttemptID, "checkpoint", observedAt)
		_ = a.store.AppendServerEvent(r.Context(), job.ID, "checkpoint_confirmed", map[string]any{"sync_id": item.ID, "file_count": len(body.Files), "label": item.Label, "step": item.Step})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) sdkMetrics(w http.ResponseWriter, r *http.Request) { a.sdkMetricSamples(w, r) }
func (a *API) sdkParams(w http.ResponseWriter, r *http.Request)  { a.sdkRecord(w, r, "params") }
func (a *API) sdkEvents(w http.ResponseWriter, r *http.Request)  { a.sdkRecord(w, r, "sdk_event") }
func (a *API) sdkRecord(w http.ResponseWriter, r *http.Request, eventType string) {
	job := jobContext(r)
	var payload map[string]any
	if !decodeJSON(w, r, &payload) {
		return
	}
	if err := a.store.AppendServerEvent(r.Context(), job.ID, eventType, payload); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(202)
}
func (a *API) sdkStop(w http.ResponseWriter, r *http.Request) {
	job := jobContext(r)
	writeJSON(w, 200, map[string]bool{"should_stop": job.DesiredStatus == domain.JobCancelled || job.Status == domain.JobStopping})
}

func (a *API) sdkCheckpoint(w http.ResponseWriter, r *http.Request) {
	job := jobContext(r)
	if job.AttemptID == "" || !domain.IsActive(job.Status) {
		writeProblem(w, http.StatusConflict, "job_not_active", "Checkpoint sync requires an active job attempt")
		return
	}
	var observation domain.CheckpointObservation
	if !decodeJSON(w, r, &observation) {
		return
	}
	observation.Label = strings.TrimSpace(observation.Label)
	if len(observation.Label) > 128 || validateObservationMetadata(observation.Metadata) != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_checkpoint_observation", "Checkpoint label or metadata is invalid")
		return
	}
	observedAt := time.Now().UTC()
	if observation.CapturedAt != nil {
		observedAt = observation.CapturedAt.UTC()
	}
	if observedAt.After(time.Now().UTC().Add(5 * time.Minute)) {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_checkpoint_timestamp", "Checkpoint timestamps cannot be more than five minutes in the future")
		return
	}
	item := domain.CheckpointSync{ID: ids.New(), JobID: job.ID, AttemptID: job.AttemptID, Status: "PENDING", RequestedAt: time.Now().UTC(), Label: observation.Label, Step: observation.Step, ObservedAt: &observedAt, Metadata: observation.Metadata}
	if err := a.store.CreateCheckpointSync(r.Context(), item); err != nil {
		writeStoreError(w, err)
		return
	}
	_ = a.store.AppendServerEvent(r.Context(), job.ID, "checkpoint_requested", map[string]any{"sync_id": item.ID})
	writeJSON(w, http.StatusAccepted, item)
}

func (a *API) sdkCheckpointStatus(w http.ResponseWriter, r *http.Request) {
	job := jobContext(r)
	item, err := a.store.CheckpointSync(r.Context(), r.PathValue("sync"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if item.JobID != job.ID {
		writeProblem(w, http.StatusForbidden, "forbidden", "Checkpoint does not belong to this job")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *API) authorizeJob(w http.ResponseWriter, r *http.Request) (domain.Job, bool) {
	job, err := a.store.Job(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return job, false
	}
	user := currentUser(r)
	if user.Role != domain.RoleAdmin && job.OwnerID != user.ID {
		writeProblem(w, 403, "forbidden", "You do not have access to this job")
		return job, false
	}
	return job, true
}
func currentUser(r *http.Request) domain.User {
	return r.Context().Value(userContextKey{}).(store.Session).User
}

type agentContextKey struct{}

func agentNode(r *http.Request) domain.Node {
	return r.Context().Value(agentContextKey{}).(domain.Node)
}

type jobContextKey struct{}

func jobContext(r *http.Request) domain.Job { return r.Context().Value(jobContextKey{}).(domain.Job) }

func (a *API) withSession(admin, csrf bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("jobdock_session")
		if err != nil {
			writeProblem(w, 401, "unauthorized", "Authentication is required")
			return
		}
		session, err := a.store.Session(r.Context(), auth.TokenHash(cookie.Value))
		if err != nil {
			writeProblem(w, 401, "unauthorized", "Session is invalid or expired")
			return
		}
		if admin && session.User.Role != domain.RoleAdmin {
			writeProblem(w, 403, "forbidden", "Administrator access is required")
			return
		}
		if csrf && r.Header.Get("X-CSRF-Token") != session.CSRFToken {
			writeProblem(w, 403, "csrf_failed", "CSRF token is missing or invalid")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, session)))
	}
}

// withAccess accepts a browser session or a scoped personal access token. CSRF
// applies only to cookie authentication; bearer credentials are never accepted
// from cookies and therefore cannot be used for browser request forgery.
func (a *API) withAccess(requiredScope string, csrf bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token := bearer(r); token != "" {
			pat, err := a.store.PersonalAccessTokenByHash(r.Context(), auth.TokenHash(token))
			if err != nil {
				writeProblem(w, http.StatusUnauthorized, "invalid_personal_access_token", "Personal access token is invalid")
				return
			}
			if pat.RevokedAt != nil {
				writeProblem(w, http.StatusUnauthorized, "personal_access_token_revoked", "Personal access token has been revoked")
				return
			}
			if pat.ExpiresAt != nil && !pat.ExpiresAt.After(time.Now().UTC()) {
				writeProblem(w, http.StatusUnauthorized, "personal_access_token_expired", "Personal access token has expired")
				return
			}
			if !hasScope(pat.Scopes, requiredScope) {
				writeProblem(w, http.StatusForbidden, "insufficient_scope", "Personal access token does not grant the required scope")
				return
			}
			_ = a.store.TouchPersonalAccessToken(r.Context(), pat.ID, time.Now().UTC())
			session := store.Session{User: pat.User}
			next(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, session)))
			return
		}
		a.withSession(false, csrf, next)(w, r)
	}
}
func (a *API) withAgent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !compatibleProtocol(r) {
			writeProblem(w, http.StatusUpgradeRequired, "incompatible_protocol", "Agent protocol version is not supported")
			return
		}
		token := bearer(r)
		node, err := a.store.NodeByCredential(r.Context(), auth.TokenHash(token))
		if token == "" || err != nil {
			writeProblem(w, 401, "invalid_agent_credential", "Agent credential is invalid")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), agentContextKey{}, node)))
	}
}
func (a *API) withBuilder(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !compatibleProtocol(r) {
			writeProblem(w, http.StatusUpgradeRequired, "incompatible_protocol", "Builder protocol version is not supported")
			return
		}
		if a.config.BuilderToken == "" {
			writeProblem(w, http.StatusServiceUnavailable, "builder_not_configured", "The isolated builder is not configured")
			return
		}
		token := bearer(r)
		provided, expected := auth.TokenHash(token), auth.TokenHash(a.config.BuilderToken)
		if token == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			writeProblem(w, http.StatusUnauthorized, "invalid_builder_credential", "Builder credential is invalid")
			return
		}
		next(w, r)
	}
}
func compatibleProtocol(r *http.Request) bool {
	return r.Header.Get("X-JobDock-Protocol-Version") == strconv.Itoa(protocolVersion)
}
func (a *API) withJob(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r)
		job, err := a.store.JobByToken(r.Context(), auth.TokenHash(token))
		if token == "" || err != nil {
			writeProblem(w, 401, "invalid_job_credential", "Job credential is invalid")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), jobContextKey{}, job)))
	}
}
func bearer(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if !strings.HasPrefix(value, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
}

func (a *API) metrics(w http.ResponseWriter, r *http.Request) {
	jobs, _ := a.store.ListJobs(r.Context(), false)
	nodes, _ := a.store.ListNodes(r.Context())
	counts := map[domain.JobStatus]int{}
	for _, job := range jobs {
		counts[job.Status]++
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "jobdock_jobs_running %d\njobdock_jobs_queued %d\njobdock_nodes_registered %d\n", counts[domain.JobRunning], counts[domain.JobQueued], len(nodes))
}
func (a *API) serveWeb(w http.ResponseWriter, r *http.Request) {
	if a.webDir == "" {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<!doctype html><html><body><h1>JobDock</h1><p>The web bundle is not installed. Use the API under <code>/api/v1</code>.</p></body></html>`)
		return
	}
	clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	path := filepath.Join(a.webDir, clean)
	root, _ := filepath.Abs(a.webDir)
	candidate, _ := filepath.Abs(path)
	if relative, err := filepath.Rel(root, candidate); err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	if clean == "." {
		path = filepath.Join(a.webDir, "index.html")
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		path = filepath.Join(a.webDir, "index.html")
	}
	if contentType := mime.TypeByExtension(filepath.Ext(path)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeFile(w, r, path)
}
func (a *API) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; style-src 'self' 'unsafe-inline'")
		next.ServeHTTP(w, r)
	})
}
func (a *API) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		a.log.Info("http_request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds())
	})
}
func (a *API) allowLogin(remote string) bool {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	cutoff := time.Now().Add(-time.Minute)
	attempts := a.loginAttempts[remote][:0]
	for _, at := range a.loginAttempts[remote] {
		if at.After(cutoff) {
			attempts = append(attempts, at)
		}
	}
	a.loginAttempts[remote] = attempts
	return len(attempts) < 10
}
func (a *API) recordLogin(remote string) {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	a.loginAttempts[remote] = append(a.loginAttempts[remote], time.Now())
}
func clientAddress(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err == nil {
		return host
	}
	return remote
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeProblem(w, 400, "invalid_json", err.Error())
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status != 204 {
		_ = json.NewEncoder(w).Encode(value)
	}
}
func writeProblem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"type": "https://jobdock.dev/problems/" + code, "title": http.StatusText(status), "status": status, "code": code, "detail": message})
}
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, 404, "not_found", "Resource was not found")
	case errors.Is(err, store.ErrConflict):
		writeProblem(w, 409, "conflict", err.Error())
	default:
		writeProblem(w, 500, "database_error", err.Error())
	}
}

type idempotencyContext struct {
	store       *store.Store
	userID, key string
}

func (a *API) beginIdempotency(w http.ResponseWriter, r *http.Request, userID string) (*idempotencyContext, bool) {
	key := r.Header.Get("Idempotency-Key")
	result := &idempotencyContext{store: a.store, userID: userID, key: key}
	if key == "" {
		return result, true
	}
	if len(key) < 16 || len(key) > 128 {
		writeProblem(w, 422, "invalid_idempotency_key", "Idempotency-Key must contain between 16 and 128 characters")
		return result, false
	}
	cached, status, data, err := a.store.ClaimIdempotency(r.Context(), userID, key, r.Method, r.URL.Path)
	if err != nil {
		writeStoreError(w, err)
		return result, false
	}
	if cached {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Idempotency-Replayed", "true")
		w.WriteHeader(status)
		_, _ = w.Write(data)
		return result, false
	}
	return result, true
}

func (i *idempotencyContext) abort(ctx context.Context) {
	if i.key != "" {
		_ = i.store.ReleaseIdempotency(ctx, i.userID, i.key)
	}
}
func (i *idempotencyContext) write(w http.ResponseWriter, ctx context.Context, status int, value any) {
	if i.key == "" {
		writeJSON(w, status, value)
		return
	}
	data, _ := json.Marshal(value)
	data = append(data, '\n')
	if err := i.store.CompleteIdempotency(ctx, i.userID, i.key, status, data); err != nil {
		writeProblem(w, 500, "idempotency_store_failed", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
