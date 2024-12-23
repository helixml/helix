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
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/server"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/inhies/go-bytesize"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/rs/zerolog/log"
)

type Options struct {
	ID       string
	APIHost  string
	APIToken string

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

	RuntimeFactory SlotFactory
}

type Runner struct {
	Ctx                   context.Context
	Options               Options
	httpClientOptions     system.ClientOptions
	activeModelInstances  *xsync.MapOf[string, ModelInstance] // the map of model instances that we have loaded and are currently running
	websocketEventChannel chan *types.WebsocketEvent          // how we write web sockets messages to the api server
	slots                 map[uuid.UUID]*Slot                 // A map recording the slots running on this runner
	slotFactory           SlotFactory                         // A factory to create new slots. Required for testing since we don't actually want to spin up ollama on each test
}

func NewRunner(
	ctx context.Context,
	options Options,
) (*Runner, error) {

	if options.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	if options.APIHost == "" {
		return nil, fmt.Errorf("api host required")
	}
	if options.APIToken == "" {
		return nil, fmt.Errorf("api token is required")
	}
	if options.RuntimeFactory == nil {
		options.RuntimeFactory = &runtimeFactory{}
	}

	// Remove trailing slash from ApiHost if present
	options.APIHost = strings.TrimSuffix(options.APIHost, "/")

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
	runner := &Runner{
		Ctx:     ctx,
		Options: options,
		httpClientOptions: system.ClientOptions{
			Host:  options.APIHost,
			Token: options.APIToken,
		},
		activeModelInstances:  xsync.NewMapOf[string, ModelInstance](),
		websocketEventChannel: make(chan *types.WebsocketEvent),
		slots:                 make(map[uuid.UUID]*Slot),
		slotFactory:           options.RuntimeFactory,
	}
	return runner, nil
}

func (r *Runner) Initialize(ctx context.Context) error {
	// connect to the runner websocket server on the api
	// when we write events down the channel - write them to the websocket
	parsedURL, err := url.Parse(system.WSURL(r.httpClientOptions, system.GetAPIPath("/ws/runner")))
	if err != nil {
		return err
	}

	fmt.Println("Connecting to controlplane", system.WSURL(r.httpClientOptions, system.GetAPIPath("/ws/runner")))

	queryParams := url.Values{}
	queryParams.Add("runnerid", r.Options.ID)
	queryParams.Add("access_token", r.Options.APIToken)
	parsedURL.RawQuery = queryParams.Encode()

	go server.ConnectRunnerWebSocketClient(
		ctx,
		parsedURL.String(),
		r.websocketEventChannel,
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

func (r *Runner) pollSlots(_ context.Context) error {
	// TODO(PHIL): The old warmup code was sketchy. It had two paths (V1/V2 thing). And mostly just
	// called the old llama:instruct model. Ideally the warmup should be orchestrated from the
	// control plane anyway. So I'm just removing it for now.

	desiredSlots, err := r.getSlots()
	if err != nil {
		return err
	}

	l := log.With().
		Str("runner_id", r.Options.ID).
		Interface("r.slots", r.slots).
		Interface("desiredSlots", desiredSlots).
		Logger()

	l.Trace().Msg("pollSlots")

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
			l.Trace().Str("slot_id", slotID.String()).Msg("deleting slot")
			runtime.Stop()

			// TODO(PHIL): Remove this only required by axolotl
			if runtime.modelInstance != nil {
				r.activeModelInstances.Delete(runtime.modelInstance.ID())
			}

			delete(r.slots, slotID)
		}
	}

	for _, slot := range desiredSlots.Data {
		// If there's no work, then we don't need to do anything for now
		if slot.Attributes.Workload == nil {
			continue
		}

		l.Debug().Str("slot_id", slot.ID.String()).Msg("slot has workload")

		// If there is work, then parse the RunnerWorkload into a Workload
		var work *scheduler.Workload
		if slot.Attributes.Workload.LLMInferenceRequest != nil {
			work, err = scheduler.NewLLMWorkload(slot.Attributes.Workload.LLMInferenceRequest)
			if err != nil {
				return err
			}
		}
		if slot.Attributes.Workload.Session != nil {
			work, err = scheduler.NewSessionWorkload(slot.Attributes.Workload.Session)
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
			l.Debug().Str("slot_id", slot.ID.String()).Msg("starting new runtime")
			runtime, err = r.startNewRuntime(slot.ID, work)
			if err != nil {
				return err
			}
			// TODO: Could do with storing this BEFORE it has started, because it takes time to start.
			r.slots[slot.ID] = runtime
			continue
		}

		// If the runtime already has work scheduled, then we don't need to do anything
		if runtime.IsScheduled() {
			l.Trace().Str("slot_id", slot.ID.String()).Msg("runtime already scheduled")
			continue
		}

		// If the runtime is already running a workload, then we don't need to do anything
		if runtime.modelInstance.IsActive() {
			l.Trace().Str("slot_id", slot.ID.String()).Msg("runtime already active")
			continue
		}

		// If we get here then that means the slot already has a runtime and is waiting for work
		switch work.WorkloadType {
		case scheduler.WorkloadTypeLLMInferenceRequest:
			log.Debug().Str("workload_id", work.ID()).Msg("enqueuing LLM inference request")
			runtime.SetLLMInferenceRequest(work)
		case scheduler.WorkloadTypeSession:
			log.Debug().Str("workload_id", work.ID()).Msg("enqueuing session request")
			runtime.SetSessionRequest(work)
		}
	}

	return nil
}

func (r *Runner) getSlots() (*types.GetDesiredRunnerSlotsResponse, error) {
	parsedURL, err := url.Parse(system.URL(r.httpClientOptions, system.GetAPIPath(fmt.Sprintf("/runner/%s/slots", r.Options.ID))))
	if err != nil {
		return nil, err
	}

	req, err := retryablehttp.NewRequestWithContext(r.Ctx, "GET", parsedURL.String(), nil)
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

	var slots *types.GetDesiredRunnerSlotsResponse
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
		system.GetAPIPath(fmt.Sprintf("/runner/%s/state", r.Options.ID)),
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

// TODO(Phil): This is currently required by the axolotl server. Since I'm updating the axolotl
// version (and server) in another branch I don't want to convert it to the slots methodology just
// yet.
// nolint:unused
func (r *Runner) getNextAPISession(_ context.Context, queryParams url.Values) (*types.Session, error) {
	parsedURL, err := url.Parse(system.URL(r.httpClientOptions, system.GetAPIPath(fmt.Sprintf("/runner/%s/nextsession", r.Options.ID))))
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
	r.activeModelInstances.Range(func(_ string, modelInstance ModelInstance) bool {
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
		return r.postWorkerResponseToAPI(res)
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

func (r *Runner) postWorkerResponseToAPI(res *types.RunnerTaskResponse) error {
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
		system.GetAPIPath(fmt.Sprintf("/runner/%s/response", r.Options.ID)),
		res,
	)
	if err != nil {
		return err
	}
	return nil
}

func (r *Runner) getState() (*types.RunnerState, error) {
	modelInstances := []*types.ModelInstanceState{}
	r.activeModelInstances.Range(func(_ string, modelInstance ModelInstance) bool {
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
		SchedulingDecisions: []string{"[Deprecated] Runners no longer make scheduling decisions. This will be removed shortly"},
		Version:             data.GetHelixVersion(),
		Slots:               r.getRunnerSlots(),
	}, nil
}

func (r *Runner) getRunnerSlots() []types.RunnerActualSlot {
	slots := []types.RunnerActualSlot{}
	for slotID, runtime := range r.slots {
		slots = append(slots, types.RunnerActualSlot{
			ID: slotID,
			Attributes: types.RunnerActualSlotAttributes{
				OriginalWorkload: runtime.OriginalWorkload(),
				CurrentWorkload:  runtime.CurrentWorkload(),
			},
		})
	}
	return slots
}

func (r *Runner) startNewRuntime(slotID uuid.UUID, work *scheduler.Workload) (*Slot, error) {
	runtime, err := r.slotFactory.NewSlot(r.Ctx, slotID, work, r.handleInferenceResponse, r.handleWorkerResponse, r.Options)
	if err != nil {
		return nil, err
	}

	// TODO(PHIL): Remove this. Required for use in the axolotl runner and dashboard data for now.
	r.activeModelInstances.Store(runtime.modelInstance.ID(), runtime.modelInstance)
	return runtime, nil
}

func ErrorSession(sessionResponseHandler func(res *types.RunnerTaskResponse) error, session *types.Session, err error) error {
	return sessionResponseHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeResult,
		SessionID: session.ID,
		Error:     err.Error(),
	})
}
