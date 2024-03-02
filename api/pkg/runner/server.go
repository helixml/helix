package runner

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
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

	subrouter := router.PathPrefix(system.API_SUB_PATH).Subrouter()

	// pull the next task for an already running wrapper
	subrouter.HandleFunc("/worker/task/{instanceid}", system.DefaultWrapperWithConfig(runnerServer.getWorkerTask, system.WrapperConfig{
		SilenceErrors: true,
	})).Methods("GET")

	// used by the Python code to know that a session has finished preparing and is ready to pull from the
	// queue - this won't actually pull the session from the queue (in the form of a task i.e. getNextTask)
	// but it gives the python code a chance to wait for Lora weights to download before loading them
	// into GPU memory - at which point it would start pulling from the queue as normal
	subrouter.HandleFunc("/worker/initial_session/{instanceid}", system.DefaultWrapperWithConfig(runnerServer.readInitialWorkerSession, system.WrapperConfig{
		SilenceErrors: true,
	})).Methods("GET")

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
func (runnerServer *RunnerServer) getWorkerTask(res http.ResponseWriter, req *http.Request) (*types.RunnerTask, error) {
	vars := mux.Vars(req)
	if vars["instanceid"] == "" {
		return nil, fmt.Errorf("instanceid is required")
	}
	return runnerServer.Controller.popNextTask(req.Context(), vars["instanceid"])
}

func (runnerServer *RunnerServer) readInitialWorkerSession(res http.ResponseWriter, req *http.Request) (*types.Session, error) {
	vars := mux.Vars(req)
	if vars["instanceid"] == "" {
		return nil, fmt.Errorf("instanceid is required")
	}
	return runnerServer.Controller.readInitialWorkerSession(vars["instanceid"])
}
