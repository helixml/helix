package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertAgentElicitation records a question the agent asked, or refreshes one we already
// know about. Refresh is the common case: the agent re-affirms outstanding questions on a
// heartbeat, and a restarted API needs those re-announcements to rebuild its view.
//
// An upsert never resurrects a resolved question — once terminal, always terminal.
// Returns true when this call created a new pending question, which is what gates the
// user notification so a heartbeat cannot re-notify.
func (s *PostgresStore) UpsertAgentElicitation(ctx context.Context, elicitation *types.AgentElicitation) (bool, error) {
	if elicitation.ID == "" {
		return false, fmt.Errorf("elicitation ID is required")
	}
	now := time.Now()
	elicitation.Updated = now
	elicitation.LastSeenAt = now
	if elicitation.Created.IsZero() {
		elicitation.Created = now
	}
	if elicitation.Status == "" {
		elicitation.Status = types.ElicitationStatusPending
	}

	existing := &types.AgentElicitation{}
	err := s.gdb.WithContext(ctx).Where("id = ?", elicitation.ID).First(existing).Error
	switch {
	case err == gorm.ErrRecordNotFound:
		if createErr := s.gdb.WithContext(ctx).Create(elicitation).Error; createErr != nil {
			return false, fmt.Errorf("failed to create elicitation: %w", createErr)
		}
		return elicitation.Status == types.ElicitationStatusPending, nil
	case err != nil:
		return false, fmt.Errorf("failed to look up elicitation: %w", err)
	}

	// Known question. Only refresh the liveness stamp and the render payload; never
	// move a resolved question back to pending.
	updates := map[string]interface{}{
		"last_seen_at": now,
		"updated":      now,
	}
	if existing.InteractionID == "" && elicitation.InteractionID != "" {
		updates["interaction_id"] = elicitation.InteractionID
	}
	if err := s.gdb.WithContext(ctx).Model(&types.AgentElicitation{}).
		Where("id = ?", elicitation.ID).Updates(updates).Error; err != nil {
		return false, fmt.Errorf("failed to refresh elicitation: %w", err)
	}
	return false, nil
}

// GetAgentElicitation loads one question by id.
func (s *PostgresStore) GetAgentElicitation(ctx context.Context, id string) (*types.AgentElicitation, error) {
	if id == "" {
		return nil, fmt.Errorf("elicitation ID is required")
	}
	elicitation := &types.AgentElicitation{}
	if err := s.gdb.WithContext(ctx).Where("id = ?", id).First(elicitation).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get elicitation: %w", err)
	}
	return elicitation, nil
}

// TransitionAgentElicitation moves a question to a new status only if it is currently in
// one of fromStatuses. Returns false when the transition did not apply.
//
// This conditional write is the whole concurrency story: two clients answering at once,
// an answer racing a cancel from the agent, and a duplicate resolved event all resolve
// here rather than by whoever writes last.
func (s *PostgresStore) TransitionAgentElicitation(
	ctx context.Context,
	id string,
	fromStatuses []string,
	toStatus string,
	reason string,
	content []byte,
) (bool, error) {
	if id == "" {
		return false, fmt.Errorf("elicitation ID is required")
	}
	updates := map[string]interface{}{
		"status":  toStatus,
		"updated": time.Now(),
	}
	if reason != "" {
		updates["resolution_reason"] = reason
	}
	if content != nil {
		updates["content"] = content
	}

	query := s.gdb.WithContext(ctx).Model(&types.AgentElicitation{}).Where("id = ?", id)
	if len(fromStatuses) > 0 {
		query = query.Where("status IN ?", fromStatuses)
	}
	result := query.Updates(updates)
	if result.Error != nil {
		return false, fmt.Errorf("failed to transition elicitation: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// TouchAgentElicitations refreshes the liveness stamp for questions the agent says it
// still holds.
func (s *PostgresStore) TouchAgentElicitations(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := s.gdb.WithContext(ctx).Model(&types.AgentElicitation{}).
		Where("id IN ?", ids).
		Update("last_seen_at", time.Now()).Error; err != nil {
		return fmt.Errorf("failed to touch elicitations: %w", err)
	}
	return nil
}

// ListLiveAgentElicitationsForSession returns the questions on a session that can still
// be answered, oldest first.
func (s *PostgresStore) ListLiveAgentElicitationsForSession(ctx context.Context, sessionID string) ([]*types.AgentElicitation, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID is required")
	}
	var elicitations []*types.AgentElicitation
	if err := s.gdb.WithContext(ctx).
		Where("session_id = ? AND status IN ?", sessionID,
			[]string{types.ElicitationStatusPending, types.ElicitationStatusSubmitting}).
		Order("created ASC").
		Find(&elicitations).Error; err != nil {
		return nil, fmt.Errorf("failed to list elicitations: %w", err)
	}
	return elicitations, nil
}

// ListLiveAgentElicitationsForSessions is the batch form, for deciding which tasks in a
// list are blocked on a human.
func (s *PostgresStore) ListLiveAgentElicitationsForSessions(ctx context.Context, sessionIDs []string) ([]*types.AgentElicitation, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	var elicitations []*types.AgentElicitation
	if err := s.gdb.WithContext(ctx).
		Where("session_id IN ? AND status IN ?", sessionIDs,
			[]string{types.ElicitationStatusPending, types.ElicitationStatusSubmitting}).
		Find(&elicitations).Error; err != nil {
		return nil, fmt.Errorf("failed to list elicitations: %w", err)
	}
	return elicitations, nil
}

// HasLiveAgentElicitation reports whether an interaction is blocked on a human answer.
// Used to keep the auto-wake worker from "recovering" a turn that is waiting on a person,
// and to stop the prompt queue deferring a follow-up behind one.
func (s *PostgresStore) HasLiveAgentElicitation(ctx context.Context, interactionID string) (bool, error) {
	if interactionID == "" {
		return false, nil
	}
	var count int64
	if err := s.gdb.WithContext(ctx).Model(&types.AgentElicitation{}).
		Where("interaction_id = ? AND status IN ?", interactionID,
			[]string{types.ElicitationStatusPending, types.ElicitationStatusSubmitting}).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to count elicitations: %w", err)
	}
	return count > 0, nil
}

// ReapStaleAgentElicitations cancels questions no agent has claimed for longer than the
// grace window.
//
// This is the only place Helix declares a question dead on its own, and it does so on
// evidence: the agent holding a question re-affirms it on a heartbeat, so silence past
// the grace window means the agent that owned the respond_tx is gone (container restart,
// crash, thread teardown). A WebSocket reconnect is explicitly NOT evidence — the
// commonest cause of one is the Helix API restarting while the agent lives on.
func (s *PostgresStore) ReapStaleAgentElicitations(ctx context.Context, olderThan time.Time) ([]*types.AgentElicitation, error) {
	var stale []*types.AgentElicitation
	err := s.gdb.WithContext(ctx).
		Model(&types.AgentElicitation{}).
		Clauses(clause.Returning{}).
		Where("status IN ? AND last_seen_at < ?",
			[]string{types.ElicitationStatusPending, types.ElicitationStatusSubmitting},
			olderThan).
		Updates(map[string]interface{}{
			"status":            types.ElicitationStatusCancelled,
			"resolution_reason": types.ElicitationReasonAgentNoLongerHolds,
			"updated":           time.Now(),
		}).
		Scan(&stale).Error
	if err != nil {
		return nil, fmt.Errorf("failed to reap stale elicitations: %w", err)
	}
	return stale, nil
}
