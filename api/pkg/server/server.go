package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/bacalhau-project/bacalhau/pkg/system"
	"github.com/bacalhau-project/lilysaas/api/pkg/controller"
	"github.com/gorilla/mux"
)

type ServerOptions struct {
	Host               string
	Port               int
	FilestoreToken     string
	FilestoreDirectory string
}

type WaterlilyAPIServer struct {
	Options    ServerOptions
	Controller *controller.Controller
}

func NewServer(
	options ServerOptions,
	controller *controller.Controller,
) (*WaterlilyAPIServer, error) {
	if options.Host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if options.Port == 0 {
		return nil, fmt.Errorf("port is required")
	}
	if options.FilestoreToken == "" {
		return nil, fmt.Errorf("filestore token is required")
	}
	if options.FilestoreDirectory == "" {
		return nil, fmt.Errorf("filestore directory is required")
	}
	if _, err := os.Stat(options.FilestoreDirectory); os.IsNotExist(err) {
		return nil, fmt.Errorf("filestore directory does not exist: %s", options.FilestoreDirectory)
	}

	return &WaterlilyAPIServer{
		Options:    options,
		Controller: controller,
	}, nil
}

func (apiServer *WaterlilyAPIServer) ListenAndServe(ctx context.Context, cm *system.CleanupManager) error {
	router := mux.NewRouter()

	subrouter := router.PathPrefix("/api/v1").Subrouter()
	subrouter.HandleFunc("/artists", apiServer.artists).Methods("GET")
	subrouter.HandleFunc("/register", apiServer.register).Methods("POST")
	subrouter.HandleFunc("/files/{path:.*}", apiServer.filestoreDownload).Methods("GET")
	subrouter.HandleFunc("/files", apiServer.filestoreUpload).Methods("POST")

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

func (apiServer *WaterlilyAPIServer) getFilestorePath(path string) string {
	return filepath.Join(apiServer.Options.FilestoreDirectory, path)
}

func (apiServer *WaterlilyAPIServer) ensureFilestorePath(path string) (string, error) {
	fullPath := apiServer.getFilestorePath(path)
	err := os.MkdirAll(fullPath, 0755)
	if err != nil {
		return "", err
	}
	return fullPath, nil
}
