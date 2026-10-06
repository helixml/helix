package gitlab

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
)

type WebhookManager interface {
	InstallGitLabWebhook(context.Context, string, string, string, string, string, []string) (int64, string, string, error)
	FindGitLabWebhook(context.Context, string, string, string) (int64, string, bool, bool, error)
}

type WebhookProvisioner struct {
	configs   *configregistry.Registry
	manager   WebhookManager
	publicURL string
}

func NewWebhookProvisioner(configs *configregistry.Registry, manager WebhookManager, publicURL string) *WebhookProvisioner {
	return &WebhookProvisioner{configs: configs, manager: manager, publicURL: publicURL}
}

func (p *WebhookProvisioner) Install(ctx context.Context, orgID string, t trigger.Trigger) (trigger.InstallResult, error) {
	config, err := t.Transport().GitLabConfig()
	if err != nil || config.RepositoryID == "" {
		return trigger.InstallResult{}, gitlabFail(trigger.FailBadRequest, "gitlab topic requires repository_id")
	}
	payloadURL, err := p.payloadURL(orgID, t.ID)
	if err != nil {
		return trigger.InstallResult{}, err
	}
	auth, err := p.ensureAuth(ctx, orgID)
	if err != nil {
		return trigger.InstallResult{}, gitlabFail(trigger.FailInternal, "ensure signing token: %v", err)
	}
	if p.manager == nil {
		return trigger.InstallResult{}, gitlabFail(trigger.FailPrecondition, "GitLab repository service is not configured")
	}
	id, htmlURL, repo, err := p.manager.InstallGitLabWebhook(ctx, orgID, config.RepositoryID, payloadURL, auth.SigningToken, auth.SecretToken, config.Events)
	if err != nil {
		return trigger.InstallResult{}, gitlabFail(trigger.FailUpstream, "create GitLab webhook: %v", err)
	}
	config.Repo, config.WebhookID, config.WebhookHTMLURL = repo, id, htmlURL
	raw, err := json.Marshal(config)
	if err != nil {
		return trigger.InstallResult{}, gitlabFail(trigger.FailInternal, "marshal GitLab config: %v", err)
	}
	return trigger.InstallResult{WebhookID: id, WebhookHTMLURL: htmlURL, PayloadURL: payloadURL, Config: raw}, nil
}

func (p *WebhookProvisioner) Status(ctx context.Context, orgID string, t trigger.Trigger) (trigger.InboundState, error) {
	config, err := t.Transport().GitLabConfig()
	if err != nil || config.RepositoryID == "" {
		return trigger.InboundState{State: "unknown", Detail: "topic has no GitLab repository selected"}, nil
	}
	payloadURL, err := p.payloadURL(orgID, t.ID)
	if err != nil {
		return trigger.InboundState{State: "unknown", Detail: err.Error()}, nil
	}
	if p.manager == nil {
		return trigger.InboundState{State: "unknown", Detail: "GitLab repository service is not configured"}, nil
	}
	id, htmlURL, found, active, err := p.manager.FindGitLabWebhook(ctx, orgID, config.RepositoryID, payloadURL)
	if err != nil {
		return trigger.InboundState{State: "unknown", Detail: err.Error(), PayloadURL: payloadURL}, nil
	}
	if !found {
		return trigger.InboundState{State: "missing", PayloadURL: payloadURL}, nil
	}
	return trigger.InboundState{State: "installed", WebhookID: id, WebhookHTMLURL: htmlURL, Active: active, PayloadURL: payloadURL}, nil
}

func (p *WebhookProvisioner) payloadURL(orgID, triggerID string) (string, error) {
	base := strings.TrimSpace(p.publicURL)
	parsed, err := url.Parse(base)
	if err != nil || parsed.Hostname() == "" {
		return "", gitlabFail(trigger.FailPrecondition, "no public URL configured for Helix")
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "localhost", "127.0.0.1", "0.0.0.0":
		return "", gitlabFail(trigger.FailPrecondition, "Helix public URL %q is not reachable by GitLab", base)
	}
	return strings.TrimRight(base, "/") + "/api/v1/orgs/" + url.PathEscape(orgID) + "/triggers/" + url.PathEscape(string(triggerID)) + "/gitlab/webhook", nil
}

func (p *WebhookProvisioner) ensureAuth(ctx context.Context, orgID string) (Config, error) {
	var config Config
	_ = p.configs.GetObject(ctx, orgID, "transport.gitlab", &config)
	if config.SigningToken != "" && config.SecretToken != "" {
		return config, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return Config{}, err
	}
	if config.SigningToken == "" {
		config.SigningToken = "whsec_" + base64.StdEncoding.EncodeToString(key)
	}
	if config.SecretToken == "" {
		config.SecretToken = base64.RawURLEncoding.EncodeToString(key)
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return Config{}, err
	}
	if err := p.configs.Set(ctx, orgID, "transport.gitlab", string(raw)); err != nil {
		return Config{}, err
	}
	return config, nil
}

func gitlabFail(kind trigger.FailKind, format string, args ...any) *trigger.Failure {
	return &trigger.Failure{Kind: kind, Err: fmt.Errorf(format, args...)}
}

var _ trigger.Inbound = (*WebhookProvisioner)(nil)
var _ = transport.KindGitLab
