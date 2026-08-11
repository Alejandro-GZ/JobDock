package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

const buildAssignmentSelect = `SELECT id,build_id,status,cancel_requested,builder_id,lease_expires_at,created_at,updated_at FROM build_assignments`

func (s *Store) QueueBuild(ctx context.Context, buildID, assignmentID string) (domain.BuildWork, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.BuildWork{}, err
	}
	defer tx.Rollback()
	build, err := scanBuild(tx.QueryRowContext(ctx, buildSelect+` WHERE id=?`, buildID))
	if err != nil {
		return domain.BuildWork{}, err
	}
	if existing, existingErr := scanBuildAssignment(tx.QueryRowContext(ctx, buildAssignmentSelect+` WHERE build_id=?`, buildID)); existingErr == nil {
		plan, _ := buildPlanTx(ctx, tx, buildID)
		return domain.BuildWork{Assignment: existing, Build: build, Plan: plan}, nil
	} else if !errors.Is(existingErr, ErrNotFound) {
		return domain.BuildWork{}, existingErr
	}
	now := time.Now().UTC()
	var plan *domain.BuildPlan
	switch build.Mode {
	case domain.BuildModeRailpack:
		if build.Status != domain.BuildAnalyzing {
			return domain.BuildWork{}, ErrConflict
		}
		plan, err = buildPlanTx(ctx, tx, buildID)
		if err != nil {
			return domain.BuildWork{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE build_plans SET confirmed_at=COALESCE(confirmed_at,?) WHERE build_id=?`, formatTime(now), buildID); err != nil {
			return domain.BuildWork{}, err
		}
		confirmed := now
		if plan.ConfirmedAt == nil {
			plan.ConfirmedAt = &confirmed
		}
	case domain.BuildModeDockerfile:
		if build.Status != domain.BuildCreated {
			return domain.BuildWork{}, ErrConflict
		}
	default:
		return domain.BuildWork{}, ErrConflict
	}
	result, err := tx.ExecContext(ctx, `UPDATE builds SET status='BUILDING',failure_reason='',started_at=COALESCE(started_at,?),finished_at=NULL,version=version+1 WHERE id=? AND status=?`, formatTime(now), buildID, build.Status)
	if err != nil {
		return domain.BuildWork{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return domain.BuildWork{}, ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO build_events(build_id,status,message,created_at) VALUES(?,?,?,?)`, buildID, domain.BuildBuilding, "Queued for isolated BuildKit execution", formatTime(now)); err != nil {
		return domain.BuildWork{}, err
	}
	assignment := domain.BuildAssignment{ID: assignmentID, BuildID: buildID, Status: domain.BuildAssignmentPending, CreatedAt: now, UpdatedAt: now}
	if err = domain.ValidateBuildAssignment(assignment); err != nil {
		return domain.BuildWork{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO build_assignments(id,build_id,status,cancel_requested,created_at,updated_at) VALUES(?,?,?,0,?,?)`, assignment.ID, assignment.BuildID, assignment.Status, formatTime(now), formatTime(now)); err != nil {
		return domain.BuildWork{}, mapConstraint(err)
	}
	if err = tx.Commit(); err != nil {
		return domain.BuildWork{}, err
	}
	build, err = s.Build(ctx, buildID)
	return domain.BuildWork{Assignment: assignment, Build: build, Plan: plan}, err
}

func (s *Store) NextBuildWork(ctx context.Context, builderID string, lease time.Duration) (domain.BuildWork, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.BuildWork{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	assignment, err := scanBuildAssignment(tx.QueryRowContext(ctx, buildAssignmentSelect+` WHERE cancel_requested=0 AND (status='PENDING' OR (status='RUNNING' AND (builder_id=? OR lease_expires_at<?))) ORDER BY CASE WHEN builder_id=? THEN 0 ELSE 1 END,created_at LIMIT 1`, builderID, formatTime(now), builderID))
	if err != nil {
		return domain.BuildWork{}, err
	}
	leaseExpiry := now.Add(lease)
	result, err := tx.ExecContext(ctx, `UPDATE build_assignments SET status='RUNNING',builder_id=?,lease_expires_at=?,updated_at=? WHERE id=? AND cancel_requested=0 AND updated_at=? AND (status='PENDING' OR (status='RUNNING' AND (builder_id=? OR lease_expires_at<?)))`, builderID, formatTime(leaseExpiry), formatTime(now), assignment.ID, formatTime(assignment.UpdatedAt), builderID, formatTime(now))
	if err != nil {
		return domain.BuildWork{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return domain.BuildWork{}, ErrConflict
	}
	assignment.Status, assignment.BuilderID, assignment.LeaseExpiresAt, assignment.UpdatedAt = domain.BuildAssignmentRunning, builderID, &leaseExpiry, now
	build, err := scanBuild(tx.QueryRowContext(ctx, buildSelect+` WHERE id=?`, assignment.BuildID))
	if err != nil {
		return domain.BuildWork{}, err
	}
	plan, err := buildPlanTx(ctx, tx, build.ID)
	if errors.Is(err, ErrNotFound) && build.Mode == domain.BuildModeDockerfile {
		plan, err = nil, nil
	}
	if err != nil {
		return domain.BuildWork{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.BuildWork{}, err
	}
	return domain.BuildWork{Assignment: assignment, Build: build, Plan: plan}, nil
}

func (s *Store) BuildAssignment(ctx context.Context, id string) (domain.BuildAssignment, error) {
	return scanBuildAssignment(s.db.QueryRowContext(ctx, buildAssignmentSelect+` WHERE id=?`, id))
}

func (s *Store) RenewBuildAssignment(ctx context.Context, id, builderID string, lease time.Duration) (domain.BuildAssignment, error) {
	now := time.Now().UTC()
	expiry := now.Add(lease)
	result, err := s.db.ExecContext(ctx, `UPDATE build_assignments SET lease_expires_at=?,updated_at=? WHERE id=? AND builder_id=? AND status='RUNNING'`, formatTime(expiry), formatTime(now), id, builderID)
	if err != nil {
		return domain.BuildAssignment{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return domain.BuildAssignment{}, ErrConflict
	}
	return s.BuildAssignment(ctx, id)
}

func (s *Store) RequestBuildCancellation(ctx context.Context, buildID string) (domain.Build, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Build{}, err
	}
	defer tx.Rollback()
	build, err := scanBuild(tx.QueryRowContext(ctx, buildSelect+` WHERE id=?`, buildID))
	if err != nil {
		return domain.Build{}, err
	}
	if build.Status == domain.BuildCancelled {
		return build, nil
	}
	if build.Status != domain.BuildBuilding {
		return domain.Build{}, ErrConflict
	}
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE build_assignments SET cancel_requested=1,status=CASE WHEN status='PENDING' THEN 'CANCELLED' ELSE status END,updated_at=? WHERE build_id=? AND status IN ('PENDING','RUNNING')`, formatTime(now), buildID)
	if err != nil {
		return domain.Build{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return domain.Build{}, ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE builds SET status='CANCELLED',failure_reason='',finished_at=?,version=version+1 WHERE id=? AND status='BUILDING'`, formatTime(now), buildID); err != nil {
		return domain.Build{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO build_events(build_id,status,message,created_at) VALUES(?,?,?,?)`, buildID, domain.BuildCancelled, "Cancellation requested by user", formatTime(now)); err != nil {
		return domain.Build{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Build{}, err
	}
	return s.Build(ctx, buildID)
}

func (s *Store) CompleteBuildAssignment(ctx context.Context, id, builderID string, status domain.BuildAssignmentStatus, digest, message string) (domain.Build, error) {
	if status != domain.BuildAssignmentSucceeded && status != domain.BuildAssignmentFailed && status != domain.BuildAssignmentCancelled {
		return domain.Build{}, errors.New("build assignment completion must be terminal")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Build{}, err
	}
	defer tx.Rollback()
	assignment, err := scanBuildAssignment(tx.QueryRowContext(ctx, buildAssignmentSelect+` WHERE id=?`, id))
	if err != nil {
		return domain.Build{}, err
	}
	if assignment.BuilderID != builderID || (assignment.Status != domain.BuildAssignmentRunning && assignment.Status != status) {
		return domain.Build{}, ErrConflict
	}
	build, err := scanBuild(tx.QueryRowContext(ctx, buildSelect+` WHERE id=?`, assignment.BuildID))
	if err != nil {
		return domain.Build{}, err
	}
	if assignment.CancelRequested || build.Status == domain.BuildCancelled {
		status, digest, message = domain.BuildAssignmentCancelled, "", "Build cancelled"
	}
	now := time.Now().UTC()
	if build.Status == domain.BuildBuilding {
		buildStatus := domain.BuildSucceeded
		failureReason := ""
		switch status {
		case domain.BuildAssignmentFailed:
			buildStatus, digest, failureReason = domain.BuildFailed, "", message
		case domain.BuildAssignmentCancelled:
			buildStatus, digest = domain.BuildCancelled, ""
		}
		candidate := build
		candidate.Status, candidate.OCIDigest, candidate.FailureReason = buildStatus, digest, failureReason
		if err = domain.ValidateBuild(candidate); err != nil {
			return domain.Build{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE builds SET status=?,oci_digest=?,failure_reason=?,finished_at=?,version=version+1 WHERE id=? AND status='BUILDING'`, buildStatus, digest, failureReason, formatTime(now), build.ID); err != nil {
			return domain.Build{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO build_events(build_id,status,message,created_at) VALUES(?,?,?,?)`, build.ID, buildStatus, message, formatTime(now)); err != nil {
			return domain.Build{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE build_assignments SET status=?,lease_expires_at=NULL,updated_at=? WHERE id=?`, status, formatTime(now), id); err != nil {
		return domain.Build{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Build{}, err
	}
	return s.Build(ctx, build.ID)
}

func buildPlanTx(ctx context.Context, tx *sql.Tx, buildID string) (*domain.BuildPlan, error) {
	var plan domain.BuildPlan
	var planJSON, infoJSON, created string
	var confirmed sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT build_id,provider,runtime,package_manager,entrypoint,railpack_version,plan_json,info_json,created_at,confirmed_at FROM build_plans WHERE build_id=?`, buildID).Scan(&plan.BuildID, &plan.Provider, &plan.Runtime, &plan.PackageManager, &plan.Entrypoint, &plan.RailpackVersion, &planJSON, &infoJSON, &created, &confirmed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	plan.Plan, plan.Info = []byte(planJSON), []byte(infoJSON)
	plan.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	plan.ConfirmedAt = parseNullTime(confirmed)
	return &plan, nil
}

func scanBuildAssignment(row scanner) (domain.BuildAssignment, error) {
	var assignment domain.BuildAssignment
	var cancel int
	var builder, lease sql.NullString
	var created, updated string
	err := row.Scan(&assignment.ID, &assignment.BuildID, &assignment.Status, &cancel, &builder, &lease, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return assignment, ErrNotFound
	}
	if err != nil {
		return assignment, err
	}
	assignment.CancelRequested = cancel != 0
	assignment.BuilderID = builder.String
	assignment.LeaseExpiresAt = parseNullTime(lease)
	assignment.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	assignment.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return assignment, nil
}
