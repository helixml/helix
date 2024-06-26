package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/server"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/inhies/go-bytesize"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/rs/zerolog/log"
)

type RunnerOptions struct {
	ID       string
	ApiHost  string
	ApiToken string

	CacheDir string

	Config *config.RunnerConfig

	// these URLs will have the instance ID appended by the model instance
	// e.g. http://localhost:8080/api/v1/worker/task/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/task
	TaskURL string
	// these URLs will have the instance ID appended by the model instance
	// e.g. http://localhost:8080/api/v1/worker/session/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/session
	InitialSessionURL string

	// add this model to the global session query filter
	// so we only run a single model
	FilterModelName string

	// if we only want to run fine-tuning or inference
	// set this and it will be added to the global session filter
	FilterMode string

	// do we want to allow multiple models of the same type to run on this GPU?
	AllowMultipleCopies bool

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

	// used when we are developing platform code without a GPU
	// it will run local python scripts that fake the output
	MockRunner bool
	// if this is defined then we will throw an error for any jobs
	// the error will be the value of this string
	MockRunnerError string

	// how many seconds to delay the mock runner
	MockRunnerDelay int

	// development settings
	// never run more than this number of model instances
	MaxModelInstances int
}

type Runner struct {
	Ctx     context.Context
	Options RunnerOptions

	httpClientOptions system.ClientOptions

	// the map of model instances that we have loaded
	// and are currently running
	activeModelInstances *xsync.MapOf[string, ModelInstance]

	// the lowest amount of memory that something can run with
	// if we have less than this amount of memory then there is
	// no point asking for more top level tasks
	// we get this on boot by asking the model package
	lowestMemoryRequirement uint64

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

	warmupSessions     []types.Session
	warmupSessionMutex sync.Mutex
}

func NewRunner(
	ctx context.Context,
	options RunnerOptions,
	warmupSessions []types.Session,
) (*Runner, error) {

	if options.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	if options.ApiHost == "" {
		return nil, fmt.Errorf("api host required")
	}
	if options.ApiToken == "" {
		return nil, fmt.Errorf("api token is required")
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
		httpClientOptions: system.ClientOptions{
			Host:  options.ApiHost,
			Token: options.ApiToken,
		},
		activeModelInstances:  xsync.NewMapOf[string, ModelInstance](),
		State:                 map[string]types.RunnerTaskResponse{},
		websocketEventChannel: make(chan *types.WebsocketEvent),
		schedulingDecisions:   []string{},
		warmupSessions:        warmupSessions,
	}
	return runner, nil
}

func (r *Runner) Initialize(ctx context.Context) error {
	// connect to the runner websocket server on the api
	// when we write events down the channel - write them to the websocket
	parsedURL, err := url.Parse(system.WSURL(r.httpClientOptions, system.GetApiPath("/ws/runner")))
	if err != nil {
		return err
	}

	fmt.Println("Connecting to controlplane", system.WSURL(r.httpClientOptions, system.GetApiPath("/ws/runner")))

	queryParams := url.Values{}
	queryParams.Add("runnerid", r.Options.ID)
	queryParams.Add("access_token", r.Options.ApiToken)
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
	session, err := r.getNextWarmupSession()
	if err != nil {
		return err
	}

	if session == nil {
		// ask the api server if it currently has any work based on the amount of
		// memory we could free if we killed stale sessions
		session, err = r.getNextGlobalSession(ctx)
		if err != nil {
			return err
		}
	}

	if session != nil {
		// if we need to kill any stale sessions, do it now

		// check for running model instances that have not seen a job in a while
		// and kill them if they are over the timeout AND the session requires it

		// TODO: get the timeout to be configurable from the api and so dynamic
		// based on load
		err = r.checkForStaleModelInstances(ctx, session)
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

func (r *Runner) reportStateLoop(_ context.Context) error {
	state, err := r.getState()
	if err != nil {
		return err
	}
	log.Trace().Msgf("🟠 Sending runner state %s %+v", r.Options.ID, state)
	_, err = system.PostRequest[*types.RunnerState, *types.RunnerState](
		r.httpClientOptions,
		system.GetApiPath(fmt.Sprintf("/runner/%s/state", r.Options.ID)),
		state,
	)
	if err != nil {
		return err
	}
	return nil
}

func GiB(bytes int64) float32 {
	return float32(bytes) / 1024 / 1024 / 1024
}

// loop over the active model instances and stop any that have not processed a job
// in the last timeout seconds
func (r *Runner) checkForStaleModelInstances(_ context.Context, newSession *types.Session) error {
	// calculate stale model instances
	// sort by memory usage
	// kill as few of them as possible to free up newSession much memory

	var (
		allModels []ModelInstance
		stales    []ModelInstance
	)

	r.activeModelInstances.Range(func(key string, activeModelInstance ModelInstance) bool {
		allModels = append(allModels, activeModelInstance)

		if activeModelInstance.Stale() {
			stales = append(stales, activeModelInstance)
		}
		return true
	})

	// sort by memory usage ascending
	sort.Slice(stales, func(i, j int) bool {
		return stales[i].Model().GetMemoryRequirements(stales[i].Filter().Mode) < stales[j].Model().GetMemoryRequirements(stales[j].Filter().Mode)
	})

	// calculate mem required by new session
	modelInstance, err := NewAxolotlModelInstance(
		r.Ctx,
		&ModelInstanceConfig{
			InitialSession:    newSession,
			InitialSessionURL: r.Options.InitialSessionURL,
			NextTaskURL:       r.Options.TaskURL,
			ResponseHandler: func(res *types.RunnerTaskResponse) error {
				return nil
			},
			RunnerOptions: r.Options,
		},
	)
	if err != nil {
		return err
	}

	// we don't need to free as much memory as we already have free
	currentlyAvailableMemory := r.getFreeMemory()
	if currentlyAvailableMemory < 0 {
		currentlyAvailableMemory = 0
	}

	// for this session
	newSessionMemory := modelInstance.model.GetMemoryRequirements(newSession.Mode)
	// this can go negative, so it needs to be a signed integer!
	requiredMemoryFreed := int64(newSessionMemory) - int64(currentlyAvailableMemory)

	if requiredMemoryFreed <= 0 {
		r.addSchedulingDecision("Didn't need to kill any stale sessions because required memory <= 0")
		return nil
	}

	for _, m := range stales {
		if requiredMemoryFreed > 0 {
			r.addSchedulingDecision(fmt.Sprintf(
				"Killing stale model instance %s (%.2fGiB) to make room for %.2fGiB model, requiredMemoryFreed=%.2fGiB, currentlyAvailableMemory=%.2fGiB",
				m.ID(), GiB(int64(m.Model().GetMemoryRequirements(m.Filter().Mode))), GiB(int64(newSessionMemory)), GiB(requiredMemoryFreed), GiB(int64(currentlyAvailableMemory))),
			)
			log.Info().Msgf("Killing stale model instance %s", m.ID())
			err := m.Stop()
			if err != nil {
				log.Error().Msgf("error stopping model instance %s: %s", m.ID(), err.Error())
			}
			r.activeModelInstances.Delete(m.ID())
			requiredMemoryFreed -= int64(m.Model().GetMemoryRequirements(m.Filter().Mode))
		} else {
			r.addSchedulingDecision(fmt.Sprintf("Cleared up enough model memory, overshot by %.2f GiB", GiB(requiredMemoryFreed)))
			log.Info().Msgf("cleared up enough model memory, overshot by %.2f GiB", GiB(requiredMemoryFreed))
			break
		}
	}
	if requiredMemoryFreed > 0 {
		// uh-oh, we didn't free as much memory as we needed to
		r.addSchedulingDecision(fmt.Sprintf(
			"uh-oh, we didn't free as much memory as we needed to for %.2f GiB model by %.2f GiB; stales=%+v, allModels=%+v",
			GiB(int64(newSessionMemory)), GiB(requiredMemoryFreed), stales, allModels,
		))
	}
	return nil
}

func (r *Runner) getNextWarmupSession() (*types.Session, error) {
	if r.Options.MockRunner {
		return nil, nil
	}

	if len(r.warmupSessions) == 0 {
		return nil, nil
	}

	r.warmupSessionMutex.Lock()
	defer r.warmupSessionMutex.Unlock()

	lastIndex := len(r.warmupSessions) - 1
	session := r.warmupSessions[lastIndex]
	r.warmupSessions = r.warmupSessions[:lastIndex]

	return &session, nil
}

// ask the master API if they have the next session for us
// we check with the various models and filter based on the currently free memory
// we pass that free memory back to the master API - it will filter out any tasks
// for models that would require more memory than we have available
func (r *Runner) getNextGlobalSession(ctx context.Context) (*types.Session, error) {
	freeMemory := r.getHypotheticalFreeMemory()

	// only run one for dev mode
	if r.Options.MaxModelInstances > 0 && r.activeModelInstances.Size() >= r.Options.MaxModelInstances {
		return nil, nil
	}

	if freeMemory < int64(r.lowestMemoryRequirement) {
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

	if !r.Options.AllowMultipleCopies {
		// if we are not allowed to run multiple copies of the same model
		// then we need to tell the api what we are currently running
		r.activeModelInstances.Range(func(key string, modelInstance ModelInstance) bool {
			queryParams.Add("reject", fmt.Sprintf(
				"%s:%s:%s",
				modelInstance.Filter().ModelName,
				modelInstance.Filter().Mode,
				modelInstance.Filter().LoraDir,
			))
			return true
		})
	}

	if r.Options.FilterModelName != "" {
		queryParams.Add("model_name", string(r.Options.FilterModelName))
	}

	if r.Options.FilterMode != "" {
		queryParams.Add("mode", string(r.Options.FilterMode))
	}

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
func (r *Runner) readInitialWorkerSession(instanceID string) (*types.Session, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("instanceid is required")
	}
	modelInstance, ok := r.activeModelInstances.Load(instanceID)
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}
	if modelInstance.NextSession() == nil {
		return nil, fmt.Errorf("no session found")
	}
	return modelInstance.NextSession(), nil
}

// we have popped the next session from the master API
// let's create a model for it
// this means instantiating the model instance and then starting it
// because this model consumes memory it means the global next job filter
// will take into account the fact this model is running
// and will add the de-prioritise filter to the next request
// so that we get a different job type
func (r *Runner) createModelInstance(ctx context.Context, initialSession *types.Session) error {
	var (
		modelInstance ModelInstance
		err           error
	)

	runtimeName := initialSession.ModelName.InferenceRuntime()

	// if we are in mock mode - we need the axolotl model instance because
	// it understands how to do a mock runner
	if r.Options.MockRunner {
		if initialSession.Type == types.SessionTypeText {
			runtimeName = types.InferenceRuntimeAxolotl
			initialSession.ModelName = types.Model_Axolotl_Mistral7b
		} else if initialSession.Type == types.SessionTypeImage {
			// I know - this looks odd, but "InferenceRuntimeAxolotl" should actually be called
			// "InferenceRuntimeDefault" - i.e. it's the original "run a python program" version
			// that does both axolotl and sdxl
			runtimeName = types.InferenceRuntimeAxolotl
			initialSession.ModelName = types.Model_Cog_SDXL
		}
	}

	switch runtimeName {
	case types.InferenceRuntimeOllama:
		log.Info().Msg("using Ollama model instance")
		modelInstance, err = NewOllamaModelInstance(
			r.Ctx,
			&ModelInstanceConfig{
				InitialSession:  initialSession,
				ResponseHandler: r.handleWorkerResponse,
				GetNextSession: func() (*types.Session, error) {
					queryParams := url.Values{}

					queryParams.Add("model_name", string(modelInstance.Filter().ModelName))
					queryParams.Add("mode", string(modelInstance.Filter().Mode))
					queryParams.Add("lora_dir", string(modelInstance.Filter().LoraDir))

					nextSession, err := r.getNextApiSession(ctx, queryParams)
					if err != nil {
						return nil, err
					}
					return nextSession, nil
				},
				RunnerOptions: r.Options,
			},
		)
		if err != nil {
			return err
		}
	default:
		// Defaulting to axolotl
		log.Info().Msg("using Axolotl model instance")
		modelInstance, err = NewAxolotlModelInstance(
			r.Ctx,
			&ModelInstanceConfig{
				InitialSession:    initialSession,
				InitialSessionURL: r.Options.InitialSessionURL,
				NextTaskURL:       r.Options.TaskURL,
				// this function will convert any files it sees locally into an upload
				// to the api server filestore - all files will be written to the filestore
				// under a session sub path - you can include tar files and they will untarred at the other end
				// into the filestore
				// TODO: support the tar feature above
				ResponseHandler: func(res *types.RunnerTaskResponse) error {
					return r.handleWorkerResponse(res)
				},
				RunnerOptions: r.Options,
			},
		)
		if err != nil {
			return err
		}

	}

	log.Info().
		Str("model_instance", modelInstance.Filter().ModelName.String()).
		Msgf("🔵 runner started model instance: %s", modelInstance.ID())

	r.activeModelInstances.Store(modelInstance.ID(), modelInstance)

	// THERE IS NOT A RACE HERE (so Kai please stop thinking there is)
	// the files are dowloading at the same time as the python process is booting
	// whilst the files are downloading - there is no session to pull as "nextSession"
	// so even if the python process starts up first - it has nothing to pull until
	// the files have downloaded
	go modelInstance.QueueSession(initialSession, true)

	err = modelInstance.Start(initialSession)
	if err != nil {
		return err
	}

	go func() {
		<-modelInstance.Done()
		log.Debug().
			Msgf("🔵 runner stop model instance: %s", modelInstance.ID())
		r.activeModelInstances.Delete(modelInstance.ID())
	}()
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

func (r *Runner) getNextApiSession(_ context.Context, queryParams url.Values) (*types.Session, error) {

	parsedURL, err := url.Parse(system.URL(r.httpClientOptions, system.GetApiPath(fmt.Sprintf("/runner/%s/nextsession", r.Options.ID))))
	if err != nil {
		return nil, err
	}
	parsedURL.RawQuery = queryParams.Encode()

	req, err := retryablehttp.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return nil, err
	}

	err = system.AddAuthHeadersRetryable(req, r.httpClientOptions.Token)
	if err != nil {
		return nil, err
	}

	client := system.NewRetryClient(3)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var buffer bytes.Buffer
	_, err = io.Copy(&buffer, resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		log.Error().Msgf("error from runner getNextApiSession GET %s: %s", parsedURL.String(), buffer.String())
		return nil, nil
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
	r.activeModelInstances.Range(func(i string, modelInstance ModelInstance) bool {
		memoryUsed += modelInstance.Model().GetMemoryRequirements(modelInstance.Filter().Mode)
		return true
	})
	return memoryUsed
}

func (r *Runner) getUsedMemoryByNonStale() uint64 {
	memoryUsed := uint64(0)

	r.activeModelInstances.Range(func(i string, modelInstance ModelInstance) bool {
		if !modelInstance.Stale() {
			memoryUsed += modelInstance.Model().GetMemoryRequirements(modelInstance.Filter().Mode)
		}
		return true
	})

	return memoryUsed
}

func (r *Runner) getFreeMemory() int64 {
	return int64(r.Options.MemoryBytes) - int64(r.getUsedMemory())
}

func (r *Runner) getHypotheticalFreeMemory() int64 {
	return int64(r.Options.MemoryBytes) - int64(r.getUsedMemoryByNonStale())
}

func (r *Runner) handleWorkerResponse(res *types.RunnerTaskResponse) error {
	// Ignore warmup sessions
	if res.SessionID == types.WarmupTextSessionID || res.SessionID == types.WarmupImageSessionID {
		return nil
	}

	switch res.Type {
	case types.WorkerTaskResponseTypeResult:
		// if it's a full result then we just post it to the api
		log.Info().Msgf("🟠 Sending task response %s %+v", res.SessionID, res)
		return r.postWorkerResponseToApi(res)
	case types.WorkerTaskResponseTypeProgress, types.WorkerTaskResponseTypeStream:
		// streaming updates it's a websocket event
		return r.sendWorkerResponseToWebsocket(res)
	default:
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
	_, err := system.PostRequest[*types.RunnerTaskResponse, *types.RunnerTaskResponse](
		r.httpClientOptions,
		system.GetApiPath(fmt.Sprintf("/runner/%s/response", r.Options.ID)),
		res,
	)
	if err != nil {
		return err
	}
	return nil
}

func (r *Runner) getState() (*types.RunnerState, error) {
	modelInstances := []*types.ModelInstanceState{}
	r.activeModelInstances.Range(func(key string, modelInstance ModelInstance) bool {
		state, err := modelInstance.GetState()
		if err != nil {
			log.Error().Msgf("error getting state for model instance %s (%s): %s", modelInstance.ID(), modelInstance.Filter().ModelName, err.Error())
			return false
		}
		modelInstances = append(modelInstances, state)
		return true
	})
	if len(modelInstances) != r.activeModelInstances.Size() {
		return nil, fmt.Errorf("error getting state, incorrect model instance count")
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
