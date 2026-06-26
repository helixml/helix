package slack

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/credential"
)

// IdentityResolver is the per-org Slack identity lookup the
// credential.Provider depends on. Production wiring
// (api/pkg/server/helix_org.go) resolves the org's connected
// slack_workspace and decrypts its bot token.
//
// Bot tokens (xoxb-) from an OAuth v2 install do not expire unless the
// app enables token rotation, so ExpiresAt is typically the zero Time;
// the provider rejects an empty token so agents get a clear failure
// rather than an opaquely-empty credential.
type IdentityResolver func(ctx context.Context, orgID, teamID string) (Identity, error)

// Identity is the minimal projection of the org's Slack workspace
// install the credential provider needs.
type Identity struct {
	Token     string
	ExpiresAt time.Time
}

// NewCredentialProvider returns a credential.Provider that hands a
// Worker the org's Slack bot token on demand (via mint_credential),
// so the Worker can drive the Slack Web API directly with curl —
// chat.postMessage, reactions.add, files.upload, etc. The transport
// owns ingress + this token; outbound richness is the agent's job.
func NewCredentialProvider(resolver IdentityResolver) credential.Provider {
	return &credentialProvider{resolver: resolver}
}

type credentialProvider struct {
	resolver IdentityResolver
}

func (p *credentialProvider) Name() string { return "slack" }

// Mint resolves the bot token for the workspace named by resource (the
// Slack team_id, from the inbound event's extra.slack_team_id). An empty
// resource falls back to the org's workspace install.
func (p *credentialProvider) Mint(ctx context.Context, orgID, resource string) (credential.Credential, error) {
	if p.resolver == nil {
		return credential.Credential{}, fmt.Errorf("slack credential provider: no identity resolver wired")
	}
	id, err := p.resolver(ctx, orgID, resource)
	if err != nil {
		return credential.Credential{}, fmt.Errorf("resolve slack identity for org %q: %w", orgID, err)
	}
	if id.Token == "" {
		return credential.Credential{}, fmt.Errorf("no slack workspace connected for org %q: install the Helix Slack app into a workspace first", orgID)
	}
	return credential.Credential{
		Token:     id.Token,
		ExpiresAt: id.ExpiresAt,
		Usage:     "export SLACK_BOT_TOKEN=<token>",
	}, nil
}
