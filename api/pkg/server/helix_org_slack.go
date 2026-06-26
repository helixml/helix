package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/crypto"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
	slackcore "github.com/helixml/helix/api/pkg/serviceconnection/slack"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// slackWorkspaceTopics keeps a workspace-scoped Slack Topic in sync with
// the slack_workspace ServiceConnection lifecycle: one Topic per connected
// workspace, created on connect and removed on disconnect. The Topic id is
// deterministic (s-slack-ws-<connID>) so this never touches user-created
// topics — same ownership-by-convention the team-topic reconciler uses.
type slackWorkspaceTopics struct {
	topics helixorgstore.Topics
	logger *slog.Logger
}

func slackWorkspaceTopicID(connID string) streaming.TopicID {
	return streaming.TopicID("s-slack-ws-" + connID)
}

func (r *slackWorkspaceTopics) ensure(ctx context.Context, orgID, connID, workspaceName, appName string) {
	if r == nil {
		return
	}
	id := slackWorkspaceTopicID(connID)
	if _, err := r.topics.Get(ctx, orgID, id); err == nil {
		return // already exists
	}
	name := slacktransport.TopicName(appName, workspaceName)
	cfg, _ := json.Marshal(transport.SlackConfig{ServiceConnectionID: connID})
	topic, err := streaming.NewTopic(id, name, "Messages from the connected Slack workspace.", "", time.Now().UTC(),
		transport.Transport{Kind: transport.KindSlack, Config: cfg}, orgID)
	if err != nil {
		r.logger.Error("slack.reconcile: build topic", "org", orgID, "conn", connID, "err", err)
		return
	}
	if err := r.topics.Create(ctx, topic); err != nil {
		r.logger.Error("slack.reconcile: create topic", "org", orgID, "conn", connID, "err", err)
	}
}

func (r *slackWorkspaceTopics) remove(ctx context.Context, orgID, connID string) {
	if r == nil {
		return
	}
	if err := r.topics.Delete(ctx, orgID, slackWorkspaceTopicID(connID)); err != nil {
		r.logger.Warn("slack.reconcile: delete topic", "org", orgID, "conn", connID, "err", err)
	}
}

// defaultSlackBotScopes are the bot scopes the "Install to Slack" flow
// requests. They cover reading channel/group/DM messages + app mentions
// (inbound), posting as a customised persona (outbound), joining channels
// (provisioner), and the richer egress gestures a Worker drives directly
// via the Slack Web API (reactions, file uploads). Keep in sync with the
// frontend manifest's BOT_SCOPES in SlackAppSetup.tsx.
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
	"reactions:write",
	"files:write",
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
// team_id is globally unique to a workspace, so the slack_workspace rows
// (listed across all orgs) are scanned for the one that matches.
func (w *slackWorkspaces) ByTeamID(ctx context.Context, teamID string) (slacktransport.Workspace, error) {
	if teamID == "" {
		return slacktransport.Workspace{}, slacktransport.ErrNoWorkspace
	}
	conns, err := w.store.ListServiceConnectionsByType(ctx, "", types.ServiceConnectionTypeSlackWorkspace)
	if err != nil {
		return slacktransport.Workspace{}, err
	}
	for _, conn := range conns {
		if conn.SlackTeamID == teamID {
			return w.toWorkspace(conn)
		}
	}
	return slacktransport.Workspace{}, slacktransport.ErrNoWorkspace
}

// resolveForOrg resolves the bot token for the credential provider. When
// teamID is set (the triggering event's extra.slack_team_id) it returns
// that exact workspace; empty teamID falls back to the org's workspace.
// Either way the candidate set is the caller's org only
// (ListServiceConnectionsByType is org-scoped), so a Worker can never
// mint another org's token. App-linked (OAuth-installed) connections win
// over a bare manual-token duplicate for the same team.
func (w *slackWorkspaces) resolveForOrg(ctx context.Context, orgID, teamID string) (slacktransport.Workspace, error) {
	conns, err := w.store.ListServiceConnectionsByType(ctx, orgID, types.ServiceConnectionTypeSlackWorkspace)
	if err != nil {
		return slacktransport.Workspace{}, err
	}
	var candidates []*types.ServiceConnection
	for _, c := range conns {
		if teamID == "" || c.SlackTeamID == teamID {
			candidates = append(candidates, c)
		}
	}
	if len(candidates) == 0 {
		return slacktransport.Workspace{}, slacktransport.ErrNoWorkspace
	}
	chosen := candidates[0]
	for _, c := range candidates {
		if c.SlackAppConnectionID != "" {
			chosen = c
			break
		}
	}
	return w.toWorkspace(chosen)
}

// kickSlackSocket asks the Socket Mode manager to re-reconcile its
// connections so a slack_app create/edit/delete applies without a
// restart. Lives on the org subsystem (not the core server) and is
// nil-safe on both the subsystem and the manager, so the admin
// service-connection handlers can call it unconditionally — it's a no-op
// when helix-org isn't mounted or Socket Mode is unused.
func (h *helixOrgHandlers) kickSlackSocket() {
	if h == nil || h.slackSocket == nil {
		return
	}
	h.slackSocket.Kick()
}

// errMultipleSlackApps is returned when an install must pick between
// several configured global Slack apps but none was named.
var errMultipleSlackApps = errors.New("multiple Slack apps configured — choose one")

func (s *HelixAPIServer) listSlackApps(ctx context.Context) ([]*types.ServiceConnection, error) {
	return s.Store.ListServiceConnectionsByType(ctx, "", types.ServiceConnectionTypeSlackApp)
}

func (s *HelixAPIServer) getSlackAppByID(ctx context.Context, id string) (*types.ServiceConnection, error) {
	conn, err := s.Store.GetServiceConnection(ctx, id)
	if err != nil {
		return nil, err
	}
	if conn.Type != types.ServiceConnectionTypeSlackApp || conn.OrganizationID != "" {
		return nil, helixstore.ErrNotFound
	}
	return conn, nil
}

// resolveSlackApp picks the app to install with: the one named by appID,
// or the only configured app when appID is empty. errMultipleSlackApps
// when ambiguous, ErrNotFound when none.
func (s *HelixAPIServer) resolveSlackApp(ctx context.Context, appID string) (*types.ServiceConnection, error) {
	if appID != "" {
		return s.getSlackAppByID(ctx, appID)
	}
	apps, err := s.listSlackApps(ctx)
	if err != nil {
		return nil, err
	}
	switch len(apps) {
	case 0:
		return nil, helixstore.ErrNotFound
	case 1:
		return apps[0], nil
	default:
		return nil, errMultipleSlackApps
	}
}

// slackSocketReconcileInterval is how often the Socket Mode manager
// re-scans the configured apps as a backstop. Create/delete handlers Kick
// it for instant pickup, so this only needs to catch changes the handlers
// can't signal (e.g. a direct DB edit) and re-establish dropped sockets.
const slackSocketReconcileInterval = 30 * time.Second

// newSlackSocketManager builds the manager that keeps Socket Mode
// connections in sync with the configured socket-mode apps. Each socket
// needs only the app-level token (xapp-); per-workspace bot tokens are
// resolved downstream (ingest → workspace by team_id), so one socket
// serves every installed workspace. The manager reconciles on an interval
// (and on Kick), so installing or editing a socket app takes effect with
// no server restart.
func (s *HelixAPIServer) newSlackSocketManager(ingest *slacktransport.Ingest, logger *slog.Logger) *slacktransport.SocketManager {
	list := func(ctx context.Context) ([]slacktransport.SocketApp, error) {
		apps, err := s.listSlackApps(ctx)
		if err != nil {
			return nil, err
		}
		key, err := s.getEncryptionKey()
		if err != nil {
			return nil, err
		}
		var out []slacktransport.SocketApp
		for _, app := range apps {
			if app.SlackIngressMode != "socket" || app.SlackAppToken == "" {
				continue
			}
			appToken, err := crypto.DecryptAES256GCM(app.SlackAppToken, key)
			if err != nil {
				logger.Error("slack.socketmode: decrypt app token", "app", app.ID, "err", err)
				continue
			}
			out = append(out, slacktransport.SocketApp{ID: app.ID, AppToken: string(appToken)})
		}
		return out, nil
	}
	// connect opens one Socket Mode connection bound to appCtx (a child of
	// the manager's run context). slackcore.SocketMode.Run self-heals on
	// transient drops until appCtx is cancelled; the returned stop cancels
	// it (used when the app is deleted or its token changes).
	connect := func(ctx context.Context, app slacktransport.SocketApp) func() {
		appCtx, cancel := context.WithCancel(ctx)
		go func() {
			connector := slackcore.NewConnector(app.AppToken, "", "", logger)
			runner := slackcore.NewSocketMode(ingest.OnEvent, connector, logger)
			if err := runner.Run(appCtx); err != nil && appCtx.Err() == nil {
				logger.Error("slack.socketmode: connection exited", "app", app.ID, "err", err)
			}
		}()
		return cancel
	}
	return slacktransport.NewSocketManager(list, connect, logger)
}

// slackSigningSecrets returns every configured app's decrypted signing
// secret. The Events handler accepts a request that verifies against any
// of them. Empty slice = no app configured (handler stays inert).
func (s *HelixAPIServer) slackSigningSecrets(ctx context.Context) ([]string, error) {
	apps, err := s.listSlackApps(ctx)
	if err != nil || len(apps) == 0 {
		return nil, nil
	}
	key, err := s.getEncryptionKey()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, app := range apps {
		if app.SlackSigningSecret == "" {
			continue
		}
		if dec, err := crypto.DecryptAES256GCM(app.SlackSigningSecret, key); err == nil {
			out = append(out, string(dec))
		}
	}
	return out, nil
}

// slackRedirectURI is the OAuth callback URL Slack redirects back to
// after the admin approves the install. Must exactly match a Redirect
// URL configured on the Slack app.
func (s *HelixAPIServer) slackRedirectURI() string {
	return s.Cfg.WebServer.URL + "/api/v1/slack/oauth/callback"
}

// listOrgSlackApps (GET /api/v1/orgs/{org}/slack/apps) lists the
// deployment's global Slack apps so an org admin can pick which to install
// when more than one is configured. Org-member gated; secrets are hidden
// by ToResponse.
// @Summary List installable Slack apps
// @Description List the deployment's global Slack apps available to install into a workspace
// @Tags slack
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Success 200 {array} types.ServiceConnectionResponse
// @Router /api/v1/orgs/{org}/slack/apps [get]
// @Security BearerAuth
func (s *HelixAPIServer) listOrgSlackApps(w http.ResponseWriter, r *http.Request) {
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
	apps, err := s.listSlackApps(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]*types.ServiceConnectionResponse, len(apps))
	for i, a := range apps {
		out[i] = a.ToResponse()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
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
// @Param app_id query string false "Slack app id to install (when multiple are configured)"
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

	app, err := s.resolveSlackApp(r.Context(), r.URL.Query().Get("app_id"))
	if err != nil {
		if errors.Is(err, errMultipleSlackApps) {
			http.Error(w, "Multiple Slack apps configured — pass app_id to choose one", http.StatusBadRequest)
			return
		}
		http.Error(w, "Slack app not configured by the administrator", http.StatusServiceUnavailable)
		return
	}
	if app.SlackClientID == "" {
		http.Error(w, "Slack app is missing its client id (OAuth install requires it)", http.StatusServiceUnavailable)
		return
	}

	key, err := s.getEncryptionKey()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	state, err := crypto.EncryptAES256GCM([]byte(org.ID+"|"+app.ID), key)
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
	stateBytes, err := crypto.DecryptAES256GCM(state, key)
	if err != nil {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	orgID, appConnID, _ := strings.Cut(string(stateBytes), "|")

	app, err := s.resolveSlackApp(r.Context(), appConnID)
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

	if err := s.upsertSlackWorkspace(r.Context(), orgID, install, app.ID); err != nil {
		log.Error().Err(err).Str("org", orgID).Msg("slack oauth: persist workspace failed")
		http.Error(w, "Failed to save Slack install", http.StatusInternalServerError)
		return
	}

	// Redirect back to the org's integrations UI.
	http.Redirect(w, r, fmt.Sprintf("/orgs/%s?slack_installed=1", url.PathEscape(orgID)), http.StatusFound)
}

// connectSlackWorkspaceRequest is the body of the manual
// (Socket Mode / on-prem) workspace connect: the operator pastes the bot
// token they got by installing the app into their workspace, and names
// which global app it belongs to so the UI can show it.
type connectSlackWorkspaceRequest struct {
	BotToken        string `json:"bot_token"`
	AppConnectionID string `json:"app_connection_id"`
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
	if err := s.upsertSlackWorkspace(r.Context(), org.ID, install, req.AppConnectionID); err != nil {
		http.Error(w, "Failed to save workspace: "+err.Error(), http.StatusInternalServerError)
		return
	}

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
func (s *HelixAPIServer) upsertSlackWorkspace(ctx context.Context, orgID string, install slackcore.Install, appConnID string) error {
	key, err := s.getEncryptionKey()
	if err != nil {
		return err
	}
	encToken, err := crypto.EncryptAES256GCM([]byte(install.BotToken), key)
	if err != nil {
		return fmt.Errorf("encrypt bot token: %w", err)
	}

	// Resolve the installed app's name so the auto-created Topic is named
	// after it rather than the workspace's opaque connection id.
	appName := ""
	if appConnID != "" {
		if app, err := s.getSlackAppByID(ctx, appConnID); err == nil {
			appName = app.Name
		}
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
			if appConnID != "" {
				conn.SlackAppConnectionID = appConnID
			}
			if err := s.Store.UpdateServiceConnection(ctx, conn); err != nil {
				return err
			}
			s.helixOrg.slackTopics.ensure(ctx, orgID, conn.ID, conn.Name, appName)
			return nil
		}
	}

	conn := &types.ServiceConnection{
		ID:                   uuid.New().String(),
		OrganizationID:       orgID,
		Name:                 slackWorkspaceName(install),
		Type:                 types.ServiceConnectionTypeSlackWorkspace,
		SlackTeamID:          install.TeamID,
		SlackTeamName:        install.TeamName,
		SlackBotUserID:       install.BotUserID,
		SlackAppID:           install.AppID,
		SlackAppConnectionID: appConnID,
		SlackBotToken:        encToken,
	}
	if err := s.Store.CreateServiceConnection(ctx, conn); err != nil {
		return err
	}
	s.helixOrg.slackTopics.ensure(ctx, orgID, conn.ID, conn.Name, appName)
	return nil
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
	if err := s.deleteSlackWorkspaceAndTopic(r.Context(), org.ID, connID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteSlackWorkspaceAndTopic removes a workspace install and the
// auto-managed Topic bound to it. Shared by the org-facing disconnect
// handler and the slack_app delete cascade.
func (s *HelixAPIServer) deleteSlackWorkspaceAndTopic(ctx context.Context, orgID, connID string) error {
	if err := s.Store.DeleteServiceConnection(ctx, connID); err != nil {
		return err
	}
	s.helixOrg.slackTopics.remove(ctx, orgID, connID)
	return nil
}

// cascadeDeleteSlackAppWorkspaces removes every workspace install made
// from a global slack_app (and each one's Topic) when that app is
// deleted. The installs depend on the app's signing secret / app token
// for inbound delivery, so they are dead without it. Spans all orgs — one
// global app can be installed into many. Best-effort: a failure on one
// workspace is logged and the rest proceed.
func (s *HelixAPIServer) cascadeDeleteSlackAppWorkspaces(ctx context.Context, appConnID string) {
	workspaces, err := s.Store.ListServiceConnectionsByType(ctx, "", types.ServiceConnectionTypeSlackWorkspace)
	if err != nil {
		log.Error().Err(err).Str("app", appConnID).Msg("slack app delete cascade: list workspaces")
		return
	}
	for _, ws := range workspaces {
		if ws.SlackAppConnectionID != appConnID {
			continue
		}
		if err := s.deleteSlackWorkspaceAndTopic(ctx, ws.OrganizationID, ws.ID); err != nil {
			log.Error().Err(err).Str("app", appConnID).Str("workspace", ws.ID).Msg("slack app delete cascade: delete workspace")
		}
	}
}

// reactToServiceConnectionChange is the post-mutation hook helix-org
// registers on the core service-connection handlers (in mountHelixOrg).
// It reacts only to the type helix-org owns: a slack_app create/edit
// reconciles Socket Mode, and a slack_app delete also cascade-removes the
// workspace installs (and their Topics) made from it. Every other
// connection type (github_app, ado_…) is ignored, so the generic handlers
// never need to know helix-org exists.
func (s *HelixAPIServer) reactToServiceConnectionChange(ctx context.Context, conn *types.ServiceConnection, deleted bool) {
	if conn.Type != types.ServiceConnectionTypeSlackApp {
		return
	}
	if deleted {
		s.cascadeDeleteSlackAppWorkspaces(ctx, conn.ID)
	}
	s.helixOrg.kickSlackSocket()
}
