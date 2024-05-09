package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/server/spa"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/stripe"
	"github.com/helixml/helix/api/pkg/system"
)

const API_PREFIX = "/api/v1"

type ServerOptions struct {
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
	// the list of tool ids that are allowed to be used by any user
	// this is returned to the frontend as part of the /config route
	ToolsGlobalIDS []string
}

type HelixAPIServer struct {
	Cfg            *config.ServerConfig
	Store          store.Store
	Stripe         *stripe.Stripe
	Controller     *controller.Controller
	Janitor        *janitor.Janitor
	authMiddleware *authMiddleware
	pubsub         pubsub.PubSub
	router         *mux.Router
}

func NewServer(
	cfg *config.ServerConfig,
	store store.Store,
	ps pubsub.PubSub,
	authenticator auth.Authenticator,
	stripe *stripe.Stripe,
	controller *controller.Controller,
	janitor *janitor.Janitor,
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
		Cfg:        cfg,
		Store:      store,
		Stripe:     stripe,
		Controller: controller,
		Janitor:    janitor,
		authMiddleware: newAuthMiddleware(
			authenticator,
			store,
			authMiddlewareConfig{
				adminUserIDs: cfg.WebServer.AdminIDs,
				runnerToken:  cfg.WebServer.RunnerToken,
			},
		),
		pubsub: ps,
	}, nil
}

func (apiServer *HelixAPIServer) ListenAndServe(ctx context.Context, cm *system.CleanupManager) error {
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

func matchAllRoutes(r *http.Request, rm *mux.RouteMatch) bool {
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
	router.Use(errorLoggingMiddleware)

	// any route that lives under /api/v1
	subRouter := router.PathPrefix(API_PREFIX).Subrouter()

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
	})).Methods("GET")

	subRouter.HandleFunc("/config/js", apiServer.configJS).Methods("GET")
	subRouter.Handle("/swagger", apiServer.swaggerHandler()).Methods("GET")

	// this is not authenticated because we use the webhook signing secret
	// the stripe library handles http management
	subRouter.HandleFunc("/stripe/webhook", apiServer.subscriptionWebhook).Methods("POST")

	authRouter.HandleFunc("/github/status", system.DefaultWrapper(apiServer.githubStatus)).Methods("GET")
	authRouter.HandleFunc("/github/callback", apiServer.githubCallback).Methods("GET")
	authRouter.HandleFunc("/github/repos", system.DefaultWrapper(apiServer.listGithubRepos)).Methods("GET")
	subRouter.HandleFunc("/github/webhook", apiServer.githubWebhook).Methods("POST")

	authRouter.HandleFunc("/status", system.DefaultWrapper(apiServer.status)).Methods("GET")

	// the auth here is handled because we prefix the user path based on the auth context
	// e.g. /sessions/123 becomes /users/456/sessions/123
	// so - the point is, the auth is done by injecting the user id based on the token
	authRouter.HandleFunc("/filestore/config", system.DefaultWrapper(apiServer.filestoreConfig)).Methods("GET")
	authRouter.HandleFunc("/filestore/list", system.DefaultWrapper(apiServer.filestoreList)).Methods("GET")
	authRouter.HandleFunc("/filestore/get", system.DefaultWrapper(apiServer.filestoreGet)).Methods("GET")
	authRouter.HandleFunc("/filestore/folder", system.DefaultWrapper(apiServer.filestoreCreateFolder)).Methods("POST")
	authRouter.HandleFunc("/filestore/upload", system.DefaultWrapper(apiServer.filestoreUpload)).Methods("POST")
	authRouter.HandleFunc("/filestore/rename", system.DefaultWrapper(apiServer.filestoreRename)).Methods("PUT")
	authRouter.HandleFunc("/filestore/delete", system.DefaultWrapper(apiServer.filestoreDelete)).Methods("DELETE")

	authRouter.HandleFunc("/subscription/new", system.DefaultWrapper(apiServer.subscriptionCreate)).Methods("POST")
	authRouter.HandleFunc("/subscription/manage", system.DefaultWrapper(apiServer.subscriptionManage)).Methods("POST")

	authRouter.HandleFunc("/api_keys", system.DefaultWrapper(apiServer.createAPIKey)).Methods("POST")
	authRouter.HandleFunc("/api_keys", system.DefaultWrapper(apiServer.getAPIKeys)).Methods("GET")
	authRouter.HandleFunc("/api_keys", system.DefaultWrapper(apiServer.deleteAPIKey)).Methods("DELETE")
	authRouter.HandleFunc("/api_keys/check", system.DefaultWrapper(apiServer.checkAPIKey)).Methods("GET")

	if apiServer.Cfg.WebServer.LocalFilestorePath != "" {
		// disable directory listings
		fileServer := http.FileServer(neuteredFileSystem{http.Dir(apiServer.Cfg.WebServer.LocalFilestorePath)})

		// we handle our own auth from inside this function
		// but we need to use the maybeAuthRouter because it uses the keycloak middleware
		// that will extract the bearer token into a user id for us
		subRouter.PathPrefix("/filestore/viewer/").Handler(
			http.StripPrefix(fmt.Sprintf("%s/filestore/viewer/", API_PREFIX), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	router.HandleFunc("/v1/chat/completions", apiServer.createChatCompletion).Methods("POST", "OPTIONS")

	authRouter.HandleFunc("/sessions", system.DefaultWrapper(apiServer.getSessions)).Methods("GET")
	authRouter.HandleFunc("/sessions", system.DefaultWrapper(apiServer.createSession)).Methods("POST")

	// api/v1beta/sessions is the new route for creating sessions
	authRouter.HandleFunc("/sessions/chat", apiServer.startSessionHandler).Methods("POST")

	subRouter.HandleFunc("/sessions/{id}", system.Wrapper(apiServer.getSession)).Methods("GET")
	subRouter.HandleFunc("/sessions/{id}/summary", system.Wrapper(apiServer.getSessionSummary)).Methods("GET")
	authRouter.HandleFunc("/sessions/{id}", system.Wrapper(apiServer.updateSession)).Methods("PUT")
	authRouter.HandleFunc("/sessions/{id}", system.Wrapper(apiServer.deleteSession)).Methods("DELETE")
	authRouter.HandleFunc("/sessions/{id}/restart", system.Wrapper(apiServer.restartSession)).Methods("PUT")
	authRouter.HandleFunc("/sessions/{id}/config", system.Wrapper(apiServer.updateSessionConfig)).Methods("PUT")

	authRouter.HandleFunc("/sessions/{id}/meta", system.Wrapper(apiServer.updateSessionMeta)).Methods("PUT")
	authRouter.HandleFunc("/sessions/{id}/finetune/start", system.Wrapper(apiServer.startSessionFinetune)).Methods("POST")
	authRouter.HandleFunc("/sessions/{id}/finetune/documents", system.Wrapper(apiServer.finetuneAddDocuments)).Methods("PUT")
	authRouter.HandleFunc("/sessions/{id}/finetune/clone/{interaction}/{mode}", system.Wrapper(apiServer.cloneFinetuneInteraction)).Methods("POST")
	authRouter.HandleFunc("/sessions/{id}/finetune/text/retry", system.Wrapper(apiServer.retryTextFinetune)).Methods("PUT")
	subRouter.HandleFunc("/sessions/{id}/finetune/text/conversations/{interaction}", system.Wrapper(apiServer.getSessionFinetuneConversation)).Methods("GET")
	authRouter.HandleFunc("/sessions/{id}/finetune/text/conversations/{interaction}", system.Wrapper(apiServer.setSessionFinetuneConversation)).Methods("PUT")

	authRouter.HandleFunc("/tools", system.Wrapper(apiServer.listTools)).Methods("GET")
	authRouter.HandleFunc("/tools", system.Wrapper(apiServer.createTool)).Methods("POST")
	authRouter.HandleFunc("/tools/{id}", system.Wrapper(apiServer.updateTool)).Methods("PUT")
	authRouter.HandleFunc("/tools/{id}", system.Wrapper(apiServer.deleteTool)).Methods("DELETE")

	authRouter.HandleFunc("/apps", system.Wrapper(apiServer.listApps)).Methods("GET")
	authRouter.HandleFunc("/apps", system.Wrapper(apiServer.createApp)).Methods("POST")
	authRouter.HandleFunc("/apps/{id}", system.Wrapper(apiServer.updateApp)).Methods("PUT")
	authRouter.HandleFunc("/apps/{id}", system.Wrapper(apiServer.deleteApp)).Methods("DELETE")

	// we know which app this is by the token that is used (which is linked to the app)
	// this is so frontend devs don't need anything other than their access token
	// and can auto-connect to this endpoint
	// we handle CORs by loading the app from the token.app_id and it knowing which domains are allowed
	authRouter.HandleFunc("/apps/script", system.Wrapper(apiServer.appRunScript)).Methods("POST", "OPTIONS")

	adminRouter.HandleFunc("/dashboard", system.DefaultWrapper(apiServer.dashboard)).Methods("GET")

	// all these routes are secured via runner tokens
	runnerRouter.HandleFunc("/runner/{runnerid}/nextsession", system.DefaultWrapper(apiServer.getNextRunnerSession)).Methods("GET")
	runnerRouter.HandleFunc("/runner/{runnerid}/response", system.DefaultWrapper(apiServer.handleRunnerResponse)).Methods("POST")
	runnerRouter.HandleFunc("/runner/{runnerid}/state", system.DefaultWrapper(apiServer.handleRunnerMetrics)).Methods("POST")
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/download/file", apiServer.runnerSessionDownloadFile).Methods("GET")
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/download/folder", apiServer.runnerSessionDownloadFolder).Methods("GET")
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/upload/files", system.DefaultWrapper(apiServer.runnerSessionUploadFiles)).Methods("POST")
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/upload/folder", system.DefaultWrapper(apiServer.runnerSessionUploadFolder)).Methods("POST")

	// proxy /admin -> keycloak
	apiServer.registerKeycloakHandler(router)

	// proxy other routes to frontend
	apiServer.registerDefaultHandler(router)

	apiServer.router = router

	return subRouter, nil
}

func getID(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["id"]
}

func (apiServer *HelixAPIServer) registerKeycloakHandler(router *mux.Router) {
	u, err := url.Parse(apiServer.Cfg.Keycloak.URL)
	if err != nil {
		log.Error().Err(err).Msg("failed to parse keycloak URL, authentication might not work")
		return
	}

	// Strip path prefix, otherwise we would have to use /auth/auth/realms/helix/protocol/openid-connect/token
	u.Path = ""

	router.PathPrefix("/auth").Handler(httputil.NewSingleHostReverseProxy(u))
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
