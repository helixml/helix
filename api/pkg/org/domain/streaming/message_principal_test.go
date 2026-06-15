package streaming_test

import (
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// TestFromPrincipalInfersKind pins B6.3: callers can read a typed
// Principal from Message.From without consulting any external store.
// Kind is inferred from value shape — the existing alpha encodes
// worker IDs as "w-…" and transport-native senders as their
// transport-native string (email, Slack ID, phone). Empty From maps
// to the zero-Principal ("no sender").
func TestFromPrincipalInfersKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		from string
		want streaming.Principal
	}{
		{"empty is zero-Principal", "", streaming.Principal{}},
		{"w- prefix → Worker", "w-alice", streaming.NewPrincipalWorker("w-alice")},
		{"email → Transport", "alice@example.com", streaming.NewPrincipalTransport("alice@example.com")},
		{"Slack-shaped → Transport", "U0123ABCD", streaming.NewPrincipalTransport("U0123ABCD")},
		{"phone → Transport", "+15551234567", streaming.NewPrincipalTransport("+15551234567")},
		{"hyphenated handle without w- → Transport", "ops-team", streaming.NewPrincipalTransport("ops-team")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := streaming.Message{From: tc.from}
			if got := m.FromPrincipal(); got != tc.want {
				t.Errorf("FromPrincipal() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
