// Package githubwebhook owns the GitHub-webhook integration use cases
// behind the two REST endpoints POST /streams/{id}/github/install-webhook
// and GET /streams/{id}/github/webhook-status. The orchestration —
// resolve the public payload URL, validate it, resolve the org's GitHub
// token, ensure the per-org webhook secret, call GitHub to upsert / find
// the hook, and persist the hook coordinates back onto the stream's
// transport config — lived inline in the api adapter; it is genuine
// business logic with decisions, so it belongs in an application service.
//
// The GitHub API surface is a narrow Client port so this package never
// imports the concrete github client; the composition root provides the
// adapter. Reads go through the Queries facade, the persist-back goes
// through the streams service, and the secret/public-url config goes
// through the config registry — never the store directly.
package githubwebhook

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/queries"
	"github.com/helixml/helix/api/pkg/org/application/streams"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// Hook is the subset of a GitHub repo webhook this package cares about.
type Hook struct {
	ID      int64
	HTMLURL string
	Active  bool
}

// Client is the narrow GitHub-webhook API the service depends on. The
// composition root supplies an adapter over the concrete github client;
// keeping it a port keeps this application package free of that import
// and makes the service testable with a fake.
type Client interface {
	// Upsert registers (or adopts an existing) "web" webhook on
	// owner/repo pointing at payloadURL for the given events, HMAC-signed
	// with secret. Returns the resulting hook.
	Upsert(ctx context.Context, token, owner, repo, payloadURL string, events []string, secret string) (Hook, error)
	// Find returns the webhook on owner/repo whose config URL matches
	// payloadURL, with found=false when none matches.
	Find(ctx context.Context, token, owner, repo, payloadURL string) (hook Hook, found bool, err error)
}

// TokenResolver returns the GitHub token to act as for an org (the
// installed Helix App bot, or a borrowed member OAuth token). Empty
// token means "no GitHub credentials for this org".
type TokenResolver func(ctx context.Context, orgID string) (string, error)

// FailKind categorises an Install failure so the REST adapter can map it
// to the right HTTP status without string-matching.
type FailKind int

const (
	// FailBadRequest — malformed input / wrong transport (400).
	FailBadRequest FailKind = iota
	// FailPrecondition — a precondition isn't met yet: no public URL,
	// loopback URL, no token/credentials (412).
	FailPrecondition
	// FailUpstream — GitHub itself rejected or errored (502).
	FailUpstream
	// FailInternal — our side failed (secret persist, marshal) (500).
	FailInternal
	// FailNotFound — the stream doesn't exist (404).
	FailNotFound
)

// Failure carries the failure category alongside the operator-facing
// message. Install returns it; the adapter switches on Kind.
type Failure struct {
	Kind FailKind
	Err  error
}

func (f *Failure) Error() string { return f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

func fail(kind FailKind, format string, args ...any) *Failure {
	return &Failure{Kind: kind, Err: fmt.Errorf(format, args...)}
}

// Service owns the install + status use cases.
type Service struct {
	queries   *queries.Queries
	streams   *streams.Streams
	configs   *configregistry.Registry
	token     TokenResolver
	client    Client
	publicURL string
}

// Deps are the constructor-injected collaborators. PublicServerURL is the
// operator-configured external base URL the webhook payload posts back to.
type Deps struct {
	Queries         *queries.Queries
	Streams         *streams.Streams
	Configs         *configregistry.Registry
	Token           TokenResolver
	Client          Client
	PublicServerURL string
}

// New constructs the service.
func New(deps Deps) *Service {
	return &Service{
		queries:   deps.Queries,
		streams:   deps.Streams,
		configs:   deps.Configs,
		token:     deps.Token,
		client:    deps.Client,
		publicURL: deps.PublicServerURL,
	}
}

// InstallResult is what a successful install returns.
type InstallResult struct {
	WebhookID      int64
	WebhookHTMLURL string
	PayloadURL     string
}

// Install registers (or adopts) the repo webhook for a github stream and
// persists the hook coordinates onto the stream's transport config.
// Returns a *Failure (switch on Kind) on every failure mode.
func (s *Service) Install(ctx context.Context, orgID string, streamID streaming.StreamID) (InstallResult, error) {
	// Resolve + validate the public payload URL. A loopback URL is a hard
	// refusal here (GitHub refuses unreachable hosts), unlike the inbound
	// warning-only path.
	publicURL := s.resolvePublicURL(ctx, orgID)
	if publicURL == "" {
		return InstallResult{}, fail(FailPrecondition, "no public URL configured for helix. Set `streams.public_url` on the helix-org Settings page (or SERVER_URL in helix's .env), then re-install the webhook.")
	}
	if u, err := url.Parse(publicURL); err == nil {
		host := strings.ToLower(u.Hostname())
		if host == "localhost" || host == "127.0.0.1" || host == "0.0.0.0" {
			return InstallResult{}, fail(FailPrecondition, "public URL %q is a loopback address — GitHub refuses to install webhooks pointed at unreachable hosts. Set `streams.public_url` on the helix-org Settings page to a publicly reachable hostname (cloudflared / ngrok / reverse proxy), or update SERVER_URL in helix's .env and restart the api container", publicURL)
		}
	}

	cfg, name, description, ferr := s.githubStream(ctx, orgID, streamID)
	if ferr != nil {
		return InstallResult{}, ferr
	}
	if cfg.Repo == "" {
		return InstallResult{}, fail(FailBadRequest, "stream's github config has no repo set; edit the stream first")
	}
	if len(cfg.Events) == 0 {
		cfg.Events = []string{"*"}
	}

	token, ferr := s.resolveToken(ctx, orgID)
	if ferr != nil {
		return InstallResult{}, ferr
	}
	secret, err := s.ensureWebhookSecret(ctx, orgID)
	if err != nil {
		return InstallResult{}, fail(FailInternal, "ensure webhook secret: %w", err)
	}
	owner, repoName, ferr := splitRepo(cfg.Repo)
	if ferr != nil {
		return InstallResult{}, ferr
	}
	payloadURL := s.payloadURL(publicURL, orgID, streamID)

	hook, err := s.client.Upsert(ctx, token, owner, repoName, payloadURL, cfg.Events, secret)
	if err != nil {
		return InstallResult{}, fail(FailUpstream, "create github webhook: %w", err)
	}

	cfg.WebhookID = hook.ID
	cfg.WebhookHTMLURL = hook.HTMLURL
	cfgRaw, err := json.Marshal(cfg)
	if err != nil {
		return InstallResult{}, fail(FailInternal, "re-marshal config: %w", err)
	}
	// Persist the hook id/url back onto the stream's transport config via
	// the streams service (a transport-config patch that leaves
	// name/description/kind untouched).
	if _, err := s.streams.Update(ctx, orgID, streamID, streams.UpdateParams{
		Name:        name,
		Description: description,
		Transport:   &streams.TransportPatch{Config: cfgRaw},
	}); err != nil {
		return InstallResult{}, fail(FailInternal, "update stream after webhook install: %w", err)
	}
	return InstallResult{WebhookID: hook.ID, WebhookHTMLURL: hook.HTMLURL, PayloadURL: payloadURL}, nil
}

// StatusResult reports the LIVE state of a github stream's repo webhook.
// State is "installed" | "missing" | "unknown"; Detail explains "unknown".
type StatusResult struct {
	State          string
	WebhookID      int64
	WebhookHTMLURL string
	Active         bool
	PayloadURL     string
	Detail         string
}

// Status reports the live webhook state. It is read-only and never
// returns an error for a "can't tell" condition — those degrade to
// State="unknown" with a Detail so the UI can fall back to stored config.
// A genuine bad-input (not a github stream, malformed config) or missing
// stream returns a *Failure.
func (s *Service) Status(ctx context.Context, orgID string, streamID streaming.StreamID) (StatusResult, error) {
	cfg, _, _, ferr := s.githubStream(ctx, orgID, streamID)
	if ferr != nil {
		return StatusResult{}, ferr
	}
	if cfg.Repo == "" {
		return StatusResult{State: "unknown", Detail: "stream has no repo set"}, nil
	}
	owner, repoName, ferr := splitRepo(cfg.Repo)
	if ferr != nil {
		return StatusResult{State: "unknown", Detail: ferr.Error()}, nil
	}
	publicURL := s.resolvePublicURL(ctx, orgID)
	if publicURL == "" {
		return StatusResult{State: "unknown", Detail: "no public URL configured for helix"}, nil
	}
	payloadURL := s.payloadURL(publicURL, orgID, streamID)

	if s.token == nil {
		return StatusResult{State: "unknown", Detail: "no GitHubTokenResolver wired"}, nil
	}
	token, err := s.token(ctx, orgID)
	if err != nil {
		return StatusResult{State: "unknown", Detail: fmt.Sprintf("resolve github token: %v", err)}, nil
	}
	if token == "" {
		return StatusResult{State: "unknown", Detail: "no GitHub credentials for this org (install the Helix GitHub App or connect GitHub OAuth)"}, nil
	}
	hook, found, err := s.client.Find(ctx, token, owner, repoName, payloadURL)
	if err != nil {
		return StatusResult{State: "unknown", Detail: fmt.Sprintf("list webhooks on %s: %v", cfg.Repo, err)}, nil
	}
	if !found {
		return StatusResult{State: "missing", PayloadURL: payloadURL}, nil
	}
	return StatusResult{
		State:          "installed",
		WebhookID:      hook.ID,
		WebhookHTMLURL: hook.HTMLURL,
		Active:         hook.Active,
		PayloadURL:     payloadURL,
	}, nil
}

// githubStream loads the stream, confirms it is a github transport, and
// parses its github config. Returns the config plus the stream's
// name/description (Install needs them for the persist-back patch).
func (s *Service) githubStream(ctx context.Context, orgID string, streamID streaming.StreamID) (transport.GitHubConfig, string, string, *Failure) {
	st, err := s.queries.GetStream(ctx, orgID, streamID)
	if err != nil {
		return transport.GitHubConfig{}, "", "", fail(FailNotFound, "get stream %s: %w", streamID, err)
	}
	if st.Transport.Kind != transport.KindGitHub {
		return transport.GitHubConfig{}, "", "", fail(FailBadRequest, "stream %s is not a github transport (kind=%s)", streamID, st.Transport.Kind)
	}
	cfg, err := st.Transport.GitHubConfig()
	if err != nil {
		return transport.GitHubConfig{}, "", "", fail(FailBadRequest, "parse github config: %w", err)
	}
	// Empty repo is left for the caller to judge: Install rejects it
	// (400), Status tolerates it (degrades to "unknown").
	return cfg, st.Name, st.Description, nil
}

func (s *Service) resolveToken(ctx context.Context, orgID string) (string, *Failure) {
	if s.token == nil {
		return "", fail(FailPrecondition, "no GitHubTokenResolver wired")
	}
	token, err := s.token(ctx, orgID)
	if err != nil {
		return "", fail(FailInternal, "resolve github token: %w", err)
	}
	if token == "" {
		return "", fail(FailPrecondition, "no GitHub credentials for this org: install the Helix GitHub App (preferred) or connect GitHub OAuth on the Connected Services page")
	}
	return token, nil
}

// resolvePublicURL returns the base URL for the webhook payload URL:
// `streams.public_url` org config wins; PublicServerURL is the fallback.
func (s *Service) resolvePublicURL(ctx context.Context, orgID string) string {
	publicURL := strings.TrimSpace(s.publicURL)
	if s.configs != nil {
		if override, err := s.configs.GetString(ctx, orgID, "streams.public_url"); err == nil && strings.TrimSpace(override) != "" {
			publicURL = strings.TrimSpace(override)
		}
	}
	return publicURL
}

func (s *Service) payloadURL(publicURL, orgID string, streamID streaming.StreamID) string {
	return strings.TrimRight(publicURL, "/") +
		"/api/v1/orgs/" + url.PathEscape(orgID) +
		"/streams/" + url.PathEscape(string(streamID)) + "/github/webhook"
}

// ensureWebhookSecret reads the org's transport.github webhook_secret,
// generating + persisting a 32-byte hex secret when unset so future
// installs and the inbound HMAC verifier share one value.
func (s *Service) ensureWebhookSecret(ctx context.Context, orgID string) (string, error) {
	if s.configs == nil {
		return "", errors.New("config registry not wired")
	}
	var cfg struct {
		Token         string `json:"token,omitempty"`
		WebhookSecret string `json:"webhook_secret,omitempty"`
	}
	_ = s.configs.GetObject(ctx, orgID, "transport.github", &cfg)
	if cfg.WebhookSecret != "" {
		return cfg.WebhookSecret, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	cfg.WebhookSecret = hex.EncodeToString(buf)
	out, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	// Persist as the system owner — secret bootstrap is helix self-care.
	if err := s.configs.Set(ctx, orgID, "transport.github", string(out), orgchart.WorkerID("w-owner")); err != nil {
		return "", fmt.Errorf("persist secret: %w", err)
	}
	return cfg.WebhookSecret, nil
}

func splitRepo(repo string) (owner, name string, ferr *Failure) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return "", "", fail(FailBadRequest, "malformed repo %q", repo)
	}
	return parts[0], parts[1], nil
}
