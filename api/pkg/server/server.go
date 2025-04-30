package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/controller/knowledge"
	"github.com/helixml/helix/api/pkg/gptscript"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/server/spa"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/stripe"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/version"

	"crypto/tls"
	"crypto/x509"
	"net"
	_ "net/http/pprof" // enable profiling
)

const APIPrefix = "/api/v1"

type Options struct {
	Config      *config.ServerConfig
	URL         string
	Host        string
	Port        int
	FrontendURL string // Can either be a URL to frontend or a path to static files
	RunnerToken string
	// a list of keycloak ids that are considered admins
	// if the string '*' is included it means ALL users
	AdminIDs []string
	// if this is specified then we provide the option to clone entire
	// sessions into this user without having to logout and login
	EvalUserID string
	// this is for when we are running localfs filesystem
	// and we need to add a route to view files based on their path
	// we are assuming all file storage is open right now
	// so we just deep link to the object path and don't apply auth
	// (this is so helix nodes can see files)
	// later, we might add a token to the URLs
	LocalFilestorePath string
}

type HelixAPIServer struct {
	Cfg               *config.ServerConfig
	Store             store.Store
	Stripe            *stripe.Stripe
	Controller        *controller.Controller
	Janitor           *janitor.Janitor
	authMiddleware    *authMiddleware
	pubsub            pubsub.PubSub
	providerManager   manager.ProviderManager
	gptScriptExecutor gptscript.Executor
	inferenceServer   *openai.InternalHelixServer
	knowledgeManager  knowledge.Manager
	router            *mux.Router
	scheduler         *scheduler.Scheduler
	pingService       *version.PingService
	oidcClient        auth.OIDC
	oauthManager      *oauth.Manager
	fileServerHandler http.Handler
	cache             *ristretto.Cache[string, string]
}

func NewServer(
	cfg *config.ServerConfig,
	store store.Store,
	ps pubsub.PubSub,
	gptScriptExecutor gptscript.Executor,
	providerManager manager.ProviderManager,
	inferenceServer *openai.InternalHelixServer,
	authenticator auth.Authenticator,
	stripe *stripe.Stripe,
	controller *controller.Controller,
	janitor *janitor.Janitor,
	knowledgeManager knowledge.Manager,
	scheduler *scheduler.Scheduler,
	pingService *version.PingService,
	oauthManager *oauth.Manager,
) (*HelixAPIServer, error) {
	if cfg.WebServer.URL == "" {
		return nil, fmt.Errorf("server url is required")
	}

	if cfg.WebServer.Host == "" {
		return nil, fmt.Errorf("server host is required")
	}

	if cfg.WebServer.Port == 0 {
		return nil, fmt.Errorf("server port is required")
	}

	if cfg.WebServer.RunnerToken == "" {
		return nil, fmt.Errorf("runner token is required")
	}

	helixRedirectURL := fmt.Sprintf("%s/api/v1/auth/callback", cfg.WebServer.URL)
	var oidcClient auth.OIDC
	if cfg.OIDC.Enabled {
		if cfg.OIDC.Audience == "" {
			return nil, fmt.Errorf("oidc audience is required")
		}
		client, err := auth.NewOIDCClient(controller.Ctx, auth.OIDCConfig{
			ProviderURL:  cfg.OIDC.URL,
			ClientID:     cfg.OIDC.ClientID,
			ClientSecret: cfg.OIDC.ClientSecret,
			RedirectURL:  helixRedirectURL,
			AdminUserIDs: cfg.WebServer.AdminIDs,
			AdminUserSrc: cfg.WebServer.AdminSrc,
			Audience:     cfg.OIDC.Audience,
			Scopes:       strings.Split(cfg.OIDC.Scopes, ","),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create oidc client: %w", err)
		}
		oidcClient = client
	} else if cfg.Keycloak.KeycloakEnabled {
		keycloakURL, err := url.Parse(cfg.Keycloak.KeycloakFrontEndURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse keycloak front end url: %w", err)
		}
		// Strip any trailing slashes from the path
		keycloakURL.Path = strings.TrimRight(keycloakURL.Path, "/")
		keycloakURL.Path = fmt.Sprintf("%s/realms/%s", keycloakURL.Path, cfg.Keycloak.Realm)
		client, err := auth.NewOIDCClient(controller.Ctx, auth.OIDCConfig{
			ProviderURL:  keycloakURL.String(),
			ClientID:     cfg.Keycloak.APIClientID,
			ClientSecret: cfg.Keycloak.ClientSecret,
			RedirectURL:  helixRedirectURL,
			AdminUserIDs: cfg.WebServer.AdminIDs,
			AdminUserSrc: cfg.WebServer.AdminSrc,
			Audience:     "account",
			Scopes:       []string{"openid", "profile", "email"},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create keycloak client: %w", err)
		}
		oidcClient = client
	}
	if oidcClient == nil {
		return nil, fmt.Errorf("no oidc client found")
	}

	cache, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	return &HelixAPIServer{
		Cfg:               cfg,
		Store:             store,
		Stripe:            stripe,
		Controller:        controller,
		Janitor:           janitor,
		gptScriptExecutor: gptScriptExecutor,
		inferenceServer:   inferenceServer,
		authMiddleware: newAuthMiddleware(
			oidcClient,
			authenticator,
			store,
			authMiddlewareConfig{
				adminUserIDs: cfg.WebServer.AdminIDs,
				adminUserSrc: cfg.WebServer.AdminSrc,
				runnerToken:  cfg.WebServer.RunnerToken,
			},
		),
		providerManager:   providerManager,
		pubsub:            ps,
		knowledgeManager:  knowledgeManager,
		scheduler:         scheduler,
		pingService:       pingService,
		oidcClient:        oidcClient,
		oauthManager:      oauthManager,
		fileServerHandler: http.FileServer(neuteredFileSystem{http.Dir(cfg.FileStore.LocalFSPath)}),
		cache:             cache,
	}, nil
}

func (apiServer *HelixAPIServer) ListenAndServe(ctx context.Context, _ *system.CleanupManager) error {
	apiRouter, err := apiServer.registerRoutes(ctx)
	if err != nil {
		return err
	}

	apiServer.startUserWebSocketServer(
		ctx,
		apiRouter,
		"/ws/user",
	)

	apiServer.startRunnerWebSocketServer(
		ctx,
		apiRouter,
		"/ws/runner",
	)

	apiServer.startGptScriptRunnerWebSocketServer(
		apiRouter,
		"/ws/gptscript-runner",
	)

	// Start UNIX socket server for embeddings if configured
	if apiServer.Cfg.WebServer.EmbeddingsSocket != "" {
		go func() {
			if err := apiServer.startEmbeddingsSocketServer(ctx); err != nil {
				log.Error().Err(err).Msg("failed to start embeddings socket server")
			}
		}()
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", apiServer.Cfg.WebServer.Host, apiServer.Cfg.WebServer.Port),
		WriteTimeout:      time.Minute * 30,
		ReadTimeout:       time.Minute * 30,
		ReadHeaderTimeout: time.Minute * 30,
		IdleTimeout:       time.Minute * 60,
		Handler:           apiServer.router,
	}
	return srv.ListenAndServe()
}

func matchAllRoutes(*http.Request, *mux.RouteMatch) bool {
	return true
}

func (apiServer *HelixAPIServer) registerRoutes(_ context.Context) (*mux.Router, error) {
	router := mux.NewRouter()
	err := apiServer.Janitor.InjectMiddleware(router)
	if err != nil {
		return nil, err
	}

	// we do token extraction for all routes
	// if there is a token we will assign the user if not then oh well no user it's all gravy
	router.Use(ErrorLoggingMiddleware)

	// insecure router is under /api/v1 but not protected by auth
	insecureRouter := router.PathPrefix(APIPrefix).Subrouter()

	// any route that lives under /api/v1
	subRouter := router.PathPrefix(APIPrefix).Subrouter()

	subRouter.Use(apiServer.authMiddleware.extractMiddleware)

	// auth router requires a valid token from keycloak or api key
	authRouter := subRouter.MatcherFunc(matchAllRoutes).Subrouter()
	authRouter.Use(requireUser)

	// runner router requires a valid runner token
	runnerRouter := subRouter.MatcherFunc(matchAllRoutes).Subrouter()
	runnerRouter.Use(requireRunner)

	// admin auth requires a user with admin flag
	adminRouter := authRouter.MatcherFunc(matchAllRoutes).Subrouter()
	adminRouter.Use(requireAdmin)

	// Setup OAuth routes with the auth router (except for callback)
	apiServer.setupOAuthRoutes(authRouter)

	// Setup OAuth callback route (no auth required)
	insecureRouter.HandleFunc("/oauth/flow/callback", apiServer.handleOAuthCallback).Methods("GET")

	subRouter.HandleFunc("/config", system.DefaultWrapperWithConfig(apiServer.config, system.WrapperConfig{
		SilenceErrors: true,
	})).Methods(http.MethodGet)

	subRouter.HandleFunc("/config/js", apiServer.configJS).Methods(http.MethodGet)
	subRouter.Handle("/swagger", apiServer.swaggerHandler()).Methods(http.MethodGet)

	// this is not authenticated because we use the webhook signing secret
	// the stripe library handles http management
	subRouter.HandleFunc("/stripe/webhook", apiServer.subscriptionWebhook).Methods(http.MethodPost)

	authRouter.HandleFunc("/github/status", system.DefaultWrapper(apiServer.githubStatus)).Methods(http.MethodGet)
	authRouter.HandleFunc("/github/callback", apiServer.githubCallback).Methods(http.MethodGet)
	authRouter.HandleFunc("/github/repos", system.DefaultWrapper(apiServer.listGithubRepos)).Methods(http.MethodGet)
	subRouter.HandleFunc("/github/webhook", apiServer.githubWebhook).Methods(http.MethodPost)

	authRouter.HandleFunc("/status", system.DefaultWrapper(apiServer.status)).Methods(http.MethodGet)

	// the auth here is handled because we prefix the user path based on the auth context
	// e.g. /sessions/123 becomes /users/456/sessions/123
	// so - the point is, the auth is done by injecting the user id based on the token
	authRouter.HandleFunc("/filestore/config", system.DefaultWrapper(apiServer.filestoreConfig)).Methods(http.MethodGet)
	authRouter.HandleFunc("/filestore/list", system.DefaultWrapper(apiServer.filestoreList)).Methods(http.MethodGet)
	authRouter.HandleFunc("/filestore/get", system.DefaultWrapper(apiServer.filestoreGet)).Methods(http.MethodGet)
	authRouter.HandleFunc("/filestore/folder", system.DefaultWrapper(apiServer.filestoreCreateFolder)).Methods(http.MethodPost)
	authRouter.HandleFunc("/filestore/upload", system.DefaultWrapper(apiServer.filestoreUpload)).Methods(http.MethodPost)
	authRouter.HandleFunc("/filestore/rename", system.DefaultWrapper(apiServer.filestoreRename)).Methods(http.MethodPut)
	authRouter.HandleFunc("/filestore/delete", system.DefaultWrapper(apiServer.filestoreDelete)).Methods(http.MethodDelete)
	// Authentication is done within the handler itself based on API path
	subRouter.PathPrefix("/filestore/viewer/").Handler(http.StripPrefix(APIPrefix+"/filestore/viewer/", http.HandlerFunc(apiServer.filestoreViewerHandler)))

	authRouter.HandleFunc("/data_entities", system.DefaultWrapper(apiServer.createDataEntity)).Methods(http.MethodPost)

	authRouter.HandleFunc("/subscription/new", system.DefaultWrapper(apiServer.subscriptionCreate)).Methods(http.MethodPost)
	authRouter.HandleFunc("/subscription/manage", system.DefaultWrapper(apiServer.subscriptionManage)).Methods(http.MethodPost)

	authRouter.HandleFunc("/api_keys", system.DefaultWrapper(apiServer.createAPIKey)).Methods(http.MethodPost)
	authRouter.HandleFunc("/api_keys", system.DefaultWrapper(apiServer.getAPIKeys)).Methods(http.MethodGet)
	authRouter.HandleFunc("/api_keys", system.DefaultWrapper(apiServer.deleteAPIKey)).Methods(http.MethodDelete)
	authRouter.HandleFunc("/api_keys/check", system.DefaultWrapper(apiServer.checkAPIKey)).Methods(http.MethodGet)

	// User search endpoint
	authRouter.HandleFunc("/users/search", apiServer.searchUsers).Methods(http.MethodGet)

	// OpenAI API compatible routes
	router.HandleFunc("/v1/chat/completions", apiServer.authMiddleware.auth(apiServer.createChatCompletion)).Methods(http.MethodPost, http.MethodOptions)
	router.HandleFunc("/v1/embeddings", apiServer.authMiddleware.auth(apiServer.createEmbeddings)).Methods(http.MethodPost, http.MethodOptions)
	router.HandleFunc("/v1/models", apiServer.authMiddleware.auth(apiServer.listModels)).Methods(http.MethodGet)
	// Azure OpenAI API compatible routes
	router.HandleFunc("/openai/deployments/{model}/chat/completions", apiServer.authMiddleware.auth(apiServer.createChatCompletion)).Methods(http.MethodPost, http.MethodOptions)

	authRouter.HandleFunc("/providers", apiServer.listProviders).Methods(http.MethodGet)

	// Insecure router as unauthenticated users will see all public provider endpoints
	subRouter.HandleFunc("/provider-endpoints", apiServer.listProviderEndpoints).Methods(http.MethodGet)

	authRouter.HandleFunc("/provider-endpoints", apiServer.createProviderEndpoint).Methods(http.MethodPost)
	authRouter.HandleFunc("/provider-endpoints/{id}", apiServer.updateProviderEndpoint).Methods(http.MethodPut)
	authRouter.HandleFunc("/provider-endpoints/{id}", apiServer.deleteProviderEndpoint).Methods(http.MethodDelete)
	authRouter.HandleFunc("/provider-endpoints/{id}/daily-usage", apiServer.getProviderDailyUsage).Methods(http.MethodGet)
	authRouter.HandleFunc("/provider-endpoints/{id}/users-daily-usage", apiServer.getProviderUsersDailyUsage).Methods(http.MethodGet)
	// Helix inference route
	authRouter.HandleFunc("/sessions/chat", apiServer.startChatSessionHandler).Methods(http.MethodPost)

	// Helix learn route (i.e. create fine tune and/or RAG source)
	authRouter.HandleFunc("/sessions/learn", apiServer.startLearnSessionHandler).Methods(http.MethodPost)

	authRouter.HandleFunc("/sessions", system.DefaultWrapper(apiServer.getSessions)).Methods(http.MethodGet)

	subRouter.HandleFunc("/sessions/{id}", system.Wrapper(apiServer.getSession)).Methods(http.MethodGet)
	subRouter.HandleFunc("/sessions/{id}/summary", system.Wrapper(apiServer.getSessionSummary)).Methods(http.MethodGet)
	authRouter.HandleFunc("/sessions/{id}", system.Wrapper(apiServer.updateSession)).Methods(http.MethodPut)
	authRouter.HandleFunc("/sessions/{id}", system.Wrapper(apiServer.deleteSession)).Methods(http.MethodDelete)
	authRouter.HandleFunc("/sessions/{id}/restart", system.Wrapper(apiServer.restartSession)).Methods(http.MethodPut)
	authRouter.HandleFunc("/sessions/{id}/config", system.Wrapper(apiServer.updateSessionConfig)).Methods(http.MethodPut)

	authRouter.HandleFunc("/sessions/{id}/meta", system.Wrapper(apiServer.updateSessionMeta)).Methods(http.MethodPut)
	authRouter.HandleFunc("/sessions/{id}/finetune/start", system.Wrapper(apiServer.startSessionFinetune)).Methods(http.MethodPost)
	authRouter.HandleFunc("/sessions/{id}/finetune/documents", system.Wrapper(apiServer.finetuneAddDocuments)).Methods(http.MethodPut)
	authRouter.HandleFunc("/sessions/{id}/finetune/clone/{interaction}/{mode}", system.Wrapper(apiServer.cloneFinetuneInteraction)).Methods(http.MethodPost)
	authRouter.HandleFunc("/sessions/{id}/finetune/text/retry", system.Wrapper(apiServer.retryTextFinetune)).Methods(http.MethodPut)
	subRouter.HandleFunc("/sessions/{id}/finetune/text/conversations/{interaction}", system.Wrapper(apiServer.getSessionFinetuneConversation)).Methods(http.MethodGet)
	authRouter.HandleFunc("/sessions/{id}/finetune/text/conversations/{interaction}", system.Wrapper(apiServer.setSessionFinetuneConversation)).Methods(http.MethodPut)

	authRouter.HandleFunc("/secrets", system.Wrapper(apiServer.listSecrets)).Methods(http.MethodGet)
	authRouter.HandleFunc("/secrets", system.Wrapper(apiServer.createSecret)).Methods(http.MethodPost)
	authRouter.HandleFunc("/secrets/{id}", system.Wrapper(apiServer.updateSecret)).Methods(http.MethodPut)
	authRouter.HandleFunc("/secrets/{id}", system.Wrapper(apiServer.deleteSecret)).Methods(http.MethodDelete)

	authRouter.HandleFunc("/apps", system.Wrapper(apiServer.listApps)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps", system.Wrapper(apiServer.createApp)).Methods(http.MethodPost)
	authRouter.HandleFunc("/apps/{id}", system.Wrapper(apiServer.getApp)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}", system.Wrapper(apiServer.updateApp)).Methods(http.MethodPut)
	authRouter.HandleFunc("/apps/github/{id}", system.Wrapper(apiServer.updateGithubApp)).Methods(http.MethodPut)
	authRouter.HandleFunc("/apps/{id}", system.Wrapper(apiServer.deleteApp)).Methods(http.MethodDelete)
	authRouter.HandleFunc("/apps/{id}/daily-usage", system.Wrapper(apiServer.getAppDailyUsage)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}/users-daily-usage", system.Wrapper(apiServer.getAppUsersDailyUsage)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}/llm-calls", system.Wrapper(apiServer.listAppLLMCalls)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}/api-actions", system.Wrapper(apiServer.appRunAPIAction)).Methods(http.MethodPost)
	authRouter.HandleFunc("/apps/{id}/user-access", system.Wrapper(apiServer.getAppUserAccess)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}/access-grants", apiServer.listAppAccessGrants).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}/access-grants", apiServer.createAppAccessGrant).Methods(http.MethodPost)
	authRouter.HandleFunc("/apps/{id}/access-grants/{grant_id}", apiServer.deleteAppAccessGrant).Methods(http.MethodDelete)

	// Template app routes
	authRouter.HandleFunc("/template-apps", system.DefaultWrapper(apiServer.listTemplateApps)).Methods(http.MethodGet)
	authRouter.HandleFunc("/template-apps/{type}", system.DefaultWrapper(apiServer.getTemplateApp)).Methods(http.MethodGet)

	authRouter.HandleFunc("/search", system.Wrapper(apiServer.knowledgeSearch)).Methods(http.MethodGet)

	authRouter.HandleFunc("/knowledge", system.Wrapper(apiServer.listKnowledge)).Methods(http.MethodGet)
	authRouter.HandleFunc("/knowledge/{id}", system.Wrapper(apiServer.getKnowledge)).Methods(http.MethodGet)
	authRouter.HandleFunc("/knowledge/{id}", system.Wrapper(apiServer.deleteKnowledge)).Methods(http.MethodDelete)
	authRouter.HandleFunc("/knowledge/{id}/refresh", system.Wrapper(apiServer.refreshKnowledge)).Methods(http.MethodPost)
	authRouter.HandleFunc("/knowledge/{id}/complete", system.Wrapper(apiServer.completeKnowledgePreparation)).Methods(http.MethodPost)
	authRouter.HandleFunc("/knowledge/{id}/versions", system.Wrapper(apiServer.listKnowledgeVersions)).Methods(http.MethodGet)

	// UI @ functionality
	authRouter.HandleFunc("/context-menu", system.Wrapper(apiServer.contextMenuHandler)).Methods(http.MethodGet)

	// User auth, BFF
	insecureRouter.HandleFunc("/auth/login", apiServer.login).Methods(http.MethodPost)
	insecureRouter.HandleFunc("/auth/callback", apiServer.callback).Methods(http.MethodGet)
	insecureRouter.HandleFunc("/auth/user", apiServer.user).Methods(http.MethodGet)
	insecureRouter.HandleFunc("/auth/logout", apiServer.logout).Methods(http.MethodPost)
	insecureRouter.HandleFunc("/auth/authenticated", apiServer.authenticated).Methods(http.MethodGet)
	insecureRouter.HandleFunc("/auth/refresh", apiServer.refresh).Methods(http.MethodPost)

	// Orgs, authz
	authRouter.HandleFunc("/organizations", apiServer.listOrganizations).Methods(http.MethodGet)
	authRouter.HandleFunc("/organizations", apiServer.createOrganization).Methods(http.MethodPost)
	authRouter.HandleFunc("/organizations/{id}", apiServer.getOrganization).Methods(http.MethodGet)
	authRouter.HandleFunc("/organizations/{id}", apiServer.updateOrganization).Methods(http.MethodPut)
	authRouter.HandleFunc("/organizations/{id}", apiServer.deleteOrganization).Methods(http.MethodDelete)
	authRouter.HandleFunc("/organizations/{id}/members", apiServer.listOrganizationMembers).Methods(http.MethodGet)
	authRouter.HandleFunc("/organizations/{id}/members", apiServer.addOrganizationMember).Methods(http.MethodPost)
	authRouter.HandleFunc("/organizations/{id}/members/{user_id}", apiServer.removeOrganizationMember).Methods(http.MethodDelete)
	authRouter.HandleFunc("/organizations/{id}/members/{user_id}", apiServer.updateOrganizationMember).Methods(http.MethodPut)

	authRouter.HandleFunc("/organizations/{id}/roles", apiServer.listOrganizationRoles).Methods(http.MethodGet)

	// Teams
	authRouter.HandleFunc("/organizations/{id}/teams", apiServer.listTeams).Methods(http.MethodGet)
	authRouter.HandleFunc("/organizations/{id}/teams", apiServer.createTeam).Methods(http.MethodPost)
	authRouter.HandleFunc("/organizations/{id}/teams/{team_id}", apiServer.updateTeam).Methods(http.MethodPut)
	authRouter.HandleFunc("/organizations/{id}/teams/{team_id}", apiServer.deleteTeam).Methods(http.MethodDelete)
	authRouter.HandleFunc("/organizations/{id}/teams/{team_id}/members", apiServer.listTeamMembers).Methods(http.MethodGet)
	authRouter.HandleFunc("/organizations/{id}/teams/{team_id}/members", apiServer.addTeamMember).Methods(http.MethodPost)
	authRouter.HandleFunc("/organizations/{id}/teams/{team_id}/members/{user_id}", apiServer.removeTeamMember).Methods(http.MethodDelete)

	// we know which app this is by the token that is used (which is linked to the app)
	// this is so frontend devs don't need anything other than their access token
	// and can auto-connect to this endpoint
	// we handle CORs by loading the app from the token.app_id and it knowing which domains are allowed
	authRouter.HandleFunc("/apps/script", system.Wrapper(apiServer.appRunScript)).Methods(http.MethodPost, http.MethodOptions)
	adminRouter.HandleFunc("/dashboard", system.DefaultWrapper(apiServer.dashboard)).Methods(http.MethodGet)
	adminRouter.HandleFunc("/llm_calls", system.Wrapper(apiServer.listLLMCalls)).Methods(http.MethodGet)

	// Helix models
	authRouter.HandleFunc("/helix-models", apiServer.listHelixModels).Methods(http.MethodGet)
	// only admins can create, update, or delete helix models
	adminRouter.HandleFunc("/helix-models", apiServer.createHelixModel).Methods(http.MethodPost)
	adminRouter.HandleFunc("/helix-models/{id:.*}", apiServer.updateHelixModel).Methods(http.MethodPut)
	adminRouter.HandleFunc("/helix-models/{id:.*}", apiServer.deleteHelixModel).Methods(http.MethodDelete)

	// all these routes are secured via runner tokens
	insecureRouter.HandleFunc("/runner/{runner_id}/ws", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		runnerID := vars["runner_id"]
		log.Info().
			Str("runner_id", runnerID).
			Str("request_path", r.URL.Path).
			Msg("proxying runner websocket request to nats")

		// Upgrade the incoming HTTP connection to a WebSocket connection.
		upgrader := websocket.Upgrader{
			// TODO(Phil): check origin
			CheckOrigin: func(r *http.Request) bool {
				log.Debug().Interface("headers", r.Header).Interface("vars", r.RemoteAddr).Msg("nats check origin")
				return true
			},
		}

		clientConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Err(err).Msg("Failed to upgrade client connection")
			return
		}
		// Ensure the client connection is closed on function exit.
		defer clientConn.Close()

		// Connect to the backend WebSocket server.
		backendConn, resp, err := websocket.DefaultDialer.Dial("ws://localhost:8433", nil) // TODO(Phil): make this configurable
		if err != nil {
			log.Error().Err(err).Msg("Failed to connect to backend WebSocket server")
			return
		}
		// Ensure the backend connection is closed on function exit.
		defer backendConn.Close()
		defer resp.Body.Close()

		// Start two goroutines to copy data between the client and the backend.
		errCh := make(chan error, 2)

		// Copy messages from the client to the backend.
		go func() {
			for {
				messageType, message, err := clientConn.ReadMessage()
				if err != nil {
					errCh <- err
					return
				}
				if err := backendConn.WriteMessage(messageType, message); err != nil {
					errCh <- err
					return
				}
			}
		}()

		// Copy messages from the backend to the client.
		go func() {
			for {
				messageType, message, err := backendConn.ReadMessage()
				if err != nil {
					errCh <- err
					return
				}
				if err := clientConn.WriteMessage(messageType, message); err != nil {
					errCh <- err
					return
				}
			}
		}()

		// Wait until one side returns an error (or closes the connection).
		if err := <-errCh; err != nil {
			log.Printf("WebSocket proxy error: %v", err)
		}
	})
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/download/file", apiServer.runnerSessionDownloadFile).Methods(http.MethodGet)
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/download/folder", apiServer.runnerSessionDownloadFolder).Methods(http.MethodGet)
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/upload/files", system.DefaultWrapper(apiServer.runnerSessionUploadFiles)).Methods(http.MethodPost)
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/upload/folder", system.DefaultWrapper(apiServer.runnerSessionUploadFolder)).Methods(http.MethodPost)

	// register pprof routes
	router.PathPrefix("/debug/pprof/").Handler(http.DefaultServeMux)

	// proxy /admin -> keycloak
	apiServer.registerKeycloakHandler(router)

	// proxy other routes to frontend
	apiServer.registerDefaultHandler(router)

	// only admins can manage licenses
	adminRouter.HandleFunc("/license", apiServer.handleGetLicenseKey).Methods("GET")
	adminRouter.HandleFunc("/license", apiServer.handleSetLicenseKey).Methods("POST")

	// OAuth routes
	// These routes are already set up by apiServer.setupOAuthRoutes(authRouter) above

	apiServer.router = router

	return subRouter, nil
}

func getID(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["id"]
}

func (apiServer *HelixAPIServer) registerKeycloakHandler(router *mux.Router) {
	if !apiServer.Cfg.Keycloak.KeycloakEnabled {
		log.Info().Msg("Keycloak is disabled, skipping proxy")
		return
	}
	u, err := url.Parse(apiServer.Cfg.Keycloak.KeycloakURL)
	if err != nil {
		log.Error().Err(err).Msg("failed to parse keycloak URL, authentication might not work")
		return
	}

	// Strip path prefix, otherwise we would have to use /auth/auth/realms/helix/protocol/openid-connect/token
	u.Path = ""

	proxy := httputil.NewSingleHostReverseProxy(u)

	// Create transport with custom CA support
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   300 * time.Second, // Increased from 30 to 300 seconds
			KeepAlive: 300 * time.Second, // Increased from 30 to 300 seconds
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Load system cert pool
	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		rootCAs = x509.NewCertPool()
	}

	// Check for custom CA cert file
	if apiServer.Cfg.SSL.SSLCertFile != "" {
		cert, err := os.ReadFile(apiServer.Cfg.SSL.SSLCertFile)
		if err != nil {
			log.Error().Err(err).Str("file", apiServer.Cfg.SSL.SSLCertFile).Msg("Error reading custom CA cert file")
		} else if ok := rootCAs.AppendCertsFromPEM(cert); !ok {
			log.Error().Str("file", apiServer.Cfg.SSL.SSLCertFile).Msg("Failed to append custom CA cert to pool")
		} else {
			log.Info().Str("file", apiServer.Cfg.SSL.SSLCertFile).Msg("Added custom CA cert")
		}
	}

	// Check for custom CA cert directory
	if apiServer.Cfg.SSL.SSLCertDir != "" {
		files, err := os.ReadDir(apiServer.Cfg.SSL.SSLCertDir)
		if err != nil {
			log.Error().Err(err).Str("dir", apiServer.Cfg.SSL.SSLCertDir).Msg("Error reading cert directory")
		} else {
			for _, file := range files {
				if !file.IsDir() {
					certPath := filepath.Join(apiServer.Cfg.SSL.SSLCertDir, file.Name())
					cert, err := os.ReadFile(certPath)
					if err != nil {
						log.Error().Err(err).Str("file", certPath).Msg("Error reading cert file")
						continue
					}
					if ok := rootCAs.AppendCertsFromPEM(cert); !ok {
						log.Error().Str("file", certPath).Msg("Failed to append cert to pool")
					} else {
						log.Info().Str("file", certPath).Msg("Added cert")
					}
				}
			}
		}
	}

	transport.TLSClientConfig = &tls.Config{
		RootCAs: rootCAs,
	}

	proxy.Transport = transport

	router.PathPrefix("/auth").Handler(proxy)
}

// Static files router
func (apiServer *HelixAPIServer) registerDefaultHandler(router *mux.Router) {

	// if we are in prod - then the frontend has been burned into the filesystem of the container
	// and the FrontendURL will actually have the value "/www"
	// so this switch is "are we in dev or not"
	if strings.HasPrefix(apiServer.Cfg.WebServer.FrontendURL, "http://") || strings.HasPrefix(apiServer.Cfg.WebServer.FrontendURL, "https://") {

		router.PathPrefix("/").Handler(spa.NewSPAReverseProxyServer(
			apiServer.Cfg.WebServer.FrontendURL,
		))
	} else {
		log.Info().Msgf("serving static UI files from %s", apiServer.Cfg.WebServer.FrontendURL)

		fileSystem := http.Dir(apiServer.Cfg.WebServer.FrontendURL)

		router.PathPrefix("/").Handler(spa.NewSPAFileServer(fileSystem))
	}
}

func writeResponse(rw http.ResponseWriter, data interface{}, statusCode int) {
	rw.Header().Set("Content-Type", "application/json")

	rw.WriteHeader(statusCode)

	if data == nil {
		return
	}

	err := json.NewEncoder(rw).Encode(data)
	if err != nil {
		log.Err(err).Msg("error writing response")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
	}
}

func writeErrResponse(rw http.ResponseWriter, err error, statusCode int) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(statusCode)

	_ = json.NewEncoder(rw).Encode(&system.HTTPError{
		StatusCode: statusCode,
		Message:    err.Error(),
	})
}

// startEmbeddingsSocketServer starts a UNIX socket server that serves just the /v1/embeddings endpoint with no auth
func (apiServer *HelixAPIServer) startEmbeddingsSocketServer(ctx context.Context) error {
	socketPath := apiServer.Cfg.WebServer.EmbeddingsSocket

	// Remove socket file if it already exists
	if _, err := os.Stat(socketPath); err == nil {
		if err := os.Remove(socketPath); err != nil {
			return fmt.Errorf("failed to remove existing socket file: %w", err)
		}
	}

	// Create socket listener
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on unix socket: %w", err)
	}

	// Set socket permissions
	if err := os.Chmod(socketPath, 0666); err != nil {
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	// Create a new router for the socket server
	router := mux.NewRouter()

	// Register only the necessary endpoints with no auth
	router.HandleFunc("/v1/embeddings", apiServer.createEmbeddings).Methods(http.MethodPost, http.MethodOptions)

	// Add models endpoint to allow checking available models
	router.HandleFunc("/v1/models", apiServer.listModels).Methods(http.MethodGet, http.MethodOptions)

	// Add chat completions endpoint for Haystack LLM access
	router.HandleFunc("/v1/chat/completions", apiServer.createChatCompletion).Methods(http.MethodPost, http.MethodOptions)

	// Create HTTP server
	srv := &http.Server{
		Handler:      router,
		ReadTimeout:  time.Minute * 30, // Increased from 15 to 30 minutes
		WriteTimeout: time.Minute * 30, // Increased from 15 to 30 minutes
	}

	log.Info().Str("socket", socketPath).Msg("starting embeddings socket server")

	// Ensure the server is shut down when the context is canceled
	go func() {
		<-ctx.Done()
		log.Info().Msg("shutting down embeddings socket server")
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Error().Err(err).Msg("error shutting down embeddings socket server")
		}
		if err := listener.Close(); err != nil {
			log.Error().Err(err).Msg("error closing embeddings socket listener")
		}
	}()

	// Start the server
	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("embeddings socket server error: %w", err)
	}

	return nil
}

// listTemplateApps godoc
// @Summary List template apps
// @Description List available template apps configurations.
// @Tags    template_apps
// @Success 200 {array} types.TemplateAppConfig
// @Router /api/v1/template-apps [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listTemplateApps(_ http.ResponseWriter, _ *http.Request) ([]*types.TemplateAppConfig, error) {
	return types.GetAppTemplates(), nil
}

// getTemplateApp godoc
// @Summary Get template app by type
// @Description Get template app configuration by type.
// @Tags    template_apps
// @Param type path string true "Template app type"
// @Success 200 {object} types.TemplateAppConfig
// @Failure 404 {object} system.HTTPError "Template not found"
// @Router /api/v1/template-apps/{type} [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getTemplateApp(_ http.ResponseWriter, r *http.Request) (*types.TemplateAppConfig, error) {
	vars := mux.Vars(r)
	templateType := types.TemplateAppType(vars["type"])

	template := types.GetTemplateByType(templateType)
	if template == nil {
		return nil, system.NewHTTPError404("template not found")
	}

	return template, nil
}
