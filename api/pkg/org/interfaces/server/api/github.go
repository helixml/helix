package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	githubclient "github.com/helixml/helix/api/pkg/github"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// githubWebhook is the per-request dispatcher for POST /github/webhook.
// Builds a github.Transport bound to the request's orgID (resolved
// from the org middleware's context) and hands the request off to
// its HandleInbound. Building per-request keeps the route stateless
// — a single mounted handler serves every org.
//
// @Summary Helix-org: inbound GitHub webhook
// @Tags HelixOrg
// @Param payload body object true "Raw GitHub webhook delivery"
// @Success 204 "Delivery accepted and fanned out"
// @Success 200 "Delivery accepted but no matching topics"
// @Failure 401 {object} api.ErrorResponse "Bad or missing X-Hub-Signature-256"
// @Failure 503 {object} api.ErrorResponse "transport.github not configured"
// @Router /api/v1/orgs/{org}/github/webhook [post]
func (a *apiHandler) githubWebhook(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// The github transport (reads matching topics + appends events) is
	// built at the composition root with the store; the adapter just
	// serves the inbound handler so it never holds the store itself.
	if a.deps.GitHubInbound == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("github transport not configured"))
		return
	}
	a.deps.GitHubInbound(orgID).ServeHTTP(w, r)
}

// ---- GitHub helper endpoints -------------------------------------------

// GitHubRepoDTO is one entry in the searchable repo dropdown the
// New Topic dialog shows when transport=github is picked. Kept
// intentionally narrow: just the canonical `owner/name` and an
// optional flag so the UI can dim non-admin repos (you can't
// install a webhook without admin rights).
type GitHubRepoDTO struct {
	FullName string `json:"full_name"`
	Private  bool   `json:"private,omitempty"`
}

// GitHubInstallationStatus is the resolved install state for an org plus
// the URL to start an install when it isn't installed yet.
type GitHubInstallationStatus struct {
	// AppExists is true when a Helix GitHub App has been created/registered
	// for this org (a github_app ServiceConnection exists), even if not yet
	// installed on any repo. Drives the gate's create-vs-install branch.
	AppExists bool `json:"app_exists"`
	// Installed is true when the Helix GitHub App has an installation for
	// this org (a github_app ServiceConnection with an installation id).
	Installed bool `json:"installed"`
	// InstallURL is where the New Topic gate sends the user to install the
	// app (https://github.com/apps/<slug>/installations/new). Populated from
	// the created app's slug, or from GITHUB_APP_SLUG for a pre-existing app.
	InstallURL string `json:"install_url,omitempty"`
	// ManageURL is the app's developer-settings page on GitHub
	// (github.com/organizations/<owner>/settings/apps/<slug>) — where you edit
	// permissions, repos, and delete the app. Empty when the owner is unknown
	// (e.g. a BYO app configured without it).
	ManageURL string `json:"manage_url,omitempty"`
}

// GitHubManifestStartResponse is what the frontend needs to POST a GitHub App
// manifest on the user's behalf: the GitHub URL to POST to, the manifest JSON
// to submit as the "manifest" form field, and the CSRF state.
type GitHubManifestStartResponse struct {
	PostURL  string `json:"post_url"`
	Manifest string `json:"manifest"`
	State    string `json:"state"`
}

// getGitHubAppInstallation backs the New Topic "Install Helix" gate: it
// reports whether the org has the Helix App installed and, if not, the URL
// to install it. The user's own GitHub identity is used only for that
// install step; everything afterwards (repo listing, worker git/gh) runs as
// the bot.
//
// @Summary Helix-org: GitHub App install status for the org
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.GitHubInstallationStatus
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/github/app-installation [get]
func (a *apiHandler) getGitHubAppInstallation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if a.deps.GitHubInstallation == nil {
		// No installation resolver wired — report not-installed with no URL
		// so the gate degrades to "ask your admin" rather than 500ing.
		writeJSON(w, http.StatusOK, GitHubInstallationStatus{})
		return
	}
	status, err := a.deps.GitHubInstallation(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("resolve github app installation: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// startGitHubAppManifest builds the GitHub App Manifest flow so the frontend
// can create the Helix app on the user's behalf (org-owned). Body:
// { "github_org": "acme", "origin": "https://helix.example.com" }.
//
// @Summary Helix-org: start the GitHub App manifest (create) flow
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Success 200 {object} api.GitHubManifestStartResponse
// @Failure 412 {object} api.ErrorResponse "manifest flow not wired"
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/github/app-manifest [post]
func (a *apiHandler) startGitHubAppManifest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if a.deps.GitHubManifestStart == nil {
		writeError(w, http.StatusPreconditionFailed, errors.New("GitHub App manifest flow is not enabled on this deployment"))
		return
	}
	var body struct {
		GitHubOrg string `json:"github_org"`
		Origin    string `json:"origin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	resp, err := a.deps.GitHubManifestStart(ctx, orgID, body.GitHubOrg, body.Origin)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// GitHubReposResponse is the body of GET /github/repos.
type GitHubReposResponse struct {
	Repos []GitHubRepoDTO `json:"repos"`
	// Source identifies which token paid for this list — useful
	// when debugging "I can't see repo X" reports.
	Source string `json:"source,omitempty"`
}

// listGitHubRepos returns every repo the connected GitHub OAuth
// token can see, sorted alphabetically. Drives the searchable
// dropdown so operators don't have to remember the exact
// `owner/name` they want to wire up.
//
// @Summary Helix-org: list GitHub repos accessible to the org's connected token
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.GitHubReposResponse
// @Failure 412 {object} api.ErrorResponse "no GitHub token configured"
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/github/repos [get]
func (a *apiHandler) listGitHubRepos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// App mode: aggregate repos across every installation of the org's Helix
	// App(s) — so one app installed on winderai + helixml returns both. The
	// installation token can't call /user/repos, so this lists
	// /installation/repositories per installation server-side.
	if a.deps.GitHubAppRepos != nil {
		names, isApp, err := a.deps.GitHubAppRepos(ctx, orgID)
		if err != nil {
			writeError(w, http.StatusBadGateway, fmt.Errorf("list github app repos: %w", err))
			return
		}
		if isApp {
			out := GitHubReposResponse{Repos: make([]GitHubRepoDTO, 0, len(names)), Source: "app"}
			for _, n := range names {
				out.Repos = append(out.Repos, GitHubRepoDTO{FullName: n})
			}
			writeJSON(w, http.StatusOK, out)
			return
		}
	}

	// Legacy OAuth fallback: list the connecting user's repos.
	var token string
	switch {
	case a.deps.GitHubIdentity != nil:
		id, err := a.deps.GitHubIdentity(ctx, orgID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("resolve github identity: %w", err))
			return
		}
		token = id.Token
	case a.deps.GitHubTokenResolver != nil:
		t, err := a.deps.GitHubTokenResolver(ctx, orgID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("resolve github token: %w", err))
			return
		}
		token = t
	default:
		writeError(w, http.StatusPreconditionFailed, errors.New("no GitHub identity wired; install the Helix GitHub App or connect a GitHub account"))
		return
	}
	if token == "" {
		writeError(w, http.StatusPreconditionFailed, errors.New("Helix GitHub App not installed for this org and no GitHub OAuth connection found"))
		return
	}
	client, err := githubclient.NewGithubClient(githubclient.ClientOptions{Ctx: ctx, Token: token})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("build github client: %w", err))
		return
	}
	names, err := client.LoadRepos()
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("list github repos: %w", err))
		return
	}
	out := GitHubReposResponse{Repos: make([]GitHubRepoDTO, 0, len(names)), Source: "oauth"}
	for _, n := range names {
		out.Repos = append(out.Repos, GitHubRepoDTO{FullName: n})
	}
	writeJSON(w, http.StatusOK, out)
}

// InstallGitHubWebhookResponse is the body of POST
// /topics/{id}/github/install-webhook.
type InstallGitHubWebhookResponse struct {
	WebhookID      int64  `json:"webhook_id"`
	WebhookHTMLURL string `json:"webhook_html_url,omitempty"`
	PayloadURL     string `json:"payload_url"`
	// Warning is a non-fatal message about the just-installed
	// webhook — e.g. "SERVER_URL is a loopback address so GitHub's
	// servers can't actually deliver to this URL". The webhook IS
	// installed on GitHub; the warning just tells the operator
	// what needs fixing on their side for deliveries to flow.
	Warning string `json:"warning,omitempty"`
}

// GitHubWebhookStatusResponse is the body of GET
// /topics/{id}/github/webhook-status. It reports the LIVE state of the
// topic's repo webhook as seen on GitHub (not the stored config), so the
// detail page can show a link when the hook really exists and a re-install
// button when it doesn't — and self-correct stale stored ids.
type GitHubWebhookStatusResponse struct {
	// State is one of:
	//   "installed" — a webhook for this topic's payload URL exists on the repo
	//   "missing"   — GitHub was reachable and has no such webhook (needs install)
	//   "unknown"   — couldn't determine (no repo / no public URL / no creds /
	//                 GitHub error); see Detail. The UI falls back to stored state.
	State          string `json:"state"`
	WebhookID      int64  `json:"webhook_id,omitempty"`
	WebhookHTMLURL string `json:"webhook_html_url,omitempty"`
	Active         bool   `json:"active,omitempty"`
	PayloadURL     string `json:"payload_url,omitempty"`
	// Detail explains a "unknown" state (and is empty otherwise).
	Detail string `json:"detail,omitempty"`
}

// installGitHubWebhook calls the GitHub REST API on behalf of the
// operator to register a webhook on the topic's repo pointing at
// helix's per-topic payload URL. Idempotent: if a webhook with
// the same URL already exists on the repo, we adopt it (no
// double-install). Persists the resulting webhook id + edit-page
// URL on the topic's transport config so the detail page can
// deep-link out.
//
// Pre-conditions:
//   - transport=github topic
//   - GitHubTokenResolver returns a non-empty token
//   - transport.github.webhook_secret configured on the org; if
//     missing, helix auto-generates one and persists it (the
//     operator never has to copy it manually).
//   - PublicServerURL set to a non-localhost origin (refused
//     otherwise — GitHub's servers can't reach localhost).
//
// @Summary Helix-org: auto-install the webhook for a github topic
// @Tags HelixOrg
// @Param id path string true "Topic ID"
// @Produce json
// @Success 200 {object} api.InstallGitHubWebhookResponse
// @Failure 400 {object} api.ErrorResponse
// @Failure 412 {object} api.ErrorResponse "pre-conditions not met"
// @Failure 502 {object} api.ErrorResponse "GitHub API call failed"
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/topics/{id}/github/install-webhook [post]
func (a *apiHandler) installGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if a.deps.Topics == nil {
		writeError(w, http.StatusNotImplemented, errors.New("topics service is not wired in this deployment"))
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	topicID := streaming.TopicID(r.PathValue("id"))
	if topicID == "" {
		writeError(w, http.StatusBadRequest, errors.New("topic id is required"))
		return
	}
	// Generic seam: the topics service dispatches to the registered
	// inbound provisioner for the topic's transport (github today).
	res, err := a.deps.Topics.InstallInbound(r.Context(), orgID, topicID)
	if err != nil {
		writeError(w, inboundFailStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, InstallGitHubWebhookResponse{
		WebhookID:      res.WebhookID,
		WebhookHTMLURL: res.WebhookHTMLURL,
		PayloadURL:     res.PayloadURL,
	})
}

// inboundFailStatus maps a streaming.Failure (from the inbound-provisioning
// seam) to its HTTP code. Non-Failure errors fall through to errStatus
// (404 for a missing topic, else 500).
func inboundFailStatus(err error) int {
	var f *streaming.Failure
	if errors.As(err, &f) {
		switch f.Kind {
		case streaming.FailBadRequest:
			return http.StatusBadRequest
		case streaming.FailPrecondition:
			return http.StatusPreconditionFailed
		case streaming.FailUpstream:
			return http.StatusBadGateway
		case streaming.FailNotFound:
			return http.StatusNotFound
		default:
			return http.StatusInternalServerError
		}
	}
	return errStatus(err)
}

// getGitHubWebhookStatus reports the LIVE state of a github topic's repo
// webhook by listing the repo's hooks (as the bot) and matching this topic's
// payload URL. Read-only: it never creates, edits, or persists. Returns
// state="unknown" (rather than an HTTP error) for every "can't tell" case so
// the detail page can degrade to stored config instead of showing a scary
// failure for an ordinarily-fine condition (e.g. no public URL configured yet).
//
// @Summary Helix-org: live webhook status for a github topic
// @Tags HelixOrg
// @Param id path string true "Topic ID"
// @Produce json
// @Success 200 {object} api.GitHubWebhookStatusResponse
// @Failure 400 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/topics/{id}/github/webhook-status [get]
func (a *apiHandler) getGitHubWebhookStatus(w http.ResponseWriter, r *http.Request) {
	if a.deps.Topics == nil {
		writeError(w, http.StatusNotImplemented, errors.New("topics service is not wired in this deployment"))
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	topicID := streaming.TopicID(r.PathValue("id"))
	if topicID == "" {
		writeError(w, http.StatusBadRequest, errors.New("topic id is required"))
		return
	}
	res, err := a.deps.Topics.InboundStatus(r.Context(), orgID, topicID)
	if err != nil {
		writeError(w, inboundFailStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, GitHubWebhookStatusResponse{
		State:          res.State,
		WebhookID:      res.WebhookID,
		WebhookHTMLURL: res.WebhookHTMLURL,
		Active:         res.Active,
		PayloadURL:     res.PayloadURL,
		Detail:         res.Detail,
	})
}
