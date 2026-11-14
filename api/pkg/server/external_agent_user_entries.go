package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// userEntry is a user message the external agent reported via
// message_added(role=user).
type userEntry struct {
	threadID  string // ACP thread the entry belongs to
	entryID   string // Zed's message_id: the entry's index in that thread
	requestID string
	content   string
}

// userEntryAlreadyRecorded reports whether a user entry already has an
// interaction in this session, so it must not become a new turn.
//
// An entry is recorded when either:
//   - its request_id belongs to a turn of THIS session: the interaction the
//     request map points at, or (after message_completed consumed the map
//     entry, or an API restart lost it) the interaction that persisted it; or
//   - this session generation already holds an interaction recorded from the
//     same entry of the same ACP thread, with the same text. Zed gives every
//     native user entry a fresh request_id, so a re-reported entry is only
//     recognisable by its position. The index restarts at 0 in every new
//     thread (thread recovery, agent switch), hence the thread in the key;
//     comparing the text keeps a renumbered index that carries a genuinely new
//     message from being dropped.
func (apiServer *HelixAPIServer) userEntryAlreadyRecorded(ctx context.Context, session *types.Session, entry userEntry) (bool, error) {
	st := apiServer.Controller.Options.Store

	if entry.requestID != "" {
		apiServer.contextMappingsMutex.RLock()
		mappedID := apiServer.requestToInteractionMapping[entry.requestID]
		apiServer.contextMappingsMutex.RUnlock()
		if mappedID != "" {
			mapped, err := st.GetInteraction(ctx, mappedID)
			if err != nil && !errors.Is(err, store.ErrNotFound) && !errors.Is(err, gorm.ErrRecordNotFound) {
				return false, fmt.Errorf("load interaction %s for request %s: %w", mappedID, entry.requestID, err)
			}
			if err == nil && mapped.SessionID == session.ID {
				logRecordedUserEntry(session.ID, entry, mapped.ID, "echo of a request in flight")
				return true, nil
			}
		}

		existing, err := st.GetInteractionByExternalAgentRequestID(ctx, entry.requestID)
		switch {
		case err == nil && existing.SessionID == session.ID:
			logRecordedUserEntry(session.ID, entry, existing.ID, "echo of a persisted request")
			return true, nil
		case err != nil && !errors.Is(err, store.ErrNotFound) && !errors.Is(err, gorm.ErrRecordNotFound):
			return false, fmt.Errorf("look up interaction for request %s: %w", entry.requestID, err)
		}
	}

	existing, err := st.GetInteractionByExternalAgentUserEntry(ctx, session.ID, session.GenerationID, entry.threadID, entry.entryID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look up interaction for user entry %s/%s: %w", entry.threadID, entry.entryID, err)
	}
	if existing.PromptMessage != entry.content {
		return false, nil
	}
	logRecordedUserEntry(session.ID, entry, existing.ID, "agent re-reported the entry")
	return true, nil
}

func logRecordedUserEntry(sessionID string, entry userEntry, interactionID, why string) {
	log.Info().
		Str("helix_session_id", sessionID).
		Str("acp_thread_id", entry.threadID).
		Str("entry_id", entry.entryID).
		Str("request_id", entry.requestID).
		Str("interaction_id", interactionID).
		Str("reason", why).
		Msg("💬 [HELIX] User entry already recorded — not creating a turn")
}
