package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// CreateStreamingAccessGrant creates a new streaming access grant
func (s *PostgresStore) CreateStreamingAccessGrant(ctx context.Context, grant *types.StreamingAccessGrant) (*types.StreamingAccessGrant, error) {
	now := time.Now()
	grant.Created = now
	grant.Updated = now
	if grant.GrantedAt == nil {
		grant.GrantedAt = &now
	}

	if err := s.gdb.WithContext(ctx).Create(grant).Error; err != nil {
		return nil, err
	}

	return grant, nil
}

// GetStreamingAccessGrant retrieves a grant by ID
func (s *PostgresStore) GetStreamingAccessGrant(ctx context.Context, id string) (*types.StreamingAccessGrant, error) {
	var grant types.StreamingAccessGrant
	if err := s.gdb.WithContext(ctx).
		Where("id = ? AND revoked_at IS NULL", id).
		First(&grant).Error; err != nil {
		return nil, err
	}
	return &grant, nil
}

// GetStreamingAccessGrantByUser finds an active grant for a specific user
func (s *PostgresStore) GetStreamingAccessGrantByUser(ctx context.Context, sessionID, userID string) (*types.StreamingAccessGrant, error) {
	var grant types.StreamingAccessGrant
	query := s.gdb.WithContext(ctx).
		Where("granted_user_id = ? AND revoked_at IS NULL", userID).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now())

	// Check session or PDE
	query = query.Where("(session_id = ? OR pde_id = ?)", sessionID, sessionID)

	if err := query.First(&grant).Error; err != nil {
		return nil, err
	}
	return &grant, nil
}

// GetStreamingAccessGrantByTeam finds an active grant for a team
func (s *PostgresStore) GetStreamingAccessGrantByTeam(ctx context.Context, sessionID, teamID string) (*types.StreamingAccessGrant, error) {
	var grant types.StreamingAccessGrant
	query := s.gdb.WithContext(ctx).
		Where("granted_team_id = ? AND revoked_at IS NULL", teamID).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now())

	query = query.Where("(session_id = ? OR pde_id = ?)", sessionID, sessionID)

	if err := query.First(&grant).Error; err != nil {
		return nil, err
	}
	return &grant, nil
}

// GetStreamingAccessGrantByRole finds an active grant for a role
func (s *PostgresStore) GetStreamingAccessGrantByRole(ctx context.Context, sessionID, role string) (*types.StreamingAccessGrant, error) {
	var grant types.StreamingAccessGrant
	query := s.gdb.WithContext(ctx).
		Where("granted_role = ? AND revoked_at IS NULL", role).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now())

	query = query.Where("(session_id = ? OR pde_id = ?)", sessionID, sessionID)

	if err := query.First(&grant).Error; err != nil {
		return nil, err
	}
	return &grant, nil
}

// ListStreamingAccessGrants lists all grants for a session/PDE
func (s *PostgresStore) ListStreamingAccessGrants(ctx context.Context, sessionID string) ([]*types.StreamingAccessGrant, error) {
	var grants []*types.StreamingAccessGrant
	query := s.gdb.WithContext(ctx).
		Where("revoked_at IS NULL").
		Where("(session_id = ? OR pde_id = ?)", sessionID, sessionID).
		Order("created DESC")

	if err := query.Find(&grants).Error; err != nil {
		return nil, err
	}
	return grants, nil
}

// RevokeStreamingAccessGrant revokes a grant
func (s *PostgresStore) RevokeStreamingAccessGrant(ctx context.Context, grantID, revokedBy string) error {
	now := time.Now()
	return s.gdb.WithContext(ctx).
		Model(&types.StreamingAccessGrant{}).
		Where("id = ? AND revoked_at IS NULL", grantID).
		Updates(map[string]interface{}{
			"revoked_at": &now,
			"revoked_by": revokedBy,
			"updated":    now,
		}).Error
}

// LogStreamingAccess creates an audit log entry
func (s *PostgresStore) LogStreamingAccess(ctx context.Context, log *types.StreamingAccessAuditLog) error {
	log.Created = time.Now()
	log.AccessedAt = time.Now()

	return s.gdb.WithContext(ctx).Create(log).Error
}

// UpdateStreamingAccessDisconnect updates the audit log when user disconnects
func (s *PostgresStore) UpdateStreamingAccessDisconnect(ctx context.Context, logID string) error {
	// Get the log entry
	var auditLog types.StreamingAccessAuditLog
	if err := s.gdb.WithContext(ctx).Where("id = ?", logID).First(&auditLog).Error; err != nil {
		return err
	}

	now := time.Now()
	duration := int(now.Sub(auditLog.AccessedAt).Seconds())

	return s.gdb.WithContext(ctx).
		Model(&types.StreamingAccessAuditLog{}).
		Where("id = ?", logID).
		Updates(map[string]interface{}{
			"disconnected_at":          &now,
			"session_duration_seconds": duration,
		}).Error
}

// ListStreamingAccessAuditLogs lists audit logs with filters
func (s *PostgresStore) ListStreamingAccessAuditLogs(ctx context.Context, userID, sessionID string, limit int) ([]*types.StreamingAccessAuditLog, error) {
	var logs []*types.StreamingAccessAuditLog
	query := s.gdb.WithContext(ctx)

	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	if sessionID != "" {
		query = query.Where("(session_id = ? OR pde_id = ?)", sessionID, sessionID)
	}

	query = query.Order("accessed_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&logs).Error; err != nil {
		return nil, err
	}

	return logs, nil
}
