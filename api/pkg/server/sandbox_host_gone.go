package server

import (
	"context"
	"errors"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/connman"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
)

// sandboxHostGone reports whether a desktop teardown failed because the
// session's sandbox host has no RevDial connection past the reconnect grace
// period. Host IDs are pod names, so a replaced sandbox pod never comes back;
// a delete that waited for it would fail forever. Only use it when the caller
// is about to make the session dead to the orphan reaper (soft-deleted, or in
// a deleted project), so a host that does return is still cleaned up.
func sandboxHostGone(err error, sessionID string) bool {
	if !errors.Is(err, connman.ErrNoConnection) {
		return false
	}
	log.Warn().Err(err).Str("session_id", sessionID).
		Msg("sandbox host not connected; leaving desktop to the orphan reaper")
	return true
}

// destroyDesktopUnlessHostGone destroys a desktop the caller is deleting,
// skipping it when its sandbox host is gone (see sandboxHostGone).
func destroyDesktopUnlessHostGone(ctx context.Context, executor external_agent.Executor, sessionID, specTaskID string) error {
	err := executor.DestroyDesktop(ctx, sessionID, specTaskID)
	if err != nil && sandboxHostGone(err, sessionID) {
		return nil
	}
	return err
}
