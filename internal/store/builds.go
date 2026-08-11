package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

const buildSelect = `SELECT id,owner_id,name,mode,status,source_filename,source_size_bytes,source_sha256,oci_digest,failure_reason,created_at,started_at,finished_at,version FROM builds`

func (s *Store) CreateBuild(ctx context.Context, build domain.Build) error {
	if err := domain.ValidateBuild(build); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO builds(id,owner_id,name,mode,status,source_filename,source_size_bytes,source_sha256,created_at,version) VALUES(?,?,?,?,?,?,?,?,?,1)`, build.ID, build.OwnerID, build.Name, build.Mode, build.Status, build.Source.Filename, build.Source.Size, build.Source.SHA256, formatTime(build.CreatedAt))
	if err != nil {
		return mapConstraint(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO build_events(build_id,status,created_at) VALUES(?,?,?)`, build.ID, build.Status, formatTime(build.CreatedAt)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveBuildPlan(ctx context.Context, plan domain.BuildPlan) error {
	if err := domain.ValidateBuildPlan(plan); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status domain.BuildStatus
	if err = tx.QueryRowContext(ctx, `SELECT status FROM builds WHERE id=?`, plan.BuildID).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if status != domain.BuildAnalyzing {
		return ErrConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO build_plans(build_id,provider,runtime,package_manager,entrypoint,railpack_version,plan_json,info_json,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, plan.BuildID, plan.Provider, plan.Runtime, plan.PackageManager, plan.Entrypoint, plan.RailpackVersion, string(plan.Plan), string(plan.Info), formatTime(plan.CreatedAt))
	if err != nil {
		return mapConstraint(err)
	}
	return tx.Commit()
}

func (s *Store) BuildPlan(ctx context.Context, buildID string) (domain.BuildPlan, error) {
	var plan domain.BuildPlan
	var planJSON, infoJSON, created string
	var confirmed sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT build_id,provider,runtime,package_manager,entrypoint,railpack_version,plan_json,info_json,created_at,confirmed_at FROM build_plans WHERE build_id=?`, buildID).Scan(&plan.BuildID, &plan.Provider, &plan.Runtime, &plan.PackageManager, &plan.Entrypoint, &plan.RailpackVersion, &planJSON, &infoJSON, &created, &confirmed)
	if errors.Is(err, sql.ErrNoRows) {
		return plan, ErrNotFound
	}
	if err != nil {
		return plan, err
	}
	plan.Plan, plan.Info = json.RawMessage(planJSON), json.RawMessage(infoJSON)
	plan.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	plan.ConfirmedAt = parseNullTime(confirmed)
	return plan, nil
}

func (s *Store) ConfirmBuildPlan(ctx context.Context, buildID string) (domain.BuildPlan, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE build_plans SET confirmed_at=COALESCE(confirmed_at,?) WHERE build_id=? AND EXISTS(SELECT 1 FROM builds WHERE builds.id=build_plans.build_id AND builds.status='ANALYZING')`, formatTime(now), buildID)
	if err != nil {
		return domain.BuildPlan{}, err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return domain.BuildPlan{}, ErrConflict
	}
	return s.BuildPlan(ctx, buildID)
}

func (s *Store) Build(ctx context.Context, id string) (domain.Build, error) {
	return scanBuild(s.db.QueryRowContext(ctx, buildSelect+` WHERE id=?`, id))
}

func (s *Store) ListBuilds(ctx context.Context, ownerID string) ([]domain.Build, error) {
	query, args := buildSelect, []any{}
	if ownerID != "" {
		query += ` WHERE owner_id=?`
		args = append(args, ownerID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Build, 0)
	for rows.Next() {
		item, scanErr := scanBuild(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) BuildEvents(ctx context.Context, buildID string) ([]domain.BuildEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,build_id,status,message,created_at FROM build_events WHERE build_id=? ORDER BY id`, buildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.BuildEvent, 0)
	for rows.Next() {
		var item domain.BuildEvent
		var created string
		if err = rows.Scan(&item.ID, &item.BuildID, &item.Status, &item.Message, &created); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		items = append(items, item)
	}
	return items, rows.Err()
}

// UpdateBuildStatus records a lifecycle transition atomically. It is the
// persistence boundary used by future analyzers and isolated builders.
func (s *Store) UpdateBuildStatus(ctx context.Context, id string, status domain.BuildStatus, ociDigest, message string) (domain.Build, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Build{}, err
	}
	defer tx.Rollback()
	current, err := scanBuild(tx.QueryRowContext(ctx, buildSelect+` WHERE id=?`, id))
	if err != nil {
		return domain.Build{}, err
	}
	if current.Status == status {
		return current, nil
	}
	if !domain.CanBuildTransition(current.Status, status) {
		return domain.Build{}, ErrConflict
	}
	candidate := current
	candidate.Status, candidate.OCIDigest = status, ociDigest
	if status == domain.BuildFailed {
		candidate.FailureReason = message
	} else {
		candidate.FailureReason = ""
	}
	if err = domain.ValidateBuild(candidate); err != nil {
		return domain.Build{}, err
	}
	now := time.Now().UTC()
	var startedAt, finishedAt any
	if current.StartedAt == nil && (status == domain.BuildAnalyzing || status == domain.BuildBuilding) {
		startedAt = formatTime(now)
	}
	if status == domain.BuildSucceeded || status == domain.BuildFailed || status == domain.BuildCancelled {
		finishedAt = formatTime(now)
	}
	result, err := tx.ExecContext(ctx, `UPDATE builds SET status=?,oci_digest=?,failure_reason=?,started_at=COALESCE(started_at,?),finished_at=?,version=version+1 WHERE id=? AND status=?`, status, ociDigest, candidate.FailureReason, startedAt, finishedAt, id, current.Status)
	if err != nil {
		return domain.Build{}, err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return domain.Build{}, ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO build_events(build_id,status,message,created_at) VALUES(?,?,?,?)`, id, status, message, formatTime(now)); err != nil {
		return domain.Build{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Build{}, err
	}
	return s.Build(ctx, id)
}

func scanBuild(row scanner) (domain.Build, error) {
	var build domain.Build
	var created string
	var started, finished sql.NullString
	err := row.Scan(&build.ID, &build.OwnerID, &build.Name, &build.Mode, &build.Status, &build.Source.Filename, &build.Source.Size, &build.Source.SHA256, &build.OCIDigest, &build.FailureReason, &created, &started, &finished, &build.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return build, ErrNotFound
	}
	if err != nil {
		return build, err
	}
	build.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	build.StartedAt = parseNullTime(started)
	build.FinishedAt = parseNullTime(finished)
	return build, nil
}
