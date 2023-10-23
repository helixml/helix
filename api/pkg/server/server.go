package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/lukemarsden/helix/api/pkg/controller"
	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/system"
)

type ServerOptions struct {
	URL           string
	Host          string
	Port          int
	KeyCloakURL   string
	KeyCloakToken string
	// this is for when we are running localfs filesystem
	// and we need to add a route to view files based on their path
	// we are assuming all file storage is open right now
	// so we just deep link to the object path and don't apply auth
	// (this is so lilypad nodes can see files)
	// later, we might add a token to the URLs
	LocalFilestorePath string
}

type HelixAPIServer struct {
	Options    ServerOptions
	Store      store.Store
	Controller *controller.Controller
}

func NewServer(
	options ServerOptions,
	store store.Store,
	controller *controller.Controller,
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
		Controller: controller,
	}, nil
}

func (apiServer *HelixAPIServer) ListenAndServe(ctx context.Context, cm *system.CleanupManager) error {
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

	authRouter.HandleFunc("/status", Wrapper(apiServer.status)).Methods("GET")
	authRouter.HandleFunc("/transactions", Wrapper(apiServer.getTransactions)).Methods("GET")

	authRouter.HandleFunc("/filestore/config", Wrapper(apiServer.filestoreConfig)).Methods("GET")
	authRouter.HandleFunc("/filestore/list", Wrapper(apiServer.filestoreList)).Methods("GET")
	authRouter.HandleFunc("/filestore/get", Wrapper(apiServer.filestoreGet)).Methods("GET")
	authRouter.HandleFunc("/filestore/folder", Wrapper(apiServer.filestoreCreateFolder)).Methods("POST")
	authRouter.HandleFunc("/filestore/upload", Wrapper(apiServer.filestoreUpload)).Methods("POST")
	authRouter.HandleFunc("/filestore/rename", Wrapper(apiServer.filestoreRename)).Methods("PUT")
	authRouter.HandleFunc("/filestore/delete", Wrapper(apiServer.filestoreDelete)).Methods("DELETE")

	if apiServer.Options.LocalFilestorePath != "" {
		fileServer := http.FileServer(http.Dir(apiServer.Options.LocalFilestorePath))
		subrouter.PathPrefix("/filestore/viewer/").Handler(http.StripPrefix("/api/v1/filestore/viewer/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fileServer.ServeHTTP(w, r)
		})))
	}

	authRouter.HandleFunc("/sessions", Wrapper(apiServer.getSessions)).Methods("GET")
	authRouter.HandleFunc("/sessions", Wrapper(apiServer.createSession)).Methods("POST")
	authRouter.HandleFunc("/sessions/{id}", Wrapper(apiServer.getSession)).Methods("GET")
	authRouter.HandleFunc("/sessions/{id}", Wrapper(apiServer.updateSession)).Methods("PUT")
	authRouter.HandleFunc("/sessions/{id}", Wrapper(apiServer.deleteSession)).Methods("DELETE")

	// TODO: this has no auth right now
	// we need to add JWTs to the urls we are using to connect models to the worker
	// the task filters (mode, type and modelName) are all given as query params
	subrouter.HandleFunc("/worker/task", Wrapper(apiServer.getWorkerTask)).Methods("GET")
	subrouter.HandleFunc("/worker/response", Wrapper(apiServer.respondWorkerTask)).Methods("POST")

	StartWebSocketServer(
		ctx,
		subrouter,
		"/ws",
		apiServer.Controller.SessionUpdatesChan,
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
