package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime/debug"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/inhies/go-bytesize"
	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/server"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type RunnerOptions struct {
	ID       string
	ApiHost  string
	ApiToken string

	// these URLs will have the instance ID appended by the model instance
	// e.g. http://localhost:8080/api/v1/worker/task/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/task
	TaskURL string
	// e.g. http://localhost:8080/api/v1/worker/response/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/response
	ResponseURL string

	// how long without running a job before we close a model instance
	ModelInstanceTimeoutSeconds int
	// how many bytes of memory does our GPU have?
	// we report this back to the api when we ask
	// for the global next task (well, this minus the
	// currently running models)
	MemoryBytes uint64
	// if this is defined then we convert it usng
	// github.com/inhies/go-bytesize
	MemoryString string
}

type Runner struct {
	Ctx     context.Context
	Options RunnerOptions

	httpClientOptions server.ClientOptions

	modelMutex sync.RWMutex
	// the map of model instances that we have loaded
	// and are currently running
	activeModelInstances map[string]*ModelInstance

	// the lowest amount of memory that something can run with
	// if we have less than this amount of memory then there is
	// no point asking for more top level tasks
	// we get this on boot by asking the model package
	lowestMemoryRequirement uint64
}

func NewRunner(
	ctx context.Context,
	options RunnerOptions,
) (*Runner, error) {
	if options.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	if options.ApiHost == "" {
		return nil, fmt.Errorf("api url is required")
	}
	if options.ApiToken == "" {
		return nil, fmt.Errorf("api token is required")
	}
	if options.MemoryString != "" {
		bytes, err := bytesize.Parse(options.MemoryString)
		if err != nil {
			return nil, err
		}
		options.MemoryBytes = uint64(bytes)
	}
	if options.MemoryBytes == 0 {
		return nil, fmt.Errorf("memory is required")
	}
	lowestMemoryRequirement, err := model.GetLowestMemoryRequirement()
	if err != nil {
		return nil, err
	}
	runner := &Runner{
		Ctx:                     ctx,
		Options:                 options,
		lowestMemoryRequirement: lowestMemoryRequirement,
		httpClientOptions: server.ClientOptions{
			Host:  options.ApiHost,
			Token: options.ApiToken,
		},
		activeModelInstances: map[string]*ModelInstance{},
	}
	return runner, nil
}

// this should be run in a go-routine
func (r *Runner) StartLooping() {
	for {
		select {
		case <-r.Ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
			err := r.loop(r.Ctx)
			if err != nil {
				log.Error().Msgf("error in runner loop: %s", err.Error())
				debug.PrintStack()
			}
		}
	}
}

func (r *Runner) loop(ctx context.Context) error {
	// check for running model instances that have not seen a job in a while
	// and kill them if they are over the timeout
	// TODO: get the timeout to be configurable from the api and so dynamic
	// based on load
	err := r.checkForStaleModelInstances(ctx, time.Second*time.Duration(r.Options.ModelInstanceTimeoutSeconds))
	if err != nil {
		return err
	}

	// ask the api server if it currently has any work based on our free memory
	session, err := r.getNextGlobalSession(ctx)
	if err != nil {
		return err
	}

	if session != nil {
		log.Debug().
			Msgf("🔵 runner start model instance")
		spew.Dump(session)
		err = r.createModelInstance(ctx, session)
		if err != nil {
			return err
		}
	}

	return nil
}

// loop over the active model instances and stop any that have not processed a job
// in the last timeout seconds
func (r *Runner) checkForStaleModelInstances(ctx context.Context, timeout time.Duration) error {
	r.modelMutex.Lock()
	defer r.modelMutex.Unlock()
	for _, activeModelInstance := range r.activeModelInstances {
		if activeModelInstance.lastJobCompletedTimestamp == 0 {
			continue
		}
		if activeModelInstance.lastJobCompletedTimestamp+int64(timeout.Seconds()) < time.Now().Unix() {
			log.Info().Msgf("Killing stale model instance %s", activeModelInstance.id)
			err := activeModelInstance.stopProcess()
			if err != nil {
				log.Error().Msgf("error stopping model instance %s: %s", activeModelInstance.id, err.Error())
				continue
			}
		}
	}
	return nil
}

// we have popped the next session from the master API
// let's create a model for it
// this means instantiating the model instance and then starting it
// because this model consumes memory it means the global next job filter
// will take into account the fact this model is running
// and will add the de-prioritise filter to the next request
// so that we get a different job type
func (r *Runner) createModelInstance(ctx context.Context, session *types.Session) error {
	r.modelMutex.Lock()
	defer r.modelMutex.Unlock()
	log.Info().Msgf("Add global session %s", session.ID)
	spew.Dump(session)
	model, err := NewModelInstance(
		r.Ctx,
		session.ModelName,
		session.Mode,
		r.Options.TaskURL,
		r.Options.ResponseURL,
		func(res *types.WorkerTaskResponse) error {

			log.Debug().Msgf("🟠 Sending task response %s", session.ID)
			spew.Dump(res)

			// this function will write any task responses back to the api server for it to process
			// we will only hear WorkerTaskResponseTypeStreamContinue and WorkerTaskResponseTypeResult
			// and both of these will have an interaction ID and the full, latest copy of the text
			// the job of the api server is to ensure the existence of the instance (create or update)
			// and replace it's message property - this is the text streaming case
			// if the model does not return a text stream - then all we will hear is a WorkerTaskResponseTypeResult
			// and the api server is just appending to the session
			res, err := server.PostRequest[*types.WorkerTaskResponse, *types.WorkerTaskResponse](
				r.httpClientOptions,
				fmt.Sprintf("/runner/%s/response", r.Options.ID),
				res,
			)
			if err != nil {
				return err
			}
			return nil
		},
	)
	model.initialSession = session
	if err != nil {
		return err
	}
	err = model.startProcess()
	if err != nil {
		return err
	}
	r.activeModelInstances[model.id] = model
	go func() {
		<-model.finishChan
		r.modelMutex.Lock()
		defer r.modelMutex.Unlock()
		log.Info().Msgf("Remove global session %s", session.ID)
		delete(r.activeModelInstances, model.id)
	}()
	return nil
}

func (r *Runner) getNextSession(ctx context.Context, queryParams url.Values) (*types.Session, error) {
	parsedURL, err := url.Parse(server.URL(r.httpClientOptions, fmt.Sprintf("/runner/%s/nextsession", r.Options.ID)))
	if err != nil {
		return nil, err
	}
	parsedURL.RawQuery = queryParams.Encode()

	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", r.httpClientOptions.Token))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, nil
	}

	var buffer bytes.Buffer
	_, err = io.Copy(&buffer, resp.Body)
	if err != nil {
		return nil, err
	}

	var session *types.Session
	err = json.Unmarshal(buffer.Bytes(), &session)
	if err != nil {
		return nil, err
	}

	return session, nil
}

// ask the master API if they have the next session for us
// we check with the various models and filter based on the currently free memory
// we pass that free memory back to the master API - it will filter out any tasks
// for models that would require more memory than we have available
func (r *Runner) getNextGlobalSession(ctx context.Context) (*types.Session, error) {
	freeMemory := r.getFreeMemory()

	if freeMemory < r.lowestMemoryRequirement {
		// we don't have enough memory to run anything
		// so we just wait for more memory to become available
		return nil, nil
	}

	queryParams := url.Values{}

	// this means "only give me sessions that will fit in this much RAM"
	queryParams.Add("memory", fmt.Sprintf("%d", freeMemory))

	// now let's loop over our running model instances and de-prioritise them
	// we might still get sessions of this type but only if there isn't another
	// type in the queue - this is to avoid running only one type of model
	// because of the random order the requests arrived in
	// (i.e. if we get 100 text inferences then the chance is we boot 100 model instances)
	// before trying to run another type of model

	for _, modelInstance := range r.activeModelInstances {
		queryParams.Add("deprioritize", fmt.Sprintf("%s:%s", modelInstance.filter.ModelName, modelInstance.filter.Mode))
	}

	return r.getNextSession(ctx, queryParams)
}

func (r *Runner) getUsedMemory() uint64 {
	r.modelMutex.RLock()
	defer r.modelMutex.RUnlock()
	memoryUsed := uint64(0)
	for _, modelInstance := range r.activeModelInstances {
		memoryUsed += modelInstance.model.GetMemoryRequirements(modelInstance.filter.Mode)
	}
	return memoryUsed
}

func (r *Runner) getFreeMemory() uint64 {
	return r.Options.MemoryBytes - r.getUsedMemory()
}

// given a running model instance id
// get the next session that it should run
// this is either the nextSession property on the instance
// or it's what is returned by the master API server (if anything)
// this function being called means "I am ready for more work"
// because the child processes are blocking - the child will not be
// asking for more work until it's ready to accept and run it
func (r *Runner) getNextTask(ctx context.Context, instanceID string) (*types.WorkerTask, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("instanceid is required")
	}
	modelInstance, ok := r.activeModelInstances[instanceID]
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	var session *types.Session
	var err error

	// we've already got a session lined up
	// this happens when a session is the first one for a model instance to
	// process and we get into the state where the model instance is booting
	// and it will turn around and start asking for work and we should reply with the
	// initial session
	if modelInstance.initialSession != nil {
		session = modelInstance.initialSession
		modelInstance.initialSession = nil
	} else {
		queryParams := url.Values{}

		queryParams.Add("model_name", string(modelInstance.filter.ModelName))
		queryParams.Add("mode", string(modelInstance.filter.Mode))

		session, err = r.getNextSession(ctx, queryParams)
		if err != nil {
			return nil, err
		}
	}

	// we don't have any work for this model instance
	if session == nil {
		return nil, fmt.Errorf("no session found")
	}

	task, err := modelInstance.assignCurrentSession(ctx, session)
	if err != nil {
		return nil, err
	}

	// TODO: work out what to do with the text stream

	return task, err
}

func (r *Runner) handleTaskResponse(ctx context.Context, instanceID string, taskResponse *types.WorkerTaskResponse) (*types.WorkerTaskResponse, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("instanceid is required")
	}
	if taskResponse == nil {
		return nil, fmt.Errorf("task response is required")
	}
	modelInstance, ok := r.activeModelInstances[instanceID]
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}
	switch {
	case taskResponse.Type == types.WorkerTaskResponseTypeStream:
		err := modelInstance.handleStream(ctx, taskResponse)
		if err != nil {
			log.Error().Msgf("error opening text stream: %s", err.Error())
			return nil, err
		}
	case taskResponse.Type == types.WorkerTaskResponseTypeResult:
		err := modelInstance.handleResult(ctx, taskResponse)
		if err != nil {
			log.Error().Msgf("error opening text stream: %s", err.Error())
			return nil, err
		}
	}

	return taskResponse, nil
}
