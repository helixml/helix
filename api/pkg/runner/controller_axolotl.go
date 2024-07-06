package runner

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// TODO: move server into the axolotl model instance struct and
// start it together with the model

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

// used by the Python code to know that a session has finished preparing and is ready to pull from the
// queue - this won't actually pull the session from the queue (in the form of a task i.e. getNextTask)
// but it gives the python code a chance to wait for Lora weights to download before loading them
// into GPU memory - at which point it would start pulling from the queue as normal
func (r *Runner) readInitialWorkerSession(instanceID string) (*types.Session, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("instanceid is required")
	}
	genericModelInstance, ok := r.activeModelInstances.Load(instanceID)
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	modelInstance, ok := genericModelInstance.(*AxolotlModelInstance)
	if !ok {
		return nil, fmt.Errorf("expected axolotl model instance, got :%v", genericModelInstance)
	}

	if modelInstance.NextSession() == nil {
		return nil, fmt.Errorf("no session found")
	}
	return modelInstance.NextSession(), nil
}

// given a running model instance id
// get the next session that it should run
// this is either the nextSession property on the instance
// or it's what is returned by the master API server (if anything)
// this function being called means "I am ready for more work"
// because the child processes are blocking - the child will not be
// asking for more work until it's ready to accept and run it
func (r *Runner) popNextTask(ctx context.Context, instanceID string) (*types.RunnerTask, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("instanceid is required")
	}
	genericModelInstance, ok := r.activeModelInstances.Load(instanceID)
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	modelInstance, ok := genericModelInstance.(*AxolotlModelInstance)
	if !ok {
		return nil, fmt.Errorf("expected axolotl model instance, got :%v", genericModelInstance)
	}

	var session *types.Session

	if modelInstance.NextSession() != nil {
		// if there is a session in the nextSession cache then we return it immediately
		log.Debug().Msgf("🟣🟣 loading modelInstance.nextSession %+v", modelInstance.NextSession())
		session = modelInstance.NextSession()
		modelInstance.SetNextSession(nil)
	} else if modelInstance.GetQueuedSession() != nil {
		// if there is a session in the queuedSession cache then we are waiting for
		// a task to complete before we want to actually run the session
		log.Debug().Msgf("🟡🟡 waiting modelInstance.queuedSession %+v", modelInstance.GetQueuedSession())
	} else {
		// ask the upstream api server if there is another task
		// if there is - then assign it to the queuedSession
		// and call "pre"
		if r.httpClientOptions.Host != "" {
			queryParams := url.Values{}

			queryParams.Add("model_name", string(modelInstance.Filter().ModelName))
			queryParams.Add("mode", string(modelInstance.Filter().Mode))
			queryParams.Add("lora_dir", string(modelInstance.Filter().LoraDir))

			apiSession, err := r.getNextApiSession(ctx, queryParams)
			if err != nil {
				return nil, err
			}

			if apiSession != nil {
				go modelInstance.QueueSession(apiSession, false)
			}
		}
	}

	// we don't have any work for this model instance
	if session == nil {
		// TODO: this should be a 404 not a 500?
		return nil, fmt.Errorf("no session found")
	}

	task, err := modelInstance.AssignSessionTask(ctx, session)
	if err != nil {
		return nil, err
	}

	return task, nil
}
