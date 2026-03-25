package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CreateAttentionEvent creates a new attention event. If an event with the same
// idempotency key already exists, the existing row is returned without error.
func (s *PostgresStore) CreateAttentionEvent(ctx context.Context, event *types.AttentionEvent) (*types.AttentionEvent, error) {
	if event.ID == "" {
		return nil, fmt.Errorf("event ID is required")
	}
	if event.UserID == "" {
		return nil, fmt.Errorf("user ID is required")
	}
	if event.ProjectID == "" {
		return nil, fmt.Errorf("project ID is required")
	}
	if event.SpecTaskID == "" {
		return nil, fmt.Errorf("spec task ID is required")
	}
	if event.EventType == "" {
		return nil, fmt.Errorf("event type is required")
	}

	result := s.gdb.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "idempotency_key"}},
			DoNothing: true,
		}).
		Create(event)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to create attention event: %w", result.Error)
	}

	// If DoNothing fired (duplicate key), RowsAffected == 0.
	// Look up the existing row so the caller gets the original event back.
	if result.RowsAffected == 0 && event.IdempotencyKey != "" {
		var existing types.AttentionEvent
		if err := s.gdb.WithContext(ctx).
			Where("idempotency_key = ?", event.IdempotencyKey).
			First(&existing).Error; err != nil {
			return nil, fmt.Errorf("failed to fetch existing attention event: %w", err)
		}
		return &existing, nil
	}

	log.Info().
		Str("event_id", event.ID).
		Str("event_type", string(event.EventType)).
		Str("spec_task_id", event.SpecTaskID).
		Msg("Created attention event")

	return event, nil
}

// ListAttentionEvents returns active (not dismissed, not snoozed) events for a user.
func (s *PostgresStore) ListAttentionEvents(ctx context.Context, userID, organizationID string, filters types.AttentionEventFilters) ([]*types.AttentionEvent, error) {
	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	var events []*types.AttentionEvent
	query := s.gdb.WithContext(ctx).
		Where("attention_events.user_id = ?", userID).
		Where("attention_events.dismissed_at IS NULL").
		Where("attention_events.snoozed_until IS NULL OR attention_events.snoozed_until < ?", time.Now())

	if organizationID != "" {
		query = query.Where("attention_events.organization_id = ?", organizationID)
	}

	if filters.MineOnly {
		// Join spec_tasks to filter by task ownership.
		// Assignee takes priority; fall back to created_by when no assignee is set.
		query = query.
			Joins("JOIN spec_tasks ON spec_tasks.id = attention_events.spec_task_id").
			Where("spec_tasks.assignee_id = ? OR (spec_tasks.assignee_id IS NULL OR spec_tasks.assignee_id = '') AND spec_tasks.created_by = ?", userID, userID)
	}

	result := query.Order("attention_events.created_at DESC").Find(&events)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list attention events: %w", result.Error)
	}

	return events, nil
}

// GetAttentionEvent returns a single attention event by ID.
func (s *PostgresStore) GetAttentionEvent(ctx context.Context, id string) (*types.AttentionEvent, error) {
	var event types.AttentionEvent
	result := s.gdb.WithContext(ctx).Where("id = ?", id).First(&event)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("attention event not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get attention event: %w", result.Error)
	}
	return &event, nil
}

// UpdateAttentionEvent applies an update to an attention event (acknowledge, dismiss, snooze).
func (s *PostgresStore) UpdateAttentionEvent(ctx context.Context, id string, update *types.AttentionEventUpdateRequest) error {
	updates := map[string]interface{}{}

	if update.Acknowledge {
		now := time.Now()
		updates["acknowledged_at"] = &now
	}
	if update.Dismiss {
		now := time.Now()
		updates["dismissed_at"] = &now
	}
	if update.SnoozedUntil != nil {
		updates["snoozed_until"] = update.SnoozedUntil
	}

	if len(updates) == 0 {
		return nil
	}

	result := s.gdb.WithContext(ctx).
		Model(&types.AttentionEvent{}).
		Where("id = ?", id).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update attention event: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("attention event not found: %s", id)
	}

	return nil
}

// BulkDismissAttentionEvents dismisses all active events for a user.
func (s *PostgresStore) BulkDismissAttentionEvents(ctx context.Context, userID, organizationID string) (int64, error) {
	if userID == "" {
		return 0, fmt.Errorf("user ID is required")
	}

	now := time.Now()
	query := s.gdb.WithContext(ctx).
		Model(&types.AttentionEvent{}).
		Where("user_id = ?", userID).
		Where("dismissed_at IS NULL")

	if organizationID != "" {
		query = query.Where("organization_id = ?", organizationID)
	}

	result := query.Update("dismissed_at", &now)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to bulk dismiss attention events: %w", result.Error)
	}

	return result.RowsAffected, nil
}

// CleanupExpiredAttentionEvents deletes dismissed events older than the given duration.
func (s *PostgresStore) CleanupExpiredAttentionEvents(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)

	result := s.gdb.WithContext(ctx).
		Where("dismissed_at IS NOT NULL AND dismissed_at < ?", cutoff).
		Delete(&types.AttentionEvent{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to cleanup expired attention events: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		log.Info().
			Int64("deleted", result.RowsAffected).
			Time("cutoff", cutoff).
			Msg("Cleaned up expired attention events")
	}

	return result.RowsAffected, nil
}
