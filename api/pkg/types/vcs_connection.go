package types

// VCSConnectionState is the per-provider lozenge state on the project board.
type VCSConnectionState string

const (
	// VCSConnectionVerified — connected and the account can reach the project's repos.
	VCSConnectionVerified VCSConnectionState = "verified"
	// VCSConnectionNeedsAttention — connected but the account can't reach a repo
	// (or is missing scopes). The lozenge prompts to switch/grant.
	VCSConnectionNeedsAttention VCSConnectionState = "needs_attention"
	// VCSConnectionDisconnected — no connection for this provider yet.
	VCSConnectionDisconnected VCSConnectionState = "disconnected"
)

// VCSActingUser is the Helix user the board is rendered for (who you're acting as).
type VCSActingUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// VCSPushingAs is the VCS account a user-initiated push would be attributed to.
type VCSPushingAs struct {
	Username     string `json:"username"`      // e.g. "@tonychapman-prog"
	ConnectionID string `json:"connection_id"` // OAuthConnection ID (for switch/disconnect)
}

// VCSRepoAccess is the verified-access result for one of the project's repos.
type VCSRepoAccess struct {
	Repo      string `json:"repo"`       // "owner/repo"
	HasAccess bool   `json:"has_access"` // true if the connection can reach it (or access is unverifiable for this provider)
	Verified  bool   `json:"verified"`   // true if we actually probed the provider (false = optimistic/unverifiable)
}

// VCSConnectionInfo is one lozenge: a distinct VCS provider present among the
// project's external repos, with the acting user, the account pushes go as, and
// per-repo verified access. One entry is returned per provider the project uses.
type VCSConnectionInfo struct {
	Provider      ExternalRepositoryType `json:"provider"`
	State         VCSConnectionState     `json:"state"`
	ActingUser    VCSActingUser          `json:"acting_user"`
	PushingAs     *VCSPushingAs          `json:"pushing_as,omitempty"`
	Repos         []VCSRepoAccess        `json:"repos"`
	MissingScopes []string               `json:"missing_scopes"`
}
