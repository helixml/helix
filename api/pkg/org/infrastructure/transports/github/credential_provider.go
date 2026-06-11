package github

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/credential"
)

// IdentityResolver is the per-org identity lookup the GitHub
// credential.Provider depends on. The production wiring
// (api/pkg/server/helix_org.go) projects the same OrgGitHubIdentity
// that newOrgGitHubIdentityResolver already returns.
//
// The returned Identity carries the bot token and the GitHub-reported
// expiry (~1h for an installation token); the OAuth fallback leaves
// ExpiresAt as the zero Time, and the provider rejects an empty token
// case below so agents get a clear failure rather than an opaquely-
// empty credential.
type IdentityResolver func(ctx context.Context, orgID string) (Identity, error)

// Identity is the minimal projection of the org's resolved GitHub
// identity the credential provider needs. Defined here (rather than
// importing api/pkg/server) so the transport stays the boundary owner.
type Identity struct {
	Token     string
	ExpiresAt time.Time
}

// NewCredentialProvider returns a credential.Provider that mints a
// fresh GitHub App installation token on demand. It is registered next
// to NewSecretInjector in helix-org's bootstrap, so adding a sibling
// provider (Slack, …) is a new file in that transport's package plus
// one registration line — no edits here, no edits to the
// mint_credential MCP tool.
func NewCredentialProvider(resolver IdentityResolver) credential.Provider {
	return &credentialProvider{resolver: resolver}
}

type credentialProvider struct {
	resolver IdentityResolver
}

func (p *credentialProvider) Name() string { return "github" }

func (p *credentialProvider) Mint(ctx context.Context, orgID string) (credential.Credential, error) {
	if p.resolver == nil {
		return credential.Credential{}, fmt.Errorf("github credential provider: no identity resolver wired")
	}
	id, err := p.resolver(ctx, orgID)
	if err != nil {
		return credential.Credential{}, fmt.Errorf("resolve github identity for org %q: %w", orgID, err)
	}
	if id.Token == "" {
		return credential.Credential{}, fmt.Errorf("no github identity configured for org %q: install the Helix GitHub App or connect a GitHub OAuth account", orgID)
	}
	return credential.Credential{
		Token:     id.Token,
		ExpiresAt: id.ExpiresAt,
		Usage:     "export GH_TOKEN=<token>",
	}, nil
}
