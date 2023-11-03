package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/lukemarsden/helix/api/pkg/server"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"gopkg.in/yaml.v3"
)

type RunnerServerOptions struct {
	Host string
	Port int
}

type RunnerServer struct {
	Options    RunnerServerOptions
	Controller *Runner
	// in-memory state to record status that would normally be posted up as a result
	State    map[string]types.WorkerTaskResponse
	StateMtx sync.Mutex
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

	subrouter := router.PathPrefix(server.API_SUB_PATH).Subrouter()

	// TODO: record worker response state locally, _in memory_ if we are in "local only mode"
	// an endpoint to add our next session
	subrouter.HandleFunc("/worker/session", server.Wrapper(runnerServer.setNextGlobalSession)).Methods("POST")

	// an endpoint to query the local state
	subrouter.HandleFunc("/worker/state", server.Wrapper(runnerServer.state)).Methods("GET")

	// pull the next task for an already running wrapper
	subrouter.HandleFunc("/worker/task/{instanceid}", server.WrapperWithConfig(runnerServer.getWorkerTask, server.WrapperConfig{
		SilenceErrors: true,
	})).Methods("GET")

	// post a response for an already running wrapper
	subrouter.HandleFunc("/worker/response/{instanceid}", server.Wrapper(runnerServer.respondWorkerTask)).Methods("POST")

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
	return runnerServer.Controller.getNextTask(req.Context(), vars["instanceid"])
}

func (runnerServer *RunnerServer) respondWorkerTask(res http.ResponseWriter, req *http.Request) (*types.WorkerTaskResponse, error) {
	vars := mux.Vars(req)
	taskResponse := &types.WorkerTaskResponse{}
	err := json.NewDecoder(req.Body).Decode(taskResponse)
	if err != nil {
		return nil, err
	}

	taskResponse, err = runnerServer.Controller.handleTaskResponse(req.Context(), vars["instanceid"], taskResponse)
	if err != nil {
		return nil, err
	}

	// record in-memory for any local clients who want to query us
	runnerServer.State[vars["instanceid"]] = *taskResponse

	stateYAML, err := yaml.Marshal(runnerServer.State)
	if err != nil {
		return nil, err
	}
	fmt.Println("==========================================")
	fmt.Println("             LOCAL STATE")
	fmt.Println("==========================================")
	fmt.Println(string(stateYAML))
	fmt.Println("==========================================")

	return taskResponse, nil
}

func (runnerServer *RunnerServer) state(res http.ResponseWriter, req *http.Request) (map[string]types.WorkerTaskResponse, error) {
	return runnerServer.State, nil
}

func (runnerServer *RunnerServer) setNextGlobalSession(res http.ResponseWriter, req *http.Request) (*types.WorkerTask, error) {
	session := &types.Session{}
	err := json.NewDecoder(req.Body).Decode(session)
	if err != nil {
		return nil, fmt.Errorf("error decoding session as post body: %s", err)
	}

	// just start it instantly for now...
	// TODO: what's the distinction between session and task?
	//
	// why does getNextGlobalSession always immediately start a new model
	// instance? shouldn't it assign it to an existing one potentially?

	// what to do next: try running this code, get it working, then figure out
	// how to make 'helix run' reuse an existing session (which i'm pretty sure
	// this won't do) - also figure out how to write out results to disk, etc

	runnerServer.Controller.createModelInstance(req.Context(), session)

	// TODO: Implement the logic to set the next global session

	return nil, nil
}
