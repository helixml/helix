package store

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

func (s *PostgresStore) CreateOrgAuditLog(ctx context.Context, entry *types.OrgAuditLog) error {
	if err := s.gdb.WithContext(ctx).Create(entry).Error; err != nil {
		return fmt.Errorf("failed to create org audit log: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListOrgAuditLogs(ctx context.Context, filters *types.OrgAuditLogFilters) (*types.OrgAuditLogResponse, error) {
	var logs []*types.OrgAuditLog
	var total int64

	db := s.gdb.WithContext(ctx).Model(&types.OrgAuditLog{})
	if filters != nil {
		if filters.OrganizationID != "" {
			db = db.Where("organization_id = ?", filters.OrganizationID)
		}
		if filters.ProjectID != "" {
			db = db.Where("project_id = ?", filters.ProjectID)
		}
		if filters.UserID != "" {
			db = db.Where("user_id = ?", filters.UserID)
		}
		if filters.ActorID != "" {
			db = db.Where("actor_id = ?", filters.ActorID)
		}
		if filters.AssetID != "" {
			db = db.Where("asset_id = ?", filters.AssetID)
		}
		if filters.EventType != "" {
			db = db.Where("event_type = ?", filters.EventType)
		}
		if filters.Action != "" {
			db = db.Where("action = ?", filters.Action)
		}
		if filters.Status != "" {
			db = db.Where("status = ?", filters.Status)
		}
		if filters.StartDate != nil {
			db = db.Where("created_at >= ?", *filters.StartDate)
		}
		if filters.EndDate != nil {
			db = db.Where("created_at <= ?", *filters.EndDate)
		}
		if filters.Search != "" {
			query := "%" + filters.Search + "%"
			db = db.Where("action ILIKE ? OR CAST(metadata AS TEXT) ILIKE ?", query, query)
		}
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count org audit logs: %w", err)
	}

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
	if err := db.Order("created_at DESC").Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("failed to list org audit logs: %w", err)
	}

	return &types.OrgAuditLogResponse{Logs: logs, Total: total, Limit: limit, Offset: offset}, nil
}
