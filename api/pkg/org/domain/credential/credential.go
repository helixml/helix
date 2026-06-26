// Package credential defines the small Provider interface every
// per-provider credential minter implements, and the Credential value
// type the mint_credential MCP tool returns to callers.
//
// Concrete Provider implementations live next to the transport that
// owns the provider (e.g. the GitHub provider in
// api/pkg/org/infrastructure/transports/github/credential_provider.go).
// The mint_credential tool dispatches across a name-keyed registry of
// Providers wired in helix-org's server bootstrap; adding a new
// provider is a new file + one registration line, with no edits to the
// tool itself. This is the same shape as the transport.Strategy
// registry — see api/pkg/org/domain/transport/transport.go.
//
// Tenant scoping is enforced by the caller: Mint takes the orgID
// resolved from the calling Worker's identity, never from the
// agent-supplied tool args. See application/tools/mint_credential.go.
package credential

import (
	"context"
	"time"
)

// Provider mints a short-lived credential for one external provider,
// scoped to a single organization. Implementations are expected to be
// safe for concurrent use.
type Provider interface {
	// Name is the stable identifier the mint_credential tool dispatches
	// on (e.g. "github", "slack"). Must be unique across registered
	// providers.
	Name() string

	// Mint returns a fresh credential for orgID. resource is an optional,
	// provider-specific scope the agent passes to disambiguate which
	// identity to mint when an org has several (e.g. for slack, the
	// workspace team_id from the inbound event's extra.slack_team_id);
	// empty means "the org's default/only identity". Implementations
	// should return an error (not an empty Credential) when no identity
	// is configured so callers can surface a clear failure.
	Mint(ctx context.Context, orgID, resource string) (Credential, error)
}

// Credential is the value returned by Provider.Mint. The mint_credential
// MCP tool marshals it as JSON for the calling agent.
type Credential struct {
	// Token is the raw credential value (e.g. a GitHub App installation
	// access token). The agent is expected to export it into its shell
	// immediately before running provider-authenticated commands.
	Token string

	// ExpiresAt is the wall-clock UTC time after which Token will be
	// rejected by the provider. Agents use this to decide when to mint
	// again proactively; the canonical recovery path is still
	// re-mint-on-401/403.
	ExpiresAt time.Time

	// Usage is a short human-readable hint shown to the agent, typically
	// the exact shell command to make Token effective
	// (e.g. "export GH_TOKEN=<token>"). Optional but recommended.
	Usage string
}
