package github

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	githubclient "github.com/helixml/helix/api/pkg/github"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// WebhookProvisioner is the GitHub implementation of
// streaming.Inbound: it registers / inspects the repo webhook
// for a github-transport Topic. It is the single home for the
// GitHub-API specifics (payload-URL resolution, loopback refusal, the
// auto-generated webhook secret, the repo webhook upsert/find) — the
// application layer only dispatches on the Topic's transport Kind, so
// adding Slack later is a sibling provisioner, not a new app service.
type WebhookProvisioner struct {
	configs   *configregistry.Registry
	token     TokenResolver
	publicURL string
}

// NewWebhookProvisioner builds the provisioner. configs backs the
// topics.public_url override + the per-org webhook secret; token
// resolves the org's GitHub credential; publicServerURL is the fallback
// external base URL.
func NewWebhookProvisioner(configs *configregistry.Registry, token TokenResolver, publicServerURL string) *WebhookProvisioner {
	return &WebhookProvisioner{configs: configs, token: token, publicURL: publicServerURL}
}

func fail(kind streaming.FailKind, format string, args ...any) *streaming.Failure {
	return &streaming.Failure{Kind: kind, Err: fmt.Errorf(format, args...)}
}

// Install registers (or adopts) the repo webhook for the github topic
// and returns the hook coordinates + the transport config to persist.
func (p *WebhookProvisioner) Install(ctx context.Context, orgID string, topic streaming.Topic) (streaming.InstallResult, error) {
	publicURL := p.resolvePublicURL(ctx, orgID)
	if publicURL == "" {
		return streaming.InstallResult{}, fail(streaming.FailPrecondition, "no public URL configured for helix. Set `topics.public_url` on the helix-org Settings page (or SERVER_URL in helix's .env), then re-install the webhook.")
	}
	if u, err := url.Parse(publicURL); err == nil {
		host := strings.ToLower(u.Hostname())
		if host == "localhost" || host == "127.0.0.1" || host == "0.0.0.0" {
			return streaming.InstallResult{}, fail(streaming.FailPrecondition, "public URL %q is a loopback address — GitHub refuses to install webhooks pointed at unreachable hosts. Set `topics.public_url` on the helix-org Settings page to a publicly reachable hostname (cloudflared / ngrok / reverse proxy), or update SERVER_URL in helix's .env and restart the api container", publicURL)
		}
	}

	cfg, err := topic.Transport.GitHubConfig()
	if err != nil {
		return streaming.InstallResult{}, fail(streaming.FailBadRequest, "parse github config: %w", err)
	}
	if cfg.Repo == "" {
		return streaming.InstallResult{}, fail(streaming.FailBadRequest, "topic's github config has no repo set; edit the topic first")
	}
	if len(cfg.Events) == 0 {
		cfg.Events = []string{"*"}
	}
	owner, repoName, ferr := splitRepo(cfg.Repo)
	if ferr != nil {
		return streaming.InstallResult{}, ferr
	}

	token, ferr := p.resolveToken(ctx, orgID)
	if ferr != nil {
		return streaming.InstallResult{}, ferr
	}
	secret, err := p.ensureWebhookSecret(ctx, orgID)
	if err != nil {
		return streaming.InstallResult{}, fail(streaming.FailInternal, "ensure webhook secret: %w", err)
	}
	payloadURL := p.payloadURL(publicURL, orgID, topic.ID)

	client, err := githubclient.NewGithubClient(githubclient.ClientOptions{Ctx: ctx, Token: token})
	if err != nil {
		return streaming.InstallResult{}, fail(streaming.FailInternal, "build github client: %w", err)
	}
	hook, err := client.UpsertWebhook(owner, repoName, "web", payloadURL, cfg.Events, secret)
	if err != nil {
		return streaming.InstallResult{}, fail(streaming.FailUpstream, "create github webhook: %w", err)
	}
	htmlURL := githubclient.WebhookSettingsURL(owner, repoName, hook.ID)
	cfg.WebhookID = hook.ID
	cfg.WebhookHTMLURL = htmlURL
	cfgRaw, err := json.Marshal(cfg)
	if err != nil {
		return streaming.InstallResult{}, fail(streaming.FailInternal, "re-marshal config: %w", err)
	}
	return streaming.InstallResult{
		WebhookID:      hook.ID,
		WebhookHTMLURL: htmlURL,
		PayloadURL:     payloadURL,
		Config:         cfgRaw,
	}, nil
}

// Status reports the live webhook state by listing the repo's hooks and
// matching this topic's payload URL. Read-only; every "can't tell" case
// degrades to State="unknown" with a Detail.
func (p *WebhookProvisioner) Status(ctx context.Context, orgID string, topic streaming.Topic) (streaming.InboundState, error) {
	cfg, err := topic.Transport.GitHubConfig()
	if err != nil {
		return streaming.InboundState{}, fail(streaming.FailBadRequest, "parse github config: %w", err)
	}
	unknown := func(detail string) (streaming.InboundState, error) {
		return streaming.InboundState{State: "unknown", Detail: detail}, nil
	}
	if cfg.Repo == "" {
		return unknown("topic has no repo set")
	}
	owner, repoName, ferr := splitRepo(cfg.Repo)
	if ferr != nil {
		return unknown(fmt.Sprintf("malformed repo %q", cfg.Repo))
	}
	publicURL := p.resolvePublicURL(ctx, orgID)
	if publicURL == "" {
		return unknown("no public URL configured for helix")
	}
	payloadURL := p.payloadURL(publicURL, orgID, topic.ID)
	if p.token == nil {
		return unknown("no GitHubTokenResolver wired")
	}
	token, err := p.token(ctx, orgID)
	if err != nil {
		return unknown(fmt.Sprintf("resolve github token: %v", err))
	}
	if token == "" {
		return unknown("no GitHub credentials for this org (install the Helix GitHub App or connect GitHub OAuth)")
	}
	client, err := githubclient.NewGithubClient(githubclient.ClientOptions{Ctx: ctx, Token: token})
	if err != nil {
		return unknown(fmt.Sprintf("build github client: %v", err))
	}
	hook, found, err := client.FindWebhook(owner, repoName, payloadURL)
	if err != nil {
		return unknown(fmt.Sprintf("list webhooks on %s: %v", cfg.Repo, err))
	}
	if !found {
		return streaming.InboundState{State: "missing", PayloadURL: payloadURL}, nil
	}
	return streaming.InboundState{
		State:          "installed",
		WebhookID:      hook.ID,
		WebhookHTMLURL: githubclient.WebhookSettingsURL(owner, repoName, hook.ID),
		Active:         hook.Active,
		PayloadURL:     payloadURL,
	}, nil
}

func (p *WebhookProvisioner) resolveToken(ctx context.Context, orgID string) (string, *streaming.Failure) {
	if p.token == nil {
		return "", fail(streaming.FailPrecondition, "no GitHubTokenResolver wired")
	}
	token, err := p.token(ctx, orgID)
	if err != nil {
		return "", fail(streaming.FailInternal, "resolve github token: %w", err)
	}
	if token == "" {
		return "", fail(streaming.FailPrecondition, "no GitHub credentials for this org: install the Helix GitHub App (preferred) or connect GitHub OAuth on the Connected Services page")
	}
	return token, nil
}

// resolvePublicURL: `topics.public_url` org config wins; publicServerURL
// is the fallback.
func (p *WebhookProvisioner) resolvePublicURL(ctx context.Context, orgID string) string {
	publicURL := strings.TrimSpace(p.publicURL)
	if p.configs != nil {
		if override, err := p.configs.GetString(ctx, orgID, "topics.public_url"); err == nil && strings.TrimSpace(override) != "" {
			publicURL = strings.TrimSpace(override)
		}
	}
	return publicURL
}

func (p *WebhookProvisioner) payloadURL(publicURL, orgID string, topicID streaming.TopicID) string {
	return strings.TrimRight(publicURL, "/") +
		"/api/v1/orgs/" + url.PathEscape(orgID) +
		"/topics/" + url.PathEscape(string(topicID)) + "/github/webhook"
}

// ensureWebhookSecret reads transport.github webhook_secret, generating +
// persisting a 32-byte hex secret when unset so future installs and the
// inbound HMAC verifier share one value.
func (p *WebhookProvisioner) ensureWebhookSecret(ctx context.Context, orgID string) (string, error) {
	if p.configs == nil {
		return "", fmt.Errorf("config registry not wired")
	}
	var cfg struct {
		Token         string `json:"token,omitempty"`
		WebhookSecret string `json:"webhook_secret,omitempty"`
	}
	_ = p.configs.GetObject(ctx, orgID, "transport.github", &cfg)
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
	if err := p.configs.Set(ctx, orgID, "transport.github", string(out)); err != nil {
		return "", fmt.Errorf("persist secret: %w", err)
	}
	return cfg.WebhookSecret, nil
}

func splitRepo(repo string) (owner, name string, ferr *streaming.Failure) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return "", "", fail(streaming.FailBadRequest, "malformed repo %q", repo)
	}
	return parts[0], parts[1], nil
}

// compile-time assertion that the provisioner satisfies the port.
var _ streaming.Inbound = (*WebhookProvisioner)(nil)

// guard against transport.KindGitHub drifting — referenced so the import
// is used even if the provisioner is registered by string elsewhere.
var _ = transport.KindGitHub
