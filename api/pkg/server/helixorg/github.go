package helixorg

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/go-github/v61/github"
	"github.com/rs/zerolog/log"

	githubskill "github.com/helixml/helix/api/pkg/agent/skill/github"
	"github.com/helixml/helix/api/pkg/crypto"
	githubclient "github.com/helixml/helix/api/pkg/github"
	"github.com/helixml/helix/api/pkg/oauth"
	helixorgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// NewGitHubOAuthResolver builds the GitHubTokenResolver the helix-org
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
func NewGitHubOAuthResolver(manager *oauth.Manager, st helixstore.Store) func(context.Context, string) (string, error) {
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
//
// ExpiresAt is populated for Mode="app" — GitHub App installation tokens
// have a server-reported expiry (~1h) that the mint_credential MCP tool
// surfaces to the calling agent. It is the zero Time for Mode="oauth"
// because borrowed user OAuth tokens have no comparable per-mint expiry
// here.
type OrgGitHubIdentity struct {
	Mode           string // "app" | "oauth"
	Token          string
	ExpiresAt      time.Time
	AppID          int64
	InstallationID int64
	BaseURL        string
}

// NewOrgGitHubIdentityResolver returns the org's GitHub identity, preferring
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
// mintFn is a seam: production wiring passes
// github.MintInstallationCredential; unit tests stub it so they never make
// the live GitHub call ghinstallation's Token() performs. The returned
// expiry travels onto OrgGitHubIdentity.ExpiresAt so the mint_credential
// MCP tool can pass it back to the agent.
// decryptAppKey decrypts a github_app ServiceConnection's stored PEM with the
// server encryption key. Shared by the install-status, repo-aggregation and
// identity resolvers so the decrypt + error wrapping lives in one place.
func decryptAppKey(getKey func() ([]byte, error), conn *types.ServiceConnection) (string, error) {
	if conn == nil || conn.GitHubPrivateKey == "" {
		return "", fmt.Errorf("github app connection has no stored private key")
	}
	key, err := getKey()
	if err != nil {
		return "", fmt.Errorf("get encryption key: %w", err)
	}
	pem, err := crypto.DecryptAES256GCM(conn.GitHubPrivateKey, key)
	if err != nil {
		return "", fmt.Errorf("decrypt github app key: %w", err)
	}
	return string(pem), nil
}

// MintedInstallation is the bundle returned by the mint-fn seam: the
// installation access token and its server-reported expiry. Kept in this
// package so test seams don't have to depend on api/pkg/agent/skill/github.
type MintedInstallation struct {
	Token     string
	ExpiresAt time.Time
}

func NewOrgGitHubIdentityResolver(
	getKey func() ([]byte, error),
	st helixstore.Store,
	oauthFallback func(context.Context, string) (string, error),
	mintFn func(ctx context.Context, appID, installationID int64, pem, baseURL string) (MintedInstallation, error),
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
			pem, err := decryptAppKey(getKey, c)
			if err != nil {
				log.Warn().Err(err).Str("org_id", orgID).Str("conn_id", c.ID).Msg("decrypt github app key failed; trying next connection")
				continue
			}
			minted, err := mintFn(ctx, c.GitHubAppID, c.GitHubInstallationID, pem, c.BaseURL)
			if err != nil {
				log.Warn().Err(err).Str("org_id", orgID).Int64("app_id", c.GitHubAppID).Msg("mint installation token failed; falling back to OAuth")
				break
			}
			return OrgGitHubIdentity{
				Mode:           "app",
				Token:          minted.Token,
				ExpiresAt:      minted.ExpiresAt,
				AppID:          c.GitHubAppID,
				InstallationID: c.GitHubInstallationID,
				BaseURL:        c.BaseURL,
			}, nil
		}
		return oauth(ctx, orgID)
	}
}

// ServiceConnections is the narrow slice of the host store the GitHub
// integration needs — small enough to fake in a unit test.
type ServiceConnections interface {
	ListServiceConnectionsByType(ctx context.Context, organizationID string, connType types.ServiceConnectionType) ([]*types.ServiceConnection, error)
	UpdateServiceConnection(ctx context.Context, connection *types.ServiceConnection) error
	DeleteServiceConnection(ctx context.Context, id string) error
}

// GitHubIntegration owns the host-side GitHub-App reads the helix-org
// REST API needs: the New Stream "Install Helix" gate (InstallationStatus)
// and the repo picker (AppRepos). Lifted verbatim out of the inline
// closures that used to live in initHelixOrgHandler — that function is a
// composition root, not a home for GitHub-integration logic.
//
// The GitHub-call seams (decrypt / listInstalls / mintToken /
// installRepos) are struct fields so the install-status sync (stale-conn
// delete, installation-id + owner backfill) is unit-testable against a
// fake store without hitting GitHub or real crypto.
type GitHubIntegration struct {
	conns   ServiceConnections
	decrypt func(*types.ServiceConnection) (string, error)
	appSlug string // operator's BYO/pre-existing app slug fallback
	webURL  string // GitHub web base URL for install/manage links

	listInstalls func(ctx context.Context, appID int64, privateKey, baseURL string) ([]*github.Installation, error)
	mintToken    func(ctx context.Context, appID, installationID int64, privateKey, baseURL string) (string, error)
	installRepos func(ctx context.Context, token, baseURL string) ([]string, error)
}

// NewGitHubIntegration wires the production seams: decrypt via the
// server encryption key, and the real go-github / skill calls.
func NewGitHubIntegration(conns ServiceConnections, getKey func() ([]byte, error), appSlug, webURL string) *GitHubIntegration {
	return &GitHubIntegration{
		conns:        conns,
		decrypt:      func(c *types.ServiceConnection) (string, error) { return decryptAppKey(getKey, c) },
		appSlug:      appSlug,
		webURL:       webURL,
		listInstalls: githubclient.ListAppInstallations,
		mintToken:    githubskill.MintInstallationToken,
		installRepos: func(ctx context.Context, token, baseURL string) ([]string, error) {
			client, err := githubclient.NewGithubClient(githubclient.ClientOptions{Ctx: ctx, Token: token, BaseURL: baseURL})
			if err != nil {
				return nil, err
			}
			return client.LoadInstallationRepos()
		},
	}
}

// InstallationStatus backs the New Stream "Install Helix" gate. It checks
// for a github_app ServiceConnection with an installation id (no token
// minting — cheaper than the identity resolver for a UI probe), verifying
// against GitHub on every check so the gate reflects reality: it syncs
// the installation id (incl. down to 0 when the user uninstalls) and
// removes the stored connection when the app has been deleted on GitHub
// (so the gate reverts to "Create the Helix app"). Transient errors fall
// back to the stored state rather than mutating it.
func (g *GitHubIntegration) InstallationStatus(ctx context.Context, orgID string) (helixorgapi.GitHubInstallationStatus, error) {
	conns, err := g.conns.ListServiceConnectionsByType(ctx, orgID, types.ServiceConnectionTypeGitHubApp)
	if err != nil {
		return helixorgapi.GitHubInstallationStatus{}, fmt.Errorf("list github_app service connections: %w", err)
	}
	appExists := false
	installed := false
	slug := g.appSlug // BYO/pre-existing app fallback
	var owner string
	for _, c := range conns {
		if c == nil || c.GitHubAppID == 0 {
			continue
		}
		if pem, derr := g.decrypt(c); derr == nil {
			insts, ierr := g.listInstalls(ctx, c.GitHubAppID, pem, c.BaseURL)
			switch {
			case errors.Is(ierr, githubclient.ErrAppNotFound):
				log.Warn().Str("org_id", orgID).Int64("app_id", c.GitHubAppID).Msg("Helix GitHub App no longer exists on GitHub; removing stale connection")
				if derr := g.conns.DeleteServiceConnection(ctx, c.ID); derr == nil {
					continue // gone — don't count it
				} else {
					log.Error().Err(derr).Str("org_id", orgID).Msg("delete stale github app connection failed")
				}
			case ierr != nil:
				log.Warn().Err(ierr).Str("org_id", orgID).Msg("verify github app installation failed; using stored state")
			default:
				var newInstallID int64
				if len(insts) > 0 {
					newInstallID = insts[0].GetID()
				}
				// Backfill the owner (for the manage URL) on apps created
				// before the field existed — for our org-owned/installed-on-
				// same-org flow the install account is the owner.
				newOwner := c.GitHubAppOwner
				if newOwner == "" && len(insts) > 0 {
					newOwner = insts[0].GetAccount().GetLogin()
				}
				if newInstallID != c.GitHubInstallationID || newOwner != c.GitHubAppOwner {
					c.GitHubInstallationID = newInstallID
					c.GitHubAppOwner = newOwner
					if uerr := g.conns.UpdateServiceConnection(ctx, c); uerr != nil {
						log.Warn().Err(uerr).Str("org_id", orgID).Msg("persist synced installation id failed")
					} else {
						log.Info().Str("org_id", orgID).Int64("installation_id", newInstallID).Msg("synced Helix GitHub App installation from GitHub")
					}
				}
			}
		}
		appExists = true
		if c.GitHubAppSlug != "" {
			slug = c.GitHubAppSlug // prefer the created app's own slug
		}
		if c.GitHubAppOwner != "" {
			owner = c.GitHubAppOwner
		}
		if c.GitHubInstallationID != 0 {
			installed = true
		}
	}
	var installURL, manageURL string
	if slug != "" {
		installURL = g.webURL + "/apps/" + slug + "/installations/new"
	}
	if slug != "" && owner != "" {
		manageURL = g.webURL + "/organizations/" + owner + "/settings/apps/" + slug
	}
	return helixorgapi.GitHubInstallationStatus{AppExists: appExists, Installed: installed, InstallURL: installURL, ManageURL: manageURL}, nil
}

// AppRepos aggregates repos across every installation of the org's Helix
// App(s) — so one app installed on multiple GitHub orgs returns all of
// their repos. Mints a per-installation token and lists the installation
// repositories for each. isApp is false when the org has no app (caller
// then falls back to the user's OAuth repos).
func (g *GitHubIntegration) AppRepos(ctx context.Context, orgID string) ([]string, bool, error) {
	conns, err := g.conns.ListServiceConnectionsByType(ctx, orgID, types.ServiceConnectionTypeGitHubApp)
	if err != nil {
		return nil, false, fmt.Errorf("list github_app service connections: %w", err)
	}
	isApp := false
	seen := map[string]struct{}{}
	var repos []string
	for _, c := range conns {
		if c == nil || c.GitHubAppID == 0 || c.GitHubPrivateKey == "" {
			continue
		}
		isApp = true
		pem, derr := g.decrypt(c)
		if derr != nil {
			continue
		}
		installs, ierr := g.listInstalls(ctx, c.GitHubAppID, pem, c.BaseURL)
		if ierr != nil {
			// App deleted or a transient error — skip this app rather than
			// fail the whole listing (other apps may still resolve).
			log.Warn().Err(ierr).Str("org_id", orgID).Int64("app_id", c.GitHubAppID).Msg("list app installations for repo picker failed")
			continue
		}
		for _, inst := range installs {
			tok, terr := g.mintToken(ctx, c.GitHubAppID, inst.GetID(), pem, c.BaseURL)
			if terr != nil {
				log.Warn().Err(terr).Str("org_id", orgID).Int64("installation_id", inst.GetID()).Msg("mint installation token for repo picker failed")
				continue
			}
			names, lerr := g.installRepos(ctx, tok, c.BaseURL)
			if lerr != nil {
				log.Warn().Err(lerr).Str("org_id", orgID).Int64("installation_id", inst.GetID()).Msg("list installation repos failed")
				continue
			}
			for _, n := range names {
				if _, ok := seen[n]; ok {
					continue
				}
				seen[n] = struct{}{}
				repos = append(repos, n)
			}
		}
	}
	return repos, isApp, nil
}
