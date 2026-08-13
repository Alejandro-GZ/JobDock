package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

type PersonalAccessToken struct {
	ID         string      `json:"id"`
	UserID     string      `json:"user_id"`
	Name       string      `json:"name"`
	Prefix     string      `json:"prefix"`
	Scopes     []string    `json:"scopes"`
	ExpiresAt  *time.Time  `json:"expires_at,omitempty"`
	LastUsedAt *time.Time  `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time  `json:"revoked_at,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
	User       domain.User `json:"-"`
}

func (s *Store) CreatePersonalAccessToken(ctx context.Context, token PersonalAccessToken, tokenHash string) error {
	scopes, _ := json.Marshal(token.Scopes)
	var expires any
	if token.ExpiresAt != nil {
		expires = formatTime(*token.ExpiresAt)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO personal_access_tokens(id,user_id,name,token_hash,token_prefix,scopes_json,expires_at,created_at) VALUES(?,?,?,?,?,?,?,?)`, token.ID, token.UserID, token.Name, tokenHash, token.Prefix, scopes, expires, formatTime(token.CreatedAt))
	return mapConstraint(err)
}

func (s *Store) ListPersonalAccessTokens(ctx context.Context, userID string) ([]PersonalAccessToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,user_id,name,token_prefix,scopes_json,expires_at,last_used_at,revoked_at,created_at FROM personal_access_tokens WHERE user_id=? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []PersonalAccessToken{}
	for rows.Next() {
		item, err := scanPersonalAccessToken(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) PersonalAccessTokenByHash(ctx context.Context, tokenHash string) (PersonalAccessToken, error) {
	var item PersonalAccessToken
	var scopes string
	var expires, lastUsed, revoked sql.NullString
	var userCreated, created string
	err := s.db.QueryRowContext(ctx, `SELECT p.id,p.user_id,p.name,p.token_prefix,p.scopes_json,p.expires_at,p.last_used_at,p.revoked_at,p.created_at,u.id,u.username,u.role,u.created_at FROM personal_access_tokens p JOIN users u ON u.id=p.user_id WHERE p.token_hash=?`, tokenHash).Scan(&item.ID, &item.UserID, &item.Name, &item.Prefix, &scopes, &expires, &lastUsed, &revoked, &created, &item.User.ID, &item.User.Username, &item.User.Role, &userCreated)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrNotFound
	}
	if err != nil {
		return item, err
	}
	parsePATTimes(&item, expires, lastUsed, revoked, created)
	item.User.CreatedAt, _ = time.Parse(time.RFC3339Nano, userCreated)
	_ = json.Unmarshal([]byte(scopes), &item.Scopes)
	return item, nil
}

func (s *Store) TouchPersonalAccessToken(ctx context.Context, id string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE personal_access_tokens SET last_used_at=? WHERE id=? AND (last_used_at IS NULL OR last_used_at<?)`, formatTime(now), id, formatTime(now.Add(-time.Minute)))
	return err
}

func (s *Store) RevokePersonalAccessToken(ctx context.Context, userID, id string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE personal_access_tokens SET revoked_at=COALESCE(revoked_at,?) WHERE id=? AND user_id=?`, formatTime(now), id, userID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

type patScanner interface{ Scan(...any) error }

func scanPersonalAccessToken(row patScanner) (PersonalAccessToken, error) {
	var item PersonalAccessToken
	var scopes, created string
	var expires, lastUsed, revoked sql.NullString
	err := row.Scan(&item.ID, &item.UserID, &item.Name, &item.Prefix, &scopes, &expires, &lastUsed, &revoked, &created)
	if err != nil {
		return item, err
	}
	parsePATTimes(&item, expires, lastUsed, revoked, created)
	_ = json.Unmarshal([]byte(scopes), &item.Scopes)
	return item, nil
}

func parsePATTimes(item *PersonalAccessToken, expires, lastUsed, revoked sql.NullString, created string) {
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if expires.Valid {
		value, _ := time.Parse(time.RFC3339Nano, expires.String)
		item.ExpiresAt = &value
	}
	if lastUsed.Valid {
		value, _ := time.Parse(time.RFC3339Nano, lastUsed.String)
		item.LastUsedAt = &value
	}
	if revoked.Valid {
		value, _ := time.Parse(time.RFC3339Nano, revoked.String)
		item.RevokedAt = &value
	}
}
