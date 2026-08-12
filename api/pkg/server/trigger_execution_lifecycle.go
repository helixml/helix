package server

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// failRunningTriggerExecution marks a recurring job terminal when its external
// agent or sandbox fails. The shared failure handlers also serve ordinary chat
// sessions; those have no linked running execution and are therefore no-ops.
func (s *HelixAPIServer) failRunningTriggerExecution(sessionID, failure string) {
	if sessionID == "" {
		return
	}
	ctx := context.Background()
	if _, err := s.Store.FinishTriggerExecution(ctx, sessionID, types.TriggerExecutionStatusError, failure); err != nil && !errors.Is(err, store.ErrNotFound) {
		log.Error().Err(err).Str("session_id", sessionID).Msg("failed to mark recurring task execution as failed")
	}
}
