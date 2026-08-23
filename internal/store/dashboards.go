package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type DashboardPreference struct {
	UserID                string     `json:"-"`
	JobID                 string     `json:"-"`
	SchemaVersion         int        `json:"schema_version"`
	ConfigJSON            []byte     `json:"-"`
	TemplateID            string     `json:"-"`
	TemplateVersion       int        `json:"-"`
	TemplateSchemaVersion int        `json:"-"`
	TemplateAppliedAt     *time.Time `json:"-"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func (s *Store) DashboardPreference(ctx context.Context, userID, jobID string) (DashboardPreference, error) {
	var item DashboardPreference
	var updated string
	var applied sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT user_id,job_id,schema_version,config_json,COALESCE(template_id,''),COALESCE(template_version,0),COALESCE(template_schema_version,0),template_applied_at,updated_at FROM dashboard_preferences WHERE user_id=? AND job_id=?`, userID, jobID).Scan(&item.UserID, &item.JobID, &item.SchemaVersion, &item.ConfigJSON, &item.TemplateID, &item.TemplateVersion, &item.TemplateSchemaVersion, &applied, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrNotFound
	}
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
	item.UpdatedAt = item.UpdatedAt.UTC()
	var applied any
	if item.TemplateAppliedAt != nil {
		applied = formatTime(item.TemplateAppliedAt.UTC())
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO dashboard_preferences(user_id,job_id,schema_version,config_json,template_id,template_version,template_schema_version,template_applied_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(user_id,job_id) DO UPDATE SET schema_version=excluded.schema_version,config_json=excluded.config_json,template_id=excluded.template_id,template_version=excluded.template_version,template_schema_version=excluded.template_schema_version,template_applied_at=excluded.template_applied_at,updated_at=excluded.updated_at`, item.UserID, item.JobID, item.SchemaVersion, string(item.ConfigJSON), nullable(item.TemplateID), nullablePositive(item.TemplateVersion), nullablePositive(item.TemplateSchemaVersion), applied, formatTime(item.UpdatedAt))
	return mapConstraint(err)
}

func nullablePositive(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}
