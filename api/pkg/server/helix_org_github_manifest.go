package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/crypto"
	githubclient "github.com/helixml/helix/api/pkg/github"
	helixorgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// GitHub App Manifest flow — "create the Helix app on the user's behalf".
//
// 1. start (authed): build a Helix-authored manifest + an encrypted state,
//    return them so the frontend can POST the manifest to GitHub.
// 2. GitHub creates the app and redirects the browser to the manifest's
//    redirect_url (our callback) with a one-hour ?code=.
// 3. callback (insecure, state-validated): exchange the code for the app's
//    id/slug/PEM, store a github_app ServiceConnection, then redirect the
//    browser to the app's install page.
// 4. The user installs; GitHub redirects to the manifest's setup_url (our
//    app-setup handler) with ?installation_id=, which we persist onto the
//    ServiceConnection. The page then postMessages the opener so the New
//    Stream dialog flips to "installed".
//
// The Helix org is carried in the redirect_url/setup_url path; the encrypted
// state carries the org + expiry for CSRF on the callback.

// githubManifestStateTTL bounds how long a started manifest flow stays valid.
const githubManifestStateTTL = time.Hour

// githubManifestState is the CSRF/context blob round-tripped (encrypted) as
// the GitHub ?state= parameter.
type githubManifestState struct {
	OrgID     string `json:"o"`
	GitHubOrg string `json:"g"`
	ExpiresAt int64  `json:"e"`
}

// githubManifest is the GitHub App manifest we POST on the user's behalf.
// Field names match GitHub's manifest schema.
type githubManifest struct {
	Name               string            `json:"name"`
	URL                string            `json:"url"`
	RedirectURL        string            `json:"redirect_url"`
	Public             bool              `json:"public"`
	DefaultPermissions map[string]string `json:"default_permissions"`
}

// helixAppPermissions is the minimum the Worker bot needs: clone/push,
// open PRs, manage issues, plus repository_hooks so the bot can install the
// per-repo webhook for a github Stream (delivery is per-repo, not via an
// app-level firehose). Metadata is mandatory + auto-granted.
var helixAppPermissions = map[string]string{
	"contents":         "write",
	"pull_requests":    "write",
	"issues":           "write",
	"repository_hooks": "write",
	"metadata":         "read",
}

func encodeGitHubManifestState(s githubManifestState, key []byte) (string, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return crypto.EncryptAES256GCM(raw, key)
}

func decodeGitHubManifestState(state string, key []byte) (githubManifestState, error) {
	raw, err := crypto.DecryptAES256GCM(state, key)
	if err != nil {
		return githubManifestState{}, err
	}
	var s githubManifestState
	if err := json.Unmarshal(raw, &s); err != nil {
		return githubManifestState{}, err
	}
	if time.Now().Unix() > s.ExpiresAt {
		return githubManifestState{}, fmt.Errorf("manifest state expired")
	}
	return s, nil
}

// normalizeOrigin validates a caller-supplied origin (window.location.origin)
// and returns just scheme://host, guarding against open-redirect abuse via a
// crafted origin.
func normalizeOrigin(origin string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil {
		return "", fmt.Errorf("invalid origin: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("origin must be an http(s) URL with a host")
	}
	return u.Scheme + "://" + u.Host, nil
}

// newGitHubManifestStart builds the start resolver wired into the org API
// Deps. It returns the GitHub POST URL, the manifest JSON to submit, and an
// encrypted state. getKey provides the server encryption key (for the state).
// webURL is the GitHub web origin (https://github.com or a GHES origin) the
// app create/install links are built against.
func newGitHubManifestStart(getKey func() ([]byte, error), webURL string) func(ctx context.Context, orgID, githubOrg, origin string) (helixorgapi.GitHubManifestStartResponse, error) {
	return func(_ context.Context, orgID, githubOrg, origin string) (helixorgapi.GitHubManifestStartResponse, error) {
		githubOrg = strings.TrimSpace(githubOrg)
		if githubOrg == "" {
			return helixorgapi.GitHubManifestStartResponse{}, fmt.Errorf("github organization is required")
		}
		base, err := normalizeOrigin(origin)
		if err != nil {
			return helixorgapi.GitHubManifestStartResponse{}, err
		}
		key, err := getKey()
		if err != nil {
			return helixorgapi.GitHubManifestStartResponse{}, fmt.Errorf("get encryption key: %w", err)
		}
		state, err := encodeGitHubManifestState(githubManifestState{
			OrgID:     orgID,
			GitHubOrg: githubOrg,
			ExpiresAt: time.Now().Add(githubManifestStateTTL).Unix(),
		}, key)
		if err != nil {
			return helixorgapi.GitHubManifestStartResponse{}, fmt.Errorf("encode state: %w", err)
		}

		manifest := githubManifest{
			Name:        fmt.Sprintf("Helix %s", githubOrg),
			URL:         "https://helix.ml",
			RedirectURL: base + "/api/v1/orgs/" + url.PathEscape(orgID) + "/github/app-manifest/callback",
			// Public ("Any account") so the one app can be installed on more
			// than one GitHub org (e.g. winderai AND helixml). A private app
			// can only be installed on its owner org. Each install is a
			// separate installation with its own per-org token; Helix
			// aggregates repos across installations and routes tokens by owner.
			Public:             true,
			DefaultPermissions: helixAppPermissions,
		}
		// No app-level webhook: event delivery is per-repo. The bot installs a
		// repo webhook (via repository_hooks permission) for each github Stream,
		// pointed at /streams/{id}/github/webhook. So the manifest deliberately
		// omits hook_attributes/default_events.
		manifestJSON, err := json.Marshal(manifest)
		if err != nil {
			return helixorgapi.GitHubManifestStartResponse{}, fmt.Errorf("marshal manifest: %w", err)
		}

		postURL := webURL + "/organizations/" + url.PathEscape(githubOrg) + "/settings/apps/new?state=" + url.QueryEscape(state)
		return helixorgapi.GitHubManifestStartResponse{
			PostURL:  postURL,
			Manifest: string(manifestJSON),
			State:    state,
		}, nil
	}
}

// newGitHubManifestCallbackHandler handles the browser redirect GitHub makes
// after the app is created. It exchanges the code, stores the app as a
// github_app ServiceConnection for the org, and redirects to the install
// page. Mounted on the insecure router (validated by the encrypted state,
// not the helix session) because it is a top-level navigation from github.com.
// webURL is the GitHub web origin for the install redirect; apiBaseURL is the
// API origin passed to the github client (empty for github.com) and stored on
// the created ServiceConnection so later calls target the right host (GHES).
func newGitHubManifestCallbackHandler(
	getKey func() ([]byte, error),
	st helixstore.Store,
	newID func() string,
	webURL string,
	apiBaseURL string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		if code == "" || state == "" {
			http.Error(w, "missing code or state", http.StatusBadRequest)
			return
		}
		key, err := getKey()
		if err != nil {
			http.Error(w, "server key error", http.StatusInternalServerError)
			return
		}
		parsed, err := decodeGitHubManifestState(state, key)
		if err != nil {
			http.Error(w, "invalid or expired state", http.StatusBadRequest)
			return
		}

		cfg, err := githubclient.CompleteAppManifest(ctx, code, apiBaseURL)
		if err != nil {
			log.Error().Err(err).Str("org_id", parsed.OrgID).Msg("github app manifest conversion failed")
			http.Error(w, "failed to complete app creation with GitHub: "+err.Error(), http.StatusBadGateway)
			return
		}
		if cfg.GetID() == 0 || cfg.GetPEM() == "" || cfg.GetSlug() == "" {
			http.Error(w, "GitHub returned an incomplete app config", http.StatusBadGateway)
			return
		}

		encPEM, err := crypto.EncryptAES256GCM([]byte(cfg.GetPEM()), key)
		if err != nil {
			http.Error(w, "failed to encrypt app key", http.StatusInternalServerError)
			return
		}

		id := newID()
		conn := &types.ServiceConnection{
			ID:               id,
			OrganizationID:   parsed.OrgID,
			Name:             cfg.GetName(),
			Description:      fmt.Sprintf("Helix GitHub App (created via manifest for %s)", parsed.GitHubOrg),
			Type:             types.ServiceConnectionTypeGitHubApp,
			ProviderType:     types.ExternalRepositoryTypeGitHub,
			GitHubAppID:      cfg.GetID(),
			GitHubAppSlug:    cfg.GetSlug(),
			GitHubAppOwner:   parsed.GitHubOrg,
			GitHubPrivateKey: encPEM,
			// Empty for github.com; the GHES origin otherwise, so the install
			// reconcile + token minting target the right API host.
			BaseURL: apiBaseURL,
		}
		if err := st.CreateServiceConnection(ctx, conn); err != nil {
			log.Error().Err(err).Str("org_id", parsed.OrgID).Msg("store github app service connection failed")
			http.Error(w, "failed to store the created app", http.StatusInternalServerError)
			return
		}
		log.Info().Str("org_id", parsed.OrgID).Int64("app_id", cfg.GetID()).Str("slug", cfg.GetSlug()).Msg("created Helix GitHub App via manifest")

		// No app-level webhook secret to persist: the app has no webhook.
		// Per-repo webhooks (installed by the bot for each github Stream) use
		// the per-org transport.github.webhook_secret minted by
		// ensureGitHubWebhookSecret at install time.

		// Chain straight into installation so the user picks repos. A
		// just-created app's install page (…/installations/new →
		// select_target) 404s for a few seconds until GitHub finishes
		// provisioning it, so wait until it's live before redirecting (a real
		// readiness check, not a blind sleep) — bounded so we never hang.
		installURL := webURL + "/apps/" + url.PathEscape(cfg.GetSlug()) + "/installations/new"
		waitForGitHubAppInstallReady(ctx, installURL, 20*time.Second)
		http.Redirect(w, r, installURL, http.StatusFound)
	}
}

// waitForGitHubAppInstallReady polls the app's install URL (following GitHub's
// redirect to select_target / login) until it stops returning 404, i.e. GitHub
// has finished provisioning a freshly-created app. Bounded by timeout; on
// timeout it returns and the caller redirects anyway (best effort).
func waitForGitHubAppInstallReady(ctx context.Context, installURL string, timeout time.Duration) {
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, installURL, nil)
		if err != nil {
			return
		}
		resp, err := client.Do(req)
		if err == nil {
			status := resp.StatusCode
			resp.Body.Close()
			if status != http.StatusNotFound {
				return // install page is live
			}
		}
		if time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}
