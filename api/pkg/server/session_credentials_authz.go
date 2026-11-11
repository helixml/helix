package server

import (
	"github.com/helixml/helix/api/pkg/types"
)

// canUseSessionAgentCredential reports whether the caller may read or replace
// the decrypted agent subscription (Claude or Codex) a session resolves to.
//
// The legitimate caller is the settings-sync daemon inside the session's
// sandbox: it authenticates with the ephemeral API key minted for that one
// session (GetOrCreateSessionAPIKey), or on older deployments with the runner
// token. The subscription a session resolves to is often not the session
// owner's: an org-owned subscription, or one another member delegated to the
// org. Being the session owner is therefore not enough, or any member could
// start a session and read (or overwrite) a colleague's or the org's tokens.
//
// Outside the sandbox only the subscription's own user may use it.
func canUseSessionAgentCredential(user *types.User, session *types.Session, ownerID string, ownerType types.OwnerType) bool {
	if user == nil || session == nil {
		return false
	}
	if user.TokenType == types.TokenTypeRunner {
		return true
	}
	if user.TokenType == types.TokenTypeAPIKey && user.SessionID != "" {
		return user.SessionID == session.ID
	}
	return session.Owner == user.ID && ownerType == types.OwnerTypeUser && ownerID == user.ID
}
