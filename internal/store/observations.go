package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

func (s *Store) DefineMilestones(ctx context.Context, jobID, attemptID string, items []domain.Milestone) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM job_milestone_definitions WHERE attempt_id=?`, attemptID); err != nil {
		return err
	}
	for index, item := range items {
		metadata, _ := json.Marshal(item.Metadata)
		if len(item.Metadata) == 0 {
			metadata = nil
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO job_milestone_definitions(job_id,attempt_id,position,name,weight,metadata_json) VALUES(?,?,?,?,?,?)`, jobID, attemptID, index, item.Name, item.Weight, metadata); err != nil {
			return mapConstraint(err)
		}
	}
	return tx.Commit()
}

func (s *Store) AppendProgress(ctx context.Context, jobID, attemptID, kind string, observation domain.ProgressObservation) error {
	metadata, _ := json.Marshal(observation.Metadata)
	if len(observation.Metadata) == 0 {
		metadata = nil
	}
	captured := time.Now().UTC()
	if observation.CapturedAt != nil {
		captured = observation.CapturedAt.UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = ensureDeclaredSourceType(ctx, tx, attemptID, "progress", "progress"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO job_progress_observations(job_id,attempt_id,kind,value,milestone,step,captured_at,metadata_json) VALUES(?,?,?,?,?,?,?,?)`, jobID, attemptID, kind, observation.Value, nullableString(observation.Milestone), observation.Step, captured.UnixMilli(), metadata); err != nil {
		return mapConstraint(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO job_rich_observable_descriptors(job_id,attempt_id,kind,name,created_at,updated_at) VALUES(?,?, 'progress','progress',?,?) ON CONFLICT(job_id,attempt_id,kind,name) DO UPDATE SET updated_at=excluded.updated_at`, jobID, attemptID, captured.UnixMilli(), captured.UnixMilli()); err != nil {
		return mapConstraint(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO job_observation_updates(job_id,attempt_id,kind,observed_at) VALUES(?,?, 'progress',?)`, jobID, attemptID, captured.UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ProgressState(ctx context.Context, jobID, attemptID string) (domain.ProgressState, error) {
	state := domain.ProgressState{AttemptID: attemptID, Milestones: []domain.Milestone{}, Reached: []string{}}
	rows, err := s.db.QueryContext(ctx, `SELECT name,weight,metadata_json FROM job_milestone_definitions WHERE job_id=? AND attempt_id=? ORDER BY position`, jobID, attemptID)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var item domain.Milestone
		var weight sql.NullFloat64
		var metadata sql.NullString
		if err = rows.Scan(&item.Name, &weight, &metadata); err != nil {
			rows.Close()
			return state, err
		}
		if weight.Valid {
			item.Weight = &weight.Float64
		}
		if metadata.Valid {
			_ = json.Unmarshal([]byte(metadata.String), &item.Metadata)
		}
		state.Milestones = append(state.Milestones, item)
	}
	rows.Close()
	progressRows, err := s.db.QueryContext(ctx, `SELECT kind,value,milestone,step,captured_at,metadata_json FROM job_progress_observations WHERE job_id=? AND attempt_id=? ORDER BY captured_at,id`, jobID, attemptID)
	if err != nil {
		return state, err
	}
	reached := map[string]bool{}
	var latest *time.Time
	for progressRows.Next() {
		var kind string
		var value sql.NullFloat64
		var milestone sql.NullString
		var step sql.NullInt64
		var captured int64
		var metadata sql.NullString
		if err = progressRows.Scan(&kind, &value, &milestone, &step, &captured, &metadata); err != nil {
			progressRows.Close()
			return state, err
		}
		at := time.UnixMilli(captured).UTC()
		latest = &at
		observation := domain.ProgressObservation{Milestone: milestone.String, ObservationContext: domain.ObservationContext{CapturedAt: &at}}
		if value.Valid {
			observation.Value = value.Float64
		}
		if step.Valid {
			observation.Step = &step.Int64
		}
		if metadata.Valid {
			_ = json.Unmarshal([]byte(metadata.String), &observation.Metadata)
		}
		switch kind {
		case "simple":
			state.Simple = &observation
		case "segment":
			state.Current = &observation
		case "milestone":
			if !reached[observation.Milestone] {
				reached[observation.Milestone] = true
				state.Reached = append(state.Reached, observation.Milestone)
			}
		}
	}
	progressRows.Close()
	state.UpdatedAt = latest
	state.GlobalProgress = calculateGlobalProgress(state)
	return state, nil
}

func calculateGlobalProgress(state domain.ProgressState) *float64 {
	if len(state.Milestones) == 0 {
		return nil
	}
	total, completed := 0.0, 0.0
	allWeighted := true
	reached := map[string]bool{}
	for _, name := range state.Reached {
		reached[name] = true
	}
	for _, item := range state.Milestones {
		if item.Weight == nil {
			allWeighted = false
			break
		}
		total += *item.Weight
		if reached[item.Name] {
			completed += *item.Weight
		}
	}
	if !allWeighted || total <= 0 {
		return nil
	}
	if state.Current != nil && !reached[state.Current.Milestone] {
		for _, item := range state.Milestones {
			if item.Name == state.Current.Milestone {
				completed += *item.Weight * state.Current.Value
				break
			}
		}
	}
	value := completed / total
	if value > 1 {
		value = 1
	}
	return &value
}

func (s *Store) AppendMatrix(ctx context.Context, item domain.MatrixObservation) (domain.MatrixObservation, error) {
	labels, _ := json.Marshal(item.Labels)
	values, _ := json.Marshal(item.Values)
	metadata, _ := json.Marshal(item.Metadata)
	if len(item.Metadata) == 0 {
		metadata = nil
	}
	captured := time.Now().UTC()
	if item.CapturedAt != nil {
		captured = item.CapturedAt.UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return item, err
	}
	defer tx.Rollback()
	if err = ensureDeclaredSourceType(ctx, tx, item.AttemptID, item.Name, "matrix"); err != nil {
		return item, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO job_matrix_observations(job_id,attempt_id,name,step,captured_at,labels_json,values_json,metadata_json) VALUES(?,?,?,?,?,?,?,?)`, item.JobID, item.AttemptID, item.Name, item.Step, captured.UnixMilli(), labels, values, metadata)
	if err != nil {
		return item, mapConstraint(err)
	}
	item.ID, _ = result.LastInsertId()
	item.CapturedAt = &captured
	if _, err = tx.ExecContext(ctx, `INSERT INTO job_rich_observable_descriptors(job_id,attempt_id,kind,name,created_at,updated_at) VALUES(?,?, 'matrix',?,?,?) ON CONFLICT(job_id,attempt_id,kind,name) DO UPDATE SET updated_at=excluded.updated_at`, item.JobID, item.AttemptID, item.Name, captured.UnixMilli(), captured.UnixMilli()); err != nil {
		return item, mapConstraint(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO job_observation_updates(job_id,attempt_id,kind,observed_at) VALUES(?,?, 'matrix',?)`, item.JobID, item.AttemptID, captured.UnixMilli()); err != nil {
		return item, err
	}
	return item, tx.Commit()
}

func (s *Store) Matrices(ctx context.Context, jobID, attemptID, name string, step *int64, limit int) ([]domain.MatrixObservation, error) {
	args := []any{jobID, attemptID}
	where := "job_id=? AND attempt_id=?"
	if name != "" {
		where += " AND name=?"
		args = append(args, name)
	}
	if step != nil {
		where += " AND step=?"
		args = append(args, *step)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,step,captured_at,labels_json,values_json,metadata_json FROM job_matrix_observations WHERE `+where+` ORDER BY captured_at DESC,id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.MatrixObservation{}
	for rows.Next() {
		var item domain.MatrixObservation
		var st sql.NullInt64
		var captured int64
		var labels, values string
		var metadata sql.NullString
		if err = rows.Scan(&item.ID, &item.Name, &st, &captured, &labels, &values, &metadata); err != nil {
			return nil, err
		}
		item.AttemptID = attemptID
		at := time.UnixMilli(captured).UTC()
		item.CapturedAt = &at
		if st.Valid {
			item.Step = &st.Int64
		}
		_ = json.Unmarshal([]byte(labels), &item.Labels)
		_ = json.Unmarshal([]byte(values), &item.Values)
		if metadata.Valid {
			_ = json.Unmarshal([]byte(metadata.String), &item.Metadata)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ConfirmedCheckpoints(ctx context.Context, jobID, attemptID string, after int64, limit int) ([]domain.CheckpointSync, bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT rowid,id FROM checkpoint_syncs WHERE job_id=? AND attempt_id=? AND confirmed_at IS NOT NULL AND rowid>? ORDER BY rowid LIMIT ?`, jobID, attemptID, after, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	type record struct {
		cursor int64
		id     string
	}
	records := []record{}
	for rows.Next() {
		var item record
		if err = rows.Scan(&item.cursor, &item.id); err != nil {
			return nil, false, err
		}
		records = append(records, item)
	}
	more := len(records) > limit
	if more {
		records = records[:limit]
	}
	items := []domain.CheckpointSync{}
	for _, record := range records {
		item, loadErr := s.CheckpointSync(ctx, record.id)
		if loadErr != nil {
			return nil, false, loadErr
		}
		item.Cursor = record.cursor
		items = append(items, item)
	}
	return items, more, rows.Err()
}

func (s *Store) AppendObservationUpdate(ctx context.Context, jobID, attemptID, kind string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO job_observation_updates(job_id,attempt_id,kind,observed_at) VALUES(?,?,?,?)`, jobID, attemptID, kind, at.UTC().UnixMilli())
	return err
}
func (s *Store) ObservationUpdates(ctx context.Context, jobID, attemptID string, after int64, limit int) ([]map[string]any, bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,kind,observed_at FROM job_observation_updates WHERE job_id=? AND attempt_id=? AND id>? ORDER BY id LIMIT ?`, jobID, attemptID, after, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, at int64
		var kind string
		if err = rows.Scan(&id, &kind, &at); err != nil {
			return nil, false, err
		}
		items = append(items, map[string]any{"cursor": id, "attempt_id": attemptID, "kind": kind, "observed_at": time.UnixMilli(at).UTC()})
	}
	more := len(items) > limit
	if more {
		items = items[:limit]
	}
	return items, more, rows.Err()
}

func (s *Store) LatestObservationCursor(ctx context.Context, jobID, attemptID string) (int64, error) {
	var cursor sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(id) FROM job_observation_updates WHERE job_id=? AND attempt_id=?`, jobID, attemptID).Scan(&cursor); err != nil {
		return 0, err
	}
	return cursor.Int64, nil
}

func scanCheckpointObservation(row *sql.Row) (domain.CheckpointSync, error) {
	var item domain.CheckpointSync
	var requested string
	var confirmed sql.NullString
	var label, metadata sql.NullString
	var step, observed sql.NullInt64
	err := row.Scan(&item.ID, &item.JobID, &item.AttemptID, &requested, &confirmed, &item.FileCount, &item.ByteCount, &label, &step, &observed, &metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrNotFound
	}
	if err != nil {
		return item, err
	}
	item.RequestedAt, _ = time.Parse(time.RFC3339Nano, requested)
	item.Status = "PENDING"
	if confirmed.Valid {
		value, _ := time.Parse(time.RFC3339Nano, confirmed.String)
		item.ConfirmedAt = &value
		item.Status = "CONFIRMED"
	}
	item.Label = label.String
	if step.Valid {
		item.Step = &step.Int64
	}
	if observed.Valid {
		value := time.UnixMilli(observed.Int64).UTC()
		item.ObservedAt = &value
	}
	if metadata.Valid {
		_ = json.Unmarshal([]byte(metadata.String), &item.Metadata)
	}
	return item, nil
}
