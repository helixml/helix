package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/sanitize"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *PostgresStore) ResetRunningInteractions(ctx context.Context) error {
	err := s.gdb.WithContext(ctx).Model(&types.Interaction{}).
		Where("state = ?", types.InteractionStateWaiting).
		Updates(map[string]any{
			"state": types.InteractionStateError,
			"error": "Interrupted",
		}).
		Error
	if err != nil {
		return err
	}
	return nil
}

// GetInteractionsSummary returns lightweight metadata (count + max updated) for
// a session's interactions. Used to compute ETags without loading full rows.
func (s *PostgresStore) GetInteractionsSummary(ctx context.Context, sessionID string, generationID int) (int64, time.Time, error) {
	var result struct {
		Count      int64
		MaxUpdated *time.Time
	}

	q := s.gdb.WithContext(ctx).
		Model(&types.Interaction{}).
		Select("COUNT(*) as count, MAX(updated) as max_updated").
		Where("session_id = ?", sessionID)

	if generationID > 0 {
		q = q.Where("generation_id = ?", generationID)
	}

	if err := q.Scan(&result).Error; err != nil {
		return 0, time.Time{}, err
	}

	maxUpdated := time.Time{}
	if result.MaxUpdated != nil {
		maxUpdated = *result.MaxUpdated
	}

	return result.Count, maxUpdated, nil
}

func (s *PostgresStore) CreateInteraction(ctx context.Context, interaction *types.Interaction) (*types.Interaction, error) {
	if interaction.SessionID == "" {
		return nil, errors.New("session_id is required")
	}

	if interaction.UserID == "" {
		return nil, errors.New("user_id is required")
	}

	if interaction.ID == "" {
		interaction.ID = system.GenerateInteractionID()
	}

	db := s.gdb.WithContext(ctx)

	// Allows overwriting the interaction with the same primary key (ID and generation ID)
	err := db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(&interaction).Error
	if err != nil {
		return nil, err
	}

	return interaction, nil
}

func (s *PostgresStore) CreateInteractions(ctx context.Context, interactions ...*types.Interaction) error {
	if len(interactions) == 0 {
		return nil
	}

	for idx, interaction := range interactions {
		if interaction.SessionID == "" {
			return errors.New("session_id is required")
		}

		if interaction.UserID == "" {
			return errors.New("user_id is required")
		}

		if interaction.ID == "" {
			interactions[idx].ID = system.GenerateInteractionID()
		}

		if idx == 0 {
			interactions[idx].Created = time.Now()
		}
	}

	db := s.gdb.WithContext(ctx)

	// Allows overwriting the interaction with the same primary key (ID and generation ID)
	err := db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(&interactions).Error
	if err != nil {
		return err
	}

	return nil
}

// ListStuckWaitingInteractions returns interactions that look like they
// belong to a turn the agent silently dropped — `state=waiting` with no
// streamed response_message and no response_entries, and old enough that
// the agent ought to have produced something by now. The auto-wake
// worker calls this every ~10 s.
func (s *PostgresStore) ListStuckWaitingInteractions(ctx context.Context, olderThan time.Time, limit int) ([]*types.Interaction, error) {
	if limit <= 0 {
		limit = 50
	}
	var interactions []*types.Interaction
	err := s.gdb.WithContext(ctx).
		Model(&types.Interaction{}).
		Where("state = ?", types.InteractionStateWaiting).
		Where("response_message = ''").
		Where("response_entries IS NULL").
		Where("created < ?", olderThan).
		Order("created ASC").
		Limit(limit).
		Find(&interactions).Error
	if err != nil {
		return nil, err
	}
	return interactions, nil
}

// CountAutoWakeAttemptsSince counts auto-wake interactions in `sessionID`
// created strictly after `since`. DEPRECATED: see store.go.
func (s *PostgresStore) CountAutoWakeAttemptsSince(ctx context.Context, sessionID string, since time.Time) (int64, error) {
	var count int64
	err := s.gdb.WithContext(ctx).
		Model(&types.Interaction{}).
		Where("session_id = ?", sessionID).
		Where("auto_wake_count > 0").
		Where("created > ?", since).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

// IncrementInteractionAutoWakeCount atomically increments auto_wake_count
// on the named interaction and returns the new value. Targeted column
// UPDATE — does not race with the streaming path's full-row Save that
// would otherwise zero the field back from the streaming context's
// in-memory copy.
func (s *PostgresStore) IncrementInteractionAutoWakeCount(ctx context.Context, interactionID string) (int, error) {
	if interactionID == "" {
		return 0, errors.New("interaction_id is required")
	}
	// Use SQL `auto_wake_count + 1` so concurrent increments on the same
	// row also serialize correctly at the DB level.
	if err := s.gdb.WithContext(ctx).
		Model(&types.Interaction{}).
		Where("id = ?", interactionID).
		UpdateColumn("auto_wake_count", gorm.Expr("auto_wake_count + 1")).
		Error; err != nil {
		return 0, err
	}
	// Read back the new value. Two queries; the increment itself is
	// atomic, the read-after is best-effort for the caller's logging.
	var updated types.Interaction
	if err := s.gdb.WithContext(ctx).
		Select("auto_wake_count").
		Where("id = ?", interactionID).
		First(&updated).Error; err != nil {
		return 0, err
	}
	return updated.AutoWakeCount, nil
}

func (s *PostgresStore) GetInteraction(ctx context.Context, interactionID string) (*types.Interaction, error) {
	db := s.gdb.WithContext(ctx)

	if interactionID == "" {
		return nil, errors.New("interaction_id is required")
	}

	var interaction types.Interaction
	err := db.Where("id = ?", interactionID).First(&interaction).Error
	if err != nil {
		return nil, err
	}

	return &interaction, nil
}

func (s *PostgresStore) UpdateInteraction(ctx context.Context, interaction *types.Interaction) (*types.Interaction, error) {
	if interaction.ID == "" {
		return nil, errors.New("id is required")
	}

	// Sanitize string fields that may contain LLM or agent output with characters
	// that PostgreSQL rejects in text/jsonb columns (null bytes, surrogates, etc.)
	interaction.PromptMessage = sanitize.ForPostgres(interaction.PromptMessage)
	interaction.ResponseMessage = sanitize.ForPostgres(interaction.ResponseMessage)
	interaction.Error = sanitize.ForPostgres(interaction.Error)
	interaction.ResponseEntries = sanitize.JSONForPostgres(interaction.ResponseEntries)

	db := s.gdb.WithContext(ctx)

	// CRITICAL: Use Save() which works with composite PK when struct has both fields populated
	// The original OnConflict clause ensures upsert behavior
	result := db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Save(&interaction)

	if result.Error != nil {
		log.Error().
			Err(result.Error).
			Str("interaction_id", interaction.ID).
			Int("generation_id", interaction.GenerationID).
			Msg("❌ [STORE] UpdateInteraction failed")
		return nil, result.Error
	}

	return interaction, nil
}

// UpdateInteractionSummary updates just the summary field of an interaction
func (s *PostgresStore) UpdateInteractionSummary(ctx context.Context, interactionID string, summary string) error {
	now := time.Now()
	return s.gdb.WithContext(ctx).
		Model(&types.Interaction{}).
		Where("id = ?", interactionID).
		Updates(map[string]interface{}{
			"summary":            summary,
			"summary_updated_at": now,
		}).Error
}

func (s *PostgresStore) DeleteInteraction(ctx context.Context, interactionID string) error {
	db := s.gdb.WithContext(ctx)

	err := db.Delete(&types.Interaction{}, "id = ?", interactionID).Error
	if err != nil {
		return err
	}

	return nil
}

func (s *PostgresStore) ListInteractions(ctx context.Context, query *types.ListInteractionsQuery) ([]*types.Interaction, int64, error) {
	db := s.gdb.WithContext(ctx)

	q := db.Model(&types.Interaction{})

	if query.PerPage == 0 {
		query.PerPage = -1
	}

	offset := query.Page * query.PerPage

	if query.SessionID != "" {
		q = q.Where("session_id = ?", query.SessionID)
	}

	if query.AppID != "" {
		q = q.Where("app_id = ?", query.AppID)
	}

	if query.InteractionID != "" {
		q = q.Where("id = ?", query.InteractionID)
	}

	if query.UserID != "" {
		q = q.Where("user_id = ?", query.UserID)
	}

	if query.GenerationID > 0 {
		q = q.Where("generation_id = ?", query.GenerationID)
	}

	if query.Order == "" {
		query.Order = "id ASC"
	}

	if query.Feedback != "" {
		q = q.Where("feedback = ?", query.Feedback)
	}

	totalCount := int64(0)

	err := q.Count(&totalCount).Error
	if err != nil {
		return nil, 0, err
	}

	var interactions []*types.Interaction
	// Oldest to newest
	err = q.Order(query.Order).Offset(offset).Limit(query.PerPage).Find(&interactions).Error
	if err != nil {
		return nil, 0, err
	}

	return interactions, totalCount, nil
}
