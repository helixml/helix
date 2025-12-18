package store

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

// CreateProjectAuditLog creates a new audit log entry
// This is an append-only operation - entries are never updated
func (s *PostgresStore) CreateProjectAuditLog(ctx context.Context, log *types.ProjectAuditLog) error {
	err := s.gdb.WithContext(ctx).Create(log).Error
	if err != nil {
		return fmt.Errorf("failed to create project audit log: %w", err)
	}
	return nil
}

// ListProjectAuditLogs retrieves audit logs with filtering and pagination
func (s *PostgresStore) ListProjectAuditLogs(ctx context.Context, filters *types.ProjectAuditLogFilters) (*types.ProjectAuditLogResponse, error) {
	var logs []*types.ProjectAuditLog
	var total int64

	db := s.gdb.WithContext(ctx).Model(&types.ProjectAuditLog{})

	// Apply filters
	if filters != nil {
		if filters.ProjectID != "" {
			db = db.Where("project_id = ?", filters.ProjectID)
		}
		if filters.EventType != "" {
			db = db.Where("event_type = ?", filters.EventType)
		}
		if filters.UserID != "" {
			db = db.Where("user_id = ?", filters.UserID)
		}
		if filters.SpecTaskID != "" {
			db = db.Where("spec_task_id = ?", filters.SpecTaskID)
		}
		if filters.StartDate != nil {
			db = db.Where("created_at >= ?", *filters.StartDate)
		}
		if filters.EndDate != nil {
			db = db.Where("created_at <= ?", *filters.EndDate)
		}
		if filters.Search != "" {
			db = db.Where("prompt_text ILIKE ?", "%"+filters.Search+"%")
		}
	}

	// Get total count before pagination
	if err := db.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count project audit logs: %w", err)
	}

	// Apply pagination
	limit := 50
	offset := 0
	if filters != nil {
		if filters.Limit > 0 && filters.Limit <= 100 {
			limit = filters.Limit
		}
		if filters.Offset > 0 {
			offset = filters.Offset
		}
	}

	// Order by created_at descending (most recent first)
	err := db.Order("created_at DESC").Limit(limit).Offset(offset).Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list project audit logs: %w", err)
	}

	return &types.ProjectAuditLogResponse{
		Logs:   logs,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}
