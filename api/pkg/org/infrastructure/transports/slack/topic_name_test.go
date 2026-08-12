package slack

import "testing"

func TestTopicName(t *testing.T) {
	cases := []struct {
		name, app, workspace, want string
	}{
		{"app and workspace", "Helix Prod", "Winder.AI", "Slack: Winder.AI (Helix Prod)"},
		{"workspace only", "", "Winder.AI", "Slack: Winder.AI"},
		{"app only", "Helix Prod", "", "Slack: Helix Prod"},
		{"neither — never a uuid", "", "", "Slack workspace"},
		{"trims whitespace", "  Helix Prod  ", "  Winder.AI  ", "Slack: Winder.AI (Helix Prod)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TopicName(c.app, c.workspace); got != c.want {
				t.Fatalf("TopicName(%q, %q) = %q, want %q", c.app, c.workspace, got, c.want)
			}
		})
	}
}
