package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// SyncPromptHistory syncs prompt history entries from the frontend
// For new entries: creates them with all frontend fields
// For existing entries: updates only frontend-owned fields (interrupt, queuePosition)
// Backend-owned fields (status, retryCount, nextRetryAt) are preserved
func (s *PostgresStore) SyncPromptHistory(ctx context.Context, userID string, req *types.PromptHistorySyncRequest) (*types.PromptHistorySyncResponse, error) {
	synced := 0
	updated := 0

	for _, entry := range req.Entries {
		// Convert timestamp from milliseconds to time.Time
		createdAt := time.UnixMilli(entry.Timestamp)

		// Default interrupt to false if not specified (queue mode is the safe default)
		interrupt := false
		if entry.Interrupt != nil {
			interrupt = *entry.Interrupt
		}

		// Default pinned to false if not specified
		pinned := false
		if entry.Pinned != nil {
			pinned = *entry.Pinned
		}

		// Check if entry already exists
		var existingEntry types.PromptHistoryEntry
		result := s.gdb.WithContext(ctx).Where("id = ?", entry.ID).First(&existingEntry)

		if result.Error != nil && result.Error.Error() != "record not found" {
			return nil, result.Error
		}

		if result.RowsAffected == 0 {
			// Entry doesn't exist - create it with all frontend fields
			dbEntry := &types.PromptHistoryEntry{
				ID:            entry.ID,
				UserID:        userID,
				ProjectID:     req.ProjectID,
				SpecTaskID:    req.SpecTaskID,
				SessionID:     entry.SessionID,
				Content:       entry.Content,
				Status:        entry.Status,
				Interrupt:     interrupt,
				QueuePosition: entry.QueuePosition,
				Pinned:        pinned,
				Tags:          entry.Tags,
				CreatedAt:     createdAt,
				UpdatedAt:     time.Now(),
			}

			if err := s.gdb.WithContext(ctx).Create(dbEntry).Error; err != nil {
				return nil, err
			}
			synced++
		} else {
			// Entry exists - only update frontend-owned fields
			// Preserve backend-owned fields: status, retryCount, nextRetryAt
			updateFields := map[string]interface{}{
				"interrupt":      interrupt,
				"queue_position": entry.QueuePosition,
				"content":        entry.Content,
				"updated_at":     time.Now(),
			}

			if err := s.gdb.WithContext(ctx).
				Model(&types.PromptHistoryEntry{}).
				Where("id = ?", entry.ID).
				Updates(updateFields).Error; err != nil {
				return nil, err
			}
			updated++
		}
	}

	// Return all non-deleted entries for this user+spec_task so client can merge
	var allEntries []types.PromptHistoryEntry
	err := s.gdb.WithContext(ctx).
		Where("user_id = ? AND spec_task_id = ? AND deleted_at IS NULL", userID, req.SpecTaskID).
		Order("created_at DESC").
		Limit(100). // Reasonable limit
		Find(&allEntries).Error

	if err != nil {
		return nil, err
	}

	return &types.PromptHistorySyncResponse{
		Synced:   synced,
		Existing: updated, // Number of existing entries that were updated
		Entries:  allEntries,
	}, nil
}

// GetPromptHistoryEntry returns a single prompt history entry by ID
func (s *PostgresStore) GetPromptHistoryEntry(ctx context.Context, id string) (*types.PromptHistoryEntry, error) {
	var entry types.PromptHistoryEntry
	result := s.gdb.WithContext(ctx).Where("id = ?", id).First(&entry)
	if result.Error != nil {
		if result.Error.Error() == "record not found" {
			return nil, nil
		}
		return nil, result.Error
	}
	return &entry, nil
}

// GetNextPendingPrompt returns the next pending or failed non-interrupt prompt for a session
// Used by the queue processor to send non-interrupt messages after the current conversation completes
// Failed prompts are also included for automatic retry (only if next_retry_at has passed)
func (s *PostgresStore) GetNextPendingPrompt(ctx context.Context, sessionID string) (*types.PromptHistoryEntry, error) {
	var entry types.PromptHistoryEntry

	// Atomically claim the next pending prompt by setting status='sending' in a single query.
	// This prevents race conditions where two concurrent callers both read the same prompt.
	now := time.Now()
	result := s.gdb.WithContext(ctx).Raw(`
		UPDATE prompt_history_entries SET status = 'sending', updated_at = NOW()
		WHERE id = (
			SELECT id FROM prompt_history_entries
			WHERE session_id = ? AND interrupt = false AND deleted_at IS NULL
			AND (status = 'pending' OR (status = 'failed' AND (next_retry_at IS NULL OR next_retry_at <= ?)))
			ORDER BY COALESCE(queue_position, 999999) ASC, created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *
	`, sessionID, now).Scan(&entry)

	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil // No pending prompts
	}

	return &entry, nil
}

// GetAnyPendingPrompt returns the next pending or failed prompt for a session (interrupt or not)
// Used when the session is idle to process any pending prompts
// Prioritizes interrupt=true messages (they should be processed first when session is idle)
func (s *PostgresStore) GetAnyPendingPrompt(ctx context.Context, sessionID string) (*types.PromptHistoryEntry, error) {
	var entry types.PromptHistoryEntry

	// Atomically claim the next prompt (same pattern as GetNextPendingPrompt)
	now := time.Now()
	result := s.gdb.WithContext(ctx).Raw(`
		UPDATE prompt_history_entries SET status = 'sending', updated_at = NOW()
		WHERE id = (
			SELECT id FROM prompt_history_entries
			WHERE session_id = ? AND deleted_at IS NULL
			AND (status = 'pending' OR (status = 'failed' AND (next_retry_at IS NULL OR next_retry_at <= ?)))
			ORDER BY interrupt DESC, COALESCE(queue_position, 999999) ASC, created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *
	`, sessionID, now).Scan(&entry)

	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}

	return &entry, nil
}

// GetNextInterruptPrompt returns the next pending or failed interrupt prompt for a session
// Used by sync/list operations - only interrupt prompts should be sent immediately
// Non-interrupt (queue) prompts wait for message_completed
func (s *PostgresStore) GetNextInterruptPrompt(ctx context.Context, sessionID string) (*types.PromptHistoryEntry, error) {
	var entry types.PromptHistoryEntry

	// Atomically claim the next interrupt prompt (same pattern as GetNextPendingPrompt)
	now := time.Now()
	result := s.gdb.WithContext(ctx).Raw(`
		UPDATE prompt_history_entries SET status = 'sending', updated_at = NOW()
		WHERE id = (
			SELECT id FROM prompt_history_entries
			WHERE session_id = ? AND interrupt = true AND deleted_at IS NULL
			AND (status = 'pending' OR (status = 'failed' AND (next_retry_at IS NULL OR next_retry_at <= ?)))
			ORDER BY COALESCE(queue_position, 999999) ASC, created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *
	`, sessionID, now).Scan(&entry)

	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}

	return &entry, nil
}

// ListPromptHistoryBySpecTask returns all non-deleted prompt history entries for a spec task
// Used by the queue processor to find pending prompts across all sessions
func (s *PostgresStore) ListPromptHistoryBySpecTask(ctx context.Context, specTaskID string) ([]*types.PromptHistoryEntry, error) {
	var entries []*types.PromptHistoryEntry
	err := s.gdb.WithContext(ctx).
		Where("spec_task_id = ? AND deleted_at IS NULL", specTaskID).
		Order("created_at ASC").
		Find(&entries).Error

	if err != nil {
		return nil, err
	}

	return entries, nil
}

// MarkPromptAsPending marks a prompt as pending (used before retry)
func (s *PostgresStore) MarkPromptAsPending(ctx context.Context, promptID string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ?", promptID).
		Update("status", "pending").
		Error
}

// MarkPromptAsSent marks a prompt as sent
func (s *PostgresStore) MarkPromptAsSent(ctx context.Context, promptID string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ?", promptID).
		Update("status", "sent").
		Error
}

// ClaimPromptForSending atomically transitions a prompt from pending/failed to "sending".
// Returns true if this caller won the race (rows affected > 0).
// If false is returned, another goroutine already claimed the prompt and the caller must not send it.
func (s *PostgresStore) ClaimPromptForSending(ctx context.Context, promptID string) (bool, error) {
	result := s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ? AND status IN ('pending', 'failed')", promptID).
		Update("status", "sending")
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// RequeueBouncedPrompt finds the most recent in-flight prompt for a session
// (status 'sent' or 'sending') and marks it as "failed" so the retry mechanism
// picks it up.
//
// Both statuses are considered: under the deferred-mark-sent flow, dispatched
// prompts stay in 'sending' until Zed actually starts streaming. A bounce
// before Zed's first message_added arrives leaves the prompt 'sending'. Older
// flows marked sent on dispatch, leaving bounced prompts 'sent'. Either way,
// the most recent in-flight prompt for the session is the one to requeue.
func (s *PostgresStore) RequeueBouncedPrompt(ctx context.Context, sessionID string) error {
	var prompt types.PromptHistoryEntry
	err := s.gdb.WithContext(ctx).
		Where("session_id = ? AND status IN ('sent', 'sending')", sessionID).
		Order("created_at DESC").
		First(&prompt).Error
	if err != nil {
		return err // no matching prompt found (e.g. Zed user message, not from queue)
	}
	return s.MarkPromptAsFailed(ctx, prompt.ID, "agent returned an empty response (bounce); requeueing for retry")
}

// promptErrorMessageMaxLen caps the persisted error string. The UI renders it
// inline; longer messages produce a wall-of-text in the queue list. Server
// logs still carry the full error for debugging.
const promptErrorMessageMaxLen = 500

// crashedPromptRetrySentinel is the next_retry_at value used by
// MarkPromptAsCrashed to suppress auto-retry. Picked far enough in the future
// that the queue's exponential-backoff selector will never match it, but a
// fixed timestamp (not max time.Time) so it serialises cleanly through GORM
// and Postgres timestamptz. ResetCrashedPromptsForSession identifies crashed
// rows by matching exactly this value.
var crashedPromptRetrySentinel = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

// MarkPromptAsFailed marks a prompt as failed with exponential backoff retry,
// recording the failure reason for display in the UI.
func (s *PostgresStore) MarkPromptAsFailed(ctx context.Context, promptID string, errorMsg string) error {
	// First get the current prompt to read retry count
	var prompt types.PromptHistoryEntry
	if err := s.gdb.WithContext(ctx).Where("id = ?", promptID).First(&prompt).Error; err != nil {
		return err
	}

	// Calculate next retry time with exponential backoff (2s, 4s, 8s, 16s, max 30s)
	newRetryCount := prompt.RetryCount + 1
	backoffSeconds := 2 << (newRetryCount - 1) // 2, 4, 8, 16, 32...
	if backoffSeconds > 30 {
		backoffSeconds = 30
	}
	nextRetry := time.Now().Add(time.Duration(backoffSeconds) * time.Second)

	if len(errorMsg) > promptErrorMessageMaxLen {
		errorMsg = errorMsg[:promptErrorMessageMaxLen]
	}

	return s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ?", promptID).
		Updates(map[string]interface{}{
			"status":        "failed",
			"retry_count":   newRetryCount,
			"next_retry_at": nextRetry,
			"error_message": errorMsg,
		}).
		Error
}

// MarkPromptAsCrashed marks a prompt as failed but pins next_retry_at to a
// far-future sentinel so the queue's auto-retry never picks it back up. Used
// for terminal failures (the Claude Agent process inside Zed exited and Helix
// can't respawn it without a fresh thread). The user must explicitly click
// Restart to recover, which calls ResetCrashedPromptsForSession.
//
// We don't introduce a separate 'crashed' status because GetNextPendingPrompt
// only selects 'pending' and 'failed' — pinning next_retry_at to year 9999
// keeps the row out of the selector while still letting existing failed-state
// UI render naturally with the error message.
func (s *PostgresStore) MarkPromptAsCrashed(ctx context.Context, promptID string, errorMsg string) error {
	if len(errorMsg) > promptErrorMessageMaxLen {
		errorMsg = errorMsg[:promptErrorMessageMaxLen]
	}

	return s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ?", promptID).
		Updates(map[string]interface{}{
			"status":        "failed",
			"next_retry_at": crashedPromptRetrySentinel,
			"error_message": errorMsg,
		}).
		Error
}

// ReconcileStuckSendingPrompts catches up prompt_history_entries that were
// orphaned by the old in-memory interactionToPromptMapping pre-2026-04-30:
// the dispatched interaction reached state='complete' (Zed responded) but the
// in-memory map was lost (API restart, dispatch-failure path, etc.) before
// MarkPromptAsSent could fire, so the prompt sat in 'sending' indefinitely
// while the response was already in the chat. New code path persists the link
// on Interaction.PromptID so this only matters for legacy rows + the brief
// window where new code hasn't shipped to all instances. Idempotent and safe
// to re-run; the WHERE clause is the no-op gate.
//
// Two queries cover both cases:
//
//  1. New rows where interactions.prompt_id is set — precise join, mark sent
//     when the linked interaction is complete.
//  2. Legacy rows where prompt_id is NULL — heuristic match on session_id +
//     timing. Only matches prompts older than 5 minutes (skip live work) whose
//     session has at least one Complete interaction since the prompt was
//     created.
func (s *PostgresStore) ReconcileStuckSendingPrompts(ctx context.Context) (int, error) {
	var total int

	// Path 1: precise — the new PromptID column links the interaction back.
	{
		result := s.gdb.WithContext(ctx).Exec(`
			UPDATE prompt_history_entries
			SET status = 'sent', updated_at = NOW()
			WHERE id IN (
				SELECT p.id
				FROM prompt_history_entries p
				JOIN interactions i ON i.prompt_id = p.id
				WHERE p.status = 'sending'
				  AND p.deleted_at IS NULL
				  AND i.state = 'complete'
			)
		`)
		if result.Error != nil {
			return total, result.Error
		}
		total += int(result.RowsAffected)
	}

	// Path 2: legacy heuristic — the prompt predates the PromptID column, so
	// rely on session_id + the existence of any Complete interaction created
	// after the prompt as evidence the agent processed it. The 5-minute floor
	// avoids racing newly-dispatched prompts that just haven't reached Zed yet.
	{
		result := s.gdb.WithContext(ctx).Exec(`
			UPDATE prompt_history_entries
			SET status = 'sent', updated_at = NOW()
			WHERE id IN (
				SELECT p.id
				FROM prompt_history_entries p
				WHERE p.status = 'sending'
				  AND p.deleted_at IS NULL
				  AND p.created_at < NOW() - INTERVAL '5 minutes'
				  AND EXISTS (
					SELECT 1 FROM interactions i
					WHERE i.session_id = p.session_id
					  AND i.state = 'complete'
					  AND i.created >= p.created_at
				  )
			)
		`)
		if result.Error != nil {
			return total, result.Error
		}
		total += int(result.RowsAffected)
	}

	return total, nil
}

// ResetCrashedPromptsForSession resets every prompt for sessionID that was
// marked crashed by MarkPromptAsCrashed: status becomes 'pending', retry_count
// is zeroed, next_retry_at and error_message are cleared. The auto-retry
// selector picks the prompts up on the next idle tick. Returns the number of
// prompts reset.
//
// Identifies crashed rows by next_retry_at = the sentinel; that's a tighter
// match than checking the error_message text and survives error-message
// rewording.
func (s *PostgresStore) ResetCrashedPromptsForSession(ctx context.Context, sessionID string) (int, error) {
	result := s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("session_id = ? AND status = ? AND next_retry_at = ? AND deleted_at IS NULL",
			sessionID, "failed", crashedPromptRetrySentinel).
		Updates(map[string]interface{}{
			"status":        "pending",
			"retry_count":   0,
			"next_retry_at": nil,
			"error_message": "",
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
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

// UpdatePromptPin sets the pinned status of a prompt
func (s *PostgresStore) UpdatePromptPin(ctx context.Context, promptID string, pinned bool) error {
	return s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ?", promptID).
		Update("pinned", pinned).
		Error
}

// UpdatePromptTags updates the tags of a prompt (JSON array string)
func (s *PostgresStore) UpdatePromptTags(ctx context.Context, promptID string, tags string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ?", promptID).
		Update("tags", tags).
		Error
}

// ListPinnedPrompts returns all pinned prompts for a user in a spec task
func (s *PostgresStore) ListPinnedPrompts(ctx context.Context, userID, specTaskID string) ([]*types.PromptHistoryEntry, error) {
	var entries []*types.PromptHistoryEntry
	query := s.gdb.WithContext(ctx).
		Where("user_id = ? AND pinned = ?", userID, true)

	if specTaskID != "" {
		query = query.Where("spec_task_id = ?", specTaskID)
	}

	err := query.
		Order("created_at DESC").
		Find(&entries).Error

	if err != nil {
		return nil, err
	}

	return entries, nil
}

// IncrementPromptUsage increments usage count and updates last_used_at
func (s *PostgresStore) IncrementPromptUsage(ctx context.Context, promptID string) error {
	now := time.Now()
	return s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ?", promptID).
		Updates(map[string]interface{}{
			"usage_count":  s.gdb.Raw("usage_count + 1"),
			"last_used_at": now,
		}).
		Error
}

// DeletePromptHistoryEntry soft-deletes a prompt history entry by setting deleted_at.
// Deleted entries are excluded from queue processing and sync responses.
func (s *PostgresStore) DeletePromptHistoryEntry(ctx context.Context, id string) error {
	now := time.Now()
	return s.gdb.WithContext(ctx).
		Model(&types.PromptHistoryEntry{}).
		Where("id = ?", id).
		Update("deleted_at", now).
		Error
}

// SearchPrompts searches prompts by content using ILIKE (case-insensitive)
func (s *PostgresStore) SearchPrompts(ctx context.Context, userID, query string, limit int) ([]*types.PromptHistoryEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var entries []*types.PromptHistoryEntry
	err := s.gdb.WithContext(ctx).
		Where("user_id = ? AND content ILIKE ?", userID, "%"+query+"%").
		Order("pinned DESC, usage_count DESC, created_at DESC").
		Limit(limit).
		Find(&entries).Error

	if err != nil {
		return nil, err
	}

	return entries, nil
}

// UnifiedSearch searches across projects, tasks, sessions, and prompts
func (s *PostgresStore) UnifiedSearch(ctx context.Context, userID string, req *types.UnifiedSearchRequest) (*types.UnifiedSearchResponse, error) {
	if req.Limit <= 0 {
		req.Limit = 10
	}

	results := make([]types.UnifiedSearchResult, 0)
	searchQuery := "%" + req.Query + "%"

	// Determine which types to search
	searchTypes := req.Types
	if len(searchTypes) == 0 {
		searchTypes = []string{"projects", "tasks", "sessions", "prompts", "knowledge", "repositories", "agents"}
	}

	for _, searchType := range searchTypes {
		switch searchType {
		case "projects":
			// Search projects
			var projects []types.Project
			query := s.gdb.WithContext(ctx).
				Where("owner = ? AND (name ILIKE ? OR description ILIKE ?)", userID, searchQuery, searchQuery).
				Limit(req.Limit).
				Order("updated_at DESC")

			if req.OrgID != "" {
				query = query.Where("owner = ? AND owner_type = ?", req.OrgID, types.OwnerTypeOrg)
			}

			if err := query.Find(&projects).Error; err == nil {
				for _, p := range projects {
					results = append(results, types.UnifiedSearchResult{
						Type:        "project",
						ID:          p.ID,
						Title:       p.Name,
						Description: truncateText(p.Description, 150),
						URL:         "/projects/" + p.ID,
						Icon:        "folder",
						Metadata: map[string]string{
							"status": p.Status,
						},
						CreatedAt: p.CreatedAt.Format(time.RFC3339),
						UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
					})
				}
			}

		case "tasks":
			// Search spec tasks
			var tasks []types.SpecTask
			query := s.gdb.WithContext(ctx).
				Where("owner_id = ? AND (name ILIKE ? OR original_prompt ILIKE ?)", userID, searchQuery, searchQuery).
				Limit(req.Limit).
				Order("updated_at DESC")

			if err := query.Find(&tasks).Error; err == nil {
				// Collect project IDs to fetch names
				projectIDs := make([]string, 0)
				for _, t := range tasks {
					if t.ProjectID != "" {
						projectIDs = append(projectIDs, t.ProjectID)
					}
				}

				// Fetch project names in bulk
				projectNames := make(map[string]string)
				if len(projectIDs) > 0 {
					var projects []types.Project
					if err := s.gdb.WithContext(ctx).
						Select("id", "name").
						Where("id IN ?", projectIDs).
						Find(&projects).Error; err == nil {
						for _, p := range projects {
							projectNames[p.ID] = p.Name
						}
					}
				}

				for _, t := range tasks {
					meta := map[string]string{
						"status": string(t.Status),
					}
					if t.ProjectID != "" {
						meta["projectId"] = t.ProjectID
						if name, ok := projectNames[t.ProjectID]; ok {
							meta["projectName"] = name
						}
					}

					results = append(results, types.UnifiedSearchResult{
						Type:        "task",
						ID:          t.ID,
						Title:       t.Name,
						Description: truncateText(t.OriginalPrompt, 150),
						URL:         "/tasks/" + t.ID,
						Icon:        "task",
						Metadata:    meta,
						CreatedAt:   t.CreatedAt.Format(time.RFC3339),
						UpdatedAt:   t.UpdatedAt.Format(time.RFC3339),
					})
				}
			}

		case "sessions":
			// Search sessions by name
			var sessions []types.Session
			query := s.gdb.WithContext(ctx).
				Preload("Interactions", func(db *gorm.DB) *gorm.DB {
					return db.Limit(1).Order("created ASC")
				}).
				Where("owner = ? AND name ILIKE ?", userID, searchQuery).
				Limit(req.Limit).
				Order("updated DESC")

			if err := query.Find(&sessions).Error; err == nil {
				// Collect task IDs to fetch names
				taskIDs := make([]string, 0)
				for _, sess := range sessions {
					if sess.Metadata.SpecTaskID != "" {
						taskIDs = append(taskIDs, sess.Metadata.SpecTaskID)
					}
				}

				// Fetch task names in bulk
				taskNames := make(map[string]string)
				if len(taskIDs) > 0 {
					var tasks []types.SpecTask
					if err := s.gdb.WithContext(ctx).
						Select("id", "name").
						Where("id IN ?", taskIDs).
						Find(&tasks).Error; err == nil {
						for _, t := range tasks {
							taskNames[t.ID] = t.Name
						}
					}
				}

				for _, sess := range sessions {
					var desc string
					if len(sess.Interactions) > 0 {
						desc = truncateText(sess.Interactions[0].PromptMessage, 150)
					}

					meta := map[string]string{
						"mode": string(sess.Mode),
					}
					if sess.ParentApp != "" {
						meta["appId"] = sess.ParentApp
					}
					if sess.Metadata.SpecTaskID != "" {
						meta["taskId"] = sess.Metadata.SpecTaskID
						if name, ok := taskNames[sess.Metadata.SpecTaskID]; ok {
							meta["taskName"] = name
						}
					}
					if sess.Metadata.ProjectID != "" {
						meta["projectId"] = sess.Metadata.ProjectID
					}

					results = append(results, types.UnifiedSearchResult{
						Type:        "session",
						ID:          sess.ID,
						Title:       sess.Name,
						Description: desc,
						URL:         "/session/" + sess.ID,
						Icon:        "chat",
						Metadata:    meta,
						CreatedAt:   sess.Created.Format(time.RFC3339),
						UpdatedAt:   sess.Updated.Format(time.RFC3339),
					})
				}
			}

		case "prompts":
			// Search prompt history
			var prompts []*types.PromptHistoryEntry
			query := s.gdb.WithContext(ctx).
				Where("user_id = ? AND content ILIKE ?", userID, searchQuery).
				Limit(req.Limit).
				Order("pinned DESC, created_at DESC")

			if err := query.Find(&prompts).Error; err == nil {
				for _, p := range prompts {
					meta := map[string]string{
						"status":    p.Status,
						"projectId": p.ProjectID,
						"taskId":    p.SpecTaskID,
					}
					if p.Pinned {
						meta["pinned"] = "true"
					}

					results = append(results, types.UnifiedSearchResult{
						Type:        "prompt",
						ID:          p.ID,
						Title:       truncateText(p.Content, 80),
						Description: truncateText(p.Content, 200),
						URL:         "/tasks/" + p.SpecTaskID,
						Icon:        "prompt",
						Metadata:    meta,
						CreatedAt:   p.CreatedAt.Format(time.RFC3339),
						UpdatedAt:   p.UpdatedAt.Format(time.RFC3339),
					})
				}
			}

		case "knowledge":
			// Search knowledge sources (RAG)
			var knowledgeList []*types.Knowledge
			query := s.gdb.WithContext(ctx).
				Where("owner = ? AND (name ILIKE ? OR description ILIKE ?)", userID, searchQuery, searchQuery).
				Limit(req.Limit).
				Order("updated DESC")

			if err := query.Find(&knowledgeList).Error; err == nil {
				for _, k := range knowledgeList {
					sourceType := getKnowledgeSourceType(k)
					meta := map[string]string{
						"state":      string(k.State),
						"sourceType": sourceType,
					}
					if k.AppID != "" {
						meta["appId"] = k.AppID
					}

					// Navigate to the app that contains this knowledge
					url := "/knowledge/" + k.ID
					if k.AppID != "" {
						url = "/app/" + k.AppID + "/knowledge"
					}

					results = append(results, types.UnifiedSearchResult{
						Type:        "knowledge",
						ID:          k.ID,
						Title:       k.Name,
						Description: truncateText(k.Description, 150),
						URL:         url,
						Icon:        "knowledge",
						Metadata:    meta,
						CreatedAt:   k.Created.Format(time.RFC3339),
						UpdatedAt:   k.Updated.Format(time.RFC3339),
					})
				}
			}

		case "repositories":
			// Search git repositories
			var repos []types.GitRepository
			query := s.gdb.WithContext(ctx).
				Where("owner_id = ? AND (name ILIKE ? OR description ILIKE ?)", userID, searchQuery, searchQuery).
				Limit(req.Limit).
				Order("updated_at DESC")

			if err := query.Find(&repos).Error; err == nil {
				for _, r := range repos {
					meta := map[string]string{}
					if r.ExternalURL != "" {
						meta["externalUrl"] = r.ExternalURL
					}
					if r.KoditIndexing {
						meta["koditIndexing"] = "true"
					}

					results = append(results, types.UnifiedSearchResult{
						Type:        "repository",
						ID:          r.ID,
						Title:       r.Name,
						Description: truncateText(r.Description, 150),
						URL:         "/repositories/" + r.ID,
						Icon:        "repository",
						Metadata:    meta,
						CreatedAt:   r.CreatedAt.Format(time.RFC3339),
						UpdatedAt:   r.UpdatedAt.Format(time.RFC3339),
					})
				}
			}

		case "agents":
			// Search apps/agents - name and description are in the JSONB config field
			var apps []types.App
			query := s.gdb.WithContext(ctx).
				Where("owner = ? AND (config->'helix'->>'name' ILIKE ? OR config->'helix'->>'description' ILIKE ?)",
					userID, searchQuery, searchQuery).
				Limit(req.Limit).
				Order("updated DESC")

			if err := query.Find(&apps).Error; err == nil {
				for _, a := range apps {
					name := a.Config.Helix.Name
					if name == "" {
						name = "Untitled Agent"
					}

					results = append(results, types.UnifiedSearchResult{
						Type:        "agent",
						ID:          a.ID,
						Title:       name,
						Description: truncateText(a.Config.Helix.Description, 150),
						URL:         "/app/" + a.ID,
						Icon:        "agent",
						CreatedAt:   a.Created.Format(time.RFC3339),
						UpdatedAt:   a.Updated.Format(time.RFC3339),
					})
				}
			}
		}
	}

	return &types.UnifiedSearchResponse{
		Results: results,
		Total:   len(results),
		Query:   req.Query,
	}, nil
}

// truncateText truncates text to maxLen, adding ellipsis if needed
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}

// getKnowledgeSourceType returns a human-readable source type for knowledge
func getKnowledgeSourceType(k *types.Knowledge) string {
	if k.Source.Web != nil {
		return "web"
	}
	if k.Source.Filestore != nil {
		return "files"
	}
	if k.Source.S3 != nil {
		return "s3"
	}
	if k.Source.GCS != nil {
		return "gcs"
	}
	if k.Source.SharePoint != nil {
		return "sharepoint"
	}
	if k.Source.Text != nil {
		return "text"
	}
	return "unknown"
}
