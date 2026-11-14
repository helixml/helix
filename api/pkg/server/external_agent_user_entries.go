package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/store"
)

// userEntryAlreadyRecorded reports whether a user entry the external agent
// reported via message_added(role=user) already has an interaction in this
// session, so it must not become a new turn.
//
// An entry is recorded when either:
//   - its request_id belongs to a turn Helix knows: in the request map (even
//     after message_completed consumed it) or durably on an interaction of this
//     session (the in-memory map is lost on API restart); or
//   - the session already holds an interaction recorded from the same Zed
//     entry (message_id) with the same text. Zed gives every native user entry
//     a fresh request_id, so a re-reported entry is only recognisable by its
//     entry id. Comparing the text too keeps a genuinely new message that
//     reuses an index (after an agent restart renumbers entries) from being
//     dropped.
func (apiServer *HelixAPIServer) userEntryAlreadyRecorded(ctx context.Context, helixSessionID, requestID, entryID, content string) (bool, error) {
	st := apiServer.Controller.Options.Store

	if requestID != "" {
		apiServer.contextMappingsMutex.RLock()
		mappedID, known := apiServer.requestToInteractionMapping[requestID]
		apiServer.contextMappingsMutex.RUnlock()
		if known {
			log.Info().
				Str("helix_session_id", helixSessionID).
				Str("request_id", requestID).
				Str("interaction_id", mappedID).
				Msg("💬 [HELIX] User entry echoes a known request — reusing its interaction")
			return true, nil
		}

		existing, err := st.GetInteractionByExternalAgentRequestID(ctx, requestID)
		switch {
		case err == nil && existing.SessionID == helixSessionID:
			log.Info().
				Str("helix_session_id", helixSessionID).
				Str("request_id", requestID).
				Str("interaction_id", existing.ID).
				Msg("💬 [HELIX] User entry echoes a persisted request — reusing its interaction")
			return true, nil
		case err != nil && !errors.Is(err, store.ErrNotFound) && !errors.Is(err, gorm.ErrRecordNotFound):
			return false, fmt.Errorf("look up interaction for request %s: %w", requestID, err)
		}
	}

	existing, err := st.GetInteractionByExternalAgentUserEntry(ctx, helixSessionID, entryID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look up interaction for user entry %s: %w", entryID, err)
	}
	if existing.PromptMessage != content {
		return false, nil
	}
	log.Info().
		Str("helix_session_id", helixSessionID).
		Str("entry_id", entryID).
		Str("request_id", requestID).
		Str("interaction_id", existing.ID).
		Msg("💬 [HELIX] User entry was already recorded (agent re-reported it) — not creating a turn")
	return true, nil
}
