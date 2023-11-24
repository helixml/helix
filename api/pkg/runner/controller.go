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
	"sort"
	"sync"
	"time"

	"github.com/inhies/go-bytesize"
	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/server"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type RunnerOptions struct {
	ID       string
	ApiHost  string
	ApiToken string

	// this means a CLI will be posting jobs to us locally and we will
	// not be polling a remote api
	LocalMode bool

	// these URLs will have the instance ID appended by the model instance
	// e.g. http://localhost:8080/api/v1/worker/task/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/task
	TaskURL string
	// these URLs will have the instance ID appended by the model instance
	// e.g. http://localhost:8080/api/v1/worker/session/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/session
	InitialSessionURL string

	// how long without running a job before we close a model instance
	ModelInstanceTimeoutSeconds int

	// how long to wait between loops for the controller
	// this will affect how often we ask for a global session
	GetTaskDelayMilliseconds int

	// how often to report our overal state to the api
	ReporStateDelaySeconds int

	// how many bytes of memory does our GPU have?
	// we report this back to the api when we ask
	// for the global next task (well, this minus the
	// currently running models)
	MemoryBytes uint64
	// if this is defined then we convert it usng
	// github.com/inhies/go-bytesize
	MemoryString string

	Labels map[string]string

	SchedulingDecisionBufferSize int
	JobHistoryBufferSize         int
}

type Runner struct {
	Ctx     context.Context
	Options RunnerOptions

	httpClientOptions server.ClientOptions

	// the map of model instances that we have loaded
	// and are currently running
	activeModelInstances *xsync.MapOf[string, *ModelInstance]

	// the lowest amount of memory that something can run with
	// if we have less than this amount of memory then there is
	// no point asking for more top level tasks
	// we get this on boot by asking the model package
	lowestMemoryRequirement uint64

	// local sessions, which will be executed in no particular order
	// TODO: maybe preserve insertion order
	localSessions *xsync.MapOf[string, *types.Session]

	// how we write web sockets messages to the api server
	websocketEventChannel chan *types.WebsocketEvent

	// if we are in "local" mode (i.e. posting jobs to a local runner using "helix run")
	// then we keep state in memory
	// in-memory state to record status that would normally be posted up as a result
	State    map[string]types.RunnerTaskResponse
	StateMtx sync.Mutex

	// TODO: we could make this a struct but there are lots of various
	// things happening and I can't be bothered to define them all
	// so let's just add strings for the moment
	schedulingDecisions []string
}

func NewRunner(
	ctx context.Context,
	options RunnerOptions,
) (*Runner, error) {
	if !options.LocalMode {
		if options.ID == "" {
			return nil, fmt.Errorf("id is required")
		}
		if options.ApiHost == "" {
			return nil, fmt.Errorf("api host required")
		}
		if options.ApiToken == "" {
			return nil, fmt.Errorf("api token is required")
		}
	}
	if options.MemoryString != "" {
		bytes, err := bytesize.Parse(options.MemoryString)
		if err != nil {
			return nil, err
		}
		log.Info().Msgf("Setting memoryBytes = %d", uint64(bytes))
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
		activeModelInstances:  xsync.NewMapOf[string, *ModelInstance](),
		localSessions:         xsync.NewMapOf[string, *types.Session](),
		State:                 map[string]types.RunnerTaskResponse{},
		websocketEventChannel: make(chan *types.WebsocketEvent),
		schedulingDecisions:   []string{},
	}
	return runner, nil
}

func (r *Runner) Initialize(ctx context.Context) error {
	// connect to the runner websocket server on the api
	// when we write events down the channel - write them to the websocket
	parsedURL, err := url.Parse(server.WSURL(r.httpClientOptions, "/ws/runner"))
	if err != nil {
		return err
	}

	queryParams := url.Values{}
	queryParams.Add("runnerid", r.Options.ID)
	parsedURL.RawQuery = queryParams.Encode()

	go server.ConnectRunnerWebSocketClient(
		parsedURL.String(),
		r.websocketEventChannel,
		ctx,
	)

	return nil
}

// this should be run in a go-routine
func (r *Runner) StartLooping() {
	go r.startTaskLoop()
	go r.startReportStateLoop()
}

func (r *Runner) startTaskLoop() {
	for {
		select {
		case <-r.Ctx.Done():
			return
		case <-time.After(time.Millisecond * time.Duration(r.Options.GetTaskDelayMilliseconds)):
			err := r.taskLoop(r.Ctx)
			if err != nil {
				log.Error().Msgf("error in task loop: %s", err.Error())
				debug.PrintStack()
			}
		}
	}
}

func (r *Runner) taskLoop(ctx context.Context) error {

	// ask the api server if it currently has any work based on the amount of
	// memory we could free if we killed stale sessions
	session, err := r.getNextGlobalSession(ctx)
	if err != nil {
		return err
	}

	if session != nil {
		// if we need to kill any stale sessions, do it now

		// check for running model instances that have not seen a job in a while
		// and kill them if they are over the timeout AND the session requires it

		// TODO: get the timeout to be configurable from the api and so dynamic
		// based on load
		err = r.checkForStaleModelInstances(ctx, time.Second*time.Duration(r.Options.ModelInstanceTimeoutSeconds), session)
		if err != nil {
			return err
		}

		log.Debug().
			Msgf("🔵 runner start model instance")
		err = r.createModelInstance(ctx, session)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) startReportStateLoop() {
	for {
		select {
		case <-r.Ctx.Done():
			return
		case <-time.After(time.Second * time.Duration(r.Options.ReporStateDelaySeconds)):
			err := r.reportStateLoop(r.Ctx)
			if err != nil {
				log.Error().Msgf("error in report state loop: %s", err.Error())
				debug.PrintStack()
			}
		}
	}
}

func (r *Runner) reportStateLoop(ctx context.Context) error {
	state, err := r.getState()
	if err != nil {
		return err
	}
	log.Trace().Msgf("🟠 Sending runner state %s %+v", r.Options.ID, state)
	_, err = server.PostRequest[*types.RunnerState, *types.RunnerState](
		r.httpClientOptions,
		fmt.Sprintf("/runner/%s/state", r.Options.ID),
		state,
	)
	if err != nil {
		return err
	}
	return nil
}

func (r *Runner) AddToLocalQueue(ctx context.Context, session *types.Session) error {
	if !r.Options.LocalMode {
		return fmt.Errorf("cannot add to local queue when not in local mode")
	}
	// iterate over model instances to see if one exists and if it doesn't, create it.
	// then add session to localQueue

	// Check if a model instance exists for the session's model ID
	found := false

	// loop over r.activeModelInstances, checking whether the filters on the
	// model instance match the session mode, type and finetune
	r.activeModelInstances.Range(func(key string, modelInstance *ModelInstance) bool {
		if modelInstanceMatchesSession(modelInstance, session) {
			// no need to create another one, because there's already one which will match the session
			log.Debug().Msgf("🟠 Found modelInstance %+v which matches session %+v", modelInstance, session)
			found = true
			return false
		}
		return true
	})
	if !found {
		// Create a new model instance because it doesn't exist
		log.Debug().Msgf("🟠 No currently running modelInstance for session %+v, starting a new one", session)
		err := r.createModelInstance(ctx, session)
		if err != nil {
			return err
		}
	}

	// Add the session to the local queue
	r.localSessions.Store(session.ID, session)
	return nil
}

func GiB(bytes uint64) float32 {
	return float32(bytes) / 1024 / 1024 / 1024
}

// loop over the active model instances and stop any that have not processed a job
// in the last timeout seconds
func (r *Runner) checkForStaleModelInstances(ctx context.Context, timeout time.Duration, newSession *types.Session) error {
	// calculate stale model instances
	// sort by memory usage
	// kill as few of them as possible to free up newSession much memory

	stales := []*ModelInstance{}
	r.activeModelInstances.Range(func(key string, activeModelInstance *ModelInstance) bool {
		stale := false
		if activeModelInstance.lastActivityTimestamp == 0 {
			stale = false
		} else if activeModelInstance.lastActivityTimestamp+int64(timeout.Seconds()) < time.Now().Unix() {
			stale = true
		}
		if stale {
			stales = append(stales, activeModelInstance)
		}
		return true
	})

	// sort by memory usage ascending
	sort.Slice(stales, func(i, j int) bool {
		return stales[i].model.GetMemoryRequirements(stales[i].filter.Mode) < stales[j].model.GetMemoryRequirements(stales[j].filter.Mode)
	})

	// calculate mem required by new session
	modelInstance, err := NewModelInstance(
		r.Ctx,
		newSession,
		r.Options.TaskURL,
		r.Options.InitialSessionURL,
		func(res *types.RunnerTaskResponse) error {
			return nil
		},
		r.Options,
	)
	if err != nil {
		return err
	}

	// we don't need to free as much memory as we already have free
	currentlyAvailableMemory := r.getFreeMemory()

	// for this session
	newSessionMemory := modelInstance.model.GetMemoryRequirements(newSession.Mode)
	requiredMemoryFreed := newSessionMemory - currentlyAvailableMemory

	if requiredMemoryFreed <= 0 {
		r.addSchedulingDecision("Didn't need to kill any stale sessions because required memory <= 0")
		return nil
	}

	for _, m := range stales {
		if requiredMemoryFreed > 0 {
			r.addSchedulingDecision(fmt.Sprintf(
				"Killing stale model instance %s to make room for %.2fGiB model, requiredMemoryFreed=%.2fGiB, currentlyAvailableMemory=%.2fGiB",
				m.id, GiB(newSessionMemory), GiB(requiredMemoryFreed), GiB(currentlyAvailableMemory)),
			)
			log.Info().Msgf("Killing stale model instance %s", m.id)
			err := m.stopProcess()
			if err != nil {
				log.Error().Msgf("error stopping model instance %s: %s", m.id, err.Error())
			}
			r.activeModelInstances.Delete(m.id)
			requiredMemoryFreed -= m.model.GetMemoryRequirements(m.filter.Mode)
		} else {
			r.addSchedulingDecision(fmt.Sprintf("Cleared up enough model memory, overshot by %.2f GiB", GiB(requiredMemoryFreed)))
			log.Info().Msgf("cleared up enough model memory, overshot by %.2f GiB", GiB(requiredMemoryFreed))
			break
		}
	}
	return nil
}

// ask the master API if they have the next session for us
// we check with the various models and filter based on the currently free memory
// we pass that free memory back to the master API - it will filter out any tasks
// for models that would require more memory than we have available
func (r *Runner) getNextGlobalSession(ctx context.Context) (*types.Session, error) {
	if r.httpClientOptions.Host == "" {
		// we are in local only mode... the next session will be injected into
		// us rather than queried from the control server
		// TODO: it would be nice to still support memory limits and other
		// smarts for local tasks
		return nil, nil
	}
	freeMemory := r.getHypotheticalFreeMemory()

	if freeMemory < r.lowestMemoryRequirement {
		// we don't have enough memory to run anything
		// so we just wait for more memory to become available
		return nil, nil
	}

	queryParams := url.Values{}

	// this means "only give me sessions that will fit in this much RAM"
	queryParams.Add("memory", fmt.Sprintf("%d", freeMemory))

	// give currently running models a head start on claiming jobs - this is for
	// when we have > 1 runner
	//
	// TODO: using timing for this prioitization heuristic is flaky and adds
	// latency unnecessarily, instead we could have a bidding system where the
	// api requests bids from all the connected runners and they bid on how
	// quickly they could service the request (e.g. what is their queue length,
	// do they have the model loaded into memory)
	queryParams.Add("older", "2s")

	// now let's loop over our running model instances and de-prioritise them
	// we might still get sessions of this type but only if there isn't another
	// type in the queue - this is to avoid running only one type of model
	// because of the random order the requests arrived in
	// (i.e. if we get 100 text inferences then the chance is we boot 100 model instances)
	// before trying to run another type of model

	r.activeModelInstances.Range(func(key string, modelInstance *ModelInstance) bool {
		queryParams.Add("reject", fmt.Sprintf(
			"%s:%s:%s",
			modelInstance.filter.ModelName,
			modelInstance.filter.Mode,
			modelInstance.filter.LoraDir,
		))
		return true
	})

	session, err := r.getNextApiSession(ctx, queryParams)
	if err != nil {
		return nil, err
	}

	if session != nil {
		r.addSchedulingDecision(fmt.Sprintf("loaded global session %s from api with params %s", session.ID, queryParams.Encode()))
	}

	return session, nil
}

// used by the Python code to know that a session has finished preparing and is ready to pull from the
// queue - this won't actually pull the session from the queue (in the form of a task i.e. getNextTask)
// but it gives the python code a chance to wait for Lora weights to download before loading them
// into GPU memory - at which point it would start pulling from the queue as normal
func (r *Runner) readInitialWorkerSession(ctx context.Context, instanceID string) (*types.Session, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("instanceid is required")
	}
	modelInstance, ok := r.activeModelInstances.Load(instanceID)
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}
	if modelInstance.nextSession == nil {
		return nil, fmt.Errorf("no session found")
	}
	return modelInstance.nextSession, nil
}

// we have popped the next session from the master API
// let's create a model for it
// this means instantiating the model instance and then starting it
// because this model consumes memory it means the global next job filter
// will take into account the fact this model is running
// and will add the de-prioritise filter to the next request
// so that we get a different job type
func (r *Runner) createModelInstance(ctx context.Context, initialSession *types.Session) error {
	modelInstance, err := NewModelInstance(
		r.Ctx,
		initialSession,
		r.Options.TaskURL,
		r.Options.InitialSessionURL,

		// this function will convert any files it sees locally into an upload
		// to the api server filestore - all files will be written to the filestore
		// under a session sub path - you can include tar files and they will untarred at the other end
		// into the filestore
		// TODO: support the tar feature above
		func(res *types.RunnerTaskResponse) error {
			if r.Options.LocalMode {
				err := r.addLocalResponse(ctx, res)
				if err != nil {
					return err
				}
				return nil
			} else {
				// if the response is for the initial session then inclide
				return r.handleWorkerResponse(res)
			}
		},
		r.Options,
	)
	if err != nil {
		return err
	}

	// belt and braces in remote case and reject jobs that won't fit in local case
	modelMem := float32(modelInstance.model.GetMemoryRequirements(initialSession.Mode)) / 1024 / 1024 / 1024
	freeMem := float32(r.getFreeMemory()) / 1024 / 1024 / 1024
	if modelMem > freeMem {
		// refuse to start or record the model instance, it will just get GC'd at this point
		return fmt.Errorf("cannot fit model requiring gpu memory %.2f into available gpu memory %.2f", modelMem, freeMem)
	}
	log.Debug().Msgf("🔵 Fitting model requiring gpu memory %.2f into available gpu memory %.2f", modelMem, freeMem)
	log.Debug().
		Msgf("🔵 runner started model instance: %s", modelInstance.id)

	r.activeModelInstances.Store(modelInstance.id, modelInstance)

	// THERE IS NOT A RACE HERE (so Kai please stop thinking there is)
	// the files are dowloading at the same time as the python process is booting
	// whilst the files are downloading - there is no session to pull as "nextSession"
	// so even if the python process starts up first - it has nothing to pull until
	// the files have downloaded
	go modelInstance.queueSession(initialSession, true)

	err = modelInstance.startProcess(initialSession)
	if err != nil {
		return err
	}

	go func() {
		<-modelInstance.finishChan
		log.Debug().
			Msgf("🔵 runner stop model instance: %s", modelInstance.id)
		r.activeModelInstances.Delete(modelInstance.id)
	}()
	return nil
}

func (r *Runner) addLocalResponse(ctx context.Context, res *types.RunnerTaskResponse) error {
	r.StateMtx.Lock()
	defer r.StateMtx.Unlock()

	// record in-memory for any local clients who want to query us
	r.State[res.SessionID] = *res

	stateYAML, err := yaml.Marshal(r.State)
	if err != nil {
		return err
	}
	fmt.Println("==========================================")
	fmt.Println("             LOCAL STATE")
	fmt.Println("==========================================")
	fmt.Println(string(stateYAML))
	fmt.Println("==========================================")

	return nil
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
	modelInstance, ok := r.activeModelInstances.Load(instanceID)
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	var session *types.Session
	foundLocalQueuedSession := false

	if r.Options.LocalMode {
		r.localSessions.Range(func(i string, sess *types.Session) bool {
			if modelInstanceMatchesSession(modelInstance, sess) {
				foundLocalQueuedSession = true
				// remove it from the local queue
				r.localSessions.Delete(i)
				session = sess
				return false
			}
			return true
		})
	}

	// as the first check, we need to ask if there's a session in localQueue
	// that matches this model instance. if there is, we've got a local
	// session and it takes precedence over remote work

	// if there is, call modelInstance.queueSession on it

	if foundLocalQueuedSession {
		// queue it, and fall thru below to assign
		log.Printf("🟠🟠 Found local queued session %+v for model instance %+v", session, modelInstance)
		go modelInstance.queueSession(session, false)
	} else if modelInstance.nextSession != nil {
		// if there is a session in the nextSession cache then we return it immediately
		log.Printf("🟣🟣 loading modelInstance.nextSession %+v", modelInstance.nextSession)
		session = modelInstance.nextSession
		modelInstance.nextSession = nil
	} else if modelInstance.queuedSession != nil {
		// if there is a session in the queuedSession cache then we are waiting for
		// a task to complete before we want to actually run the session
	} else {
		// ask the upstream api server if there is another task
		// if there is - then assign it to the queuedSession
		// and call "pre"
		if r.httpClientOptions.Host != "" {
			queryParams := url.Values{}

			queryParams.Add("model_name", string(modelInstance.filter.ModelName))
			queryParams.Add("mode", string(modelInstance.filter.Mode))
			queryParams.Add("lora_dir", string(modelInstance.filter.LoraDir))

			apiSession, err := r.getNextApiSession(ctx, queryParams)
			if err != nil {
				return nil, err
			}

			if apiSession != nil {
				go modelInstance.queueSession(apiSession, false)
			}
		}
	}

	// we don't have any work for this model instance
	if session == nil {
		// TODO: this should be a 404 not a 500?
		return nil, fmt.Errorf("no session found")
	}

	task, err := modelInstance.assignSessionTask(ctx, session)
	if err != nil {
		return nil, err
	}

	return task, nil
}

func (r *Runner) getNextApiSession(ctx context.Context, queryParams url.Values) (*types.Session, error) {
	parsedURL, err := url.Parse(server.URL(r.httpClientOptions, fmt.Sprintf("/runner/%s/nextsession", r.Options.ID)))
	if err != nil {
		return nil, err
	}
	parsedURL.RawQuery = queryParams.Encode()

	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return nil, err
	}

	server.AddAutheaders(req, r.httpClientOptions.Token)

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

func (r *Runner) getUsedMemory() uint64 {
	memoryUsed := uint64(0)
	r.activeModelInstances.Range(func(i string, modelInstance *ModelInstance) bool {
		memoryUsed += modelInstance.model.GetMemoryRequirements(modelInstance.filter.Mode)
		return true
	})
	return memoryUsed
}

func (r *Runner) getUsedMemoryByNonStale() uint64 {
	timeout := time.Second * time.Duration(r.Options.ModelInstanceTimeoutSeconds)

	memoryUsed := uint64(0)
	r.activeModelInstances.Range(func(i string, modelInstance *ModelInstance) bool {
		// assume stale
		stale := true
		// this means we are booting so let's leave it alone to boot
		if modelInstance.lastActivityTimestamp == 0 {
			stale = false
		}
		// the model is not stale don't include it
		if modelInstance.lastActivityTimestamp+int64(timeout.Seconds()) < time.Now().Unix() {
			stale = false
		}
		if stale {
			memoryUsed += modelInstance.model.GetMemoryRequirements(modelInstance.filter.Mode)
		}
		return true
	})
	return memoryUsed
}

func (r *Runner) getFreeMemory() uint64 {
	return r.Options.MemoryBytes - r.getUsedMemory()
}

func (r *Runner) getHypotheticalFreeMemory() uint64 {
	return r.Options.MemoryBytes - r.getUsedMemoryByNonStale()
}

func (r *Runner) handleWorkerResponse(res *types.RunnerTaskResponse) error {
	if res.Type == types.WorkerTaskResponseTypeResult {
		// if it's a full result then we just post it to the api
		return r.postWorkerResponseToApi(res)
	} else if res.Type == types.WorkerTaskResponseTypeProgress || res.Type == types.WorkerTaskResponseTypeStream {
		// otherwise for streaming updates it's a websocket event
		return r.sendWorkerResponseToWebsocket(res)
	} else {
		return fmt.Errorf("unknown response type: %s", res.Type)
	}
}

func (r *Runner) sendWorkerResponseToWebsocket(res *types.RunnerTaskResponse) error {
	r.websocketEventChannel <- &types.WebsocketEvent{
		Type:               types.WebsocketEventWorkerTaskResponse,
		SessionID:          res.SessionID,
		Owner:              res.Owner,
		WorkerTaskResponse: res,
	}
	return nil
}

func (r *Runner) postWorkerResponseToApi(res *types.RunnerTaskResponse) error {
	log.Debug().Msgf("🟠 Sending task response %s %+v", res.SessionID, res)

	// this function will write any task responses back to the api server for it to process
	// we will only hear WorkerTaskResponseTypeStreamContinue and WorkerTaskResponseTypeResult
	// and both of these will have an interaction ID and the full, latest copy of the text
	// the job of the api server is to ensure the existence of the instance (create or update)
	// and replace it's message property - this is the text streaming case
	// if the model does not return a text stream - then all we will hear is a WorkerTaskResponseTypeResult
	// and the api server is just appending to the session
	_, err := server.PostRequest[*types.RunnerTaskResponse, *types.RunnerTaskResponse](
		r.httpClientOptions,
		fmt.Sprintf("/runner/%s/response", r.Options.ID),
		res,
	)
	if err != nil {
		return err
	}
	return nil
}

func (r *Runner) getState() (*types.RunnerState, error) {
	modelInstances := []*types.ModelInstanceState{}
	r.activeModelInstances.Range(func(key string, modelInstance *ModelInstance) bool {
		state, err := modelInstance.getState()
		if err != nil {
			return false
		}
		modelInstances = append(modelInstances, state)
		return true
	})
	if len(modelInstances) != r.activeModelInstances.Size() {
		return nil, fmt.Errorf("error getting state")
	}
	return &types.RunnerState{
		ID:                  r.Options.ID,
		Created:             time.Now(),
		TotalMemory:         r.Options.MemoryBytes,
		FreeMemory:          r.getFreeMemory(),
		Labels:              r.Options.Labels,
		ModelInstances:      modelInstances,
		SchedulingDecisions: r.schedulingDecisions,
	}, nil
}

func (r *Runner) addSchedulingDecision(decision string) {
	r.schedulingDecisions = append([]string{decision}, r.schedulingDecisions...)

	if len(r.schedulingDecisions) > r.Options.SchedulingDecisionBufferSize {
		r.schedulingDecisions = r.schedulingDecisions[:len(r.schedulingDecisions)-1]
	}
}
