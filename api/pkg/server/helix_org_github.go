package server

import (
	"context"
	"errors"
	"fmt"
	"sort"

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
