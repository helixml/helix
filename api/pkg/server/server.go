package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bacalhau-project/lilysaas/api/pkg/controller"
	"github.com/bacalhau-project/lilysaas/api/pkg/system"
	"github.com/gorilla/mux"
)

type ServerOptions struct {
	Host string
	Port int
}

type LilysaasAPIServer struct {
	Options    ServerOptions
	Controller *controller.Controller
}

func NewServer(
	options ServerOptions,
	controller *controller.Controller,
) (*LilysaasAPIServer, error) {
	if options.Host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if options.Port == 0 {
		return nil, fmt.Errorf("port is required")
	}

	return &LilysaasAPIServer{
		Options:    options,
		Controller: controller,
	}, nil
}

func (apiServer *LilysaasAPIServer) ListenAndServe(ctx context.Context, cm *system.CleanupManager) error {
	router := mux.NewRouter()

	subrouter := router.PathPrefix("/api/v1").Subrouter()
	subrouter.HandleFunc("/status", apiServer.status).Methods("GET")

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
