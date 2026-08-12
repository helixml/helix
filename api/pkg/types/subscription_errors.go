package types

import "fmt"

// Task metadata keys carrying a machine-readable reason for a failed launch.
// The human message alone forces the browser to pattern-match prose to decide
// what to offer the user, which breaks the moment the wording changes.
const (
	TaskErrorCodeKey     = "error_code"
	TaskErrorProviderKey = "error_provider"

	TaskErrorSubscriptionRequired = "subscription_required"

	SubscriptionProviderClaude = "claude"
	SubscriptionProviderCodex  = "codex"
)

// MissingSubscriptionError reports that a desktop was refused because the
// agent authenticates with the user's own Claude/ChatGPT subscription and no
// such subscription is reachable from the session.
//
// It carries the provider so callers can send the user to the right login
// rather than to a generic settings page: the fix is always "connect this
// provider", and the user cannot discover which one from prose alone.
type MissingSubscriptionError struct {
	// Provider is SubscriptionProviderClaude or SubscriptionProviderCodex.
	Provider string
	// Label is the provider's user-facing name ("Claude", "ChatGPT").
	Label string
	Owner string
	OrgID string
}

func (e *MissingSubscriptionError) Error() string {
	return fmt.Sprintf(
		"agent is configured to use a %s subscription, but no active %s subscription is available to session owner %s (org %q) — connect one in Settings, or switch the agent to API-key credentials",
		e.Label, e.Label, e.Owner, e.OrgID,
	)
}
