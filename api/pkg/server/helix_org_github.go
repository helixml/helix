package server

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/oauth"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// newGitHubOAuthResolver builds the GitHubTokenResolver the helix-org
// streams API wires into the github transport. The resolver is what
// makes "reuse the existing GitHub OAuth for Auth" real: instead of
// requiring ops to paste a PAT into transport.github, the transport
// looks up a live OAuth connection owned by a member of the calling
// org and returns its access token.
//
// Resolution path:
//
//  1. Find the global GitHub OAuth provider via the oauth Manager.
//  2. List org_memberships(orgID) to get the candidate users.
//  3. For each member, GetConnection(userID, providerID); return the
//     first non-empty AccessToken.
//
// Returns "" + nil error when no member has connected GitHub yet —
// the transport's downstream consumers treat that as "no token
// configured, degrade gracefully" (e.g. log + skip outbound action)
// rather than crashing on a missing token.
//
// Returning a closure (rather than a method on some struct) keeps the
// dependency edge from api/pkg/org back to api/pkg/server zero: the
// org package only sees the func type and never imports oauth/store.
//
// nil manager OR nil store disables the resolver — the api Deps treats
// nil GitHubTokenResolver as "no fallback", which gives the same
// degraded behaviour without crashing wiring that wants to opt out.
func newGitHubOAuthResolver(manager *oauth.Manager, st helixstore.Store) func(context.Context, string) (string, error) {
	if manager == nil || st == nil {
		return nil
	}
	return func(ctx context.Context, orgID string) (string, error) {
		// Multiple github OAuth providers can exist in
		// `o_auth_providers` (e.g. an org rotated to an org-owned
		// GitHub OAuth app to scope private-repo access). The
		// in-memory manager's `GetProviderByType` returns ONE
		// provider via map iteration (non-deterministic order), so
		// using it for token lookup can pick a stale row whose
		// `client_id` no longer has a matching connection. Walk
		// every github provider in the store instead and return
		// the first valid token.
		providers, err := st.ListOAuthProviders(ctx, &helixstore.ListOAuthProvidersQuery{
			Type:    string(types.OAuthProviderTypeGitHub),
			Enabled: true,
		})
		if err != nil {
			return "", fmt.Errorf("list github providers: %w", err)
		}
		// Newest-first so an operator who just added an org-owned
		// OAuth app (e.g. swapping to one with helixml-org access)
		// gets that provider's tokens picked up immediately,
		// instead of falling back to a stale legacy provider.
		sort.SliceStable(providers, func(i, j int) bool {
			return providers[i].CreatedAt.After(providers[j].CreatedAt)
		})
		if len(providers) == 0 {
			// No GitHub OAuth provider configured at the helix
			// level. Surface as "no token" so the transport keeps
			// working for the inbound webhook path (which only
			// needs webhook_secret).
			_ = manager // keep import live in case future logic needs the in-memory cache
			return "", nil
		}
		members, err := st.ListOrganizationMemberships(ctx, &helixstore.ListOrganizationMembershipsQuery{
			OrganizationID: orgID,
		})
		if err != nil {
			return "", fmt.Errorf("list org memberships: %w", err)
		}
		// For each provider try every org member's connection; pick
		// the first non-empty access_token we find. Provider order
		// is whatever the store returns (DB-side ordering), so
		// duplicate providers from earlier botched setups don't
		// blackhole the lookup.
		for _, p := range providers {
			for _, m := range members {
				conn, err := st.GetOAuthConnectionByUserAndProvider(ctx, m.UserID, p.ID)
				if err != nil {
					if errors.Is(err, helixstore.ErrNotFound) {
						continue
					}
					return "", fmt.Errorf("get oauth connection for %s on %s: %w", m.UserID, p.ID, err)
				}
				if conn.AccessToken != "" {
					return conn.AccessToken, nil
				}
			}
		}
		return "", nil
	}
}

// OrgGitHubIdentity is the resolved GitHub identity for an org: either the
// installed Helix App bot (Mode="app") or a borrowed member OAuth token
// (Mode="oauth", the legacy fallback). Token is the value to inject as
// GH_TOKEN / use as the git credential. AppID, InstallationID and BaseURL
// are populated in "app" mode so callers (e.g. the repo picker) can drive
// installation-scoped GitHub API calls.
type OrgGitHubIdentity struct {
	Mode           string // "app" | "oauth"
	Token          string
	AppID          int64
	InstallationID int64
	BaseURL        string
}

// newOrgGitHubIdentityResolver returns the org's GitHub identity, preferring
// the installed Helix App over a borrowed human OAuth token. If a github_app
// ServiceConnection exists for the org, it decrypts the stored PEM and mints
// a short-lived installation access token (Mode="app"); otherwise it falls
// back to the legacy member-OAuth walk (Mode="oauth").
//
// Any app-side failure (no encryption key, decrypt error, mint error) logs
// and falls through to OAuth — a broken app config must never break an org
// that OAuth could still serve. Returns Mode="oauth" with an empty Token when
// neither path yields anything (identical to the pre-app behaviour).
//
// mintFn is a seam: production wiring passes github.MintInstallationToken;
// unit tests stub it so they never make the live GitHub call ghinstallation's
// Token() performs.
func newOrgGitHubIdentityResolver(
	getKey func() ([]byte, error),
	st helixstore.Store,
	oauthFallback func(context.Context, string) (string, error),
	mintFn func(ctx context.Context, appID, installationID int64, pem, baseURL string) (string, error),
) func(context.Context, string) (OrgGitHubIdentity, error) {
	oauth := func(ctx context.Context, orgID string) (OrgGitHubIdentity, error) {
		if oauthFallback == nil {
			return OrgGitHubIdentity{Mode: "oauth"}, nil
		}
		tok, err := oauthFallback(ctx, orgID)
		if err != nil {
			return OrgGitHubIdentity{}, err
		}
		return OrgGitHubIdentity{Mode: "oauth", Token: tok}, nil
	}

	return func(ctx context.Context, orgID string) (OrgGitHubIdentity, error) {
		if st == nil || mintFn == nil {
			return oauth(ctx, orgID)
		}
		conns, err := st.ListServiceConnectionsByType(ctx, orgID, types.ServiceConnectionTypeGitHubApp)
		if err != nil {
			log.Warn().Err(err).Str("org_id", orgID).Msg("list github_app service connections failed; falling back to OAuth")
			return oauth(ctx, orgID)
		}
		// The store returns rows created_at DESC, so the first usable
		// connection is the newest — mirrors the OAuth resolver's
		// newest-first preference when an operator rotates apps.
		for _, c := range conns {
			if c == nil || c.GitHubAppID == 0 || c.GitHubInstallationID == 0 || c.GitHubPrivateKey == "" {
				continue
			}
			key, err := getKey()
			if err != nil {
				log.Warn().Err(err).Str("org_id", orgID).Msg("get encryption key failed; falling back to OAuth")
				break
			}
			pem, err := crypto.DecryptAES256GCM(c.GitHubPrivateKey, key)
			if err != nil {
				log.Warn().Err(err).Str("org_id", orgID).Str("conn_id", c.ID).Msg("decrypt github app key failed; trying next connection")
				continue
			}
			tok, err := mintFn(ctx, c.GitHubAppID, c.GitHubInstallationID, string(pem), c.BaseURL)
			if err != nil {
				log.Warn().Err(err).Str("org_id", orgID).Int64("app_id", c.GitHubAppID).Msg("mint installation token failed; falling back to OAuth")
				break
			}
			return OrgGitHubIdentity{
				Mode:           "app",
				Token:          tok,
				AppID:          c.GitHubAppID,
				InstallationID: c.GitHubInstallationID,
				BaseURL:        c.BaseURL,
			}, nil
		}
		return oauth(ctx, orgID)
	}
}
