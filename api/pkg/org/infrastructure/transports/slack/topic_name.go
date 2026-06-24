package slack

import (
	"fmt"
	"strings"
)

// TopicName is the display name for the auto-created per-workspace Slack
// Topic. It reads as "Slack: <workspace> (<app>)" so the list shows which
// workspace the messages come from and which installed app routes them —
// never the bare ServiceConnection uuid, which is opaque. The Topic id
// stays the deterministic s-slack-ws-<connID> for uniqueness; this is only
// the human-facing label.
func TopicName(appName, workspaceName string) string {
	app := strings.TrimSpace(appName)
	ws := strings.TrimSpace(workspaceName)
	switch {
	case app != "" && ws != "":
		return fmt.Sprintf("Slack: %s (%s)", ws, app)
	case ws != "":
		return "Slack: " + ws
	case app != "":
		return "Slack: " + app
	default:
		return "Slack workspace"
	}
}
