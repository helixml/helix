package external_agent

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// RunDesktopIdleChecker periodically shuts down desktops that have had no
// interaction activity for longer than idleTimeout. checkInterval controls how
// often the check runs. It blocks until ctx is cancelled and is intended to be
// run in a goroutine.
func RunDesktopIdleChecker(ctx context.Context, executor Executor, st store.Store, idleTimeout, checkInterval time.Duration) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkAndStopIdleDesktops(ctx, executor, st, idleTimeout)
		}
	}
}

func checkAndStopIdleDesktops(ctx context.Context, executor Executor, st store.Store, idleTimeout time.Duration) {
	idleSince := time.Now().Add(-idleTimeout)

	sessions, err := st.ListIdleDesktops(ctx, idleSince)
	if err != nil {
		log.Error().Err(err).Msg("failed to list idle desktops")
		return
	}

	for _, session := range sessions {
		stopIdleDesktop(ctx, executor, st, session)
	}
}

func stopIdleDesktop(ctx context.Context, executor Executor, st store.Store, session *types.Session) {
	log.Info().
		Str("session_id", session.ID).
		Str("external_agent_id", session.Metadata.ExternalAgentID).
		Msg("shutting down idle desktop")

	if err := executor.StopDesktop(ctx, session.ID); err != nil {
		log.Warn().
			Err(err).
			Str("session_id", session.ID).
			Msg("failed to stop idle desktop")
	}

	metadata := session.Metadata
	metadata.ExternalAgentStatus = "terminated_idle"
	if err := st.UpdateSessionMetadata(ctx, session.ID, metadata); err != nil {
		log.Warn().
			Err(err).
			Str("session_id", session.ID).
			Msg("failed to update session metadata after idle shutdown")
	}
}
