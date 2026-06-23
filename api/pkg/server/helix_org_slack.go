package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/crypto"
	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
	slackcore "github.com/helixml/helix/api/pkg/serviceconnection/slack"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// defaultSlackBotScopes are the bot scopes the "Install to Slack" flow
// requests. They cover reading channel/group/DM messages + app mentions
// (inbound), posting as a customised persona (outbound), and joining
// channels (provisioner).
var defaultSlackBotScopes = []string{
	"app_mentions:read",
	"channels:history",
	"channels:read",
	"channels:join",
	"groups:history",
	"groups:read",
	"im:history",
	"chat:write",
	"chat:write.customize",
}

// slackWorkspaces implements slacktransport.Workspaces over the helix
// ServiceConnection store. It is the org-aware adapter the otherwise
// org-agnostic slack transport depends on: it resolves a Slack team_id
// (or a ServiceConnection id) to a decrypted bot token + owning org.
type slackWorkspaces struct {
	store  helixstore.Store
	encKey func() ([]byte, error)
}

func newSlackWorkspaces(store helixstore.Store, encKey func() ([]byte, error)) *slackWorkspaces {
	return &slackWorkspaces{store: store, encKey: encKey}
}

func (w *slackWorkspaces) toWorkspace(conn *types.ServiceConnection) (slacktransport.Workspace, error) {
	key, err := w.encKey()
	if err != nil {
		return slacktransport.Workspace{}, fmt.Errorf("encryption key: %w", err)
	}
	var token string
	if conn.SlackBotToken != "" {
		dec, err := crypto.DecryptAES256GCM(conn.SlackBotToken, key)
		if err != nil {
			return slacktransport.Workspace{}, fmt.Errorf("decrypt slack bot token: %w", err)
		}
		token = string(dec)
	}
	return slacktransport.Workspace{
		ID:       conn.ID,
		OrgID:    conn.OrganizationID,
		TeamID:   conn.SlackTeamID,
		BotToken: token,
	}, nil
}

// ByTeamID resolves the org-scoped workspace install for a Slack team.
func (w *slackWorkspaces) ByTeamID(ctx context.Context, teamID string) (slacktransport.Workspace, error) {
	conn, err := w.store.GetServiceConnectionBySlackTeamID(ctx, teamID)
	if err != nil {
		if err == helixstore.ErrNotFound {
			return slacktransport.Workspace{}, slacktransport.ErrNoWorkspace
		}
		return slacktransport.Workspace{}, err
	}
	return w.toWorkspace(conn)
}

// ByID resolves a workspace by its ServiceConnection id.
func (w *slackWorkspaces) ByID(ctx context.Context, id string) (slacktransport.Workspace, error) {
	conn, err := w.store.GetServiceConnection(ctx, id)
	if err != nil {
		if err == helixstore.ErrNotFound {
			return slacktransport.Workspace{}, slacktransport.ErrNoWorkspace
		}
		return slacktransport.Workspace{}, err
	}
	if conn.Type != types.ServiceConnectionTypeSlackWorkspace {
		return slacktransport.Workspace{}, slacktransport.ErrNoWorkspace
	}
	return w.toWorkspace(conn)
}

// runSlackSocketMode runs the Socket Mode ingress until ctx is
// cancelled, but only when the global app is configured with
// ingress_mode=socket and an app-level token. The socket connection
// needs ONLY the app-level token (xapp-) — the bot token is per-workspace
// and is resolved from the slack_workspace ServiceConnection by team_id,
// exactly like REST. So one socket can serve multiple workspaces, each
// connected by pasting its bot token. Single-replica: a nil SingleOwner
// means this process always owns the connection; a pg advisory lock can
// gate multi-replica later.
func (s *HelixAPIServer) runSlackSocketMode(ctx context.Context, ingest *slacktransport.Ingest, logger *slog.Logger) {
	app, err := s.getGlobalSlackApp(ctx)
	if err != nil {
		logger.Info("slack.socketmode: no global app configured — not starting")
		return
	}
	if app.SlackIngressMode != "socket" {
		logger.Info("slack.socketmode: ingress mode is not 'socket' — not starting", "mode", app.SlackIngressMode)
		return
	}
	if app.SlackAppToken == "" {
		logger.Warn("slack.socketmode: socket mode requires an app-level token — not starting")
		return
	}
	key, err := s.getEncryptionKey()
	if err != nil {
		logger.Error("slack.socketmode: encryption key", "err", err)
		return
	}
	appToken, err := crypto.DecryptAES256GCM(app.SlackAppToken, key)
	if err != nil {
		logger.Error("slack.socketmode: decrypt app token", "err", err)
		return
	}

	// Connect with the app token alone; per-workspace bot tokens are
	// resolved downstream (ingest → workspace by team_id) for any posting.
	connector := slackcore.NewConnector(string(appToken), "", "", logger)
	runner := slackcore.NewSocketMode(ingest.OnEvent, nil, connector, logger)
	logger.Info("slack.socketmode: starting")
	if err := runner.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Error("slack.socketmode: runner exited", "err", err)
	}
}

// getGlobalSlackApp returns the single deployment-wide slack_app
// ServiceConnection (OrganizationID=""), or ErrNotFound. The first one
// wins — there should only ever be one.
func (s *HelixAPIServer) getGlobalSlackApp(ctx context.Context) (*types.ServiceConnection, error) {
	apps, err := s.Store.ListServiceConnectionsByType(ctx, "", types.ServiceConnectionTypeSlackApp)
	if err != nil {
		return nil, err
	}
	if len(apps) == 0 {
		return nil, helixstore.ErrNotFound
	}
	return apps[0], nil
}

// slackSigningSecret resolves the global app's decrypted REST signing
// secret. Returns "" (no error) when no app is configured, so the
// Events handler stays inert rather than erroring.
func (s *HelixAPIServer) slackSigningSecret(ctx context.Context) (string, error) {
	app, err := s.getGlobalSlackApp(ctx)
	if err != nil {
		if err == helixstore.ErrNotFound {
			return "", nil
		}
		return "", err
	}
	if app.SlackSigningSecret == "" {
		return "", nil
	}
	key, err := s.getEncryptionKey()
	if err != nil {
		return "", err
	}
	dec, err := crypto.DecryptAES256GCM(app.SlackSigningSecret, key)
	if err != nil {
		return "", err
	}
	return string(dec), nil
}

// slackRedirectURI is the OAuth callback URL Slack redirects back to
// after the admin approves the install. Must exactly match a Redirect
// URL configured on the Slack app.
func (s *HelixAPIServer) slackRedirectURI() string {
	return s.Cfg.WebServer.URL + "/api/v1/slack/oauth/callback"
}

// slackOAuthStart (GET /api/v1/orgs/{org}/slack/oauth/start) builds the
// "Add to Slack" authorize URL and returns it as JSON so the
// (token-authenticated) frontend can redirect the browser to it. The
// org id is carried through the round trip in an encrypted state param.
// @Summary Start Slack workspace install
// @Description Build the Slack OAuth authorize URL for installing the global app into an org's workspace
// @Tags slack
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Success 200 {object} map[string]string
// @Router /api/v1/orgs/{org}/slack/oauth/start [get]
// @Security BearerAuth
func (s *HelixAPIServer) slackOAuthStart(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	org, err := s.lookupOrg(r.Context(), mux.Vars(r)["org"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if _, err := s.authorizeOrgMember(r.Context(), user, org.ID); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	app, err := s.getGlobalSlackApp(r.Context())
	if err != nil {
		http.Error(w, "Slack app not configured by the administrator", http.StatusServiceUnavailable)
		return
	}
	if app.SlackClientID == "" {
		http.Error(w, "Slack app is missing its client id (REST install requires it)", http.StatusServiceUnavailable)
		return
	}

	key, err := s.getEncryptionKey()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	state, err := crypto.EncryptAES256GCM([]byte(org.ID), key)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	authorizeURL := slackcore.AuthorizeURL(app.SlackClientID, s.slackRedirectURI(), defaultSlackBotScopes, state)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"url": authorizeURL})
}

// slackOAuthCallback (GET /api/v1/slack/oauth/callback) completes the
// install: decode the org from state, exchange the code for a bot token
// + team id against the global app's client credentials, and persist
// them as an org-scoped slack_workspace ServiceConnection.
func (s *HelixAPIServer) slackOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}

	key, err := s.getEncryptionKey()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	orgBytes, err := crypto.DecryptAES256GCM(state, key)
	if err != nil {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	orgID := string(orgBytes)

	app, err := s.getGlobalSlackApp(r.Context())
	if err != nil {
		http.Error(w, "Slack app not configured", http.StatusServiceUnavailable)
		return
	}
	if app.SlackClientID == "" || app.SlackClientSecret == "" {
		http.Error(w, "Slack app is missing client credentials", http.StatusServiceUnavailable)
		return
	}
	clientSecret, err := crypto.DecryptAES256GCM(app.SlackClientSecret, key)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	install, err := slackcore.CodeExchanger{}.ExchangeCode(r.Context(), app.SlackClientID, string(clientSecret), code, s.slackRedirectURI())
	if err != nil {
		log.Error().Err(err).Str("org", orgID).Msg("slack oauth: code exchange failed")
		http.Error(w, "Slack install failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	if err := s.upsertSlackWorkspace(r.Context(), orgID, install); err != nil {
		log.Error().Err(err).Str("org", orgID).Msg("slack oauth: persist workspace failed")
		http.Error(w, "Failed to save Slack install", http.StatusInternalServerError)
		return
	}

	// Redirect back to the org's integrations UI.
	http.Redirect(w, r, fmt.Sprintf("/orgs/%s?slack_installed=1", url.PathEscape(orgID)), http.StatusFound)
}

// connectSlackWorkspaceRequest is the body of the manual
// (Socket Mode / on-prem) workspace connect: the operator pastes the bot
// token they got by installing the app into their workspace.
type connectSlackWorkspaceRequest struct {
	BotToken string `json:"bot_token"`
}

// connectSlackWorkspace (POST /api/v1/orgs/{org}/slack/workspaces) connects
// a Slack workspace to an org from a pasted bot token. This is the
// Socket Mode / on-prem counterpart to the REST OAuth install: there's no
// OAuth redirect, so the operator installs the app into their workspace
// and pastes the resulting xoxb- token. We auth.test it to derive the
// team id / name / bot user, then persist a slack_workspace connection —
// the same row shape the OAuth flow produces, so inbound/outbound resolve
// it identically by team_id.
// @Summary Connect a Slack workspace by bot token
// @Description Connect a Slack workspace to an org from a bot token (Socket Mode / on-prem)
// @Tags slack
// @Accept json
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Param request body connectSlackWorkspaceRequest true "Bot token"
// @Success 201 {object} types.ServiceConnectionResponse
// @Router /api/v1/orgs/{org}/slack/workspaces [post]
// @Security BearerAuth
func (s *HelixAPIServer) connectSlackWorkspace(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	org, err := s.lookupOrg(r.Context(), mux.Vars(r)["org"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if _, err := s.authorizeOrgMember(r.Context(), user, org.ID); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var req connectSlackWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.BotToken == "" {
		http.Error(w, "bot_token is required", http.StatusBadRequest)
		return
	}

	// Verify the token and derive the workspace identity.
	id, err := slackcore.AuthTest(r.Context(), slackcore.New(req.BotToken, ""))
	if err != nil {
		http.Error(w, "Bot token rejected by Slack: "+err.Error(), http.StatusBadGateway)
		return
	}

	install := slackcore.Install{
		BotToken:  req.BotToken,
		TeamID:    id.TeamID,
		TeamName:  id.Team,
		BotUserID: id.UserID,
	}
	if err := s.upsertSlackWorkspace(r.Context(), org.ID, install); err != nil {
		http.Error(w, "Failed to save workspace: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the freshly-stored workspace.
	conns, _ := s.Store.ListServiceConnectionsByType(r.Context(), org.ID, types.ServiceConnectionTypeSlackWorkspace)
	for _, c := range conns {
		if c.SlackTeamID == id.TeamID {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(c.ToResponse())
			return
		}
	}
	w.WriteHeader(http.StatusCreated)
}

// upsertSlackWorkspace creates or updates the org's slack_workspace
// ServiceConnection for the installed team. Re-installing the same
// workspace refreshes the bot token rather than creating a duplicate.
func (s *HelixAPIServer) upsertSlackWorkspace(ctx context.Context, orgID string, install slackcore.Install) error {
	key, err := s.getEncryptionKey()
	if err != nil {
		return err
	}
	encToken, err := crypto.EncryptAES256GCM([]byte(install.BotToken), key)
	if err != nil {
		return fmt.Errorf("encrypt bot token: %w", err)
	}

	// Reuse an existing row for the same team in this org if present.
	existing, _ := s.Store.ListServiceConnectionsByType(ctx, orgID, types.ServiceConnectionTypeSlackWorkspace)
	for _, conn := range existing {
		if conn.SlackTeamID == install.TeamID {
			conn.SlackTeamName = install.TeamName
			conn.SlackBotUserID = install.BotUserID
			conn.SlackAppID = install.AppID
			conn.SlackBotToken = encToken
			conn.Name = slackWorkspaceName(install)
			return s.Store.UpdateServiceConnection(ctx, conn)
		}
	}

	conn := &types.ServiceConnection{
		ID:             uuid.New().String(),
		OrganizationID: orgID,
		Name:           slackWorkspaceName(install),
		Type:           types.ServiceConnectionTypeSlackWorkspace,
		ProviderType:   types.ExternalRepositoryTypeSlack,
		SlackTeamID:    install.TeamID,
		SlackTeamName:  install.TeamName,
		SlackBotUserID: install.BotUserID,
		SlackAppID:     install.AppID,
		SlackBotToken:  encToken,
	}
	return s.Store.CreateServiceConnection(ctx, conn)
}

func slackWorkspaceName(install slackcore.Install) string {
	if install.TeamName != "" {
		return install.TeamName
	}
	return "Slack workspace " + install.TeamID
}

// listSlackWorkspaces (GET /api/v1/orgs/{org}/slack/workspaces) returns
// the org's connected Slack workspaces. Org-scoped: only the caller's
// org rows are ever returned.
// @Summary List org Slack workspaces
// @Description List the Slack workspaces installed for an organization
// @Tags slack
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Success 200 {array} types.ServiceConnectionResponse
// @Router /api/v1/orgs/{org}/slack/workspaces [get]
// @Security BearerAuth
func (s *HelixAPIServer) listSlackWorkspaces(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	org, err := s.lookupOrg(r.Context(), mux.Vars(r)["org"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if _, err := s.authorizeOrgMember(r.Context(), user, org.ID); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	conns, err := s.Store.ListServiceConnectionsByType(r.Context(), org.ID, types.ServiceConnectionTypeSlackWorkspace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	responses := make([]*types.ServiceConnectionResponse, len(conns))
	for i, c := range conns {
		responses[i] = c.ToResponse()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(responses)
}

// deleteSlackWorkspace (DELETE /api/v1/orgs/{org}/slack/workspaces/{id})
// removes one workspace install. The org of the connection must match
// the resolved org (cross-org delete is blocked).
// @Summary Disconnect an org Slack workspace
// @Description Remove a Slack workspace install from an organization
// @Tags slack
// @Param org path string true "Organization ID or slug"
// @Param id path string true "Workspace connection ID"
// @Success 204
// @Router /api/v1/orgs/{org}/slack/workspaces/{id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteSlackWorkspace(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	org, err := s.lookupOrg(r.Context(), mux.Vars(r)["org"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if _, err := s.authorizeOrgMember(r.Context(), user, org.ID); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	connID := mux.Vars(r)["id"]
	conn, err := s.Store.GetServiceConnection(r.Context(), connID)
	if err != nil {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}
	if conn.OrganizationID != org.ID || conn.Type != types.ServiceConnectionTypeSlackWorkspace {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}
	if err := s.Store.DeleteServiceConnection(r.Context(), connID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
