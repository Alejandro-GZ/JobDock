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
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/jobdock/jobdock/internal/domain"
)

//go:embed migrations/*.sql
var migrations embed.FS

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

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
	ID         int64          `json:"id"`
	ActorID    string         `json:"actor_id,omitempty"`
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  time.Time      `json:"created_at"`
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
	contents, err := migrations.ReadFile("migrations/001_initial.sql")
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, string(contents))
	return err
}

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var count int
	return count, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
}

func (s *Store) CreateUser(ctx context.Context, user domain.User, passwordHash string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO users(id, username, password_hash, role, created_at) VALUES(?,?,?,?,?)`, user.ID, user.Username, passwordHash, user.Role, formatTime(user.CreatedAt))
	return mapConstraint(err)
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
	rows, err := s.db.QueryContext(ctx, `SELECT id, username, role, created_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []domain.User
	for rows.Next() {
		var user domain.User
		var created string
		if err := rows.Scan(&user.ID, &user.Username, &user.Role, &created); err != nil {
			return nil, err
		}
		user.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
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
	if _, err = tx.ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,?,?,?,'ASSIGNED',?,?)`, attemptID, jobID, 1, nodeID, assignmentID, jobTokenHash, now); err != nil {
		return err
	}
	gpus, _ := json.Marshal(gpuUUIDs)
	if _, err = tx.ExecContext(ctx, `INSERT INTO assignments(id,job_id,attempt_id,node_id,gpu_uuids_json,job_token_ciphertext,created_at) VALUES(?,?,?,?,?,?,?)`, assignmentID, jobID, attemptID, nodeID, gpus, jobTokenCiphertext, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateJobStatus(ctx context.Context, jobID string, status domain.JobStatus, exitCode *int, imageDigest, reason string) error {
	job, err := s.Job(ctx, jobID)
	if err != nil {
		return err
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
	_, err = s.db.ExecContext(ctx, `UPDATE jobs SET status=?,observed_status=?,exit_code=COALESCE(?,exit_code),image_digest=CASE WHEN ?='' THEN image_digest ELSE ? END,failure_reason=CASE WHEN ?='' THEN failure_reason ELSE ? END,started_at=COALESCE(?,started_at),finished_at=COALESCE(?,finished_at),version=version+1 WHERE id=?`, status, status, exitCode, imageDigest, imageDigest, reason, reason, started, finished, jobID)
	if err == nil && (status == domain.JobSucceeded || status == domain.JobFailed || status == domain.JobCancelled) {
		_, err = s.db.ExecContext(ctx, `UPDATE job_attempts SET job_token_hash='revoked:'||id,finished_at=COALESCE(finished_at,?) WHERE job_id=?`, formatTime(now), jobID)
	}
	return err
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO nodes(id,name,status,agent_version,protocol_version,architecture,docker_version,cpu_total_millis,memory_total_bytes,workspace_free_bytes,labels_json,credential_hash,credential_created_at,last_heartbeat,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,status=excluded.status,agent_version=excluded.agent_version,protocol_version=excluded.protocol_version,architecture=excluded.architecture,docker_version=excluded.docker_version,cpu_total_millis=excluded.cpu_total_millis,memory_total_bytes=excluded.memory_total_bytes,workspace_free_bytes=excluded.workspace_free_bytes,labels_json=excluded.labels_json,last_heartbeat=excluded.last_heartbeat`, node.ID, node.Name, node.Status, node.AgentVersion, node.ProtocolVersion, node.Architecture, node.DockerVersion, node.CPUTotalMillis, node.MemoryTotalBytes, node.WorkspaceFreeBytes, labels, credentialHash, formatTime(time.Now().UTC()), formatTime(node.LastHeartbeat), formatTime(node.CreatedAt))
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM gpus WHERE node_id=?`, node.ID); err != nil {
		return err
	}
	for _, gpu := range node.GPUs {
		if _, err = tx.ExecContext(ctx, `INSERT INTO gpus(node_id,uuid,model,vram_bytes) VALUES(?,?,?,?)`, node.ID, gpu.UUID, gpu.Model, gpu.VRAMBytes); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Heartbeat(ctx context.Context, node domain.Node) error {
	labels, _ := json.Marshal(node.Labels)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE nodes SET name=?,status=CASE WHEN status='DRAINING' THEN status ELSE ? END,agent_version=?,protocol_version=?,architecture=?,docker_version=?,cpu_total_millis=?,memory_total_bytes=?,workspace_free_bytes=?,labels_json=?,last_heartbeat=? WHERE id=?`, node.Name, node.Status, node.AgentVersion, node.ProtocolVersion, node.Architecture, node.DockerVersion, node.CPUTotalMillis, node.MemoryTotalBytes, node.WorkspaceFreeBytes, labels, formatTime(node.LastHeartbeat), node.ID)
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
		if _, err = tx.ExecContext(ctx, `INSERT INTO gpus(node_id,uuid,model,vram_bytes) VALUES(?,?,?,?)`, node.ID, gpu.UUID, gpu.Model, gpu.VRAMBytes); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListNodes(ctx context.Context) ([]domain.Node, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,status,agent_version,protocol_version,architecture,docker_version,cpu_total_millis,memory_total_bytes,workspace_free_bytes,labels_json,last_heartbeat,created_at FROM nodes ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []domain.Node
	for rows.Next() {
		var node domain.Node
		var labels, heartbeat, created string
		if err := rows.Scan(&node.ID, &node.Name, &node.Status, &node.AgentVersion, &node.ProtocolVersion, &node.Architecture, &node.DockerVersion, &node.CPUTotalMillis, &node.MemoryTotalBytes, &node.WorkspaceFreeBytes, &labels, &heartbeat, &created); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(labels), &node.Labels)
		node.LastHeartbeat, _ = time.Parse(time.RFC3339Nano, heartbeat)
		node.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		gpuRows, err := s.db.QueryContext(ctx, `SELECT uuid,model,vram_bytes FROM gpus WHERE node_id=? ORDER BY uuid`, node.ID)
		if err != nil {
			return nil, err
		}
		for gpuRows.Next() {
			var gpu domain.GPU
			if err := gpuRows.Scan(&gpu.UUID, &gpu.Model, &gpu.VRAMBytes); err != nil {
				gpuRows.Close()
				return nil, err
			}
			node.GPUs = append(node.GPUs, gpu)
		}
		gpuRows.Close()
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (s *Store) NodeByCredential(ctx context.Context, credentialHash string) (domain.Node, error) {
	var node domain.Node
	err := s.db.QueryRowContext(ctx, `SELECT id,name,status FROM nodes WHERE credential_hash=? OR (previous_credential_hash=? AND previous_credential_expires_at>?)`, credentialHash, credentialHash, formatTime(time.Now().UTC())).Scan(&node.ID, &node.Name, &node.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return node, ErrNotFound
	}
	return node, err
}

func (s *Store) RotateNodeCredential(ctx context.Context, nodeID, credentialHash string) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE nodes SET previous_credential_hash=credential_hash,previous_credential_expires_at=?,credential_hash=?,credential_created_at=? WHERE id=?`, formatTime(now.Add(10*time.Minute)), credentialHash, formatTime(now), nodeID)
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
	result, err := s.db.ExecContext(ctx, `UPDATE nodes SET status=? WHERE id=?`, status, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkStaleNodes(ctx context.Context, offlineBefore, lostBefore time.Time) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE nodes SET status='OFFLINE' WHERE status IN ('ONLINE','DEGRADED') AND last_heartbeat < ?`, formatTime(offlineBefore)); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET status='LOST',observed_status='LOST',version=version+1 WHERE status IN ('ASSIGNED','PULLING_IMAGE','STARTING','RUNNING','STOPPING') AND assigned_node_id IN (SELECT id FROM nodes WHERE last_heartbeat < ?)`, formatTime(lostBefore))
	return err
}

func (s *Store) AssignmentForNode(ctx context.Context, nodeID string) (domain.Assignment, error) {
	var assignment domain.Assignment
	var specJSON, gpusJSON, created string
	err := s.db.QueryRowContext(ctx, `SELECT a.id,a.job_id,a.attempt_id,j.spec_json,a.gpu_uuids_json,a.job_token_ciphertext,a.created_at FROM assignments a JOIN jobs j ON j.id=a.job_id WHERE a.node_id=? AND a.accepted_at IS NULL ORDER BY a.created_at LIMIT 1`, nodeID).Scan(&assignment.ID, &assignment.JobID, &assignment.AttemptID, &specJSON, &gpusJSON, &assignment.JobTokenEncrypted, &created)
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
	payload, _ := json.Marshal(event.Payload)
	_, err := s.db.ExecContext(ctx, `INSERT INTO job_events(job_id,sequence,type,payload_json,created_at) VALUES(?,?,?,?,?) ON CONFLICT(job_id,sequence) DO NOTHING`, event.JobID, event.Sequence, event.Type, payload, formatTime(event.CreatedAt))
	return err
}

func (s *Store) AppendServerEvent(ctx context.Context, jobID, eventType string, payload map[string]any) error {
	data, _ := json.Marshal(payload)
	for attempts := 0; attempts < 3; attempts++ {
		_, err := s.db.ExecContext(ctx, `INSERT INTO job_events(job_id,sequence,type,payload_json,created_at) SELECT ?,COALESCE(MAX(sequence),0)+1,?,?,? FROM job_events WHERE job_id=?`, jobID, eventType, data, formatTime(time.Now().UTC()), jobID)
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
	row := s.db.QueryRowContext(ctx, jobSelect+` JOIN job_attempts a ON a.job_id=j.id WHERE a.job_token_hash=?`, tokenHash)
	return scanJob(row)
}

func (s *Store) Events(ctx context.Context, jobID string, after int64) ([]domain.Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,job_id,sequence,type,payload_json,created_at FROM job_events WHERE job_id=? AND id>? ORDER BY id LIMIT 1000`, jobID, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []domain.Event
	for rows.Next() {
		var event domain.Event
		var payload, created string
		if err := rows.Scan(&event.ID, &event.JobID, &event.Sequence, &event.Type, &payload, &created); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(payload), &event.Payload)
		event.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		events = append(events, event)
	}
	return events, rows.Err()
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
	var result []SecretMetadata
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
	data, _ := json.Marshal(metadata)
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_events(actor_id,action,target_type,target_id,metadata_json,created_at) VALUES(?,?,?,?,?,?)`, nullable(actorID), action, targetType, targetID, data, formatTime(time.Now().UTC()))
	return err
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,COALESCE(actor_id,''),action,target_type,target_id,metadata_json,created_at FROM audit_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AuditEvent
	for rows.Next() {
		var event AuditEvent
		var metadata, created string
		if err := rows.Scan(&event.ID, &event.ActorID, &event.Action, &event.TargetType, &event.TargetID, &metadata, &created); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(metadata), &event.Metadata)
		event.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		result = append(result, event)
	}
	return result, rows.Err()
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

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
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
