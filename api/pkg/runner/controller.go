package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/scheduler"
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
	ReportStateDelaySeconds int

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

	nextRequestMutex       sync.Mutex
	nextGlobalRequestMutex sync.Mutex

	slots map[uuid.UUID]*runtime
}

type runtime struct {
	modelInstance   ModelInstance
	llmWorkChan     chan *types.RunnerLLMInferenceRequest
	sessionWorkChan chan *types.Session
	currentWork     *scheduler.Workload
}

func (r *runtime) Stop() {
	err := r.modelInstance.Stop()
	if err != nil {
		log.Err(err).Msg("error stopping model instance")
	}
	if r.llmWorkChan != nil {
		close(r.llmWorkChan)
	}
	if r.sessionWorkChan != nil {
		close(r.sessionWorkChan)
	}
}

func (r *runtime) CurrentWorkload() *types.RunnerWorkload {
	if r.currentWork == nil {
		return &types.RunnerWorkload{}
	}
	return r.currentWork.ToRunnerWorkload()
}

func (r *runtime) SetLLMInferenceRequest(work *scheduler.Workload) {
	r.currentWork = work
	r.llmWorkChan <- work.LLMInferenceRequest()
}

func (r *runtime) SetSessionRequest(work *scheduler.Workload) {
	r.currentWork = work
	r.sessionWorkChan <- work.Session()
}

func (r *runtime) IsScheduled() bool {
	return len(r.sessionWorkChan) > 0 || len(r.llmWorkChan) > 0
}

func (r *Runner) startNewRuntime(work *scheduler.Workload) (*runtime, error) {
	switch work.WorkloadType {
	case scheduler.WorkloadTypeLLMInferenceRequest:
		log.Debug().Str("workload_id", work.ID()).Msg("starting new ollama runtime")
		workCh := make(chan *types.RunnerLLMInferenceRequest, 1)
		ollama, err := NewOllamaInferenceModelInstance(
			r.Ctx,
			&InferenceModelInstanceConfig{
				ResponseHandler: r.handleInferenceResponse,
				GetNextRequest: func() (*types.RunnerLLMInferenceRequest, error) {
					return <-workCh, nil
				},
				RunnerOptions: r.Options,
			},
			work.LLMInferenceRequest(),
		)
		if err != nil {
			return nil, fmt.Errorf("error creating ollama runtime: %s", err.Error())
		}
		err = ollama.Start(r.Ctx)
		if err != nil {
			return nil, fmt.Errorf("error starting ollama runtime: %s", err.Error())
		}
		return &runtime{
			modelInstance: ollama,
			llmWorkChan:   workCh,
		}, nil
	case scheduler.WorkloadTypeSession:
		log.Debug().Str("workload_id", work.ID()).Msg("starting new session runtime")
		var (
			modelInstance ModelInstance
			err           error
		)
		workCh := make(chan *types.Session, 1)
		initialSession := work.Session()
		runtimeName := model.ModelName(initialSession.ModelName).InferenceRuntime()

		// if we are in mock mode - we need the axolotl model instance because
		// it understands how to do a mock runner
		if r.Options.MockRunner {
			if initialSession.Type == types.SessionTypeText {
				runtimeName = types.InferenceRuntimeAxolotl
				initialSession.ModelName = string(model.Model_Axolotl_Mistral7b)
			} else if initialSession.Type == types.SessionTypeImage {
				// I know - this looks odd, but "InferenceRuntimeAxolotl" should actually be called
				// "InferenceRuntimeDefault" - i.e. it's the original "run a python program" version
				// that does both axolotl and sdxl
				runtimeName = types.InferenceRuntimeAxolotl
				initialSession.ModelName = string(model.Model_Cog_SDXL)
			}
		}

		switch runtimeName {
		case types.InferenceRuntimeOllama:
			log.Debug().Str("workload_id", work.ID()).Msg("starting new ollama session runtime")
			modelInstance, err = NewOllamaModelInstance(
				r.Ctx,
				&ModelInstanceConfig{
					InitialSession:  initialSession,
					ResponseHandler: r.handleWorkerResponse,
					GetNextSession: func() (*types.Session, error) {
						return <-workCh, nil
					},
					RunnerOptions: r.Options,
				},
			)
			if err != nil {
				return nil, err
			}
			err = modelInstance.Start(r.Ctx)
			if err != nil {
				return nil, err
			}
			return &runtime{
				modelInstance:   modelInstance,
				sessionWorkChan: workCh,
			}, nil
		default:
			// Defaulting to axolotl
			log.Debug().Str("workload_id", work.ID()).Msg("starting new axolotl session runtime")
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
				return nil, err
			}

			// THERE IS NOT A RACE HERE (so Kai please stop thinking there is)
			// the files are dowloading at the same time as the python process is booting
			// whilst the files are downloading - there is no session to pull as "nextSession"
			// so even if the python process starts up first - it has nothing to pull until
			// the files have downloaded
			go modelInstance.QueueSession(initialSession, true)

			err = modelInstance.Start(r.Ctx)
			if err != nil {
				return nil, err
			}

			go func() {
				select {
				case <-r.Ctx.Done():
					return
				case work := <-workCh:
					go modelInstance.QueueSession(work, false)
				case <-modelInstance.Done():
					return
				}
			}()
			return &runtime{
				modelInstance:   modelInstance,
				sessionWorkChan: workCh,
			}, nil
		}
	default:
		return nil, fmt.Errorf("unknown workload type: %s", work.WorkloadType)
	}
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

	// Remove trailing slash from ApiHost if present
	options.ApiHost = strings.TrimSuffix(options.ApiHost, "/")

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
		slots:                 make(map[uuid.UUID]*runtime),
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
func (r *Runner) Run() {
	err := r.warmupInference(context.Background())
	if err != nil {
		log.Error().Msgf("error in warmup inference: %s", err.Error())
		debug.PrintStack()
	} else {
		log.Info().Msg("🟢 warmup inference complete")
	}

	go r.startTaskLoop()
	go r.startReportStateLoop()
}

func (r *Runner) startTaskLoop() {
	for {
		select {
		case <-r.Ctx.Done():
			return
		case <-time.After(time.Millisecond * time.Duration(r.Options.GetTaskDelayMilliseconds)):
			err := r.pollSlots(r.Ctx)
			if err != nil {
				log.Err(err).Msg("error in pollSlots")
				debug.PrintStack()
			}
		}
	}
}

func (r *Runner) pollSlots(ctx context.Context) error {
	desiredSlots, err := r.getSlots()
	if err != nil {
		return err
	}

	log.Trace().Interface("r.slots", r.slots).Interface("desiredSlots", desiredSlots).Msg("desired slots")

	// First stop and delete any runtimes that are no longer needed
	for slotID, runtime := range r.slots {
		found := false
		for _, slot := range desiredSlots.Data {
			if slot.ID == slotID {
				found = true
				break
			}
		}
		if !found {
			runtime.Stop()
			delete(r.slots, slotID)
		}
	}

	for _, slot := range desiredSlots.Data {
		// If there's no work, then we don't need to do anything for now
		if slot.Attributes.Workload == nil {
			continue
		}

		// If there is work, then parse the RunnerWorkload into a Workload
		var work *scheduler.Workload
		if slot.Attributes.Workload.LLMInferenceRequest != nil {
			work, err = scheduler.NewLLMWorkload(slot.Attributes.Workload.LLMInferenceRequest)
			if err != nil {
				return err
			}
		}
		if slot.Attributes.Workload.Session != nil {
			work, err = scheduler.NewSessonWorkload(slot.Attributes.Workload.Session)
			if err != nil {
				return err
			}
		}
		if work == nil {
			return fmt.Errorf("unable to parse workload")
		}

		// Get the current runtime for the slot
		runtime, ok := r.slots[slot.ID]

		// If it doesn't exist, start a new runtime and save
		if !ok {
			runtime, err = r.startNewRuntime(work)
			if err != nil {
				return err
			}
			r.slots[slot.ID] = runtime
			continue
		}

		// If the runtime already has work scheduled, then we don't need to do anything
		if runtime.IsScheduled() {
			continue
		}

		// If the runtime is already running a workload, then we don't need to do anything
		if runtime.modelInstance.IsActive() {
			continue
		}

		// If we get here then that means the slot already has a runtime and is waiting for work
		switch work.WorkloadType {
		case scheduler.WorkloadTypeLLMInferenceRequest:
			log.Debug().Str("workload_id", work.ID()).Msg("enqueuing LLM inference request")
			runtime.llmWorkChan <- work.LLMInferenceRequest()

		case scheduler.WorkloadTypeSession:
			log.Debug().Str("workload_id", work.ID()).Msg("enqueuing session request")
			runtime.sessionWorkChan <- work.Session()
		}
	}

	return nil
}

func (r *Runner) getSlots() (*types.PatchRunnerSlots, error) {
	r.nextRequestMutex.Lock()
	defer r.nextRequestMutex.Unlock()

	parsedURL, err := url.Parse(system.URL(r.httpClientOptions, system.GetApiPath(fmt.Sprintf("/runner/%s/slots", r.Options.ID))))
	if err != nil {
		return nil, err
	}

	runnerSlots := make([]types.RunnerSlot, 0, len(r.slots))
	for slotID, runtime := range r.slots {
		runnerSlot := types.RunnerSlot{
			ID: slotID,
			Attributes: types.RunnerSlotAttributes{
				Workload: runtime.CurrentWorkload(),
			},
		}
		runnerSlots = append(runnerSlots, runnerSlot)
	}
	patch := &types.PatchRunnerSlots{
		Data: runnerSlots,
	}

	body, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}

	req, err := retryablehttp.NewRequest("GET", parsedURL.String(), body)
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
		log.Error().Int("status_code", resp.StatusCode).Str("url", parsedURL.String()).Str("body", buffer.String()).Msgf("error response from server")
		return nil, nil
	}

	var slots *types.PatchRunnerSlots
	err = json.Unmarshal(buffer.Bytes(), &slots)
	if err != nil {
		return nil, err
	}

	return slots, nil
}

func (r *Runner) startReportStateLoop() {
	for {
		select {
		case <-r.Ctx.Done():
			return
		case <-time.After(time.Second * time.Duration(r.Options.ReportStateDelaySeconds)):
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

func (r *Runner) getUsedMemory() uint64 {
	memoryUsed := uint64(0)
	r.activeModelInstances.Range(func(i string, modelInstance ModelInstance) bool {
		memoryUsed += modelInstance.Model().GetMemoryRequirements(modelInstance.Filter().Mode)
		return true
	})
	return memoryUsed
}

func (r *Runner) getFreeMemory() int64 {
	return int64(r.Options.MemoryBytes) - int64(r.getUsedMemory())
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

func (r *Runner) handleInferenceResponse(resp *types.RunnerLLMInferenceResponse) error {
	r.websocketEventChannel <- &types.WebsocketEvent{
		Type:              types.WebsocketLLMInferenceResponse,
		SessionID:         resp.SessionID,
		Owner:             resp.OwnerID,
		InferenceResponse: resp,
	}

	return nil
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
		Version:             data.GetHelixVersion(),
		Slots:               r.getCurrentSlots(),
	}, nil
}

func (r *Runner) getCurrentSlots() []types.RunnerSlot {
	slots := []types.RunnerSlot{}
	for slotID, runtime := range r.slots {
		slots = append(slots, types.RunnerSlot{
			ID: slotID,
			Attributes: types.RunnerSlotAttributes{
				Workload: runtime.CurrentWorkload(),
			},
		})
	}
	return slots
}
