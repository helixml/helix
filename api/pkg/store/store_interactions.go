package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/sanitize"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *PostgresStore) ResetRunningInteractions(ctx context.Context) error {
	err := s.gdb.WithContext(ctx).Model(&types.Interaction{}).
		Where("state = ?", types.InteractionStateWaiting).
		// External-agent runtimes live outside the API process and commonly keep
		// working through an API hot reload. Their Waiting rows are therefore not
		// abandoned. Reconnect/cancel recovery owns their eventual transition.
		Where(`session_id NOT IN (
			SELECT id FROM sessions
			WHERE config->>'agent_type' = ?
			   OR COALESCE(config->>'external_agent_id', '') <> ''
			   OR jsonb_exists(config, 'external_agent_config')
		)`, "zed_external").
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
	err := db.Omit("PendingQuestion", "QuestionHistory").Clauses(clause.OnConflict{
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
	err := db.Omit("PendingQuestion", "QuestionHistory").Clauses(clause.OnConflict{
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
	// PendingQuestion and QuestionHistory are owned by the targeted methods
	// below. A streaming/terminal caller commonly holds an older interaction
	// snapshot, so allowing Save to write those columns would erase a question
	// that arrived concurrently.
	result := db.Omit("PendingQuestion", "QuestionHistory").Clauses(clause.OnConflict{
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

// UpdateInteractionStreamingFields persists only the columns the websocket
// streaming layer owns. State / completed / error are deliberately excluded
// so a streaming flush cannot clobber a concurrent transition written by
// handleTurnCancelled or handleMessageCompleted (the lost-update race that
// turned cancelled turns into spuriously "complete" ones — see
// websocket_external_agent_sync.go).
func (s *PostgresStore) UpdateInteractionStreamingFields(ctx context.Context, interactionID string, generationID int, responseMessage string, responseEntries datatypes.JSON, lastZedMessageOffset int, lastZedMessageID string) error {
	if interactionID == "" {
		return errors.New("id is required")
	}

	responseMessage = sanitize.ForPostgres(responseMessage)
	responseEntries = sanitize.JSONForPostgres(responseEntries)

	return s.gdb.WithContext(ctx).
		Model(&types.Interaction{}).
		Where("id = ? AND generation_id = ?", interactionID, generationID).
		Updates(map[string]interface{}{
			"response_message":        responseMessage,
			"response_entries":        responseEntries,
			"last_zed_message_offset": lastZedMessageOffset,
			"last_zed_message_id":     lastZedMessageID,
			"updated":                 time.Now(),
		}).Error
}

func (s *PostgresStore) SetInteractionPendingQuestion(ctx context.Context, interactionID string, generationID int, question *types.PendingQuestion) (*types.Interaction, bool, error) {
	if interactionID == "" || question == nil || question.RequestID == "" {
		return nil, false, errors.New("interaction_id and question request_id are required")
	}
	var interaction types.Interaction
	changed := false
	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND generation_id = ?", interactionID, generationID).
			First(&interaction).Error; err != nil {
			return err
		}
		if interaction.State != types.InteractionStateWaiting {
			return nil
		}
		if interaction.PendingQuestion != nil {
			return nil
		}
		for _, resolved := range interaction.QuestionHistory {
			if resolved.RequestID == question.RequestID {
				return nil
			}
		}
		questionCopy := *question
		if questionCopy.AskedAt.IsZero() {
			questionCopy.AskedAt = time.Now()
		}
		pendingJSON, err := json.Marshal(&questionCopy)
		if err != nil {
			return fmt.Errorf("marshal pending question: %w", err)
		}
		if err := tx.Model(&types.Interaction{}).
			Where("id = ? AND generation_id = ? AND state = ?", interactionID, generationID, types.InteractionStateWaiting).
			Updates(map[string]interface{}{
				"pending_question": datatypes.JSON(pendingJSON),
				"updated":          time.Now(),
			}).Error; err != nil {
			return err
		}
		interaction.PendingQuestion = &questionCopy
		interaction.Updated = time.Now()
		changed = true
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ErrNotFound
		}
		return nil, false, err
	}
	return &interaction, changed, nil
}

func (s *PostgresStore) ResolveInteractionPendingQuestion(ctx context.Context, interactionID string, generationID int, requestID, outcome string, answers map[string]string) (*types.Interaction, bool, error) {
	if interactionID == "" || requestID == "" || outcome == "" {
		return nil, false, errors.New("interaction_id, request_id, and outcome are required")
	}
	var interaction types.Interaction
	changed := false
	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND generation_id = ?", interactionID, generationID).
			First(&interaction).Error; err != nil {
			return err
		}
		if interaction.PendingQuestion == nil || interaction.PendingQuestion.RequestID != requestID {
			return nil
		}
		resolved := types.ResolvedQuestion{
			PendingQuestion: *interaction.PendingQuestion,
			Outcome:         outcome,
			Answers:         answers,
			ResolvedAt:      time.Now(),
		}
		interaction.QuestionHistory = append(interaction.QuestionHistory, resolved)
		interaction.PendingQuestion = nil
		interaction.Updated = time.Now()
		historyJSON, err := json.Marshal(interaction.QuestionHistory)
		if err != nil {
			return fmt.Errorf("marshal question history: %w", err)
		}
		if err := tx.Model(&types.Interaction{}).
			Where("id = ? AND generation_id = ?", interactionID, generationID).
			Updates(map[string]interface{}{
				"pending_question": nil,
				"question_history": datatypes.JSON(historyJSON),
				"updated":          interaction.Updated,
			}).Error; err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ErrNotFound
		}
		return nil, false, err
	}
	return &interaction, changed, nil
}

func (s *PostgresStore) BindInteractionExternalAgentRequest(ctx context.Context, interactionID string, generationID int, requestID string) (bool, error) {
	if interactionID == "" || requestID == "" {
		return false, errors.New("interaction_id and request_id are required")
	}
	result := s.gdb.WithContext(ctx).
		Model(&types.Interaction{}).
		Where("id = ? AND generation_id = ? AND state = ?", interactionID, generationID, types.InteractionStateWaiting).
		Update("external_agent_request_id", requestID)
	return result.RowsAffected > 0, result.Error
}

func (s *PostgresStore) MarkInteractionExternalAgentDispatched(ctx context.Context, interactionID string, generationID int, requestID string) (bool, error) {
	if interactionID == "" || requestID == "" {
		return false, errors.New("interaction_id and request_id are required")
	}
	result := s.gdb.WithContext(ctx).
		Model(&types.Interaction{}).
		Where("id = ? AND generation_id = ? AND state = ?", interactionID, generationID, types.InteractionStateWaiting).
		Updates(map[string]interface{}{
			"external_agent_request_id":    requestID,
			"external_agent_dispatched_at": time.Now(),
			"updated":                      time.Now(),
		})
	return result.RowsAffected > 0, result.Error
}

func (s *PostgresStore) ClearInteractionExternalAgentDispatched(ctx context.Context, interactionID string, generationID int, requestID string) error {
	if interactionID == "" || requestID == "" {
		return errors.New("interaction_id and request_id are required")
	}
	return s.gdb.WithContext(ctx).
		Model(&types.Interaction{}).
		Where("id = ? AND generation_id = ? AND state = ? AND external_agent_request_id = ?", interactionID, generationID, types.InteractionStateWaiting, requestID).
		Update("external_agent_dispatched_at", nil).Error
}

func (s *PostgresStore) RequestInteractionCancellationIfWaiting(ctx context.Context, interactionID string, generationID int) (bool, error) {
	if interactionID == "" {
		return false, errors.New("interaction_id is required")
	}
	result := s.gdb.WithContext(ctx).
		Model(&types.Interaction{}).
		Where("id = ? AND generation_id = ? AND state = ?", interactionID, generationID, types.InteractionStateWaiting).
		Updates(map[string]interface{}{
			"external_agent_cancel_requested_at": time.Now(),
			"updated":                            time.Now(),
		})
	return result.RowsAffected > 0, result.Error
}

func (s *PostgresStore) MarkInteractionInterruptedIfWaiting(ctx context.Context, interactionID string, generationID int) (bool, error) {
	if interactionID == "" {
		return false, errors.New("interaction_id is required")
	}
	now := time.Now()
	result := s.gdb.WithContext(ctx).
		Model(&types.Interaction{}).
		Where("id = ? AND generation_id = ? AND state = ?", interactionID, generationID, types.InteractionStateWaiting).
		Updates(map[string]interface{}{
			"state":     types.InteractionStateInterrupted,
			"completed": now,
			"updated":   now,
		})
	return result.RowsAffected > 0, result.Error
}

func (s *PostgresStore) GetInteractionByExternalAgentRequestID(ctx context.Context, requestID string) (*types.Interaction, error) {
	if requestID == "" {
		return nil, errors.New("request_id is required")
	}
	var interaction types.Interaction
	err := s.gdb.WithContext(ctx).
		Where("external_agent_request_id = ?", requestID).
		Order("created DESC").
		First(&interaction).Error
	if err != nil {
		return nil, err
	}
	return &interaction, nil
}

// MarkInteractionCompleteIfWaiting atomically transitions an interaction from
// Waiting → Complete. Returns true if the row was transitioned, false if the
// row was already in a terminal state (Interrupted / Complete / Error). The
// WHERE clause ensures we cannot clobber a state set by another handler.
func (s *PostgresStore) MarkInteractionCompleteIfWaiting(ctx context.Context, interactionID string, generationID int) (bool, error) {
	if interactionID == "" {
		return false, errors.New("id is required")
	}
	now := time.Now()
	result := s.gdb.WithContext(ctx).
		Model(&types.Interaction{}).
		Where("id = ? AND generation_id = ? AND state = ?", interactionID, generationID, types.InteractionStateWaiting).
		Updates(map[string]interface{}{
			"state":     types.InteractionStateComplete,
			"completed": now,
			"updated":   now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// MarkInteractionErrorIfWaiting atomically transitions an interaction from
// Waiting to Error without clobbering a concurrent terminal transition.
func (s *PostgresStore) MarkInteractionErrorIfWaiting(ctx context.Context, interactionID string, generationID int, reason string) (bool, error) {
	if interactionID == "" {
		return false, errors.New("id is required")
	}
	now := time.Now()
	result := s.gdb.WithContext(ctx).
		Model(&types.Interaction{}).
		Where("id = ? AND generation_id = ? AND state = ?", interactionID, generationID, types.InteractionStateWaiting).
		Updates(map[string]interface{}{
			"state":     types.InteractionStateError,
			"error":     sanitize.ForPostgres(reason),
			"completed": now,
			"updated":   now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// ReapWaitingInteractions transitions every interaction still in state=waiting
// for sessionID to newState (typically interrupted), stamping completed/updated.
//
// Used when the external agent that could have completed the turn has gone away
// — the desktop was idle-stopped, crashed, or is found stopped when a new prompt
// arrives. A waiting interaction with no live agent will never receive
// message_completed; left alone it deadlocks the prompt-queue busy-check in
// processPendingPromptsForIdleSessions (which treats "latest interaction waiting"
// as "busy, defer") so the desktop is never allowed to resume.
//
// This is deliberately different from the auto-wake worker: this bulk operation
// handles a disconnected session, while auto-wake leaves connected interactions
// waiting and retries cold-start when no WebSocket has connected.
//
// Targeted column UPDATE guarded on state=waiting (not a full-row Save) so it
// cannot clobber a concurrent streaming write. Returns the reaped interactions
// (with the new state reflected in the returned copies) so callers can publish
// frontend updates.
func (s *PostgresStore) ReapWaitingInteractions(ctx context.Context, sessionID string, newState types.InteractionState, reason string) ([]*types.Interaction, error) {
	if sessionID == "" {
		return nil, errors.New("session_id is required")
	}
	now := time.Now()
	var reaped []*types.Interaction
	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var candidates []*types.Interaction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("session_id = ? AND state = ?", sessionID, types.InteractionStateWaiting).
			Order("created ASC, id ASC").
			Find(&candidates).Error; err != nil {
			return err
		}
		if len(candidates) == 0 {
			return nil
		}
		for _, interaction := range candidates {
			settledHistory := interaction.QuestionHistory
			updates := map[string]interface{}{
				"state":     newState,
				"completed": now,
				"updated":   now,
			}
			if interaction.PendingQuestion != nil {
				settledHistory = append(settledHistory, types.ResolvedQuestion{
					PendingQuestion: *interaction.PendingQuestion,
					Outcome:         "cancelled",
					ResolvedAt:      now,
				})
				historyJSON, err := json.Marshal(settledHistory)
				if err != nil {
					return fmt.Errorf("marshal reaped question history: %w", err)
				}
				updates["pending_question"] = nil
				updates["question_history"] = datatypes.JSON(historyJSON)
			}

			result := tx.Model(&types.Interaction{}).
				Where(
					"id = ? AND generation_id = ? AND state = ?",
					interaction.ID,
					interaction.GenerationID,
					types.InteractionStateWaiting,
				).
				Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				continue
			}
			interaction.State = newState
			interaction.Completed = now
			interaction.Updated = now
			interaction.PendingQuestion = nil
			interaction.QuestionHistory = settledHistory
			reaped = append(reaped, interaction)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(reaped) > 0 {
		log.Info().
			Str("session_id", sessionID).
			Int("count", len(reaped)).
			Str("new_state", string(newState)).
			Str("reason", reason).
			Msg("reaped waiting interactions (agent gone)")
	}
	return reaped, nil
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

// ClearSessionInteractions hard-deletes all interactions for a session in a
// single statement. The session row is untouched. Deleting zero rows is not an
// error, so this is idempotent on an already-empty session.
func (s *PostgresStore) ClearSessionInteractions(ctx context.Context, sessionID string) error {
	return s.gdb.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&types.Interaction{}).Error
}

// GetLatestInteractionsForSessions returns the newest interaction for each
// supplied session ID, keyed by session_id. Sessions with no interactions are
// absent from the returned map. Used by the spec-task list handler to derive
// agent_work_state from the in-flight interaction state without N round-trips.
func (s *PostgresStore) GetLatestInteractionsForSessions(ctx context.Context, sessionIDs []string) (map[string]*types.Interaction, error) {
	result := make(map[string]*types.Interaction, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return result, nil
	}

	var interactions []*types.Interaction
	err := s.gdb.WithContext(ctx).
		Raw(`SELECT DISTINCT ON (session_id) *
		     FROM interactions
		     WHERE session_id IN ?
		     ORDER BY session_id, updated DESC, generation_id DESC`, sessionIDs).
		Scan(&interactions).Error
	if err != nil {
		return nil, err
	}
	for _, interaction := range interactions {
		result[interaction.SessionID] = interaction
	}
	return result, nil
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
