package server

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/types"
)

// canUseSessionAgentCredential reports whether the caller may read or replace
// the decrypted agent subscription (Claude or Codex) a session resolves to.
//
// The legitimate caller is the settings-sync daemon inside the session's
// sandbox: it authenticates with the ephemeral API key minted for that one
// session (GetOrCreateSessionAPIKey). A per-runner token for the session's own
// sandbox host may too; the shared runner token may not. The subscription a
// session resolves to is often not the session owner's: an org-owned
// subscription, or one another member delegated to the org. Being the session owner is therefore not enough, or any member could
// start a session and read (or overwrite) a colleague's or the org's tokens.
//
// Outside the sandbox only the subscription's own user may use it.
func canUseSessionAgentCredential(user *types.User, session *types.Session, ownerID string, ownerType types.OwnerType) bool {
	if user == nil || session == nil {
		return false
	}
	if user.TokenType == types.TokenTypeRunner {
		return canRunnerReadSession(user, session)
	}
	if user.TokenType == types.TokenTypeAPIKey && user.SessionID != "" {
		return user.SessionID == session.ID
	}
	return ownsSessionInKeyScope(user, session) && ownerType == types.OwnerTypeUser && ownerID == user.ID
}

// ownsSessionInKeyScope reports whether the caller owns session and holds a
// credential whose project and organization scope reach it: a key scoped to
// another project or organization does not carry its owner's ownership.
func ownsSessionInKeyScope(user *types.User, session *types.Session) bool {
	return callerOwns(user, session.Owner, session.OrganizationID) && enforceKeySessionScope(user, session) == nil
}

// authorizeSessionSandboxConfig gates the endpoints that hand a session's
// sandbox its configuration, which embeds the session's API key and agent
// credentials. A per-runner token may read only the config of sessions on its
// own sandbox host; the shared runner token is refused. Otherwise the
// caller must own the session, pass session RBAC (which confines
// project-scoped keys to their project), and, when calling with a key minted
// for one session, be calling about that session.
func (apiServer *HelixAPIServer) authorizeSessionSandboxConfig(ctx context.Context, user *types.User, session *types.Session) error {
	if user == nil || session == nil {
		return errors.New("access denied")
	}
	if user.TokenType == types.TokenTypeRunner {
		if !canRunnerReadSession(user, session) {
			return errors.New("access denied: runner token is not bound to this session's sandbox host")
		}
		return nil
	}
	if session.Owner != user.ID {
		return errors.New("access denied")
	}
	if user.TokenType == types.TokenTypeAPIKey && user.SessionID != "" && user.SessionID != session.ID {
		return errors.New("access denied: key is bound to another session")
	}
	return apiServer.authorizeUserToSession(ctx, user, session, types.ActionGet)
}
