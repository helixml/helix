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
	// ProjectID / SpecTaskID are required for spec-task events but empty for
	// org_message events (a bot messaging a person has no task). The columns
	// are NOT NULL but empty string is a valid non-null value.
	if event.EventType != types.AttentionEventOrgMessage {
		if event.ProjectID == "" {
			return nil, fmt.Errorf("project ID is required")
		}
		if event.SpecTaskID == "" {
			return nil, fmt.Errorf("spec task ID is required")
		}
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

	// Deduplicate server-side: keep only the most recent event per spec_task_id.
	// The frontend groups by task and only shows the latest anyway, so returning
	// multiple events per task is wasted bandwidth (235 events → ~30 unique tasks).
	var events []*types.AttentionEvent

	orgFilter := ""
	args := []interface{}{userID, time.Now()}
	if organizationID != "" {
		orgFilter = "AND organization_id = ?"
		args = append(args, organizationID)
	}

	mineFilter := ""
	if filters.MineOnly {
		// Task events: mine when I'm the assignee (priority) or the creator.
		// Org messages have no spec task but are addressed straight at this user
		// (the outer user_id = ? already scopes them), so an ask_human sent to me
		// is always "mine" — include every event that has no spec task.
		mineFilter = "AND (spec_task_id = '' OR spec_task_id IN (" +
			"SELECT id FROM spec_tasks " +
			"WHERE assignee_id = ? " +
			"OR ((assignee_id IS NULL OR assignee_id = '') AND created_by = ?)))"
		args = append(args, userID, userID)
	}

	// Dedup key: spec_task_id for task events, but fall back to the event id for
	// org_messages (which have an empty spec_task_id) so every org_message is
	// kept — otherwise DISTINCT ON collapses them all into the single latest one.
	result := s.gdb.WithContext(ctx).Raw(`
		SELECT * FROM (
			SELECT DISTINCT ON (COALESCE(NULLIF(spec_task_id, ''), id)) *
			FROM attention_events
			WHERE user_id = ?
			  AND dismissed_at IS NULL
			  AND (snoozed_until IS NULL OR snoozed_until < ?)
			  `+orgFilter+`
			  `+mineFilter+`
			ORDER BY COALESCE(NULLIF(spec_task_id, ''), id), created_at DESC
		) AS deduped
		ORDER BY created_at DESC
	`, args...).Scan(&events)
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
	if update.Reply {
		now := time.Now()
		updates["replied_at"] = &now
		updates["acknowledged_at"] = &now
	}
	if update.SnoozedUntil != nil {
		updates["snoozed_until"] = update.SnoozedUntil
	}

	// Dismissal is handled separately: dismiss all events for the same task so
	// that deduplicated older events don't resurface after the cache invalidates.
	if update.Dismiss {
		var event types.AttentionEvent
		if err := s.gdb.WithContext(ctx).
			Select("spec_task_id", "user_id").
			Where("id = ?", id).
			First(&event).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("attention event not found: %s", id)
			}
			return fmt.Errorf("failed to fetch attention event for dismiss: %w", err)
		}
		now := time.Now()
		q := s.gdb.WithContext(ctx).Model(&types.AttentionEvent{})
		if event.SpecTaskID != "" {
			// Task events: dismiss the whole task's group so deduplicated older
			// events don't resurface after the cache invalidates.
			q = q.Where("spec_task_id = ? AND user_id = ?", event.SpecTaskID, event.UserID)
		} else {
			// org_messages have no spec task — dismiss only this one, not every
			// org_message for the user.
			q = q.Where("id = ?", id)
		}
		result := q.Update("dismissed_at", &now)
		if result.Error != nil {
			return fmt.Errorf("failed to dismiss attention events: %w", result.Error)
		}
		if len(updates) == 0 {
			return nil
		}
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

// DismissAttentionEventsForTask marks every active (not yet dismissed) attention
// event for the given spec task as dismissed. Used to auto-clear notifications
// when a task transitions to a terminal state. Idempotent.
func (s *PostgresStore) DismissAttentionEventsForTask(ctx context.Context, specTaskID string) (int64, error) {
	if specTaskID == "" {
		return 0, fmt.Errorf("spec task ID is required")
	}

	now := time.Now()
	result := s.gdb.WithContext(ctx).
		Model(&types.AttentionEvent{}).
		Where("spec_task_id = ? AND dismissed_at IS NULL", specTaskID).
		Update("dismissed_at", &now)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to dismiss attention events for task: %w", result.Error)
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
