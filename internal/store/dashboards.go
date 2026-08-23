package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type DashboardPreference struct {
	ID                    string     `json:"id"`
	UserID                string     `json:"-"`
	JobID                 string     `json:"-"`
	Name                  string     `json:"name"`
	SchemaVersion         int        `json:"schema_version"`
	ConfigJSON            []byte     `json:"-"`
	SortOrder             int        `json:"sort_order"`
	IsDefault             bool       `json:"is_default"`
	TemplateID            string     `json:"-"`
	TemplateVersion       int        `json:"-"`
	TemplateSchemaVersion int        `json:"-"`
	TemplateAppliedAt     *time.Time `json:"-"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func (s *Store) DashboardPreference(ctx context.Context, userID, jobID string) (DashboardPreference, error) {
	var dashboardID string
	err := s.db.QueryRowContext(ctx, `SELECT dashboard_id FROM active_job_dashboards WHERE user_id=? AND job_id=?`, userID, jobID).Scan(&dashboardID)
	if errors.Is(err, sql.ErrNoRows) {
		return DashboardPreference{}, ErrNotFound
	}
	if err != nil {
		return DashboardPreference{}, err
	}
	return s.Dashboard(ctx, userID, jobID, dashboardID)
}

func (s *Store) Dashboard(ctx context.Context, userID, jobID, dashboardID string) (DashboardPreference, error) {
	var item DashboardPreference
	var created, updated string
	var applied sql.NullString
	var isDefault int
	err := s.db.QueryRowContext(ctx, `SELECT id,user_id,job_id,name,schema_version,config_json,sort_order,is_default,COALESCE(template_id,''),COALESCE(template_version,0),COALESCE(template_schema_version,0),template_applied_at,created_at,updated_at FROM job_dashboards WHERE id=? AND user_id=? AND job_id=?`, dashboardID, userID, jobID).Scan(&item.ID, &item.UserID, &item.JobID, &item.Name, &item.SchemaVersion, &item.ConfigJSON, &item.SortOrder, &isDefault, &item.TemplateID, &item.TemplateVersion, &item.TemplateSchemaVersion, &applied, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrNotFound
	}
	item.IsDefault = isDefault == 1
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if applied.Valid {
		parsed, parseErr := time.Parse(time.RFC3339Nano, applied.String)
		if parseErr == nil {
			item.TemplateAppliedAt = &parsed
		}
	}
	return item, err
}

func (s *Store) PutDashboardPreference(ctx context.Context, item DashboardPreference) error {
	if item.ID == "" {
		existing, err := s.DashboardPreference(ctx, item.UserID, item.JobID)
		if err == ErrNotFound {
			item.ID = "legacy-" + item.UserID + "-" + item.JobID
			item.Name = "Dashboard"
			item.IsDefault = true
			if item.CreatedAt.IsZero() {
				item.CreatedAt = item.UpdatedAt
			}
			return s.CreateDashboard(ctx, item, true)
		}
		if err != nil {
			return err
		}
		item.ID = existing.ID
		item.Name, item.CreatedAt = existing.Name, existing.CreatedAt
	}
	return s.PutDashboard(ctx, item)
}

func (s *Store) PutDashboard(ctx context.Context, item DashboardPreference) error {
	item.UpdatedAt = item.UpdatedAt.UTC()
	var applied any
	if item.TemplateAppliedAt != nil {
		applied = formatTime(item.TemplateAppliedAt.UTC())
	}
	result, err := s.db.ExecContext(ctx, `UPDATE job_dashboards SET schema_version=?,config_json=?,template_id=?,template_version=?,template_schema_version=?,template_applied_at=?,updated_at=? WHERE id=? AND user_id=? AND job_id=?`, item.SchemaVersion, string(item.ConfigJSON), nullable(item.TemplateID), nullablePositive(item.TemplateVersion), nullablePositive(item.TemplateSchemaVersion), applied, formatTime(item.UpdatedAt), item.ID, item.UserID, item.JobID)
	if err != nil {
		return mapConstraint(err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListDashboards(ctx context.Context, userID, jobID string) ([]DashboardPreference, string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,user_id,job_id,name,schema_version,config_json,sort_order,is_default,COALESCE(template_id,''),COALESCE(template_version,0),COALESCE(template_schema_version,0),template_applied_at,created_at,updated_at FROM job_dashboards WHERE user_id=? AND job_id=? ORDER BY sort_order,id`, userID, jobID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]DashboardPreference, 0)
	for rows.Next() {
		var item DashboardPreference
		var isDefault int
		var applied sql.NullString
		var created, updated string
		if err = rows.Scan(&item.ID, &item.UserID, &item.JobID, &item.Name, &item.SchemaVersion, &item.ConfigJSON, &item.SortOrder, &isDefault, &item.TemplateID, &item.TemplateVersion, &item.TemplateSchemaVersion, &applied, &created, &updated); err != nil {
			return nil, "", err
		}
		item.IsDefault = isDefault == 1
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		if applied.Valid {
			if parsed, parseErr := time.Parse(time.RFC3339Nano, applied.String); parseErr == nil {
				item.TemplateAppliedAt = &parsed
			}
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, "", err
	}
	var activeID string
	err = s.db.QueryRowContext(ctx, `SELECT dashboard_id FROM active_job_dashboards WHERE user_id=? AND job_id=?`, userID, jobID).Scan(&activeID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return items, activeID, err
}

func (s *Store) CreateDashboard(ctx context.Context, item DashboardPreference, makeActive bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count, nextOrder int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MAX(sort_order),-1)+1 FROM job_dashboards WHERE user_id=? AND job_id=?`, item.UserID, item.JobID).Scan(&count, &nextOrder); err != nil {
		return err
	}
	if count >= 32 {
		return ErrConflict
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = item.CreatedAt
	}
	item.SortOrder = nextOrder
	item.IsDefault = count == 0 || item.IsDefault
	var applied any
	if item.TemplateAppliedAt != nil {
		applied = formatTime(item.TemplateAppliedAt.UTC())
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO job_dashboards(id,user_id,job_id,name,schema_version,config_json,sort_order,is_default,template_id,template_version,template_schema_version,template_applied_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.ID, item.UserID, item.JobID, item.Name, item.SchemaVersion, string(item.ConfigJSON), item.SortOrder, boolInt(item.IsDefault), nullable(item.TemplateID), nullablePositive(item.TemplateVersion), nullablePositive(item.TemplateSchemaVersion), applied, formatTime(item.CreatedAt.UTC()), formatTime(item.UpdatedAt.UTC()))
	if err != nil {
		return mapConstraint(err)
	}
	if makeActive || count == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO active_job_dashboards(user_id,job_id,dashboard_id,updated_at) VALUES(?,?,?,?) ON CONFLICT(user_id,job_id) DO UPDATE SET dashboard_id=excluded.dashboard_id,updated_at=excluded.updated_at`, item.UserID, item.JobID, item.ID, formatTime(item.UpdatedAt.UTC()))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) RenameDashboard(ctx context.Context, userID, jobID, dashboardID, name string) (DashboardPreference, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE job_dashboards SET name=?,updated_at=? WHERE id=? AND user_id=? AND job_id=?`, name, formatTime(time.Now().UTC()), dashboardID, userID, jobID)
	if err != nil {
		return DashboardPreference{}, mapConstraint(err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return DashboardPreference{}, ErrNotFound
	}
	return s.Dashboard(ctx, userID, jobID, dashboardID)
}

func (s *Store) SetActiveDashboard(ctx context.Context, userID, jobID, dashboardID string) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_dashboards WHERE id=? AND user_id=? AND job_id=?`, dashboardID, userID, jobID).Scan(&exists); err != nil {
		return err
	}
	if exists != 1 {
		return ErrNotFound
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO active_job_dashboards(user_id,job_id,dashboard_id,updated_at) VALUES(?,?,?,?) ON CONFLICT(user_id,job_id) DO UPDATE SET dashboard_id=excluded.dashboard_id,updated_at=excluded.updated_at`, userID, jobID, dashboardID, formatTime(time.Now().UTC()))
	return err
}

func (s *Store) SetDefaultDashboard(ctx context.Context, userID, jobID, dashboardID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_dashboards WHERE id=? AND user_id=? AND job_id=?`, dashboardID, userID, jobID).Scan(&exists); err != nil {
		return err
	}
	if exists != 1 {
		return ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `UPDATE job_dashboards SET is_default=0 WHERE user_id=? AND job_id=?`, userID, jobID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE job_dashboards SET is_default=1,updated_at=? WHERE id=?`, formatTime(time.Now().UTC()), dashboardID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteDashboard(ctx context.Context, userID, jobID, dashboardID string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var wasDefault int
	if err = tx.QueryRowContext(ctx, `SELECT is_default FROM job_dashboards WHERE id=? AND user_id=? AND job_id=?`, dashboardID, userID, jobID).Scan(&wasDefault); errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_dashboards WHERE user_id=? AND job_id=?`, userID, jobID).Scan(&count); err != nil {
		return "", err
	}
	if count <= 1 {
		return "", ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM job_dashboards WHERE id=?`, dashboardID); err != nil {
		return "", err
	}
	var fallback string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM job_dashboards WHERE user_id=? AND job_id=? ORDER BY is_default DESC,sort_order,id LIMIT 1`, userID, jobID).Scan(&fallback); err != nil {
		return "", err
	}
	if wasDefault == 1 {
		if _, err = tx.ExecContext(ctx, `UPDATE job_dashboards SET is_default=1,updated_at=? WHERE id=?`, formatTime(time.Now().UTC()), fallback); err != nil {
			return "", err
		}
	}
	var active string
	scanErr := tx.QueryRowContext(ctx, `SELECT dashboard_id FROM active_job_dashboards WHERE user_id=? AND job_id=?`, userID, jobID).Scan(&active)
	if errors.Is(scanErr, sql.ErrNoRows) || active == dashboardID {
		_, err = tx.ExecContext(ctx, `INSERT INTO active_job_dashboards(user_id,job_id,dashboard_id,updated_at) VALUES(?,?,?,?) ON CONFLICT(user_id,job_id) DO UPDATE SET dashboard_id=excluded.dashboard_id,updated_at=excluded.updated_at`, userID, jobID, fallback, formatTime(time.Now().UTC()))
		if err != nil {
			return "", err
		}
	} else if scanErr != nil {
		return "", scanErr
	}
	return fallback, tx.Commit()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullablePositive(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}
