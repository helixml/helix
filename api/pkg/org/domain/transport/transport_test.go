// Package transport_test characterises the public behaviour of the
// transport types — Kind enum, Transport struct, the per-Kind Config
// types, and the Validate() / *Config() parsers — lifted from
// helix-org/domain in B1.
//
// The test cases were authored against the unmoved code (with a
// temporary upward import to helix-org/domain) and ran green before
// the B1 step 2 lift; only the import path and symbol references
// changed in the lift commit. Names lost the redundant "Transport"
// prefix on the way through (domain.TransportKind -> transport.Kind,
// domain.TransportEmail -> transport.KindEmail, etc.).
//
// Coverage widened versus the legacy helix-org/domain/transport_test.go
// in two places: direct round-trip tests for the Email and GitHub
// config parsers (previously only exercised indirectly via Validate),
// and explicit Validate cases for the GitHub transport (previously
// missing entirely).
package transport_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/transport"
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

		// Custom event names are accepted — GitHub adds event types
		// over time (push, release, workflow_run, …) and operators can
		// opt in without us shipping a code change.
		{"custom event push", `{"repo":"a/b","events":["push"]}`, ""},
		{"custom event workflow_run", `{"repo":"a/b","events":["workflow_run"]}`, ""},
		{"mixed known + custom", `{"repo":"a/b","events":["issues","release"]}`, ""},
		// Wildcard "*" honoured — same semantics as GitHub's webhook
		// API (deliver every event the repo emits).
		{"wildcard only", `{"repo":"a/b","events":["*"]}`, ""},
		{"wildcard mixed", `{"repo":"a/b","events":["*","issues"]}`, ""},

		// Malformed names still rejected at create_topic time.
		{"uppercase event", `{"repo":"a/b","events":["Push"]}`, `invalid event "Push"`},
		{"dash event", `{"repo":"a/b","events":["pull-request"]}`, `invalid event "pull-request"`},
		{"leading digit event", `{"repo":"a/b","events":["1_event"]}`, `invalid event "1_event"`},
		{"empty event string", `{"repo":"a/b","events":[""]}`, `invalid event ""`},
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

func TestTransportValidate_Slack(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     string
		wantErr string
	}{
		{"valid", `{"service_connection_id":"sc-123"}`, ""},

		{"missing connection (no config)", "", "service_connection_id is required"},
		{"empty connection", `{"service_connection_id":""}`, "service_connection_id is required"},

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
		!strings.Contains(err.Error(), `"github"`) ||
		!strings.Contains(err.Error(), `"cron"`) ||
		!strings.Contains(err.Error(), `"slack"`) {
		t.Fatalf("unknown-kind error should list every valid kind; got %q", err)
	}
}

// TestTransportValidate_Cron exercises the cron kind end-to-end via
// Transport.Validate, mirroring the per-kind tests above. Fine-grained
// CronConfig.Validate cases live in cron_test.go.
func TestTransportValidate_Cron(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     string
		wantErr string
	}{
		{"valid 5-field", `{"schedule":"0 9 * * 1"}`, ""},
		{"valid daily midnight", `{"schedule":"0 0 * * *"}`, ""},
		{"valid weekdays midnight", `{"schedule":"0 0 * * 1-5"}`, ""},
		{"valid weekend range", `{"schedule":"0 0 * * 0,6"}`, ""},
		{"valid with timezone prefix", `{"schedule":"CRON_TZ=Europe/London 0 9 * * 1"}`, ""},
		{"valid as bare string", `"0 9 * * 1"`, ""},

		{"missing schedule (no config)", "", "schedule is required"},
		{"empty schedule", `{"schedule":""}`, "schedule is required"},

		// DoS-prevention: every-minute is exactly 60s between fires,
		// below the 90s floor. The error message must name the limit so
		// the frontend can surface it verbatim.
		{"every minute", `{"schedule":"* * * * *"}`, "more often than the 1m30s minimum"},
		// 6-field (per-second) specs are not parsed by ParseStandard,
		// so they bounce out at parse time — never reaching the gap
		// check. This is intentional: closing the syntactic door is
		// stronger than closing the semantic one.
		{"per-second", `{"schedule":"*/30 * * * * *"}`, "invalid cron schedule"},

		{"malformed json", `{not json`, "parse cron config"},
		{"nonsense schedule", `{"schedule":"not a cron"}`, "invalid cron schedule"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := transport.Transport{Kind: transport.KindCron}
			if tc.cfg != "" {
				tr.Config = json.RawMessage(tc.cfg)
			}
			err := tr.Validate()
			assertError(t, err, tc.wantErr)
		})
	}
}

// TestStrategiesMatchKindOrder pins the public invariant that every
// Kind in kindOrder has a Strategy registered. Adding a new Kind to
// kindOrder without a strategies entry would otherwise fall over at
// Validate() with the bewildering "unknown transport kind" error
// rather than at compile time / boot time. The test catches the drift.
func TestStrategiesMatchKindOrder(t *testing.T) {
	t.Parallel()
	for _, k := range transport.KindValues() {
		tr := transport.Transport{Kind: k}
		// Validate with no config is enough to exercise the registry
		// lookup — we don't care whether the kind's own validation
		// passes (some kinds require config), only that the dispatch
		// found a Strategy. The "unknown transport kind" error is the
		// failure mode we're guarding against.
		if err := tr.Validate(); err != nil && strings.Contains(err.Error(), "unknown transport kind") {
			t.Fatalf("Kind %q is in kindOrder but has no Strategy registered", k)
		}
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

// --- SlackConfig parser --------------------------------------------------

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
	if c.ServiceConnectionID != "" {
		t.Fatalf("zero value violated: %+v", c)
	}
}

func TestSlackConfigParse_PopulatedConfigRoundTrips(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"service_connection_id":"sc-123","channel_id":"C123"}`)
	c, err := transport.Transport{Kind: transport.KindSlack, Config: raw}.SlackConfig()
	if err != nil {
		t.Fatalf("SlackConfig() = %v", err)
	}
	if c.ServiceConnectionID != "sc-123" || c.ChannelID != "C123" {
		t.Fatalf("round-trip violated: %+v", c)
	}
}

func TestSlackConfigParse_MalformedJSONFails(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{not json`)
	_, err := transport.Transport{Kind: transport.KindSlack, Config: raw}.SlackConfig()
	assertError(t, err, "parse slack config")
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
		transport.KindGitLab,
		transport.KindCron,
		transport.KindSlack,
		transport.KindHelixEvents,
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
