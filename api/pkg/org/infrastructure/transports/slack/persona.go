package slack

import "context"

// Persona is the name + avatar a Worker's message is posted under
// through the shared bot (FR-11). A channel has exactly one real Slack
// member (the bot); personas let Slack participants tell Workers apart.
type Persona struct {
	// Username is the display name applied to the message
	// (chat.postMessage `username`).
	Username string
	// IconURL is the avatar applied to the message
	// (chat.postMessage `icon_url`). Empty ⇒ Slack falls back to the
	// bot's default avatar. The plumbing ships now; its source (a
	// Worker-avatar attribute) is deferred — §13.
	IconURL string
}

// PersonaResolver maps a posting Worker to its Persona. Modelled as an
// injected port (like the GitHub TokenResolver) rather than a field on
// streaming.Message, so the shared envelope stays transport-neutral.
type PersonaResolver func(ctx context.Context, orgID, workerID string) (Persona, error)

// DefaultPersonaResolver derives the persona from the Worker's identity
// as it exists today: the bare Worker id as the username, no avatar (so
// Slack shows the bot's default). A richer resolver — pulling a
// friendly name and avatar from the Worker's identity/role — can be
// injected at the composition root without touching the outbound path.
func DefaultPersonaResolver(_ context.Context, _ string, workerID string) (Persona, error) {
	return Persona{Username: workerID}, nil
}
