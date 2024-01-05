package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/lukemarsden/helix/api/pkg/controller"
	"github.com/lukemarsden/helix/api/pkg/janitor"
	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/stripe"
	"github.com/lukemarsden/helix/api/pkg/system"
)

const API_PREFIX = "/api/v1"

type ServerOptions struct {
	URL           string
	Host          string
	Port          int
	KeyCloakURL   string
	KeyCloakToken string
	RunnerToken   string
	// a list of keycloak ids that are considered admins
	// if the string '*' is included it means ALL users
	AdminIDs []string
	// this is for when we are running localfs filesystem
	// and we need to add a route to view files based on their path
	// we are assuming all file storage is open right now
	// so we just deep link to the object path and don't apply auth
	// (this is so helix nodes can see files)
	// later, we might add a token to the URLs
	LocalFilestorePath string
}

type HelixAPIServer struct {
	Options    ServerOptions
	Store      store.Store
	Stripe     *stripe.Stripe
	Controller *controller.Controller
	Janitor    *janitor.Janitor
	runnerAuth *runnerAuth
	adminAuth  *adminAuth
}

func NewServer(
	options ServerOptions,
	store store.Store,
	stripe *stripe.Stripe,
	controller *controller.Controller,
	janitor *janitor.Janitor,
) (*HelixAPIServer, error) {
	if options.URL == "" {
		return nil, fmt.Errorf("server url is required")
	}
	if options.Host == "" {
		return nil, fmt.Errorf("server host is required")
	}
	if options.Port == 0 {
		return nil, fmt.Errorf("server port is required")
	}
	if options.KeyCloakURL == "" {
		return nil, fmt.Errorf("keycloak url is required")
	}
	if options.KeyCloakToken == "" {
		return nil, fmt.Errorf("keycloak token is required")
	}
	return &HelixAPIServer{
		Options:    options,
		Store:      store,
		Stripe:     stripe,
		Controller: controller,
		Janitor:    janitor,
		runnerAuth: newRunnerAuth(options.RunnerToken),
		adminAuth:  newAdminAuth(options.AdminIDs),
	}, nil
}

func (apiServer *HelixAPIServer) ListenAndServe(ctx context.Context, cm *system.CleanupManager) error {
	router := mux.NewRouter()
	err := apiServer.Janitor.InjectMiddleware(router)
	if err != nil {
		return err
	}
	router.Use(apiServer.corsMiddleware)
	router.Use(errorLoggingMiddleware)

	subrouter := router.PathPrefix(API_PREFIX).Subrouter()

	// auth router requires a valid token from keycloak
	authRouter := subrouter.MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
		return true
	}).Subrouter()

	maybeAuthRouter := subrouter.MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
		return true
	}).Subrouter()

	keycloak := newKeycloak(apiServer.Options)
	keyCloakMiddleware := newMiddleware(keycloak, apiServer.Options, apiServer.Store)
	authRouter.Use(keyCloakMiddleware.enforceVerifyToken)
	maybeAuthRouter.Use(keyCloakMiddleware.maybeVerifyToken)

	// runner router requires a valid runner token
	runnerRouter := subrouter.MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
		return true
	}).Subrouter()
	runnerRouter.Use(apiServer.runnerAuth.middleware)

	// admin auth
	adminRouter := authRouter.MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
		return true
	}).Subrouter()
	adminRouter.Use(apiServer.adminAuth.middleware)
	subrouter.HandleFunc("/config", system.DefaultWrapperWithConfig(apiServer.config, system.WrapperConfig{
		SilenceErrors: true,
	})).Methods("GET")

	subrouter.HandleFunc("/config/js", apiServer.configJS).Methods("GET")

	// this is not authenticated because we use the webhook signing secret
	// the stripe library handles http management
	subrouter.HandleFunc("/stripe/webhook", apiServer.subscriptionWebhook).Methods("POST")

	authRouter.HandleFunc("/status", system.DefaultWrapper(apiServer.status)).Methods("GET")
	authRouter.HandleFunc("/transactions", system.DefaultWrapper(apiServer.getTransactions)).Methods("GET")

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

	if apiServer.Options.LocalFilestorePath != "" {
		// disable directory listings
		fileServer := http.FileServer(neuteredFileSystem{http.Dir(apiServer.Options.LocalFilestorePath)})
		subrouter.PathPrefix("/filestore/viewer/").Handler(http.StripPrefix(fmt.Sprintf("%s/filestore/viewer/", API_PREFIX), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fileServer.ServeHTTP(w, r)
		})))
	}

	authRouter.HandleFunc("/sessions", system.DefaultWrapper(apiServer.getSessions)).Methods("GET")
	authRouter.HandleFunc("/sessions", system.DefaultWrapper(apiServer.createSession)).Methods("POST")
	maybeAuthRouter.HandleFunc("/sessions/{id}", system.Wrapper(apiServer.getSession)).Methods("GET")
	maybeAuthRouter.HandleFunc("/sessions/{id}/summary", system.Wrapper(apiServer.getSessionSummary)).Methods("GET")
	authRouter.HandleFunc("/sessions/{id}", system.Wrapper(apiServer.updateSession)).Methods("PUT")
	authRouter.HandleFunc("/sessions/{id}", system.Wrapper(apiServer.deleteSession)).Methods("DELETE")
	authRouter.HandleFunc("/sessions/{id}/restart", system.Wrapper(apiServer.restartSession)).Methods("PUT")
	authRouter.HandleFunc("/sessions/{id}/config", system.Wrapper(apiServer.updateSessionConfig)).Methods("PUT")

	authRouter.HandleFunc("/sessions/{id}/meta", system.Wrapper(apiServer.updateSessionMeta)).Methods("PUT")
	authRouter.HandleFunc("/sessions/{id}/finetune/start", system.Wrapper(apiServer.startSessionFinetune)).Methods("POST")
	authRouter.HandleFunc("/sessions/{id}/finetune/documents", system.Wrapper(apiServer.finetuneAddDocuments)).Methods("PUT")
	authRouter.HandleFunc("/sessions/{id}/finetune/clone/{interaction}/{mode}", system.Wrapper(apiServer.cloneFinetuneInteraction)).Methods("POST")
	authRouter.HandleFunc("/sessions/{id}/finetune/text/retry", system.Wrapper(apiServer.retryTextFinetune)).Methods("PUT")
	maybeAuthRouter.HandleFunc("/sessions/{id}/finetune/text/conversations/{interaction}", system.Wrapper(apiServer.getSessionFinetuneConversation)).Methods("GET")
	authRouter.HandleFunc("/sessions/{id}/finetune/text/conversations/{interaction}", system.Wrapper(apiServer.setSessionFinetuneConversation)).Methods("PUT")

	// authRouter.HandleFunc("/bots", system.DefaultWrapper(apiServer.getBots)).Methods("GET")
	// authRouter.HandleFunc("/bots", system.DefaultWrapper(apiServer.createBot)).Methods("POST")
	// authRouter.HandleFunc("/bots/{id}", system.DefaultWrapper(apiServer.updateBot)).Methods("PUT")
	// authRouter.HandleFunc("/bots/{id}", system.DefaultWrapper(apiServer.deleteBot)).Methods("DELETE")

	adminRouter.HandleFunc("/dashboard", system.DefaultWrapper(apiServer.dashboard)).Methods("GET")

	runnerRouter.HandleFunc("/runner/{runnerid}/nextsession", system.DefaultWrapper(apiServer.getNextRunnerSession)).Methods("GET")
	runnerRouter.HandleFunc("/runner/{runnerid}/response", system.DefaultWrapper(apiServer.handleRunnerResponse)).Methods("POST")
	runnerRouter.HandleFunc("/runner/{runnerid}/state", system.DefaultWrapper(apiServer.handleRunnerMetrics)).Methods("POST")
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/download/file", apiServer.runnerSessionDownloadFile).Methods("GET")
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/download/folder", apiServer.runnerSessionDownloadFolder).Methods("GET")
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/upload/files", system.DefaultWrapper(apiServer.runnerSessionUploadFiles)).Methods("POST")
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/upload/folder", system.DefaultWrapper(apiServer.runnerSessionUploadFolder)).Methods("POST")

	StartUserWebSocketServer(
		ctx,
		subrouter,
		apiServer.Controller,
		"/ws/user",
		apiServer.Controller.UserWebsocketEventChanWriter,
		keyCloakMiddleware.userIDFromRequest,
	)

	StartRunnerWebSocketServer(
		ctx,
		subrouter,
		apiServer.Controller,
		"/ws/runner",
		apiServer.Controller.RunnerWebsocketEventChanReader,
		apiServer.runnerAuth.isRequestAuthenticated,
	)

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", apiServer.Options.Host, apiServer.Options.Port),
		WriteTimeout:      time.Minute * 15,
		ReadTimeout:       time.Minute * 15,
		ReadHeaderTimeout: time.Minute * 15,
		IdleTimeout:       time.Minute * 60,
		Handler:           router,
	}
	return srv.ListenAndServe()
}
