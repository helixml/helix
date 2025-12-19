package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// SyncPromptHistory syncs prompt history entries from the frontend
// Uses a simple union operation - new entries are added, existing ones are skipped
func (s *PostgresStore) SyncPromptHistory(ctx context.Context, userID string, req *types.PromptHistorySyncRequest) (*types.PromptHistorySyncResponse, error) {
	synced := 0
	existing := 0

	for _, entry := range req.Entries {
		// Convert timestamp from milliseconds to time.Time
		createdAt := time.UnixMilli(entry.Timestamp)

		dbEntry := &types.PromptHistoryEntry{
			ID:         entry.ID,
			UserID:     userID,
			ProjectID:  req.ProjectID,
			SpecTaskID: req.SpecTaskID,
			SessionID:  entry.SessionID,
			Content:    entry.Content,
			Status:     entry.Status,
			CreatedAt:  createdAt,
			UpdatedAt:  time.Now(),
		}

		// Use ON CONFLICT DO NOTHING - if entry exists, skip it
		result := s.gdb.WithContext(ctx).
			Where("id = ?", entry.ID).
			FirstOrCreate(dbEntry)

		if result.Error != nil {
			return nil, result.Error
		}

		if result.RowsAffected > 0 {
			synced++
		} else {
			existing++
		}
	}

	// Return all entries for this user+spec_task so client can merge
	var allEntries []types.PromptHistoryEntry
	err := s.gdb.WithContext(ctx).
		Where("user_id = ? AND spec_task_id = ?", userID, req.SpecTaskID).
		Order("created_at DESC").
		Limit(100). // Reasonable limit
		Find(&allEntries).Error

	if err != nil {
		return nil, err
	}

	return &types.PromptHistorySyncResponse{
		Synced:   synced,
		Existing: existing,
		Entries:  allEntries,
	}, nil
}

// ListPromptHistory returns prompt history entries for a user
func (s *PostgresStore) ListPromptHistory(ctx context.Context, userID string, req *types.PromptHistoryListRequest) (*types.PromptHistoryListResponse, error) {
	query := s.gdb.WithContext(ctx).
		Where("user_id = ?", userID)

	// Filter by spec task (required - history is per-spec-task)
	if req.SpecTaskID != "" {
		query = query.Where("spec_task_id = ?", req.SpecTaskID)
	}

	// Filter by project if specified (optional additional filter)
	if req.ProjectID != "" {
		query = query.Where("project_id = ?", req.ProjectID)
	}

	// Filter by session if specified
	if req.SessionID != "" {
		query = query.Where("session_id = ?", req.SessionID)
	}

	// Filter by timestamp if specified (for incremental sync)
	if req.Since > 0 {
		sinceTime := time.UnixMilli(req.Since)
		query = query.Where("created_at > ?", sinceTime)
	}

	// Get total count
	var total int64
	if err := query.Model(&types.PromptHistoryEntry{}).Count(&total).Error; err != nil {
		return nil, err
	}

	// Apply limit
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	var entries []types.PromptHistoryEntry
	err := query.
		Order("created_at DESC").
		Limit(limit).
		Find(&entries).Error

	if err != nil {
		return nil, err
	}

	return &types.PromptHistoryListResponse{
		Entries: entries,
		Total:   total,
	}, nil
}
