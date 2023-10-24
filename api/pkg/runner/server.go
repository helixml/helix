package runner

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/lukemarsden/helix/api/pkg/server"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
)

type RunnerServerOptions struct {
	Host string
	Port int
}

type RunnerServer struct {
	Options    RunnerServerOptions
	Controller *Runner
}

func NewRunnerServer(
	options RunnerServerOptions,
	controller *Runner,
) (*RunnerServer, error) {
	if options.Port == 0 {
		return nil, fmt.Errorf("server port is required")
	}
	return &RunnerServer{
		Options:    options,
		Controller: controller,
	}, nil
}

func (runnerServer *RunnerServer) ListenAndServe(ctx context.Context, cm *system.CleanupManager) error {
	router := mux.NewRouter()

	subrouter := router.PathPrefix("/api/v1").Subrouter()

	// pull the next task for an already running wrapper
	subrouter.HandleFunc("/worker/task/:instanceid", server.Wrapper(runnerServer.getWorkerTask)).Methods("GET")

	// post a response for an already running wrapper
	subrouter.HandleFunc("/worker/response", server.Wrapper(runnerServer.respondWorkerTask)).Methods("POST")

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", runnerServer.Options.Host, runnerServer.Options.Port),
		WriteTimeout:      time.Minute * 15,
		ReadTimeout:       time.Minute * 15,
		ReadHeaderTimeout: time.Minute * 15,
		IdleTimeout:       time.Minute * 60,
		Handler:           router,
	}
	return srv.ListenAndServe()
}

// get the next task for a running instance
// we look at the instance by ID and check if it has a nextSession
// if it does then we assign that as the current session
// if it does not - then we need to reach out to the master API to get one
func (runnerServer *RunnerServer) getWorkerTask(res http.ResponseWriter, req *http.Request) (*types.WorkerTask, error) {
	vars := mux.Vars(req)
	instanceID := vars["instanceid"]
	_, err := runnerServer.Controller.getInstanceNextSession(req.Context(), instanceID)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (runnerServer *RunnerServer) respondWorkerTask(res http.ResponseWriter, req *http.Request) (*types.WorkerTaskResponse, error) {
	return nil, nil
}
