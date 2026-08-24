package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

type TableQuery struct {
	After, Offset int64
	Limit         int
	SortBy, Order string
	Filters       map[string]string
	AbsoluteSort  bool
}

func (s *Store) AppendTable(ctx context.Context, item domain.TableObservation) error {
	columns, _ := json.Marshal(item.Columns)
	tags, _ := json.Marshal(item.Tags)
	metadata, _ := json.Marshal(item.Metadata)
	captured := time.Now().UTC()
	if item.CapturedAt != nil {
		captured = item.CapturedAt.UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = ensureDeclaredSourceType(ctx, tx, item.AttemptID, item.Name, "table"); err != nil {
		return err
	}
	var currentSubtype, currentColumns, currentTags, currentMetadata sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT subtype,columns_json,tags_json,metadata_json FROM job_table_descriptors WHERE attempt_id=? AND name=?`, item.AttemptID, item.Name).Scan(&currentSubtype, &currentColumns, &currentTags, &currentMetadata)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && (currentSubtype.String != item.Subtype || currentColumns.String != string(columns) || currentTags.Valid != (len(item.Tags) > 0) || currentTags.Valid && currentTags.String != string(tags) || currentMetadata.Valid != (len(item.Metadata) > 0) || currentMetadata.Valid && currentMetadata.String != string(metadata)) {
		return ErrObservableDeclarationConflict
	}
	now := captured.UnixMilli()
	if _, err = tx.ExecContext(ctx, `INSERT INTO job_table_descriptors(job_id,attempt_id,name,subtype,columns_json,tags_json,metadata_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(attempt_id,name) DO UPDATE SET updated_at=excluded.updated_at`, item.JobID, item.AttemptID, item.Name, item.Subtype, columns, nullableJSON(tags, len(item.Tags)), nullableJSON(metadata, len(item.Metadata)), now, now); err != nil {
		return mapConstraint(err)
	}
	if item.Replace || item.Subtype == "categorical" || item.Subtype == "hierarchy" || item.Subtype == "waterfall" {
		if _, err = tx.ExecContext(ctx, `DELETE FROM job_table_rows WHERE job_id=? AND attempt_id=? AND name=?`, item.JobID, item.AttemptID, item.Name); err != nil {
			return err
		}
	}
	statement, err := tx.PrepareContext(ctx, `INSERT INTO job_table_rows(job_id,attempt_id,name,step,captured_at,record_json) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, record := range item.Rows {
		encoded, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = statement.ExecContext(ctx, item.JobID, item.AttemptID, item.Name, item.Step, now, encoded); err != nil {
			return mapConstraint(err)
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO job_rich_observable_descriptors(job_id,attempt_id,kind,name,created_at,updated_at,subtype,tags_json,metadata_json) VALUES(?,?,'table',?,?,?,?,?,?) ON CONFLICT(job_id,attempt_id,kind,name) DO UPDATE SET updated_at=excluded.updated_at`, item.JobID, item.AttemptID, item.Name, now, now, item.Subtype, nullableJSON(tags, len(item.Tags)), nullableJSON(metadata, len(item.Metadata))); err != nil {
		return mapConstraint(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO job_observation_updates(job_id,attempt_id,kind,observed_at) VALUES(?,?,'table',?)`, item.JobID, item.AttemptID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Table(ctx context.Context, jobID, attemptID, name string, query TableQuery) (domain.TablePage, error) {
	page := domain.TablePage{AttemptID: attemptID, Name: name, Items: []domain.TableRow{}}
	var columns, tags, metadata sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT subtype,columns_json,tags_json,metadata_json FROM job_table_descriptors WHERE job_id=? AND attempt_id=? AND name=?`, jobID, attemptID, name).Scan(&page.Subtype, &columns, &tags, &metadata); err != nil {
		return page, mapNoRows(err)
	}
	_ = json.Unmarshal([]byte(columns.String), &page.Columns)
	if tags.Valid {
		_ = json.Unmarshal([]byte(tags.String), &page.Tags)
	}
	if metadata.Valid {
		_ = json.Unmarshal([]byte(metadata.String), &page.Metadata)
	}
	columnSet := map[string]bool{}
	columnTypes := map[string]string{}
	for _, column := range page.Columns {
		columnSet[column.Name] = true
		columnTypes[column.Name] = column.Type
	}
	if query.SortBy != "" && !columnSet[query.SortBy] {
		return page, fmt.Errorf("unknown table sort column")
	}
	if query.AbsoluteSort && (query.SortBy == "" || columnTypes[query.SortBy] != "number" && columnTypes[query.SortBy] != "integer") {
		return page, fmt.Errorf("absolute table sorting requires a numeric sort column")
	}
	for column := range query.Filters {
		if !columnSet[column] {
			return page, fmt.Errorf("unknown table filter column")
		}
	}
	where := `job_id=? AND attempt_id=? AND name=?`
	args := []any{jobID, attemptID, name}
	if query.After > 0 {
		where += ` AND id>?`
		args = append(args, query.After)
	}
	for column, value := range query.Filters {
		where += ` AND CAST(json_extract(record_json, ?) AS TEXT)=?`
		if columnTypes[column] == "boolean" {
			if strings.EqualFold(value, "true") {
				value = "1"
			} else if strings.EqualFold(value, "false") {
				value = "0"
			}
		}
		args = append(args, `$."`+strings.ReplaceAll(column, `"`, `\"`)+`"`, value)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_table_rows WHERE `+where, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	order := `id ASC`
	if query.SortBy != "" {
		direction := `ASC`
		if query.Order == "desc" {
			direction = `DESC`
		}
		order = `json_extract(record_json, '$."` + strings.ReplaceAll(query.SortBy, `"`, `\"`) + `"') ` + direction + `,id ` + direction
	}
	if query.AbsoluteSort {
		direction := `ASC`
		if query.Order == "desc" {
			direction = `DESC`
		}
		order = `ABS(CAST(json_extract(record_json, '$."` + strings.ReplaceAll(query.SortBy, `"`, `\"`) + `"') AS REAL)) ` + direction + `,id ` + direction
	}
	rowArgs := append(append([]any{}, args...), query.Limit, query.Offset)
	rows, err := s.db.QueryContext(ctx, `SELECT id,step,captured_at,record_json FROM job_table_rows WHERE `+where+` ORDER BY `+order+` LIMIT ? OFFSET ?`, rowArgs...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var item domain.TableRow
		var step sql.NullInt64
		var captured int64
		var record string
		if err = rows.Scan(&item.Cursor, &step, &captured, &record); err != nil {
			return page, err
		}
		if step.Valid {
			item.Step = &step.Int64
		}
		item.CapturedAt = time.UnixMilli(captured).UTC()
		_ = json.Unmarshal([]byte(record), &item.Values)
		page.Items = append(page.Items, item)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) == query.Limit && query.SortBy == "" {
		next := page.Items[len(page.Items)-1].Cursor
		page.Next = &next
	}
	return page, nil
}

func mapNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
