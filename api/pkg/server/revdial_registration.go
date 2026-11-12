package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

// runnerTokenMayRegisterRevDial reports whether a runner-token caller may
// register a RevDial tunnel under runnerID. Sandbox hosts register
// "hydra-{id}" (Hydra) and "sandbox-{id}"; every other tunnel (notably
// "desktop-{session}") is registered with a key bound to its session.
func runnerTokenMayRegisterRevDial(runnerID string) bool {
	for _, prefix := range []string{"hydra-", "sandbox-"} {
		if rest, ok := strings.CutPrefix(runnerID, prefix); ok && rest != "" {
			return true
		}
	}
	return false
}

// revDialKeyMayRegisterDesktop checks the key side of a "desktop-{sessionID}"
// registration; the handler separately checks that the caller owns the
// session or sandbox.
//
// Delegated keys other than a bot instance's (app and embed keys, held by
// people who are not the owner) never register tunnels. A key minted for one
// session's sandbox (User.SessionID set) registers only that session's
// desktop, so a sandbox cannot take over its owner's other desktops.
func revDialKeyMayRegisterDesktop(user *types.User, sessionID string) error {
	if isDelegatedAPIKey(user.APIKeyType) && user.APIKeyType != types.APIkeytypeBotInstance {
		return errors.New("this key type cannot register desktop tunnels")
	}
	if user.SessionID != "" && user.SessionID != sessionID {
		return errors.New("key is scoped to another session")
	}
	return nil
}

// revDialOrigin describes where a RevDial registration came from, for logs:
// the peer address, plus the client chain a reverse proxy reported.
func revDialOrigin(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return r.RemoteAddr + " (forwarded for " + forwarded + ")"
	}
	return r.RemoteAddr
}
