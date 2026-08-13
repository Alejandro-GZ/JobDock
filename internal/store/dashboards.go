package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type DashboardPreference struct {
	UserID        string    `json:"-"`
	JobID         string    `json:"-"`
	SchemaVersion int       `json:"schema_version"`
	ConfigJSON    []byte    `json:"-"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (s *Store) DashboardPreference(ctx context.Context, userID, jobID string) (DashboardPreference, error) {
	var item DashboardPreference
	var updated string
	err := s.db.QueryRowContext(ctx, `SELECT user_id,job_id,schema_version,config_json,updated_at FROM dashboard_preferences WHERE user_id=? AND job_id=?`, userID, jobID).Scan(&item.UserID, &item.JobID, &item.SchemaVersion, &item.ConfigJSON, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrNotFound
	}
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return item, err
}

func (s *Store) PutDashboardPreference(ctx context.Context, item DashboardPreference) error {
	item.UpdatedAt = item.UpdatedAt.UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO dashboard_preferences(user_id,job_id,schema_version,config_json,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(user_id,job_id) DO UPDATE SET schema_version=excluded.schema_version,config_json=excluded.config_json,updated_at=excluded.updated_at`, item.UserID, item.JobID, item.SchemaVersion, string(item.ConfigJSON), formatTime(item.UpdatedAt))
	return mapConstraint(err)
}
