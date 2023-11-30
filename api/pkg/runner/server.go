package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
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

	subrouter := router.PathPrefix(system.API_SUB_PATH).Subrouter()

	// pull the next task for an already running wrapper
	subrouter.HandleFunc("/worker/task/{instanceid}", system.WrapperWithConfig(runnerServer.getWorkerTask, system.WrapperConfig{
		SilenceErrors: true,
	})).Methods("GET")

	// used by the Python code to know that a session has finished preparing and is ready to pull from the
	// queue - this won't actually pull the session from the queue (in the form of a task i.e. getNextTask)
	// but it gives the python code a chance to wait for Lora weights to download before loading them
	// into GPU memory - at which point it would start pulling from the queue as normal
	subrouter.HandleFunc("/worker/initial_session/{instanceid}", system.WrapperWithConfig(runnerServer.readInitialWorkerSession, system.WrapperConfig{
		SilenceErrors: true,
	})).Methods("GET")

	if runnerServer.Controller.Options.LocalMode {
		// TODO: record worker response state locally, _in memory_ if we are in "local only mode"
		// an endpoint to add our next session
		subrouter.HandleFunc("/worker/session", system.Wrapper(runnerServer.setNextLocalSession)).Methods("POST")

		// an endpoint to query the local state
		subrouter.HandleFunc("/worker/state", system.Wrapper(runnerServer.state)).Methods("GET")
	}

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
	return runnerServer.Controller.readInitialWorkerSession(req.Context(), vars["instanceid"])
}

func (runnerServer *RunnerServer) state(res http.ResponseWriter, req *http.Request) (map[string]types.RunnerTaskResponse, error) {
	runnerServer.Controller.StateMtx.Lock()
	defer runnerServer.Controller.StateMtx.Unlock()

	// stateYAML, err := yaml.Marshal(runnerServer.Controller.State)
	// if err != nil {
	// 	return nil, err
	// }
	// fmt.Println("==========================================")
	// fmt.Println("             LOCAL STATE")
	// fmt.Println("==========================================")
	// fmt.Println(string(stateYAML))
	// fmt.Println("==========================================")

	return runnerServer.Controller.State, nil
}

func (runnerServer *RunnerServer) setNextLocalSession(res http.ResponseWriter, req *http.Request) (*types.RunnerTask, error) {
	session := &types.Session{}
	err := json.NewDecoder(req.Body).Decode(session)
	if err != nil {
		return nil, fmt.Errorf("error decoding session as post body: %s", err)
	}

	err = runnerServer.Controller.AddToLocalQueue(req.Context(), session)
	if err != nil {
		return nil, err
	}
	response := &types.RunnerTask{
		SessionID: session.ID,
	}

	return response, nil
}
