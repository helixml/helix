package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bacalhau-project/lilysaas/api/pkg/controller"
	"github.com/bacalhau-project/lilysaas/api/pkg/store"
	"github.com/bacalhau-project/lilysaas/api/pkg/system"
	"github.com/gorilla/mux"
)

type ServerOptions struct {
	URL           string
	Host          string
	Port          int
	KeyCloakURL   string
	KeyCloakToken string
}

type LilysaasAPIServer struct {
	Options    ServerOptions
	Store      store.Store
	Controller *controller.Controller
}

func NewServer(
	options ServerOptions,
	store store.Store,
	controller *controller.Controller,
) (*LilysaasAPIServer, error) {
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

	return &LilysaasAPIServer{
		Options:    options,
		Store:      store,
		Controller: controller,
	}, nil
}

func (apiServer *LilysaasAPIServer) ListenAndServe(ctx context.Context, cm *system.CleanupManager) error {
	router := mux.NewRouter()
	router.Use(apiServer.corsMiddleware)

	subrouter := router.PathPrefix("/api/v1").Subrouter()

	// add one more subrouter for the authenticated service methods
	authRouter := subrouter.MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
		return true
	}).Subrouter()

	keycloak := newKeycloak(apiServer.Options)
	keyCloakMiddleware := newMiddleware(keycloak, apiServer.Options)
	authRouter.Use(keyCloakMiddleware.verifyToken)

	subrouter.HandleFunc("/modules", wrapper(apiServer.getModules)).Methods("GET")

	authRouter.HandleFunc("/status", wrapper(apiServer.status)).Methods("GET")
	authRouter.HandleFunc("/jobs", wrapper(apiServer.getJobs)).Methods("GET")
	authRouter.HandleFunc("/jobs", wrapper(apiServer.createJob)).Methods("POST")

	StartWebSocketServer(
		ctx,
		subrouter,
		"/ws",
		apiServer.Controller.JobUpdatesChan,
		keyCloakMiddleware.userIDFromRequest,
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
