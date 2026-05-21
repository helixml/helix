// Package transport_test characterises the public behaviour of the
// helix-org transport types — TransportKind enum, Transport struct, the
// per-Kind Config types, and the Validate() / *Config() parsers — prior
// to the B1 lift.
//
// Today the types being tested live in `helix-org/domain`. After B1
// step 2 they move into this package (`api/pkg/org/transport`) and the
// import below changes to a local reference. The test cases themselves
// must not change between step 1 (these tests, against the unmoved
// code) and step 2 (the lift). If they do, B1 is no longer
// behaviour-preserving — see the "Characterisation tests" rule in
// helix-org/CLAUDE.md.
//
// Coverage was widened versus the legacy `helix-org/domain/transport_test.go`
// in two places: direct round-trip tests for the Email and GitHub
// config parsers (previously only exercised indirectly via Validate),
// and explicit Validate cases for the GitHub transport (previously
// missing entirely). The legacy file is deleted in this same commit;
// every case it pinned is preserved below.
package transport_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/transport"
)

// --- Validate -----------------------------------------------------------

func TestTransportValidate_LocalAcceptsAnyConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  json.RawMessage
	}{
		{"no config", nil},
		{"junk config ignored", json.RawMessage(`not json at all`)},
		{"empty object", json.RawMessage(`{}`)},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := transport.Transport{Kind: transport.KindLocal, Config: tc.cfg}
			if err := tr.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestTransportValidate_Webhook(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     string
		wantErr string // substring; empty means accept
	}{
		{"inbound only (no config)", "", ""},
		{"inbound only (empty config)", `{}`, ""},
		{"outbound https", `{"outbound_url":"https://example.com/hook"}`, ""},
		{"outbound http localhost", `{"outbound_url":"http://localhost:9000"}`, ""},

		{"malformed json", `{not json`, "parse webhook config"},
		{"non-http scheme", `{"outbound_url":"ftp://example.com/hook"}`, "absolute http(s) URL"},
		{"relative url", `{"outbound_url":"/just/a/path"}`, "absolute http(s) URL"},
		{"no host", `{"outbound_url":"http:///nohost"}`, "no host"},
		{"malformed url", `{"outbound_url":"http://%zz"}`, "outbound_url"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := transport.Transport{Kind: transport.KindWebhook}
			if tc.cfg != "" {
				tr.Config = json.RawMessage(tc.cfg)
			}
			err := tr.Validate()
			assertError(t, err, tc.wantErr)
		})
	}
}

func TestTransportValidate_Email(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     string
		wantErr string
	}{
		{"valid alias", `{"alias":"sam"}`, ""},
		{"valid alias with dash", `{"alias":"customer-service"}`, ""},
		{"valid alias with underscore", `{"alias":"sales_team"}`, ""},
		{"valid alias with digits", `{"alias":"team42"}`, ""},

		{"missing alias (no config)", "", "alias is required"},
		{"empty alias", `{"alias":""}`, "alias is required"},
		{"alias contains @", `{"alias":"sam@x"}`, "lowercase alphanumeric"},
		{"alias contains +", `{"alias":"sa+m"}`, "lowercase alphanumeric"},
		{"alias contains dot", `{"alias":"sam.x"}`, "lowercase alphanumeric"},
		{"alias uppercase", `{"alias":"Sam"}`, "lowercase alphanumeric"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := transport.Transport{Kind: transport.KindEmail}
			if tc.cfg != "" {
				tr.Config = json.RawMessage(tc.cfg)
			}
			err := tr.Validate()
			assertError(t, err, tc.wantErr)
		})
	}
}

func TestTransportValidate_GitHub(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     string
		wantErr string
	}{
		{"valid", `{"repo":"helixml/helix","events":["issues","pull_request"]}`, ""},
		{"valid single event", `{"repo":"a/b","events":["issues"]}`, ""},
		{"valid all known events",
			`{"repo":"a/b","events":["issues","issue_comment","pull_request","pull_request_review","pull_request_review_comment"]}`,
			""},

		{"missing repo (no config)", "", "repo is required"},
		{"empty repo", `{"repo":"","events":["issues"]}`, "repo is required"},
		{"repo no slash", `{"repo":"helix","events":["issues"]}`, "must be of the form owner/name"},
		{"repo three slashes", `{"repo":"a/b/c","events":["issues"]}`, "must be of the form owner/name"},
		{"repo leading slash", `{"repo":"/b","events":["issues"]}`, "must be of the form owner/name"},
		{"repo trailing slash", `{"repo":"a/","events":["issues"]}`, "must be of the form owner/name"},

		{"missing events", `{"repo":"a/b"}`, "events whitelist is required"},
		{"empty events", `{"repo":"a/b","events":[]}`, "events whitelist is required"},
		{"unknown event", `{"repo":"a/b","events":["push"]}`, `unknown event "push"`},
		{"unknown event lists supported", `{"repo":"a/b","events":["push"]}`, "supported:"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := transport.Transport{Kind: transport.KindGitHub}
			if tc.cfg != "" {
				tr.Config = json.RawMessage(tc.cfg)
			}
			err := tr.Validate()
			assertError(t, err, tc.wantErr)
		})
	}
}

func TestTransportValidate_EmptyKindFails(t *testing.T) {
	t.Parallel()
	err := (transport.Transport{}).Validate()
	assertError(t, err, "transport kind is empty")
}

func TestTransportValidate_UnknownKindFails(t *testing.T) {
	t.Parallel()
	err := transport.Transport{Kind: "smtp"}.Validate()
	assertError(t, err, "unknown transport kind")
	// The error message lists every valid kind — Roles read it, so
	// stability of this format is part of the public surface.
	if !strings.Contains(err.Error(), `"local"`) ||
		!strings.Contains(err.Error(), `"webhook"`) ||
		!strings.Contains(err.Error(), `"email"`) ||
		!strings.Contains(err.Error(), `"github"`) {
		t.Fatalf("unknown-kind error should list every valid kind; got %q", err)
	}
}

// --- WebhookConfig parser ------------------------------------------------

func TestWebhookConfigParse_RejectsWrongKind(t *testing.T) {
	t.Parallel()
	_, err := transport.Transport{Kind: transport.KindLocal}.WebhookConfig()
	if err == nil {
		t.Fatalf("expected error parsing local transport as webhook")
	}
}

func TestWebhookConfigParse_EmptyConfigReturnsZeroValue(t *testing.T) {
	t.Parallel()
	c, err := transport.Transport{Kind: transport.KindWebhook}.WebhookConfig()
	if err != nil {
		t.Fatalf("WebhookConfig() = %v, want nil", err)
	}
	if c.OutboundURL != "" {
		t.Fatalf("OutboundURL = %q, want empty", c.OutboundURL)
	}
}

func TestWebhookConfigParse_PopulatedConfigRoundTrips(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"outbound_url":"https://example.com/x"}`)
	c, err := transport.Transport{Kind: transport.KindWebhook, Config: raw}.WebhookConfig()
	if err != nil {
		t.Fatalf("WebhookConfig() = %v", err)
	}
	if c.OutboundURL != "https://example.com/x" {
		t.Fatalf("OutboundURL = %q", c.OutboundURL)
	}
}

func TestWebhookConfigParse_UnknownFieldsIgnored(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"outbound_url":"https://example.com/x","future":"ignored"}`)
	c, err := transport.Transport{Kind: transport.KindWebhook, Config: raw}.WebhookConfig()
	if err != nil {
		t.Fatalf("WebhookConfig() = %v", err)
	}
	if c.OutboundURL != "https://example.com/x" {
		t.Fatalf("OutboundURL = %q", c.OutboundURL)
	}
}

// --- EmailConfig parser --------------------------------------------------

func TestEmailConfigParse_RejectsWrongKind(t *testing.T) {
	t.Parallel()
	_, err := transport.Transport{Kind: transport.KindLocal}.EmailConfig()
	if err == nil {
		t.Fatalf("expected error parsing local transport as email")
	}
}

func TestEmailConfigParse_EmptyConfigReturnsZeroValue(t *testing.T) {
	t.Parallel()
	c, err := transport.Transport{Kind: transport.KindEmail}.EmailConfig()
	if err != nil {
		t.Fatalf("EmailConfig() = %v, want nil", err)
	}
	if c.Alias != "" {
		t.Fatalf("Alias = %q, want empty", c.Alias)
	}
}

func TestEmailConfigParse_PopulatedConfigRoundTrips(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"alias":"customer-service"}`)
	c, err := transport.Transport{Kind: transport.KindEmail, Config: raw}.EmailConfig()
	if err != nil {
		t.Fatalf("EmailConfig() = %v", err)
	}
	if c.Alias != "customer-service" {
		t.Fatalf("Alias = %q", c.Alias)
	}
}

func TestEmailConfigParse_UnknownFieldsIgnored(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"alias":"sam","future":"ignored"}`)
	c, err := transport.Transport{Kind: transport.KindEmail, Config: raw}.EmailConfig()
	if err != nil {
		t.Fatalf("EmailConfig() = %v", err)
	}
	if c.Alias != "sam" {
		t.Fatalf("Alias = %q", c.Alias)
	}
}

func TestEmailConfigParse_MalformedJSONFails(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{not json`)
	_, err := transport.Transport{Kind: transport.KindEmail, Config: raw}.EmailConfig()
	assertError(t, err, "parse email config")
}

// --- GitHubConfig parser -------------------------------------------------

func TestGitHubConfigParse_RejectsWrongKind(t *testing.T) {
	t.Parallel()
	_, err := transport.Transport{Kind: transport.KindLocal}.GitHubConfig()
	if err == nil {
		t.Fatalf("expected error parsing local transport as github")
	}
}

func TestGitHubConfigParse_EmptyConfigReturnsZeroValue(t *testing.T) {
	t.Parallel()
	c, err := transport.Transport{Kind: transport.KindGitHub}.GitHubConfig()
	if err != nil {
		t.Fatalf("GitHubConfig() = %v, want nil", err)
	}
	if c.Repo != "" || len(c.Events) != 0 {
		t.Fatalf("zero value violated: %+v", c)
	}
}

func TestGitHubConfigParse_PopulatedConfigRoundTrips(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"repo":"helixml/helix","events":["issues","pull_request"]}`)
	c, err := transport.Transport{Kind: transport.KindGitHub, Config: raw}.GitHubConfig()
	if err != nil {
		t.Fatalf("GitHubConfig() = %v", err)
	}
	if c.Repo != "helixml/helix" {
		t.Fatalf("Repo = %q", c.Repo)
	}
	if want := []string{"issues", "pull_request"}; !sliceEqual(c.Events, want) {
		t.Fatalf("Events = %v, want %v", c.Events, want)
	}
}

func TestGitHubConfigParse_UnknownFieldsIgnored(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"repo":"a/b","events":["issues"],"future":"ignored"}`)
	c, err := transport.Transport{Kind: transport.KindGitHub, Config: raw}.GitHubConfig()
	if err != nil {
		t.Fatalf("GitHubConfig() = %v", err)
	}
	if c.Repo != "a/b" {
		t.Fatalf("Repo = %q", c.Repo)
	}
}

func TestGitHubConfigParse_MalformedJSONFails(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{not json`)
	_, err := transport.Transport{Kind: transport.KindGitHub, Config: raw}.GitHubConfig()
	assertError(t, err, "parse github config")
}

// --- Constructors and enum invariants ------------------------------------

func TestLocalTransport_IsZeroConfig(t *testing.T) {
	t.Parallel()
	lt := transport.LocalTransport()
	if lt.Kind != transport.KindLocal {
		t.Fatalf("Kind = %q, want %q", lt.Kind, transport.KindLocal)
	}
	if lt.Config != nil {
		t.Fatalf("Config = %q, want nil", lt.Config)
	}
	// Constructor output is always valid.
	if err := lt.Validate(); err != nil {
		t.Fatalf("LocalTransport().Validate() = %v, want nil", err)
	}
}

func TestTransportKindValues_ListsEveryKnownKind(t *testing.T) {
	t.Parallel()
	got := transport.KindValues()
	want := []transport.Kind{
		transport.KindLocal,
		transport.KindWebhook,
		transport.KindEmail,
		transport.KindGitHub,
	}
	if len(got) != len(want) {
		t.Fatalf("TransportKindValues() length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TransportKindValues()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// --- helpers -------------------------------------------------------------

func assertError(t *testing.T, err error, wantSubstring string) {
	t.Helper()
	if wantSubstring == "" {
		if err != nil {
			t.Fatalf("got error %q, want nil", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("got nil error, want one containing %q", wantSubstring)
	}
	if !strings.Contains(err.Error(), wantSubstring) {
		t.Fatalf("got error %q, want one containing %q", err, wantSubstring)
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
