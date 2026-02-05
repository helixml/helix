package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
)

// CreateUserSession creates a new user session
func (s *PostgresStore) CreateUserSession(ctx context.Context, session *types.UserSession) (*types.UserSession, error) {
	if session.ID == "" {
		session.ID = uuid.New().String()
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now()
	}
	if session.LastUsedAt.IsZero() {
		session.LastUsedAt = time.Now()
	}

	err := s.gdb.WithContext(ctx).Create(session).Error
	if err != nil {
		return nil, err
	}

	return session, nil
}

// GetUserSession retrieves a user session by ID
func (s *PostgresStore) GetUserSession(ctx context.Context, id string) (*types.UserSession, error) {
	var session types.UserSession
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetUserSessionsByUser retrieves all sessions for a user
func (s *PostgresStore) GetUserSessionsByUser(ctx context.Context, userID string) ([]*types.UserSession, error) {
	var sessions []*types.UserSession
	err := s.gdb.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// UpdateUserSession updates an existing user session
func (s *PostgresStore) UpdateUserSession(ctx context.Context, session *types.UserSession) (*types.UserSession, error) {
	session.UpdatedAt = time.Now()
	err := s.gdb.WithContext(ctx).Save(session).Error
	if err != nil {
		return nil, err
	}
	return session, nil
}

// DeleteUserSession deletes a user session by ID
func (s *PostgresStore) DeleteUserSession(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).Where("id = ?", id).Delete(&types.UserSession{}).Error
}

// DeleteUserSessionsByUser deletes all sessions for a user (e.g., on logout from all devices)
func (s *PostgresStore) DeleteUserSessionsByUser(ctx context.Context, userID string) error {
	return s.gdb.WithContext(ctx).Where("user_id = ?", userID).Delete(&types.UserSession{}).Error
}

// GetUserSessionsNearOIDCExpiry retrieves OIDC sessions that need token refresh
// This is used by the background refresh job
func (s *PostgresStore) GetUserSessionsNearOIDCExpiry(ctx context.Context, expiresBefore time.Time) ([]*types.UserSession, error) {
	var sessions []*types.UserSession
	err := s.gdb.WithContext(ctx).
		Where("auth_provider = ?", types.AuthProviderOIDC).
		Where("oidc_refresh_token != ''").
		Where("oidc_token_expiry < ?", expiresBefore).
		Where("expires_at > ?", time.Now()). // Only non-expired sessions
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// DeleteExpiredUserSessions deletes all expired sessions
// This should be run periodically to clean up the database
func (s *PostgresStore) DeleteExpiredUserSessions(ctx context.Context) error {
	return s.gdb.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&types.UserSession{}).Error
}
