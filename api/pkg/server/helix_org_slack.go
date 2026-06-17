package server

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/dispatch"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// transportSlackKind aliases the Slack transport Kind for the wiring.
const transportSlackKind = transport.KindSlack

// slackSocketIdle is how long the Socket Mode connector waits before
// re-checking config when Socket Mode isn't currently active.
const slackSocketIdle = 30 * time.Second

// noopLocker is the single-owner Locker used when no Postgres handle is
// available (e.g. tests / non-Postgres dev): it always grants the lock,
// which is correct for a single-replica deployment.
type noopLocker struct{}

func (noopLocker) TryLock(context.Context) (bool, error) { return true, nil }
func (noopLocker) Unlock(context.Context) error          { return nil }

// sleepCtxServer sleeps for d unless ctx is cancelled. Returns false on
// cancellation.
func sleepCtxServer(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// resolveAnyBotToken returns the bot token of the single Slack install
// (the Socket Mode on-premise case is one workspace). It scans the orgs
// that own Slack streams and returns the first configured bot token.
func resolveAnyBotToken(ctx context.Context, reg *configregistry.Registry, st *helixorgstore.Store) string {
	streams, err := st.Streams.ListByTransportKind(ctx, transport.KindSlack)
	if err != nil {
		return ""
	}
	seen := map[string]bool{}
	for _, s := range streams {
		if seen[s.OrganizationID] {
			continue
		}
		seen[s.OrganizationID] = true
		var cfg slacktransport.Config
		if err := reg.GetObject(ctx, s.OrganizationID, "transport.slack", &cfg); err == nil && cfg.BotToken != "" {
			return cfg.BotToken
		}
	}
	return ""
}

// slackOAuthScopes are the bot scopes the per-org "Add to Slack" install
// requests. chat:write posts as personas; channels:* + groups:read let
// the bot join and read bound channels; app_mentions:read surfaces
// @mentions of the bot.
var slackOAuthScopes = []string{
	"chat:write",
	"channels:read",
	"channels:history",
	"channels:join",
	"groups:read",
	"app_mentions:read",
}

// slackWiring bundles the HTTP handlers and the Socket Mode runner the
// composition root mounts/starts for the Slack transport.
type slackWiring struct {
	// events is the REST Events API handler (POST /slack/events). One
	// global endpoint; the ingest routes to the right org by team id.
	events http.Handler
	// oauthStart begins the per-org "Add to Slack" install
	// (GET /orgs/{org}/slack/oauth/start). Auth-routed (an admin clicks
	// it from settings); it redirects to Slack's authorize page.
	oauthStart http.Handler
	// oauthCallback completes the install (GET /slack/oauth/callback).
	// Insecure — it's a top-level redirect from slack.com authenticated
	// by the encrypted state param.
	oauthCallback http.Handler
	// runSocket runs the Socket Mode ingress under a single-owner lock.
	// No-op unless the global app is enabled with ingress mode "socket".
	runSocket func(ctx context.Context)
}

// slackWiringDeps are the collaborators buildSlackWiring needs from the
// composition root.
type slackWiringDeps struct {
	configReg     *configregistry.Registry
	orgStore      *helixorgstore.Store
	broadcaster   *wakebus.Bus
	dispatcher    *dispatch.Dispatcher
	helixStore    helixstore.Store
	sqlDB         *sql.DB // for the Socket Mode single-owner advisory lock; nil ⇒ no-op lock
	encryptionKey func() ([]byte, error)
	publicURL     string // externally-reachable base URL for the OAuth redirect
	logger        *slog.Logger
}

// buildSlackWiring assembles the Slack transport's ingest, outbound,
// provisioner, OAuth install, and ingress sources from the global app
// (an OAuthProvider(type=slack) row) and the per-org install config.
// It registers the outbound emitter on the dispatcher and returns the
// handlers + socket runner for the server to mount/start.
func buildSlackWiring(deps slackWiringDeps) slackWiring {
	logger := deps.logger
	if logger == nil {
		logger = slog.Default()
	}

	// GlobalApp port: the single admin-configured Slack app. Read live so
	// enabling/disabling or switching ingress mode in the admin panel
	// takes effect without a restart.
	globalApp := func(ctx context.Context) (slacktransport.App, error) {
		providers, err := deps.helixStore.ListOAuthProviders(ctx, &helixstore.ListOAuthProvidersQuery{
			Type:    string(types.OAuthProviderTypeSlack),
			Enabled: true,
		})
		if err != nil {
			return slacktransport.App{}, err
		}
		if len(providers) == 0 {
			return slacktransport.App{}, slacktransport.ErrNoApp
		}
		p := providers[0]
		return slacktransport.App{
			ClientID:      p.ClientID,
			ClientSecret:  p.ClientSecret,
			SigningSecret: p.SlackSigningSecret,
			AppToken:      p.SlackAppToken,
			BotToken:      p.SlackBotToken,
			IngressMode:   p.SlackIngressMode,
			Enabled:       p.Enabled,
		}, nil
	}

	// The one shared inbound path. Single instance for every org; Receive
	// resolves the owning org from the event's team id.
	ingest := slacktransport.NewIngest(deps.configReg, deps.orgStore, deps.broadcaster, deps.dispatcher, logger)

	// Outbound: register the persona-posting emitter so worker publishes
	// on KindSlack streams reach Slack (ingress-agnostic, FR-12).
	outbound := slacktransport.NewOutbound(deps.configReg, deps.orgStore, slacktransport.DefaultPersonaResolver, logger)
	deps.dispatcher.RegisterOutbound(transportSlackKind, outbound)

	// REST Events API handler — verifies the global app's signing secret.
	// Returning "" makes the handler inert when no app is configured (FR-3).
	signingSecret := func(ctx context.Context) (string, error) {
		app, err := globalApp(ctx)
		if err != nil || !app.Enabled {
			return "", nil
		}
		return app.SigningSecret, nil
	}
	events := slacktransport.NewEventsAPI(ingest, signingSecret, logger).Handler()

	// OAuth install flow. The org id rides through the round trip in the
	// encrypted state param.
	encode := func(orgID string) (string, error) {
		key, err := deps.encryptionKey()
		if err != nil {
			return "", err
		}
		return crypto.EncryptAES256GCM([]byte(orgID), key)
	}
	decode := func(state string) (string, error) {
		key, err := deps.encryptionKey()
		if err != nil {
			return "", err
		}
		plain, err := crypto.DecryptAES256GCM(state, key)
		if err != nil {
			return "", err
		}
		return string(plain), nil
	}
	redirectURI := deps.publicURL + APIPrefix + "/slack/oauth/callback"
	oauth := slacktransport.NewOAuth(globalApp, deps.configReg, encode, decode, redirectURI, logger)

	oauthStart := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The orgID is resolved + membership-authorised by the
		// withHelixOrgScope wrapper this handler is mounted behind, then
		// stashed on the context. Reading it from there (rather than
		// re-deriving from the path) is what prevents a non-member from
		// starting an install bound to another org's workspace.
		orgID := helixorgserver.OrgIDFromContext(r.Context())
		if orgID == "" {
			http.Error(w, "missing org", http.StatusBadRequest)
			return
		}
		url, err := oauth.StartURL(r.Context(), orgID, slackOAuthScopes)
		if err != nil {
			http.Error(w, "slack not configured: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		http.Redirect(w, r, url, http.StatusFound)
	})

	oauthCallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		if code == "" || state == "" {
			http.Error(w, "missing code or state", http.StatusBadRequest)
			return
		}
		if err := oauth.HandleCallback(r.Context(), code, state); err != nil {
			logger.Error("slack.oauth.callback", "err", err)
			http.Error(w, "slack install failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		// Land the admin back on the app's settings page.
		http.Redirect(w, r, "/?slack_installed=1", http.StatusFound)
	})

	// Socket Mode runner. The connector reads the global app + a resolved
	// bot token fresh on each connect, so it stays inert until the admin
	// switches ingress mode to "socket" and an org has installed.
	runSocket := func(ctx context.Context) {
		var locker slacktransport.Locker
		if deps.sqlDB != nil {
			locker = slacktransport.NewPgAdvisoryLock(deps.sqlDB, slacktransport.SocketModeLockKey)
		} else {
			locker = noopLocker{}
		}
		owner := slacktransport.NewSingleOwner(locker, logger)
		connector := func(ctx context.Context, handle func(teamID string, ev slacktransport.Event)) error {
			app, err := globalApp(ctx)
			if err != nil || !app.Enabled || app.IngressMode != slacktransport.IngressSocket || app.AppToken == "" {
				// Not configured for Socket Mode — idle until it might be.
				if !sleepCtxServer(ctx, slackSocketIdle) {
					return ctx.Err()
				}
				return nil
			}
			// Socket Mode posts with the single-workspace bot token
			// configured on the global app. Fall back to scanning per-org
			// installs for the rare case the workspace was onboarded via
			// the OAuth flow rather than a pasted token.
			botToken := app.BotToken
			if botToken == "" {
				botToken = resolveAnyBotToken(ctx, deps.configReg, deps.orgStore)
			}
			return slacktransport.NewSlackConnector(app.AppToken, botToken, "", logger)(ctx, handle)
		}
		runner := slacktransport.NewSocketMode(ingest, owner, connector, logger)
		if err := runner.Run(ctx); err != nil && ctx.Err() == nil {
			logger.Error("slack.socketmode: runner exited", "err", err)
		}
	}

	return slackWiring{
		events:        events,
		oauthStart:    oauthStart,
		oauthCallback: oauthCallback,
		runSocket:     runSocket,
	}
}
