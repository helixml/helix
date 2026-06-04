package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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
	// `["*"]` is the GitHub wildcard meaning "all events" — that's
	// the default the UI's "send me everything" mode picks.
	// Required and non-empty.
	Events []string `json:"events,omitempty"`

	// WebhookID is the GitHub-side hook id (returned by `POST
	// /repos/{owner}/{repo}/hooks`) once helix has auto-installed
	// the webhook for this stream. Zero when no auto-install has
	// happened yet. Used to surface a deep-link to the webhook's
	// edit page on GitHub and to support future un-install /
	// re-install flows.
	WebhookID int64 `json:"webhook_id,omitempty"`

	// WebhookHTMLURL is the operator-facing GitHub URL of the
	// installed webhook (e.g. https://github.com/<owner>/<name>/settings/hooks/<id>).
	// Captured at install time so the UI can deep-link out without
	// re-calling the GitHub API on every page load.
	WebhookHTMLURL string `json:"webhook_html_url,omitempty"`
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
		// "*" is GitHub's webhook wildcard meaning "all events" — we
		// honour it as a special case (no need to enumerate every
		// known event name). Anything else has to match the slug
		// pattern.
		if ev == "*" {
			continue
		}
		if !githubEventNamePattern.MatchString(ev) {
			return fmt.Errorf("github transport: invalid event %q (must match %s, e.g. issues, pull_request, push, workflow_run, or \"*\" for all events)", ev, githubEventNamePattern.String())
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

// githubEventNamePattern matches a plausible GitHub webhook event
// name: lowercase letters, digits, and underscores, 2-64 chars,
// starting with a letter. We deliberately don't pin a whitelist —
// GitHub adds event types over time (push, release, workflow_run,
// deployment, registry_package, …) and operators can opt in to any of
// them without us shipping a code change. The format check still
// catches the typo case (uppercase, dashes, leading digits) at
// create_stream time. The frontend mirrors this pattern as
// GITHUB_EVENT_PATTERN.
var githubEventNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

// SuggestedGitHubEvents is the curated list of event types the
// frontend offers in the New Stream dialog and that the transport's
// envelope mapping has been hand-tested against. Anything outside
// this list is still accepted by Validate (see
// githubEventNamePattern) — these are just the well-trodden picks.
var SuggestedGitHubEvents = []string{
	"issues",
	"issue_comment",
	"pull_request",
	"pull_request_review",
	"pull_request_review_comment",
}

