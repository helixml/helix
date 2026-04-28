package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// TransportKind names the implementation that owns a Stream's I/O.
// Every Stream has one. The default — TransportLocal — means events
// live in the SQLite events table and are delivered through the
// in-process broadcaster and dispatcher; nothing crosses a network.
// Other kinds (Slack, email, webhook, RSS, tick…) compose external
// I/O over the same local store.
type TransportKind string

const (
	// TransportLocal is the default: SQLite + broadcaster + dispatcher.
	// No external I/O.
	TransportLocal TransportKind = "local"

	// TransportWebhook is a bidirectional HTTP transport.
	//
	// Inbound: POSTs to /webhooks/<streamID> are turned into Events on
	// the Stream. No config required — the path uses the Stream's own
	// ID as the secret-by-obscurity, which is enough for low-stakes
	// use; production callers should add a signing secret on top.
	//
	// Outbound: when Config sets `outbound_url`, every Event appended
	// to the Stream (regardless of who appended it — webhook handler,
	// publish tool, dm tool) is POSTed to that URL with the event body
	// as the request body. Failures are logged and dropped; the append
	// itself still succeeds.
	//
	// A Stream can be inbound-only (no config), outbound-only (config
	// with outbound_url), or both at once.
	TransportWebhook TransportKind = "webhook"

	// TransportEmail is a bidirectional email transport. Provider
	// credentials live at server level (see config.transport.postmark);
	// per-stream config carries only the routing identity (`alias`).
	//
	// Inbound: an external service (Postmark today) POSTs parsed
	// inbound mail to /email/<provider>; the transport reads the
	// recipient address, extracts the alias, and routes to the
	// matching Stream. The body becomes a Message envelope on that
	// Stream — From=sender, To=[recipient], Subject, Body, MessageID,
	// InReplyTo, ThreadID populated from the email's headers.
	//
	// Outbound: every Event appended to an email Stream is rendered
	// to a provider API call (Postmark /email today). The Message
	// envelope's From/To/Subject/InReplyTo/ThreadID drive the
	// outbound headers; the global `from` from server config is the
	// envelope sender unless the Stream's Message specifies
	// otherwise.
	TransportEmail TransportKind = "email"

	// TransportGitHub is an inbound-only GitHub webhooks transport.
	// Provider credentials live at server level (see
	// config.transport.github); per-stream config carries the routing
	// `repo` and an `events` whitelist.
	//
	// Inbound: GitHub POSTs to a single installation URL
	// (/github/webhook). The transport HMAC-verifies the delivery
	// against the installation's webhook_secret, then fans the event
	// out to every Stream whose Config.Repo matches the payload's
	// repository.full_name and whose Config.Events list contains the
	// X-GitHub-Event header value. The Message envelope is mapped
	// from the upstream payload verbatim — Subject = issue/PR title,
	// Body = issue/PR/comment/review body, ThreadID = "#<number>",
	// MessageID = X-GitHub-Delivery, From = sender.login, Extra = the
	// full payload with one synthetic top-level key (`event`) added
	// from the X-GitHub-Event header.
	//
	// Outbound: not supported. Acting on a repo (label, comment,
	// review, open PR) is the Worker's job via `gh` in its
	// Environment; the github transport rejects publish calls
	// loudly rather than silently dropping. See
	// design/github-transport.md.
	TransportGitHub TransportKind = "github"
)

// Transport describes how events on a Stream move to and from the
// outside world. Internal Streams use TransportLocal — that is still a
// transport, just one whose endpoints are both inside the system.
//
// Config is opaque per-Kind JSON. The local transport ignores it; other
// transports parse it according to their own schema (see WebhookConfig).
type Transport struct {
	Kind   TransportKind
	Config json.RawMessage
}

// LocalTransport is the zero-config default returned when a caller does
// not specify a transport. Treat the returned value as immutable.
func LocalTransport() Transport {
	return Transport{Kind: TransportLocal}
}

// WebhookConfig is the parsed shape of Transport.Config when
// Kind == TransportWebhook. All fields are optional; a webhook stream
// with a zero WebhookConfig is inbound-only.
type WebhookConfig struct {
	// OutboundURL, when set, makes the Stream emit each appended Event
	// as an HTTP POST to this URL. Must be an absolute http(s) URL.
	OutboundURL string `json:"outbound_url,omitempty"`
}

// EmailConfig is the parsed shape of Transport.Config when
// Kind == TransportEmail. Provider credentials live in server-level
// config; the only per-stream knob is the alias used for routing.
type EmailConfig struct {
	// Alias is the routing identifier for this Stream. Inbound mail
	// addressed to <hash>+<alias>@inbound.postmarkapp.com (no-domain
	// path) or <alias>@yourdomain.com (with-domain path) lands on
	// this Stream. Required and unique within the installation.
	Alias string `json:"alias,omitempty"`
}

// GitHubConfig is the parsed shape of Transport.Config when
// Kind == TransportGitHub. Provider credentials (token, webhook
// secret) live in server-level config; per-stream config carries
// the routing identity.
type GitHubConfig struct {
	// Repo is the GitHub `owner/name` whose webhook deliveries land
	// on this Stream. Matched case-insensitively against
	// `repository.full_name` in the payload.
	Repo string `json:"repo,omitempty"`

	// Events is the whitelist of GitHub event types
	// (X-GitHub-Event header values) the Stream wants. Anything not
	// listed is dropped at the transport without becoming an Event,
	// so subscribed Workers don't activate for events they'd ignore.
	// Required and non-empty.
	Events []string `json:"events,omitempty"`
}

// knownGitHubEvents enumerates the event types the transport
// currently accepts in a Stream's `events` whitelist. The list is
// deliberately narrow — adding an event is a one-line edit here
// plus tests, but unknown event names are rejected at create_stream
// time so typos surface early.
var knownGitHubEvents = map[string]struct{}{
	"issues":                      {},
	"issue_comment":               {},
	"pull_request":                {},
	"pull_request_review":         {},
	"pull_request_review_comment": {},
}

// WebhookConfig parses Transport.Config as a WebhookConfig. Returns the
// zero value with no error when Config is empty. Errors only on JSON
// shape problems — semantic validation happens in Validate().
func (t Transport) WebhookConfig() (WebhookConfig, error) {
	if t.Kind != TransportWebhook {
		return WebhookConfig{}, fmt.Errorf("transport kind is %q, not webhook", t.Kind)
	}
	var c WebhookConfig
	if len(t.Config) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(t.Config, &c); err != nil {
		return WebhookConfig{}, fmt.Errorf("parse webhook config: %w", err)
	}
	return c, nil
}

// EmailConfig parses Transport.Config as an EmailConfig. Returns the
// zero value with no error when Config is empty. Errors only on JSON
// shape problems — semantic validation happens in Validate().
func (t Transport) EmailConfig() (EmailConfig, error) {
	if t.Kind != TransportEmail {
		return EmailConfig{}, fmt.Errorf("transport kind is %q, not email", t.Kind)
	}
	var c EmailConfig
	if len(t.Config) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(t.Config, &c); err != nil {
		return EmailConfig{}, fmt.Errorf("parse email config: %w", err)
	}
	return c, nil
}

// GitHubConfig parses Transport.Config as a GitHubConfig. Returns the
// zero value with no error when Config is empty. Errors only on JSON
// shape problems — semantic validation happens in Validate().
func (t Transport) GitHubConfig() (GitHubConfig, error) {
	if t.Kind != TransportGitHub {
		return GitHubConfig{}, fmt.Errorf("transport kind is %q, not github", t.Kind)
	}
	var c GitHubConfig
	if len(t.Config) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(t.Config, &c); err != nil {
		return GitHubConfig{}, fmt.Errorf("parse github config: %w", err)
	}
	return c, nil
}

// Validate checks that the Kind is non-empty and recognised, and that
// any per-Kind Config parses and is internally consistent. For webhook
// streams that means OutboundURL (if set) must be a valid absolute
// http(s) URL.
func (t Transport) Validate() error {
	if t.Kind == "" {
		return errors.New("transport kind is empty")
	}
	switch t.Kind {
	case TransportLocal:
		return nil
	case TransportWebhook:
		c, err := t.WebhookConfig()
		if err != nil {
			return err
		}
		if c.OutboundURL == "" {
			return nil
		}
		u, err := url.Parse(c.OutboundURL)
		if err != nil {
			return fmt.Errorf("outbound_url: %w", err)
		}
		if !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("outbound_url must be an absolute http(s) URL, got %q", c.OutboundURL)
		}
		if u.Host == "" {
			return fmt.Errorf("outbound_url has no host: %q", c.OutboundURL)
		}
		return nil
	case TransportEmail:
		c, err := t.EmailConfig()
		if err != nil {
			return err
		}
		if c.Alias == "" {
			return errors.New("email transport: alias is required")
		}
		if !isValidEmailAlias(c.Alias) {
			return fmt.Errorf("email transport: alias %q must be lowercase alphanumeric / dash / underscore (no @, +, dots)", c.Alias)
		}
		return nil
	case TransportGitHub:
		c, err := t.GitHubConfig()
		if err != nil {
			return err
		}
		if c.Repo == "" {
			return errors.New("github transport: repo is required")
		}
		// Repo must be exactly "owner/name" — one slash, both halves
		// non-empty. Anything else is a typo we'd rather catch at
		// create_stream time than have webhook deliveries silently
		// miss the stream.
		parts := strings.Split(c.Repo, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("github transport: repo %q must be of the form owner/name", c.Repo)
		}
		if len(c.Events) == 0 {
			return errors.New("github transport: events whitelist is required and must be non-empty")
		}
		for _, ev := range c.Events {
			if _, ok := knownGitHubEvents[ev]; !ok {
				return fmt.Errorf("github transport: unknown event %q (supported: %s)", ev, knownGitHubEventsList())
			}
		}
		return nil
	default:
		return errors.New("unknown transport kind: " + string(t.Kind))
	}
}

// knownGitHubEventsList renders the supported event names alphabetically
// for use in error messages. Cheap; called only on validation failures.
func knownGitHubEventsList() string {
	out := make([]string, 0, len(knownGitHubEvents))
	for k := range knownGitHubEvents {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// isValidEmailAlias enforces a conservative alias shape so it can be
// safely composed into addresses (`<hash>+<alias>@…` or
// `<alias>@yourdomain.com`) without ambiguity. ASCII letters, digits,
// dash, underscore. No `+` (we use it as the separator), no `@`, no
// `.` (avoids subaddress-of-subaddress confusion), no whitespace.
func isValidEmailAlias(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}
