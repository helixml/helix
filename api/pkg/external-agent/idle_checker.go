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

	grace := sleepGrace{idleTimeout: idleTimeout}
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			held, woke := grace.observe(now, now.Round(0).Sub(last.Round(0)), now.Sub(last))
			if woke {
				log.Info().
					Dur("wall_jump", now.Round(0).Sub(last.Round(0))-now.Sub(last)).
					Dur("grace", idleTimeout).
					Msg("host woke from sleep; holding idle desktop shutdowns")
			}
			last = now
			if held {
				continue
			}
			checkAndStopIdleDesktops(ctx, executor, st, idleTimeout)
		}
	}
}

// hostSleepJump is how far the wall clock must outrun the monotonic clock
// between two checks for the gap to count as the host having slept.
const hostSleepJump = time.Minute

// sleepGrace keeps time the host spent asleep from counting as desktop idle
// time.
//
// Helix for Mac's VM is paused while the Mac sleeps. On wake, chrony steps the
// wall clock forward by the whole sleep, so every desktop's last activity
// suddenly looks that old, and the first check after opening the lid stopped
// the desktop the user was about to use (a 17h28m sleep, stopped 3.5 minutes
// after wake). The monotonic clock does not advance while the VM is paused, so
// a wall clock that outran it between two checks marks a wake; idle shutdowns
// are then held for a full idleTimeout.
type sleepGrace struct {
	idleTimeout time.Duration
	wokeAt      time.Time
}

// observe takes the wall and monotonic time elapsed since the previous check
// and reports whether idle shutdowns are held, and whether this check
// detected a wake.
func (g *sleepGrace) observe(now time.Time, wallElapsed, monoElapsed time.Duration) (held, woke bool) {
	if wallElapsed-monoElapsed > hostSleepJump {
		g.wokeAt = now
		woke = true
	}
	held = !g.wokeAt.IsZero() && now.Sub(g.wokeAt) < g.idleTimeout
	return held, woke
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
		Str("dev_container_id", session.Metadata.DevContainerID).
		Msg("shutting down idle desktop")

	if err := executor.StopDesktop(ctx, session.ID); err != nil {
		log.Warn().
			Err(err).
			Str("session_id", session.ID).
			Msg("failed to stop idle desktop")
		return
	}

	// Reap any interaction left in state=waiting. The desktop we just stopped was
	// the only agent that could ever have delivered its message_completed. Left
	// waiting, it wedges the prompt-queue busy-check
	// (processPendingPromptsForIdleSessions) into treating the session as
	// perpetually busy — the next user message is never dispatched and the
	// desktop is never allowed to resume: a permanent deadlock. Marking it
	// interrupted lets the next message boot the desktop cleanly. This is the
	// deterministic seam — we KNOW the agent is gone here, so there is no live
	// turn worth preserving.
	if reaped, reapErr := st.ReapWaitingInteractions(ctx, session.ID, types.InteractionStateInterrupted, "desktop idle-stopped mid-turn"); reapErr != nil {
		log.Warn().Err(reapErr).Str("session_id", session.ID).Msg("failed to reap waiting interactions after idle desktop stop")
	} else if len(reaped) > 0 {
		log.Info().Str("session_id", session.ID).Int("count", len(reaped)).Msg("reaped waiting interactions after idle desktop stop")
	}

	// Re-fetch the session after StopDesktop — it saves PausedScreenshotPath
	// to the DB. Using the stale pre-stop copy would overwrite that.
	freshSession, err := st.GetSession(ctx, session.ID)
	if err != nil {
		log.Warn().Err(err).Str("session_id", session.ID).Msg("failed to re-fetch session after idle stop")
		return
	}
	freshSession.Metadata.ExternalAgentStatus = "terminated_idle"
	if _, err := st.UpdateSession(ctx, *freshSession); err != nil {
		log.Warn().
			Err(err).
			Str("session_id", session.ID).
			Msg("failed to update session metadata after idle shutdown")
	}
}
