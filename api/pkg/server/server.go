package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/controller/knowledge"
	"github.com/helixml/helix/api/pkg/gptscript"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/server/spa"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/stripe"
	"github.com/helixml/helix/api/pkg/system"
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
}

type AuthConfig struct {
	OIDCAuth auth.OIDCAuthenticator
	RunnerAuth auth.RunnerTokenAuthenticator
	ApiKeyAuth auth.ApiKeyAuthenticator
}

func NewServer(
	cfg *config.ServerConfig,
	store store.Store,
	ps pubsub.PubSub,
	gptScriptExecutor gptscript.Executor,
	providerManager manager.ProviderManager,
	inferenceServer *openai.InternalHelixServer,
	authConfig *AuthConfig,
	stripe *stripe.Stripe,
	controller *controller.Controller,
	janitor *janitor.Janitor,
	knowledgeManager knowledge.Manager,
	scheduler *scheduler.Scheduler,
	pingService *version.PingService,
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

	return &HelixAPIServer{
		Cfg:               cfg,
		Store:             store,
		Stripe:            stripe,
		Controller:        controller,
		Janitor:           janitor,
		gptScriptExecutor: gptScriptExecutor,
		inferenceServer:   inferenceServer,
		authMiddleware: newAuthMiddleware(
			authConfig.OIDCAuth,
			authConfig.RunnerAuth,
			authConfig.ApiKeyAuth,
			store,
		),
		providerManager:  providerManager,
		pubsub:           ps,
		knowledgeManager: knowledgeManager,
		scheduler:        scheduler,
		pingService:      pingService,
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

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", apiServer.Cfg.WebServer.Host, apiServer.Cfg.WebServer.Port),
		WriteTimeout:      time.Minute * 15,
		ReadTimeout:       time.Minute * 15,
		ReadHeaderTimeout: time.Minute * 15,
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

	authRouter.HandleFunc("/data_entities", system.DefaultWrapper(apiServer.createDataEntity)).Methods(http.MethodPost)

	authRouter.HandleFunc("/subscription/new", system.DefaultWrapper(apiServer.subscriptionCreate)).Methods(http.MethodPost)
	authRouter.HandleFunc("/subscription/manage", system.DefaultWrapper(apiServer.subscriptionManage)).Methods(http.MethodPost)

	authRouter.HandleFunc("/api_keys", system.DefaultWrapper(apiServer.createAPIKey)).Methods(http.MethodPost)
	authRouter.HandleFunc("/api_keys", system.DefaultWrapper(apiServer.getAPIKeys)).Methods(http.MethodGet)
	authRouter.HandleFunc("/api_keys", system.DefaultWrapper(apiServer.deleteAPIKey)).Methods(http.MethodDelete)
	authRouter.HandleFunc("/api_keys/check", system.DefaultWrapper(apiServer.checkAPIKey)).Methods(http.MethodGet)

	if apiServer.Cfg.WebServer.LocalFilestorePath != "" {
		// disable directory listings
		fileServer := http.FileServer(neuteredFileSystem{http.Dir(apiServer.Cfg.WebServer.LocalFilestorePath)})

		// we handle our own auth from inside this function
		// but we need to use the maybeAuthRouter because it uses the keycloak middleware
		// that will extract the bearer token into a user id for us
		subRouter.PathPrefix("/filestore/viewer/").Handler(
			http.StripPrefix(fmt.Sprintf("%s/filestore/viewer/", APIPrefix), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// if the session is "shared" then anyone can see the files inside the session
				// if the user is admin then can see anything
				// if the user is runner then can see anything
				// if the path is part of the user path then can see it
				// if path has presign URL
				// otherwise access denied
				canAccess, err := apiServer.isFilestoreRouteAuthorized(r)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				if !canAccess {
					http.Error(w, "Access denied", http.StatusForbidden)
					return
				}

				// read the query param called redirect_urls
				// if it's present and set to the string "true"
				// then assign a boolean
				shouldRedirectURLs := r.URL.Query().Get("redirect_urls") == "true"
				if shouldRedirectURLs && strings.HasSuffix(r.URL.Path, ".url") {
					url, err := apiServer.Controller.FilestoreReadTextFile(r.URL.Path)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
					} else {
						http.Redirect(w, r, url, http.StatusFound)
					}
				} else {
					fileServer.ServeHTTP(w, r)
				}
			})))
	}

	// OpenAI API compatible routes
	router.HandleFunc("/v1/chat/completions", apiServer.authMiddleware.auth(apiServer.createChatCompletion)).Methods(http.MethodPost, http.MethodOptions)
	router.HandleFunc("/v1/embeddings", apiServer.authMiddleware.auth(apiServer.createEmbeddings)).Methods(http.MethodPost, http.MethodOptions)
	router.HandleFunc("/v1/models", apiServer.authMiddleware.auth(apiServer.listModels)).Methods(http.MethodGet)
	// Azure OpenAI API compatible routes
	router.HandleFunc("/openai/deployments/{model}/chat/completions", apiServer.authMiddleware.auth(apiServer.createChatCompletion)).Methods(http.MethodPost, http.MethodOptions)

	authRouter.HandleFunc("/providers", apiServer.listProviders).Methods(http.MethodGet)

	authRouter.HandleFunc("/provider-endpoints", apiServer.listProviderEndpoints).Methods(http.MethodGet)
	authRouter.HandleFunc("/provider-endpoints", apiServer.createProviderEndpoint).Methods(http.MethodPost)
	authRouter.HandleFunc("/provider-endpoints/{id}", apiServer.updateProviderEndpoint).Methods(http.MethodPut)
	authRouter.HandleFunc("/provider-endpoints/{id}", apiServer.deleteProviderEndpoint).Methods(http.MethodDelete)

	// Helix inference route
	authRouter.HandleFunc("/sessions/chat", apiServer.startChatSessionHandler).Methods(http.MethodPost)

	// Helix learn route (i.e. create fine tune and/or RAG source)
	authRouter.HandleFunc("/sessions/learn", apiServer.startLearnSessionHandler).Methods(http.MethodPost)

	authRouter.HandleFunc("/sessions", system.DefaultWrapper(apiServer.getSessions)).Methods(http.MethodGet)
	// authRouter.HandleFunc("/sessions", system.DefaultWrapper(apiServer.createSession)).Methods(http.MethodPost)

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
	authRouter.HandleFunc("/apps/{id}/llm-calls", system.Wrapper(apiServer.listAppLLMCalls)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}/api-actions", system.Wrapper(apiServer.appRunAPIAction)).Methods(http.MethodPost)

	authRouter.HandleFunc("/search", system.Wrapper(apiServer.knowledgeSearch)).Methods(http.MethodGet)

	authRouter.HandleFunc("/knowledge", system.Wrapper(apiServer.listKnowledge)).Methods(http.MethodGet)
	authRouter.HandleFunc("/knowledge/{id}", system.Wrapper(apiServer.getKnowledge)).Methods(http.MethodGet)
	authRouter.HandleFunc("/knowledge/{id}", system.Wrapper(apiServer.deleteKnowledge)).Methods(http.MethodDelete)
	authRouter.HandleFunc("/knowledge/{id}/refresh", system.Wrapper(apiServer.refreshKnowledge)).Methods(http.MethodPost)
	authRouter.HandleFunc("/knowledge/{id}/versions", system.Wrapper(apiServer.listKnowledgeVersions)).Methods(http.MethodGet)

	// we know which app this is by the token that is used (which is linked to the app)
	// this is so frontend devs don't need anything other than their access token
	// and can auto-connect to this endpoint
	// we handle CORs by loading the app from the token.app_id and it knowing which domains are allowed
	authRouter.HandleFunc("/apps/script", system.Wrapper(apiServer.appRunScript)).Methods(http.MethodPost, http.MethodOptions)
	adminRouter.HandleFunc("/dashboard", system.DefaultWrapper(apiServer.dashboard)).Methods(http.MethodGet)
	adminRouter.HandleFunc("/llm_calls", system.Wrapper(apiServer.listLLMCalls)).Methods(http.MethodGet)

	// all these routes are secured via runner tokens
	insecureRouter.HandleFunc("/runner/{runner_id}/ws", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		runnerID := vars["runner_id"]
		log.Info().Msgf("proxying runner websocket request to nats for runner %s", runnerID)
		log.Debug().Interface("request", r).Msg("nats request")
		fmt.Printf("proxying runner websocket request to nats for runner %s", runnerID)

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
			log.Printf("Failed to upgrade client connection: %v", err)
			return
		}
		// Ensure the client connection is closed on function exit.
		defer clientConn.Close()

		// Connect to the backend WebSocket server.
		backendConn, resp, err := websocket.DefaultDialer.Dial("ws://localhost:8433", nil) // TODO(Phil): make this configurable
		if err != nil {
			log.Printf("Failed to connect to backend WebSocket server: %v", err)
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

	apiServer.router = router

	return subRouter, nil
}

func getID(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["id"]
}

func (apiServer *HelixAPIServer) registerKeycloakHandler(router *mux.Router) {
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
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
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
