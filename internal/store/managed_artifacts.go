package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

const managedArtifactSelect = `SELECT build_id,owner_id,digest,sha256,size_bytes,media_type,runtime_image,created_at,last_referenced_at FROM managed_build_artifacts`

func (s *Store) SaveManagedArtifact(ctx context.Context, artifact domain.ManagedArtifact) error {
	if err := domain.ValidateManagedArtifact(artifact); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var ownerID string
	var status domain.BuildStatus
	if err = tx.QueryRowContext(ctx, `SELECT owner_id,status FROM builds WHERE id=?`, artifact.BuildID).Scan(&ownerID, &status); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if ownerID != artifact.OwnerID || status != domain.BuildBuilding {
		return ErrConflict
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO managed_build_artifacts(build_id,owner_id,digest,sha256,size_bytes,media_type,runtime_image,created_at,last_referenced_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(build_id) DO UPDATE SET last_referenced_at=excluded.last_referenced_at WHERE digest=excluded.digest AND sha256=excluded.sha256 AND size_bytes=excluded.size_bytes AND runtime_image=excluded.runtime_image`, artifact.BuildID, artifact.OwnerID, artifact.Digest, artifact.SHA256, artifact.Size, artifact.MediaType, artifact.RuntimeImage, formatTime(artifact.CreatedAt), formatTime(artifact.LastReferencedAt))
	if err != nil {
		return mapConstraint(err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return ErrConflict
	}
	return tx.Commit()
}

func (s *Store) ManagedArtifact(ctx context.Context, buildID string) (domain.ManagedArtifact, error) {
	return scanManagedArtifact(s.db.QueryRowContext(ctx, managedArtifactSelect+` WHERE build_id=?`, buildID))
}

func (s *Store) ManagedArtifactForOwner(ctx context.Context, buildID, ownerID, digest string) (domain.ManagedArtifact, error) {
	return scanManagedArtifact(s.db.QueryRowContext(ctx, managedArtifactSelect+` a WHERE a.build_id=? AND a.owner_id=? AND a.digest=? AND EXISTS(SELECT 1 FROM builds b WHERE b.id=a.build_id AND b.status='SUCCEEDED')`, buildID, ownerID, digest))
}

func (s *Store) ManagedArtifactForAssignment(ctx context.Context, assignmentID, nodeID string) (domain.ManagedArtifact, error) {
	artifact, err := scanManagedArtifact(s.db.QueryRowContext(ctx, managedArtifactSelect+` a WHERE ('jobdock://build/'||a.build_id||'@'||a.digest)=(SELECT json_extract(j.spec_json,'$.image') FROM assignments x JOIN jobs j ON j.id=x.job_id WHERE x.id=? AND x.node_id=? AND x.attempt_id=j.attempt_id AND j.status IN ('ASSIGNED','PULLING_IMAGE','STARTING','RUNNING'))`, assignmentID, nodeID))
	if err == nil {
		_, _ = s.db.ExecContext(ctx, `UPDATE managed_build_artifacts SET last_referenced_at=? WHERE build_id=?`, formatTime(time.Now().UTC()), artifact.BuildID)
	}
	return artifact, err
}

func (s *Store) CreateJobWithManagedArtifact(ctx context.Context, job domain.Job, buildID, digest string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var found int
	if err = tx.QueryRowContext(ctx, `SELECT 1 FROM managed_build_artifacts a JOIN builds b ON b.id=a.build_id WHERE a.build_id=? AND a.owner_id=? AND a.digest=? AND b.status='SUCCEEDED'`, buildID, job.OwnerID, digest).Scan(&found); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	spec, _ := json.Marshal(job.Spec)
	now := formatTime(time.Now().UTC())
	if _, err = tx.ExecContext(ctx, `INSERT INTO jobs(id,owner_id,spec_json,status,desired_status,observed_status,created_at,version) VALUES(?,?,?,?,?,?,?,1)`, job.ID, job.OwnerID, spec, job.Status, job.DesiredStatus, job.ObservedStatus, formatTime(job.CreatedAt)); err != nil {
		return mapConstraint(err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE managed_build_artifacts SET last_referenced_at=? WHERE build_id=?`, now, buildID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GarbageCollectManagedArtifacts(ctx context.Context, before time.Time) ([]domain.ManagedArtifact, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, managedArtifactSelect+` a WHERE a.last_referenced_at<? AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.status<>'DELETED' AND json_extract(j.spec_json,'$.image')=('jobdock://build/'||a.build_id||'@'||a.digest))`, formatTime(before))
	if err != nil {
		return nil, err
	}
	var artifacts []domain.ManagedArtifact
	for rows.Next() {
		artifact, scanErr := scanManagedArtifact(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		artifacts = append(artifacts, artifact)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for _, artifact := range artifacts {
		if _, err = tx.ExecContext(ctx, `DELETE FROM managed_build_artifacts WHERE build_id=? AND last_referenced_at<? AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.status<>'DELETED' AND json_extract(j.spec_json,'$.image')=?)`, artifact.BuildID, formatTime(before), domain.ManagedArtifactReference(artifact.BuildID, artifact.Digest)); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return artifacts, nil
}

func scanManagedArtifact(row scanner) (domain.ManagedArtifact, error) {
	var artifact domain.ManagedArtifact
	var created, referenced string
	err := row.Scan(&artifact.BuildID, &artifact.OwnerID, &artifact.Digest, &artifact.SHA256, &artifact.Size, &artifact.MediaType, &artifact.RuntimeImage, &created, &referenced)
	if errors.Is(err, sql.ErrNoRows) {
		return artifact, ErrNotFound
	}
	if err != nil {
		return artifact, err
	}
	artifact.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	artifact.LastReferencedAt, _ = time.Parse(time.RFC3339Nano, referenced)
	return artifact, nil
}
