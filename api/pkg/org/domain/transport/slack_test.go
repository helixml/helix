// Slack transport domain tests. The Slack Kind binds a Stream to a
// single Slack channel; per-stream config carries just the channel id
// (workspace credentials live at the per-org install layer, the global
// app at the OAuthProvider layer — see
// design/2026-06-16-helix-org-slack-stream.md §9.2). These tests pin
// the same surface the other Kinds pin: Validate rules, the typed
// accessor round-trip, and KindSlack's place in the canonical order.
package transport_test

import (
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

func TestTransportValidate_Slack(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     string
		wantErr string
	}{
		{"valid channel id", `{"channel":"C0123ABCD"}`, ""},
		{"valid channel id lowercase", `{"channel":"c0123abcd"}`, ""},

		{"missing channel (no config)", "", "channel is required"},
		{"empty channel", `{"channel":""}`, "channel is required"},
		{"malformed json", `{not json`, "parse slack config"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := transport.Transport{Kind: transport.KindSlack}
			if tc.cfg != "" {
				tr.Config = json.RawMessage(tc.cfg)
			}
			err := tr.Validate()
			assertError(t, err, tc.wantErr)
		})
	}
}

func TestSlackConfigParse_RejectsWrongKind(t *testing.T) {
	t.Parallel()
	_, err := transport.Transport{Kind: transport.KindLocal}.SlackConfig()
	if err == nil {
		t.Fatalf("expected error parsing local transport as slack")
	}
}

func TestSlackConfigParse_EmptyConfigReturnsZeroValue(t *testing.T) {
	t.Parallel()
	c, err := transport.Transport{Kind: transport.KindSlack}.SlackConfig()
	if err != nil {
		t.Fatalf("SlackConfig() = %v, want nil", err)
	}
	if c.Channel != "" {
		t.Fatalf("Channel = %q, want empty", c.Channel)
	}
}

func TestSlackConfigParse_PopulatedConfigRoundTrips(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"channel":"C0123ABCD"}`)
	c, err := transport.Transport{Kind: transport.KindSlack, Config: raw}.SlackConfig()
	if err != nil {
		t.Fatalf("SlackConfig() = %v", err)
	}
	if c.Channel != "C0123ABCD" {
		t.Fatalf("Channel = %q", c.Channel)
	}
}

func TestSlackConfigParse_UnknownFieldsIgnored(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"channel":"C1","future":"ignored"}`)
	c, err := transport.Transport{Kind: transport.KindSlack, Config: raw}.SlackConfig()
	if err != nil {
		t.Fatalf("SlackConfig() = %v", err)
	}
	if c.Channel != "C1" {
		t.Fatalf("Channel = %q", c.Channel)
	}
}

func TestSlackConfigParse_MalformedJSONFails(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{not json`)
	_, err := transport.Transport{Kind: transport.KindSlack, Config: raw}.SlackConfig()
	assertError(t, err, "parse slack config")
}

// TestTransportKindValues_IncludesSlack pins KindSlack's membership and
// canonical position (last — added after cron). The bare
// TestTransportKindValues_ListsEveryKnownKind in transport_test.go
// keeps the full ordered list; this one isolates the slack addition.
func TestTransportKindValues_IncludesSlack(t *testing.T) {
	t.Parallel()
	got := transport.KindValues()
	if len(got) == 0 || got[len(got)-1] != transport.KindSlack {
		t.Fatalf("KindValues() = %v, want KindSlack last", got)
	}
}
