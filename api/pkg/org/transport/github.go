package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// KindGitHub is an inbound-only GitHub webhooks transport. Provider
// credentials live at server level (see config.transport.github);
// per-stream config carries the routing `repo` and an `events`
// whitelist.
//
// Inbound: GitHub POSTs to a single installation URL
// (/github/webhook). The transport HMAC-verifies the delivery against
// the installation's webhook_secret, then fans the event out to every
// Stream whose Config.Repo matches the payload's
// repository.full_name and whose Config.Events list contains the
// X-GitHub-Event header value. The Message envelope is mapped from
// the upstream payload verbatim — Subject = issue/PR title, Body =
// issue/PR/comment/review body, ThreadID = "#<number>",
// MessageID = X-GitHub-Delivery, From = sender.login, Extra = the
// full payload with one synthetic top-level key (`event`) added from
// the X-GitHub-Event header.
//
// Outbound: not supported. Acting on a repo (label, comment, review,
// open PR) is the Worker's job via `gh` in its Environment; the
// github transport rejects publish calls loudly rather than silently
// dropping.
const KindGitHub Kind = "github"

// GitHubConfig is the parsed shape of Transport.Config when
// Kind == KindGitHub. Provider credentials (token, webhook secret)
// live in server-level config; per-stream config carries the routing
// identity.
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

// Validate enforces: repo must be "owner/name" with both halves
// non-empty; events must be non-empty and every entry must be a
// known GitHub event type.
func (g GitHubConfig) Validate() error {
	if g.Repo == "" {
		return errors.New("github transport: repo is required")
	}
	// Repo must be exactly "owner/name" — one slash, both halves
	// non-empty. Anything else is a typo we'd rather catch at
	// create_stream time than have webhook deliveries silently miss
	// the stream.
	parts := strings.Split(g.Repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("github transport: repo %q must be of the form owner/name", g.Repo)
	}
	if len(g.Events) == 0 {
		return errors.New("github transport: events whitelist is required and must be non-empty")
	}
	for _, ev := range g.Events {
		if _, ok := knownGitHubEvents[ev]; !ok {
			return fmt.Errorf("github transport: unknown event %q (supported: %s)", ev, knownGitHubEventsList())
		}
	}
	return nil
}

// GitHubConfig parses t.Config as a GitHubConfig. Same shape and
// semantics as Transport.WebhookConfig() — see webhook.go.
func (t Transport) GitHubConfig() (GitHubConfig, error) {
	if t.Kind != KindGitHub {
		return GitHubConfig{}, fmt.Errorf("transport kind is %q, not github", t.Kind)
	}
	return parseGitHubConfig(t.Config)
}

// github is the Strategy for KindGitHub.
type github struct{}

// ParseConfig satisfies Strategy.
func (github) ParseConfig(raw json.RawMessage) (Config, error) {
	c, err := parseGitHubConfig(raw)
	return c, err
}

// parseGitHubConfig is the typed parser.
func parseGitHubConfig(raw json.RawMessage) (GitHubConfig, error) {
	var c GitHubConfig
	if len(raw) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return GitHubConfig{}, fmt.Errorf("parse github config: %w", err)
	}
	return c, nil
}

// knownGitHubEvents enumerates the event types the transport
// currently accepts in a Stream's `events` whitelist. The list is
// deliberately narrow — adding an event is a one-line edit here plus
// tests, but unknown event names are rejected at create_stream time
// so typos surface early.
var knownGitHubEvents = map[string]struct{}{
	"issues":                      {},
	"issue_comment":               {},
	"pull_request":                {},
	"pull_request_review":         {},
	"pull_request_review_comment": {},
}

// knownGitHubEventsList renders the supported event names
// alphabetically for use in error messages. Cheap; called only on
// validation failures.
func knownGitHubEventsList() string {
	out := make([]string, 0, len(knownGitHubEvents))
	for k := range knownGitHubEvents {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
