package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/jobdock/jobdock/internal/domain"
)

//go:embed migrations/*.sql
var migrations embed.FS

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrRawTelemetry = errors.New("raw resource telemetry is not persisted")
var ErrMetricDescriptorConflict = errors.New("metric descriptor conflicts with the existing series")
var ErrObservableDeclarationConflict = errors.New("observable source declaration conflicts with the existing descriptor")

type Store struct {
	db *sql.DB
}

type Session struct {
	User      domain.User
	CSRFToken string
	ExpiresAt time.Time
}

type SecretMetadata struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"owner_id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AuditEvent struct {
	ID              int64          `json:"id"`
	ActorID         string         `json:"actor_id,omitempty"`
	ActorLabel      string         `json:"actor_label,omitempty"`
	Action          string         `json:"action"`
	TargetType      string         `json:"target_type"`
	TargetID        string         `json:"target_id"`
	TargetLabel     string         `json:"target_label,omitempty"`
	TargetAvailable bool           `json:"target_available"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       time.Time      `json:"created_at"`
}

type AuditFilter struct {
	Before     int64
	Limit      int
	Category   string
	ActorID    string
	TargetType string
	From       *time.Time
	To         *time.Time
	Query      string
}

type AuditPage struct {
	Items      []AuditEvent `json:"items"`
	NextCursor int64        `json:"next_cursor,omitempty"`
}

type JobUpdate struct {
	Cursor    int64            `json:"cursor"`
	JobID     string           `json:"job_id"`
	Name      string           `json:"name"`
	Status    domain.JobStatus `json:"status"`
	Version   int64            `json:"version"`
	CreatedAt time.Time        `json:"created_at"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, parseErr := strconv.Atoi(strings.SplitN(entry.Name(), "_", 2)[0])
		if parseErr != nil {
			return fmt.Errorf("invalid migration name %s", entry.Name())
		}
		var applied int
		if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version=?`, version).Scan(&applied); err != nil {
			return err
		}
		if applied > 0 {
			continue
		}
		contents, readErr := migrations.ReadFile("migrations/" + entry.Name())
		if readErr != nil {
			return readErr
		}
		tx, beginErr := s.db.BeginTx(ctx, nil)
		if beginErr != nil {
			return beginErr
		}
		if _, err = tx.ExecContext(ctx, string(contents)); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version,applied_at) VALUES(?,?)`, version, formatTime(time.Now().UTC()))
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var count int
	return count, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
}

func (s *Store) CreateUser(ctx context.Context, user domain.User, passwordHash string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO users(id, username, password_hash, role, created_at) VALUES(?,?,?,?,?)`, user.ID, user.Username, passwordHash, user.Role, formatTime(user.CreatedAt))
	return mapConstraint(err)
}

// CreateInitialAdmin atomically creates the first account. The conditional
// insert makes a setup token single-use even when concurrent requests race.
func (s *Store) CreateInitialAdmin(ctx context.Context, user domain.User, passwordHash string) error {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO users(id, username, password_hash, role, created_at)
		SELECT ?,?,?,?,?
		WHERE NOT EXISTS (SELECT 1 FROM users)`, user.ID, user.Username, passwordHash, user.Role, formatTime(user.CreatedAt))
	if err != nil {
		return mapConstraint(err)
	}
	created, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if created != 1 {
		return ErrConflict
	}
	return nil
}

func (s *Store) UserByUsername(ctx context.Context, username string) (domain.User, string, error) {
	var user domain.User
	var hash, created string
	err := s.db.QueryRowContext(ctx, `SELECT id, username, role, password_hash, created_at FROM users WHERE username = ? COLLATE NOCASE`, username).Scan(&user.ID, &user.Username, &user.Role, &hash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return user, "", ErrNotFound
	}
	user.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return user, hash, err
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.username, u.role, u.created_at,
			(SELECT MAX(a.created_at) FROM audit_events a WHERE a.actor_id=u.id),
			(SELECT COUNT(*) FROM jobs j WHERE j.owner_id=u.id AND j.status='RUNNING')
		FROM users u ORDER BY u.username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]domain.User, 0)
	for rows.Next() {
		var user domain.User
		var created string
		var lastSeen sql.NullString
		if err := rows.Scan(&user.ID, &user.Username, &user.Role, &created, &lastSeen, &user.JobsRunning); err != nil {
			return nil, err
		}
		user.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if lastSeen.Valid {
			parsed, err := time.Parse(time.RFC3339Nano, lastSeen.String)
			if err == nil {
				user.LastSeenAt = &parsed
			}
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) CreateSession(ctx context.Context, tokenHash, csrfToken, userID string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at,created_at) VALUES(?,?,?,?,?)`, tokenHash, userID, csrfToken, formatTime(expiresAt), formatTime(time.Now().UTC()))
	return err
}

func (s *Store) Session(ctx context.Context, tokenHash string) (Session, error) {
	var session Session
	var created, expires string
	err := s.db.QueryRowContext(ctx, `SELECT u.id,u.username,u.role,u.created_at,s.csrf_token,s.expires_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>?`, tokenHash, formatTime(time.Now().UTC())).Scan(&session.User.ID, &session.User.Username, &session.User.Role, &created, &session.CSRFToken, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return session, ErrNotFound
	}
	session.User.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	session.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expires)
	return session, err
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, tokenHash)
	return err
}

func (s *Store) CreateJob(ctx context.Context, job domain.Job) error {
	spec, _ := json.Marshal(job.Spec)
	_, err := s.db.ExecContext(ctx, `INSERT INTO jobs(id,owner_id,spec_json,status,desired_status,observed_status,created_at,version) VALUES(?,?,?,?,?,?,?,1)`, job.ID, job.OwnerID, spec, job.Status, job.DesiredStatus, job.ObservedStatus, formatTime(job.CreatedAt))
	return err
}

func (s *Store) Job(ctx context.Context, id string) (domain.Job, error) {
	row := s.db.QueryRowContext(ctx, jobSelect+` WHERE j.id=?`, id)
	return scanJob(row)
}

func (s *Store) Attempts(ctx context.Context, jobID string) ([]domain.JobAttempt, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,job_id,attempt_number,node_id,cpu_package_id,gpu_uuids_json,status,image_digest,exit_code,failure_reason,outputs_json,created_at,started_at,finished_at FROM job_attempts WHERE job_id=? ORDER BY attempt_number DESC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attempts := make([]domain.JobAttempt, 0)
	for rows.Next() {
		attempt, scanErr := scanAttempt(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (s *Store) Attempt(ctx context.Context, jobID, attemptID string) (domain.JobAttempt, error) {
	return scanAttempt(s.db.QueryRowContext(ctx, `SELECT id,job_id,attempt_number,node_id,cpu_package_id,gpu_uuids_json,status,image_digest,exit_code,failure_reason,outputs_json,created_at,started_at,finished_at FROM job_attempts WHERE id=? AND job_id=?`, attemptID, jobID))
}

func (s *Store) ListJobs(ctx context.Context, includeDeleted bool) ([]domain.Job, error) {
	query := jobSelect
	if !includeDeleted {
		query += ` WHERE j.status <> 'DELETED'`
	}
	query += ` ORDER BY j.created_at DESC`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]domain.Job, 0)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) QueuedJobs(ctx context.Context) ([]domain.Job, error) {
	rows, err := s.db.QueryContext(ctx, jobSelect+` WHERE j.status='QUEUED' ORDER BY j.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []domain.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) SetQueueReason(ctx context.Context, jobID, code, message string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET queue_reason_code=?,queue_reason=?,version=version+1 WHERE id=? AND status='QUEUED'`, code, message, jobID)
	return err
}

func (s *Store) ReserveJob(ctx context.Context, jobID, nodeID, attemptID, assignmentID, jobTokenHash string, jobTokenCiphertext []byte, gpuUUIDs []string) error {
	return s.ReserveJobWithAffinity(ctx, jobID, nodeID, attemptID, assignmentID, jobTokenHash, jobTokenCiphertext, gpuUUIDs, "", "")
}

func (s *Store) ReserveJobWithAffinity(ctx context.Context, jobID, nodeID, attemptID, assignmentID, jobTokenHash string, jobTokenCiphertext []byte, gpuUUIDs []string, cpuPackageID, cpuSet string) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE jobs SET status='ASSIGNED',desired_status='RUNNING',assigned_node_id=?,attempt_id=?,queue_reason_code='',queue_reason='',version=version+1 WHERE id=? AND status='QUEUED'`, nodeID, attemptID, jobID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return ErrConflict
	}
	now := formatTime(time.Now().UTC())
	gpus, _ := json.Marshal(gpuUUIDs)
	if _, err = tx.ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,cpu_package_id,gpu_uuids_json,status,job_token_hash,created_at) SELECT ?,?,COALESCE(MAX(attempt_number),0)+1,?,?,?,?, 'ASSIGNED',?,? FROM job_attempts WHERE job_id=?`, attemptID, jobID, nodeID, assignmentID, cpuPackageID, gpus, jobTokenHash, now, jobID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO assignments(id,job_id,attempt_id,node_id,gpu_uuids_json,job_token_ciphertext,created_at,cpu_package_id,cpu_set) VALUES(?,?,?,?,?,?,?,?,?)`, assignmentID, jobID, attemptID, nodeID, gpus, jobTokenCiphertext, now, cpuPackageID, cpuSet); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO job_events(job_id,attempt_id,sequence,type,status,payload_json,created_at) SELECT ?,?,COALESCE(MAX(sequence),0)+1,'assigned','ASSIGNED','{}',? FROM job_events WHERE job_id=?`, jobID, attemptID, now, jobID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateJobStatus(ctx context.Context, jobID string, status domain.JobStatus, exitCode *int, imageDigest, reason string) error {
	// Agent events can race with reconciliation and other lifecycle requests.
	// Retry a lost compare-and-swap with fresh state; only report success when
	// both the job and its current attempt reflect the observed status.
	for attempt := 0; attempt < 3; attempt++ {
		job, err := s.Job(ctx, jobID)
		if err != nil {
			return err
		}
		if job.Status == status && job.ObservedStatus == status {
			return nil
		}
		if !domain.CanTransition(job.Status, status) {
			return fmt.Errorf("%w: invalid transition %s to %s", ErrConflict, job.Status, status)
		}
		now := time.Now().UTC()
		started, finished := any(nil), any(nil)
		if status == domain.JobRunning && job.StartedAt == nil {
			started = formatTime(now)
		}
		if status == domain.JobSucceeded || status == domain.JobFailed || status == domain.JobCancelled {
			finished = formatTime(now)
		}
		tx, beginErr := s.db.BeginTx(ctx, nil)
		if beginErr != nil {
			return beginErr
		}
		result, updateErr := tx.ExecContext(ctx, `UPDATE jobs SET status=?,observed_status=?,exit_code=COALESCE(?,exit_code),image_digest=CASE WHEN ?='' THEN image_digest ELSE ? END,failure_reason=CASE WHEN ?='' THEN failure_reason ELSE ? END,started_at=COALESCE(?,started_at),finished_at=COALESCE(?,finished_at),version=version+1 WHERE id=? AND status=? AND attempt_id=?`, status, status, exitCode, imageDigest, imageDigest, reason, reason, started, finished, jobID, job.Status, job.AttemptID)
		if updateErr != nil {
			_ = tx.Rollback()
			return updateErr
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			_ = tx.Rollback()
			continue
		}
		result, updateErr = tx.ExecContext(ctx, `UPDATE job_attempts SET status=?,exit_code=COALESCE(?,exit_code),image_digest=CASE WHEN ?='' THEN image_digest ELSE ? END,failure_reason=CASE WHEN ?='' THEN failure_reason ELSE ? END,started_at=COALESCE(?,started_at),finished_at=COALESCE(?,finished_at),job_token_hash=CASE WHEN ? THEN 'revoked:'||id ELSE job_token_hash END WHERE id=? AND job_id=?`, status, exitCode, imageDigest, imageDigest, reason, reason, started, finished, finished != nil, job.AttemptID, jobID)
		if updateErr != nil {
			_ = tx.Rollback()
			return updateErr
		}
		if changed, _ = result.RowsAffected(); changed != 1 {
			_ = tx.Rollback()
			return ErrConflict
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return commitErr
		}
		return nil
	}
	return ErrConflict
}

func (s *Store) SetAttemptOutputs(ctx context.Context, jobID, attemptID string, outputs []domain.OutputFile) error {
	data, _ := json.Marshal(outputs)
	result, err := s.db.ExecContext(ctx, `UPDATE job_attempts SET outputs_json=? WHERE id=? AND job_id=?`, data, attemptID, jobID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RerunJob(ctx context.Context, jobID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE jobs SET status='QUEUED',desired_status='RUNNING',observed_status='QUEUED',assigned_node_id=NULL,attempt_id=NULL,image_digest='',exit_code=NULL,queue_reason_code='',queue_reason='',failure_reason='',started_at=NULL,finished_at=NULL,version=version+1 WHERE id=? AND status IN ('SUCCEEDED','FAILED','CANCELLED')`, jobID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrConflict
	}
	return tx.Commit()
}

func (s *Store) RequestStop(ctx context.Context, jobID string) error {
	job, err := s.Job(ctx, jobID)
	if err != nil {
		return err
	}
	switch job.Status {
	case domain.JobQueued:
		_, err = s.db.ExecContext(ctx, `UPDATE jobs SET status='CANCELLED',desired_status='CANCELLED',observed_status='CANCELLED',finished_at=?,version=version+1 WHERE id=? AND status='QUEUED'`, formatTime(time.Now().UTC()), jobID)
	case domain.JobAssigned, domain.JobPullingImage, domain.JobStarting, domain.JobRunning, domain.JobLost:
		_, err = s.db.ExecContext(ctx, `UPDATE jobs SET status='STOPPING',desired_status='CANCELLED',version=version+1 WHERE id=?`, jobID)
		if err == nil && job.AttemptID != "" {
			_, err = s.db.ExecContext(ctx, `UPDATE job_attempts SET status='STOPPING' WHERE id=? AND job_id=?`, job.AttemptID, jobID)
		}
	default:
		err = ErrConflict
	}
	return err
}

func (s *Store) MarkDeleting(ctx context.Context, jobID string) error {
	job, err := s.Job(ctx, jobID)
	if err != nil {
		return err
	}
	if domain.IsActive(job.Status) {
		return ErrConflict
	}
	if job.Status == domain.JobDeleted || job.Status == domain.JobDeleting {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `UPDATE jobs SET status='DELETING',desired_status='DELETING',version=version+1 WHERE id=?`, jobID)
	return err
}

func (s *Store) MarkDeleted(ctx context.Context, jobID string) error {
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET status='DELETED',desired_status='DELETED',observed_status='DELETED',spec_json='{}',image_digest='',failure_reason='',deleted_at=?,version=version+1 WHERE id=?`, now, jobID)
	return err
}

func (s *Store) UpsertNode(ctx context.Context, node domain.Node, credentialHash string) error {
	labels, _ := json.Marshal(node.Labels)
	capabilities, _ := json.Marshal(node.Capabilities)
	systemInfo, _ := json.Marshal(node.System)
	runtimeInfo, _ := json.Marshal(node.Runtime)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO nodes(id,name,status,agent_version,protocol_version,architecture,docker_version,cpu_total_millis,memory_total_bytes,workspace_total_bytes,workspace_free_bytes,labels_json,credential_hash,credential_created_at,last_heartbeat,created_at,gpu_discovery_status,gpu_error_code,gpu_error_message,capabilities_json,system_info_json,runtime_info_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,status=excluded.status,agent_version=excluded.agent_version,protocol_version=excluded.protocol_version,architecture=excluded.architecture,docker_version=excluded.docker_version,cpu_total_millis=excluded.cpu_total_millis,memory_total_bytes=excluded.memory_total_bytes,workspace_total_bytes=excluded.workspace_total_bytes,workspace_free_bytes=excluded.workspace_free_bytes,labels_json=excluded.labels_json,last_heartbeat=excluded.last_heartbeat,gpu_discovery_status=excluded.gpu_discovery_status,gpu_error_code=excluded.gpu_error_code,gpu_error_message=excluded.gpu_error_message,capabilities_json=excluded.capabilities_json,system_info_json=excluded.system_info_json,runtime_info_json=excluded.runtime_info_json`, node.ID, node.Name, node.Status, node.AgentVersion, node.ProtocolVersion, node.Architecture, node.DockerVersion, node.CPUTotalMillis, node.MemoryTotalBytes, node.WorkspaceTotalBytes, node.WorkspaceFreeBytes, labels, credentialHash, formatTime(time.Now().UTC()), formatTime(node.LastHeartbeat), formatTime(node.CreatedAt), node.GPUDiscovery.Status, node.GPUDiscovery.ErrorCode, node.GPUDiscovery.Message, capabilities, systemInfo, runtimeInfo)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM gpus WHERE node_id=?`, node.ID); err != nil {
		return err
	}
	for _, gpu := range node.GPUs {
		if _, err = tx.ExecContext(ctx, `INSERT INTO gpus(node_id,uuid,model,vram_bytes,pci_bus_id,driver_version,compute_capability,utilization_basis_points,utilization_average_basis_points,utilization_peak_basis_points,utilization_sampled_at,utilization_window_seconds,utilization_sample_count,memory_used_bytes,temperature_celsius) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, node.ID, gpu.UUID, gpu.Model, gpu.VRAMBytes, gpu.PCIBusID, gpu.DriverVersion, gpu.ComputeCapability, gpu.UtilizationBasisPoints, gpu.UtilizationAverageBasisPoints, gpu.UtilizationPeakBasisPoints, optionalTime(gpu.UtilizationSampledAt), gpu.UtilizationWindowSeconds, gpu.UtilizationSampleCount, gpu.MemoryUsedBytes, gpu.TemperatureCelsius); err != nil {
			return err
		}
	}
	if err = replaceCPUPackages(ctx, tx, node.ID, node.CPUPackages); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Heartbeat(ctx context.Context, node domain.Node) error {
	labels, _ := json.Marshal(node.Labels)
	capabilities, _ := json.Marshal(node.Capabilities)
	systemInfo, _ := json.Marshal(node.System)
	runtimeInfo, _ := json.Marshal(node.Runtime)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE nodes SET name=?,status=CASE WHEN status='DRAINING' THEN status ELSE ? END,agent_version=?,protocol_version=?,architecture=?,docker_version=?,cpu_total_millis=?,memory_total_bytes=?,workspace_total_bytes=?,workspace_free_bytes=?,labels_json=?,last_heartbeat=?,gpu_discovery_status=?,gpu_error_code=?,gpu_error_message=?,capabilities_json=?,system_info_json=?,runtime_info_json=? WHERE id=? AND deleted_at IS NULL`, node.Name, node.Status, node.AgentVersion, node.ProtocolVersion, node.Architecture, node.DockerVersion, node.CPUTotalMillis, node.MemoryTotalBytes, node.WorkspaceTotalBytes, node.WorkspaceFreeBytes, labels, formatTime(node.LastHeartbeat), node.GPUDiscovery.Status, node.GPUDiscovery.ErrorCode, node.GPUDiscovery.Message, capabilities, systemInfo, runtimeInfo, node.ID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM gpus WHERE node_id=?`, node.ID); err != nil {
		return err
	}
	for _, gpu := range node.GPUs {
		if _, err = tx.ExecContext(ctx, `INSERT INTO gpus(node_id,uuid,model,vram_bytes,pci_bus_id,driver_version,compute_capability,utilization_basis_points,utilization_average_basis_points,utilization_peak_basis_points,utilization_sampled_at,utilization_window_seconds,utilization_sample_count,memory_used_bytes,temperature_celsius) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, node.ID, gpu.UUID, gpu.Model, gpu.VRAMBytes, gpu.PCIBusID, gpu.DriverVersion, gpu.ComputeCapability, gpu.UtilizationBasisPoints, gpu.UtilizationAverageBasisPoints, gpu.UtilizationPeakBasisPoints, optionalTime(gpu.UtilizationSampledAt), gpu.UtilizationWindowSeconds, gpu.UtilizationSampleCount, gpu.MemoryUsedBytes, gpu.TemperatureCelsius); err != nil {
			return err
		}
	}
	if err = replaceCPUPackages(ctx, tx, node.ID, node.CPUPackages); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListNodes(ctx context.Context) ([]domain.Node, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,COALESCE(name_override,name),status,agent_version,protocol_version,architecture,docker_version,cpu_total_millis,memory_total_bytes,workspace_total_bytes,workspace_free_bytes,COALESCE(labels_override_json,labels_json),last_heartbeat,created_at,gpu_discovery_status,gpu_error_code,gpu_error_message,capabilities_json,system_info_json,runtime_info_json FROM nodes WHERE deleted_at IS NULL ORDER BY COALESCE(name_override,name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := make([]domain.Node, 0)
	for rows.Next() {
		var node domain.Node
		node.GPUs = make([]domain.GPU, 0)
		node.CPUPackages = make([]domain.CPUPackage, 0)
		var labels, capabilities, systemInfo, runtimeInfo, heartbeat, created string
		if err := rows.Scan(&node.ID, &node.Name, &node.Status, &node.AgentVersion, &node.ProtocolVersion, &node.Architecture, &node.DockerVersion, &node.CPUTotalMillis, &node.MemoryTotalBytes, &node.WorkspaceTotalBytes, &node.WorkspaceFreeBytes, &labels, &heartbeat, &created, &node.GPUDiscovery.Status, &node.GPUDiscovery.ErrorCode, &node.GPUDiscovery.Message, &capabilities, &systemInfo, &runtimeInfo); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(labels), &node.Labels)
		json.Unmarshal([]byte(capabilities), &node.Capabilities)
		if node.Labels == nil {
			node.Labels = map[string]string{}
		}
		if node.Capabilities == nil {
			node.Capabilities = []string{}
		}
		json.Unmarshal([]byte(systemInfo), &node.System)
		json.Unmarshal([]byte(runtimeInfo), &node.Runtime)
		if node.System.Architecture == "" {
			node.System.Architecture = node.Architecture
		}
		if node.Runtime.DockerVersion == "" {
			node.Runtime.DockerVersion = node.DockerVersion
		}
		node.LastHeartbeat, _ = time.Parse(time.RFC3339Nano, heartbeat)
		node.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		gpuRows, err := s.db.QueryContext(ctx, `SELECT uuid,model,vram_bytes,pci_bus_id,driver_version,compute_capability,utilization_basis_points,utilization_average_basis_points,utilization_peak_basis_points,utilization_sampled_at,utilization_window_seconds,utilization_sample_count,memory_used_bytes,temperature_celsius FROM gpus WHERE node_id=? ORDER BY uuid`, node.ID)
		if err != nil {
			return nil, err
		}
		for gpuRows.Next() {
			var gpu domain.GPU
			var sampledAt sql.NullString
			if err := gpuRows.Scan(&gpu.UUID, &gpu.Model, &gpu.VRAMBytes, &gpu.PCIBusID, &gpu.DriverVersion, &gpu.ComputeCapability, &gpu.UtilizationBasisPoints, &gpu.UtilizationAverageBasisPoints, &gpu.UtilizationPeakBasisPoints, &sampledAt, &gpu.UtilizationWindowSeconds, &gpu.UtilizationSampleCount, &gpu.MemoryUsedBytes, &gpu.TemperatureCelsius); err != nil {
				gpuRows.Close()
				return nil, err
			}
			if sampledAt.Valid && sampledAt.String != "" {
				value, parseErr := time.Parse(time.RFC3339Nano, sampledAt.String)
				if parseErr == nil {
					gpu.UtilizationSampledAt = &value
				}
			}
			node.GPUs = append(node.GPUs, gpu)
		}
		gpuRows.Close()
		cpuRows, err := s.db.QueryContext(ctx, `SELECT package_id,vendor,model,physical_cores,logical_cpus_json,total_millis FROM cpu_packages WHERE node_id=? ORDER BY package_id`, node.ID)
		if err != nil {
			return nil, err
		}
		for cpuRows.Next() {
			var item domain.CPUPackage
			var logical string
			if err := cpuRows.Scan(&item.ID, &item.Vendor, &item.Model, &item.PhysicalCores, &logical, &item.TotalMillis); err != nil {
				cpuRows.Close()
				return nil, err
			}
			_ = json.Unmarshal([]byte(logical), &item.LogicalCPUs)
			node.CPUPackages = append(node.CPUPackages, item)
		}
		cpuRows.Close()
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func replaceCPUPackages(ctx context.Context, tx *sql.Tx, nodeID string, packages []domain.CPUPackage) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM cpu_packages WHERE node_id=?`, nodeID); err != nil {
		return err
	}
	for _, item := range packages {
		logical, _ := json.Marshal(item.LogicalCPUs)
		if _, err := tx.ExecContext(ctx, `INSERT INTO cpu_packages(node_id,package_id,vendor,model,physical_cores,logical_cpus_json,total_millis) VALUES(?,?,?,?,?,?,?)`, nodeID, item.ID, item.Vendor, item.Model, item.PhysicalCores, logical, item.TotalMillis); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpdateNodeMetadata(ctx context.Context, id, name string, labels map[string]string) error {
	data, err := json.Marshal(labels)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE nodes SET name_override=?,labels_override_json=? WHERE id=? AND deleted_at IS NULL`, name, data, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) NodeByCredential(ctx context.Context, credentialHash string) (domain.Node, error) {
	var node domain.Node
	err := s.db.QueryRowContext(ctx, `SELECT id,name,status FROM nodes WHERE deleted_at IS NULL AND (credential_hash=? OR (previous_credential_hash=? AND previous_credential_expires_at>?))`, credentialHash, credentialHash, formatTime(time.Now().UTC())).Scan(&node.ID, &node.Name, &node.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return node, ErrNotFound
	}
	return node, err
}

func (s *Store) RotateNodeCredential(ctx context.Context, nodeID, credentialHash string) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE nodes SET previous_credential_hash=credential_hash,previous_credential_expires_at=?,credential_hash=?,credential_created_at=? WHERE id=? AND deleted_at IS NULL`, formatTime(now.Add(10*time.Minute)), credentialHash, formatTime(now), nodeID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetNodeStatus(ctx context.Context, id string, status domain.NodeStatus) error {
	result, err := s.db.ExecContext(ctx, `UPDATE nodes SET status=? WHERE id=? AND deleted_at IS NULL`, status, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteNode forgets a node from scheduling without destroying historical job
// attempts that reference it. The enrollment credential is revoked immediately,
// and the row is kept only as a foreign-key anchor for immutable history.
func (s *Store) DeleteNode(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var deletedAt sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT deleted_at FROM nodes WHERE id=?`, id).Scan(&deletedAt)
	if errors.Is(err, sql.ErrNoRows) || deletedAt.Valid {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	var active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE assigned_node_id=? AND status IN ('ASSIGNED','PULLING_IMAGE','STARTING','RUNNING','STOPPING','LOST')`, id).Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return fmt.Errorf("%w: node still has %d active job(s); stop or reconcile them before deletion", ErrConflict, active)
	}

	result, err := tx.ExecContext(ctx, `UPDATE nodes SET status='OFFLINE',deleted_at=?,credential_hash='revoked:'||id,previous_credential_hash=NULL,previous_credential_expires_at=NULL WHERE id=? AND deleted_at IS NULL`, formatTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM gpus WHERE node_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MarkStaleNodes(ctx context.Context, offlineBefore, lostBefore time.Time) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE nodes SET status='OFFLINE' WHERE deleted_at IS NULL AND status IN ('ONLINE','DEGRADED') AND last_heartbeat < ?`, formatTime(offlineBefore)); err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM jobs WHERE status IN ('ASSIGNED','PULLING_IMAGE','STARTING','RUNNING','STOPPING') AND assigned_node_id IN (SELECT id FROM nodes WHERE deleted_at IS NULL AND last_heartbeat < ?)`, formatTime(lostBefore))
	if err != nil {
		return err
	}
	var jobIDs []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		jobIDs = append(jobIDs, id)
	}
	rows.Close()
	for _, jobID := range jobIDs {
		if _, err = s.db.ExecContext(ctx, `UPDATE jobs SET status='LOST',observed_status='LOST',version=version+1 WHERE id=? AND status IN ('ASSIGNED','PULLING_IMAGE','STARTING','RUNNING','STOPPING')`, jobID); err != nil {
			return err
		}
		if _, err = s.db.ExecContext(ctx, `UPDATE job_attempts SET status='LOST' WHERE id=(SELECT attempt_id FROM jobs WHERE id=?)`, jobID); err != nil {
			return err
		}
		if err = s.AppendServerEvent(ctx, jobID, "lost", map[string]any{}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) AssignmentForNode(ctx context.Context, nodeID string) (domain.Assignment, error) {
	var assignment domain.Assignment
	var specJSON, gpusJSON, created string
	err := s.db.QueryRowContext(ctx, `SELECT a.id,a.job_id,a.attempt_id,j.spec_json,a.gpu_uuids_json,a.job_token_ciphertext,a.created_at,COALESCE((SELECT MAX(sequence) FROM job_events WHERE job_id=a.job_id),0),a.cpu_package_id,a.cpu_set FROM assignments a JOIN jobs j ON j.id=a.job_id WHERE a.node_id=? AND a.attempt_id=j.attempt_id AND a.accepted_at IS NULL AND j.status IN ('ASSIGNED','PULLING_IMAGE','STARTING') ORDER BY a.created_at LIMIT 1`, nodeID).Scan(&assignment.ID, &assignment.JobID, &assignment.AttemptID, &specJSON, &gpusJSON, &assignment.JobTokenEncrypted, &created, &assignment.EventSequence, &assignment.CPUPackageID, &assignment.CPUSet)
	if errors.Is(err, sql.ErrNoRows) {
		return assignment, ErrNotFound
	}
	json.Unmarshal([]byte(specJSON), &assignment.Spec)
	json.Unmarshal([]byte(gpusJSON), &assignment.GPUUUIDs)
	assignment.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if err == nil {
		_, _ = s.db.ExecContext(ctx, `UPDATE assignments SET delivered_at=COALESCE(delivered_at,?) WHERE id=?`, formatTime(time.Now().UTC()), assignment.ID)
	}
	return assignment, err
}

func (s *Store) StopRequestsForNode(ctx context.Context, nodeID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM jobs WHERE assigned_node_id=? AND desired_status='CANCELLED' AND status IN ('STOPPING','LOST')`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) CreateCheckpointSync(ctx context.Context, sync domain.CheckpointSync) error {
	metadata, _ := json.Marshal(sync.Metadata)
	if len(sync.Metadata) == 0 {
		metadata = nil
	}
	var observed any
	if sync.ObservedAt != nil {
		observed = sync.ObservedAt.UTC().UnixMilli()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO checkpoint_syncs(id,job_id,attempt_id,requested_at,label,step,observed_at,metadata_json) VALUES(?,?,?,?,?,?,?,?)`, sync.ID, sync.JobID, sync.AttemptID, formatTime(sync.RequestedAt), nullableString(sync.Label), sync.Step, observed, metadata)
	return mapConstraint(err)
}

func (s *Store) CheckpointSync(ctx context.Context, id string) (domain.CheckpointSync, error) {
	return scanCheckpointObservation(s.db.QueryRowContext(ctx, `SELECT id,job_id,attempt_id,requested_at,confirmed_at,file_count,byte_count,label,step,observed_at,metadata_json FROM checkpoint_syncs WHERE id=?`, id))
}

func (s *Store) PendingCheckpointSyncsForNode(ctx context.Context, nodeID string) ([]domain.CheckpointSync, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.job_id,c.attempt_id,c.requested_at FROM checkpoint_syncs c JOIN jobs j ON j.id=c.job_id WHERE j.assigned_node_id=? AND j.attempt_id=c.attempt_id AND c.confirmed_at IS NULL AND j.status IN ('ASSIGNED','PULLING_IMAGE','STARTING','RUNNING','STOPPING','LOST') ORDER BY c.requested_at`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.CheckpointSync, 0)
	for rows.Next() {
		var item domain.CheckpointSync
		var requested string
		if err = rows.Scan(&item.ID, &item.JobID, &item.AttemptID, &requested); err != nil {
			return nil, err
		}
		item.RequestedAt, _ = time.Parse(time.RFC3339Nano, requested)
		item.Status = "PENDING"
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ConfirmCheckpointSync(ctx context.Context, id string, files []domain.CheckpointFile) error {
	data, err := json.Marshal(files)
	if err != nil {
		return err
	}
	var total int64
	for _, file := range files {
		total += file.Size
	}
	result, err := s.db.ExecContext(ctx, `UPDATE checkpoint_syncs SET confirmed_at=COALESCE(confirmed_at,?),file_count=CASE WHEN confirmed_at IS NULL THEN ? ELSE file_count END,byte_count=CASE WHEN confirmed_at IS NULL THEN ? ELSE byte_count END,manifest_json=CASE WHEN confirmed_at IS NULL THEN ? ELSE manifest_json END WHERE id=?`, formatTime(time.Now().UTC()), len(files), total, data, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) LatestConfirmedCheckpoint(ctx context.Context, jobID string) (domain.CheckpointSync, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM checkpoint_syncs WHERE job_id=? AND confirmed_at IS NOT NULL ORDER BY confirmed_at DESC LIMIT 1`, jobID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CheckpointSync{}, ErrNotFound
	}
	if err != nil {
		return domain.CheckpointSync{}, err
	}
	return s.CheckpointSync(ctx, id)
}

func (s *Store) AllocatedGPUUUIDs(ctx context.Context) (map[string]map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.node_id,a.gpu_uuids_json FROM assignments a JOIN jobs j ON j.id=a.job_id WHERE j.status IN ('ASSIGNED','PULLING_IMAGE','STARTING','RUNNING','STOPPING','LOST')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]map[string]bool{}
	for rows.Next() {
		var nodeID, data string
		if err := rows.Scan(&nodeID, &data); err != nil {
			return nil, err
		}
		var ids []string
		_ = json.Unmarshal([]byte(data), &ids)
		if result[nodeID] == nil {
			result[nodeID] = map[string]bool{}
		}
		for _, id := range ids {
			result[nodeID][id] = true
		}
	}
	return result, rows.Err()
}

func (s *Store) AcceptAssignment(ctx context.Context, nodeID, assignmentID, containerID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE assignments SET accepted_at=COALESCE(accepted_at,?) WHERE id=? AND node_id=?`, formatTime(time.Now().UTC()), assignmentID, nodeID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	_, err = tx.ExecContext(ctx, `UPDATE job_attempts SET container_id=?,status='STARTING' WHERE assignment_id=?`, containerID, assignmentID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AppendEvent(ctx context.Context, event domain.Event) error {
	if event.Type == "resource_sample" {
		return ErrRawTelemetry
	}
	payload, _ := json.Marshal(event.Payload)
	_, err := s.db.ExecContext(ctx, `INSERT INTO job_events(job_id,attempt_id,sequence,type,status,payload_json,created_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(job_id,sequence) DO NOTHING`, event.JobID, nullable(event.AttemptID), event.Sequence, event.Type, event.Status, payload, formatTime(event.CreatedAt))
	return err
}

func (s *Store) AppendServerEvent(ctx context.Context, jobID, eventType string, payload map[string]any) error {
	data, _ := json.Marshal(payload)
	for attempts := 0; attempts < 3; attempts++ {
		_, err := s.db.ExecContext(ctx, `INSERT INTO job_events(job_id,attempt_id,sequence,type,status,payload_json,created_at) VALUES(?,(SELECT attempt_id FROM jobs WHERE id=?),COALESCE((SELECT MAX(sequence) FROM job_events WHERE job_id=?),0)+1,?,(SELECT status FROM jobs WHERE id=?),?,?)`, jobID, jobID, jobID, eventType, jobID, data, formatTime(time.Now().UTC()))
		if err == nil {
			return nil
		}
		if !strings.Contains(strings.ToLower(err.Error()), "unique") {
			return err
		}
	}
	return ErrConflict
}

func (s *Store) JobByToken(ctx context.Context, tokenHash string) (domain.Job, error) {
	row := s.db.QueryRowContext(ctx, jobSelect+` JOIN job_attempts a ON a.job_id=j.id AND a.id=j.attempt_id WHERE a.job_token_hash=?`, tokenHash)
	return scanJob(row)
}

func (s *Store) Events(ctx context.Context, jobID string, after int64) ([]domain.Event, error) {
	return s.events(ctx, jobID, "", after)
}

func (s *Store) EventsForAttempt(ctx context.Context, jobID, attemptID string, after int64) ([]domain.Event, error) {
	return s.events(ctx, jobID, attemptID, after)
}

func (s *Store) events(ctx context.Context, jobID, attemptID string, after int64) ([]domain.Event, error) {
	query := `SELECT id,job_id,COALESCE(attempt_id,''),sequence,type,status,payload_json,created_at FROM job_events WHERE job_id=? AND id>?`
	arguments := []any{jobID, after}
	if attemptID != "" {
		query += ` AND attempt_id=?`
		arguments = append(arguments, attemptID)
	}
	query += ` ORDER BY id LIMIT 1000`
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]domain.Event, 0)
	for rows.Next() {
		var event domain.Event
		var payload, created string
		if err := rows.Scan(&event.ID, &event.JobID, &event.AttemptID, &event.Sequence, &event.Type, &event.Status, &payload, &created); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(payload), &event.Payload)
		event.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) JobUpdatesForOwner(ctx context.Context, ownerID string, after int64) ([]JobUpdate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.id,e.job_id,json_extract(j.spec_json,'$.name'),e.status,j.version,e.created_at FROM job_events e JOIN jobs j ON j.id=e.job_id WHERE j.owner_id=? AND e.id>? AND e.status<>'' ORDER BY e.id LIMIT 1000`, ownerID, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	updates := make([]JobUpdate, 0)
	for rows.Next() {
		var item JobUpdate
		var created string
		if err := rows.Scan(&item.Cursor, &item.JobID, &item.Name, &item.Status, &item.Version, &created); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		updates = append(updates, item)
	}
	return updates, rows.Err()
}

func (s *Store) LatestJobUpdateCursorForOwner(ctx context.Context, ownerID string) (int64, error) {
	var cursor int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(e.id),0) FROM job_events e JOIN jobs j ON j.id=e.job_id WHERE j.owner_id=?`, ownerID).Scan(&cursor)
	return cursor, err
}

func (s *Store) CreateEnrollmentToken(ctx context.Context, tokenHash, userID string, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO enrollment_tokens(token_hash,created_by,expires_at,created_at) VALUES(?,?,?,?)`, tokenHash, userID, formatTime(expires), formatTime(time.Now().UTC()))
	return err
}

func (s *Store) ConsumeEnrollmentToken(ctx context.Context, tokenHash string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE enrollment_tokens SET used_at=? WHERE token_hash=? AND used_at IS NULL AND expires_at>?`, formatTime(time.Now().UTC()), tokenHash, formatTime(time.Now().UTC()))
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateSecret(ctx context.Context, metadata SecretMetadata, ciphertext []byte) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO secrets(id,owner_id,name,ciphertext,kind,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, metadata.ID, metadata.OwnerID, metadata.Name, ciphertext, metadata.Kind, formatTime(metadata.CreatedAt), formatTime(metadata.UpdatedAt))
	return mapConstraint(err)
}

func (s *Store) ListSecrets(ctx context.Context, ownerID string) ([]SecretMetadata, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,owner_id,name,kind,created_at,updated_at FROM secrets WHERE owner_id=? ORDER BY name`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SecretMetadata, 0)
	for rows.Next() {
		var item SecretMetadata
		var created, updated string
		if err := rows.Scan(&item.ID, &item.OwnerID, &item.Name, &item.Kind, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) SecretMetadata(ctx context.Context, ownerID, id string) (SecretMetadata, error) {
	var item SecretMetadata
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,owner_id,name,kind,created_at,updated_at FROM secrets WHERE owner_id=? AND id=?`, ownerID, id).Scan(&item.ID, &item.OwnerID, &item.Name, &item.Kind, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrNotFound
	}
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return item, err
}

func (s *Store) SecretCiphertext(ctx context.Context, ownerID, name string) ([]byte, string, error) {
	var value []byte
	var kind string
	err := s.db.QueryRowContext(ctx, `SELECT ciphertext,kind FROM secrets WHERE owner_id=? AND name=?`, ownerID, name).Scan(&value, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	return value, kind, err
}

func (s *Store) DeleteSecret(ctx context.Context, ownerID, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM secrets WHERE owner_id=? AND id=?`, ownerID, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Audit(ctx context.Context, actorID, action, targetType, targetID string, metadata map[string]any) error {
	return s.AuditWithLabels(ctx, actorID, "", action, targetType, targetID, "", metadata)
}

func (s *Store) AuditWithLabels(ctx context.Context, actorID, actorLabel, action, targetType, targetID, targetLabel string, metadata map[string]any) error {
	if actorLabel == "" {
		if actorID == "" {
			actorLabel = "System"
		} else {
			_ = s.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id=?`, actorID).Scan(&actorLabel)
		}
	}
	if targetLabel == "" {
		targetLabel = auditMetadataLabel(metadata)
	}
	if targetLabel == "" {
		targetLabel = s.auditTargetLabel(ctx, targetType, targetID)
	}
	data, _ := json.Marshal(metadata)
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_events(actor_id,actor_label,action,target_type,target_id,target_label,metadata_json,created_at) VALUES(?,?,?,?,?,?,?,?)`, nullable(actorID), nullable(actorLabel), action, targetType, targetID, nullable(targetLabel), data, formatTime(time.Now().UTC()))
	return err
}

func auditMetadataLabel(metadata map[string]any) string {
	for _, key := range []string{"name", "username"} {
		if value, ok := metadata[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Store) auditTargetLabel(ctx context.Context, targetType, targetID string) string {
	queries := map[string]string{
		"user":                  `SELECT username FROM users WHERE id=?`,
		"node":                  `SELECT COALESCE(name_override,name) FROM nodes WHERE id=?`,
		"job":                   `SELECT json_extract(spec_json,'$.name') FROM jobs WHERE id=?`,
		"secret":                `SELECT name FROM secrets WHERE id=?`,
		"build":                 `SELECT name FROM builds WHERE id=?`,
		"dashboard":             `SELECT name FROM job_dashboards WHERE id=?`,
		"personal_access_token": `SELECT name FROM personal_access_tokens WHERE id=?`,
	}
	query, ok := queries[targetType]
	if !ok {
		return ""
	}
	var label string
	_ = s.db.QueryRowContext(ctx, query, targetID).Scan(&label)
	return label
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEvent, error) {
	page, err := s.QueryAudit(ctx, AuditFilter{Limit: limit})
	return page.Items, err
}

func (s *Store) QueryAudit(ctx context.Context, filter AuditFilter) (AuditPage, error) {
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	where, args := []string{"1=1"}, make([]any, 0, 8)
	if filter.Before > 0 {
		where, args = append(where, "id < ?"), append(args, filter.Before)
	}
	if filter.ActorID != "" {
		if filter.ActorID == "system" {
			where = append(where, "actor_id IS NULL")
		} else {
			where, args = append(where, "actor_id = ?"), append(args, filter.ActorID)
		}
	}
	if filter.TargetType != "" {
		where, args = append(where, "target_type = ?"), append(args, filter.TargetType)
	}
	if filter.From != nil {
		where, args = append(where, "created_at >= ?"), append(args, formatTime(filter.From.UTC()))
	}
	if filter.To != nil {
		where, args = append(where, "created_at <= ?"), append(args, formatTime(filter.To.UTC()))
	}
	if filter.Category != "" {
		prefixes := map[string]string{"authentication": "auth.%", "users": "user.%", "jobs": "job.%", "builds": "build.%", "nodes": "node.%", "secrets": "secret.%", "dashboards": "dashboard.%", "tokens": "auth.pat.%"}
		if filter.Category == "system" {
			where = append(where, "actor_id IS NULL")
		} else if prefix, ok := prefixes[filter.Category]; ok {
			where, args = append(where, "action LIKE ?"), append(args, prefix)
			if filter.Category == "authentication" {
				where = append(where, "action NOT LIKE 'auth.pat.%'")
			}
		}
	}
	if query := strings.TrimSpace(filter.Query); query != "" {
		like := "%" + query + "%"
		where = append(where, "(action LIKE ? OR target_id LIKE ? OR COALESCE(target_label,'') LIKE ? OR COALESCE(actor_label,'') LIKE ?)")
		args = append(args, like, like, like, like)
	}
	args = append(args, filter.Limit+1)
	availability := `CASE target_type
		WHEN 'user' THEN EXISTS(SELECT 1 FROM users WHERE users.id=audit_events.target_id)
		WHEN 'node' THEN EXISTS(SELECT 1 FROM nodes WHERE nodes.id=audit_events.target_id AND nodes.deleted_at IS NULL)
		WHEN 'job' THEN EXISTS(SELECT 1 FROM jobs WHERE jobs.id=audit_events.target_id AND jobs.deleted_at IS NULL)
		WHEN 'secret' THEN EXISTS(SELECT 1 FROM secrets WHERE secrets.id=audit_events.target_id)
		WHEN 'build' THEN EXISTS(SELECT 1 FROM builds WHERE builds.id=audit_events.target_id)
		WHEN 'dashboard' THEN EXISTS(SELECT 1 FROM job_dashboards WHERE job_dashboards.id=audit_events.target_id)
		WHEN 'personal_access_token' THEN EXISTS(SELECT 1 FROM personal_access_tokens WHERE personal_access_tokens.id=audit_events.target_id)
		ELSE 0 END`
	statement := `SELECT id,COALESCE(actor_id,''),COALESCE(actor_label,''),action,target_type,target_id,COALESCE(target_label,''),` + availability + `,metadata_json,created_at FROM audit_events WHERE ` + strings.Join(where, " AND ") + ` ORDER BY id DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return AuditPage{}, err
	}
	defer rows.Close()
	result := make([]AuditEvent, 0)
	for rows.Next() {
		var event AuditEvent
		var metadata, created string
		if err := rows.Scan(&event.ID, &event.ActorID, &event.ActorLabel, &event.Action, &event.TargetType, &event.TargetID, &event.TargetLabel, &event.TargetAvailable, &metadata, &created); err != nil {
			return AuditPage{}, err
		}
		_ = json.Unmarshal([]byte(metadata), &event.Metadata)
		event.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		result = append(result, event)
	}
	if err = rows.Err(); err != nil {
		return AuditPage{}, err
	}
	page := AuditPage{Items: result}
	if len(result) > filter.Limit {
		page.Items = result[:filter.Limit]
		page.NextCursor = page.Items[len(page.Items)-1].ID
	}
	return page, nil
}

func (s *Store) ClaimIdempotency(ctx context.Context, userID, key, method, path string) (bool, int, []byte, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO idempotency_keys(user_id,key,method,path,created_at) VALUES(?,?,?,?,?) ON CONFLICT(user_id,key) DO NOTHING`, userID, key, method, path, formatTime(time.Now().UTC()))
	if err != nil {
		return false, 0, nil, err
	}
	count, _ := result.RowsAffected()
	if count == 1 {
		return false, 0, nil, nil
	}
	var storedMethod, storedPath string
	var status sql.NullInt64
	var data []byte
	err = s.db.QueryRowContext(ctx, `SELECT method,path,response_status,response_json FROM idempotency_keys WHERE user_id=? AND key=?`, userID, key).Scan(&storedMethod, &storedPath, &status, &data)
	if err != nil {
		return false, 0, nil, err
	}
	if storedMethod != method || storedPath != path || !status.Valid {
		return false, 0, nil, ErrConflict
	}
	return true, int(status.Int64), data, nil
}

func (s *Store) CompleteIdempotency(ctx context.Context, userID, key string, status int, data []byte) error {
	_, err := s.db.ExecContext(ctx, `UPDATE idempotency_keys SET response_status=?,response_json=? WHERE user_id=? AND key=?`, status, data, userID, key)
	return err
}

func (s *Store) ReleaseIdempotency(ctx context.Context, userID, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM idempotency_keys WHERE user_id=? AND key=? AND response_status IS NULL`, userID, key)
	return err
}

const jobSelect = `SELECT j.id,j.owner_id,j.spec_json,j.status,j.desired_status,j.observed_status,COALESCE(j.assigned_node_id,''),COALESCE(j.attempt_id,''),j.image_digest,j.exit_code,j.queue_reason_code,j.queue_reason,j.failure_reason,j.created_at,j.started_at,j.finished_at,j.deleted_at,j.version FROM jobs j`

type scanner interface{ Scan(dest ...any) error }

func scanJob(row scanner) (domain.Job, error) {
	var job domain.Job
	var specJSON, created string
	var started, finished, deleted sql.NullString
	var exit sql.NullInt64
	err := row.Scan(&job.ID, &job.OwnerID, &specJSON, &job.Status, &job.DesiredStatus, &job.ObservedStatus, &job.AssignedNodeID, &job.AttemptID, &job.ImageDigest, &exit, &job.QueueReasonCode, &job.QueueReason, &job.FailureReason, &created, &started, &finished, &deleted, &job.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return job, ErrNotFound
	}
	if err != nil {
		return job, err
	}
	json.Unmarshal([]byte(specJSON), &job.Spec)
	job.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if exit.Valid {
		v := int(exit.Int64)
		job.ExitCode = &v
	}
	job.StartedAt = parseNullTime(started)
	job.FinishedAt = parseNullTime(finished)
	job.DeletedAt = parseNullTime(deleted)
	return job, nil
}

func scanAttempt(row scanner) (domain.JobAttempt, error) {
	var attempt domain.JobAttempt
	var exit sql.NullInt64
	var gpus, outputs, created string
	var started, finished sql.NullString
	err := row.Scan(&attempt.ID, &attempt.JobID, &attempt.AttemptNumber, &attempt.NodeID, &attempt.CPUPackageID, &gpus, &attempt.Status, &attempt.ImageDigest, &exit, &attempt.FailureReason, &outputs, &created, &started, &finished)
	if errors.Is(err, sql.ErrNoRows) {
		return attempt, ErrNotFound
	}
	if err != nil {
		return attempt, err
	}
	if exit.Valid {
		value := int(exit.Int64)
		attempt.ExitCode = &value
	}
	_ = json.Unmarshal([]byte(outputs), &attempt.Outputs)
	_ = json.Unmarshal([]byte(gpus), &attempt.GPUUUIDs)
	if attempt.Outputs == nil {
		attempt.Outputs = []domain.OutputFile{}
	}
	attempt.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	attempt.StartedAt = parseNullTime(started)
	attempt.FinishedAt = parseNullTime(finished)
	return attempt, nil
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func optionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}
func parseNullTime(value sql.NullString) *time.Time {
	if !value.Valid {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil
	}
	return &parsed
}
func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func mapConstraint(err error) error {
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
		return ErrConflict
	}
	return err
}
