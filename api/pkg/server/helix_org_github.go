package server

import (
	"context"
	"errors"
	"fmt"

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
		provider, err := manager.GetProviderByType(types.OAuthProviderTypeGitHub)
		if err != nil {
			// No GitHub OAuth provider configured at the helix level
			// — operators haven't even added one yet. Surface as
			// "no token" so the transport keeps working for the
			// inbound webhook path (which only needs webhook_secret).
			return "", nil
		}
		providerID := provider.GetID()
		members, err := st.ListOrganizationMemberships(ctx, &helixstore.ListOrganizationMembershipsQuery{
			OrganizationID: orgID,
		})
		if err != nil {
			return "", fmt.Errorf("list org memberships: %w", err)
		}
		for _, m := range members {
			conn, err := st.GetOAuthConnectionByUserAndProvider(ctx, m.UserID, providerID)
			if err != nil {
				if errors.Is(err, helixstore.ErrNotFound) {
					continue
				}
				return "", fmt.Errorf("get oauth connection for %s: %w", m.UserID, err)
			}
			if conn.AccessToken != "" {
				return conn.AccessToken, nil
			}
		}
		return "", nil
	}
}
