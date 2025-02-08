package scheduler

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"golang.org/x/exp/rand"
)

// TimeoutFunc defines a function type that determines if a runner has timed out based on the last activity.
type TimeoutFunc func(runnerID string, lastActivityTime time.Time) bool

func NewTimeoutFunc(ttl time.Duration) TimeoutFunc {
	return func(_ string, lastActivityTime time.Time) bool {
		return lastActivityTime.Add(ttl).Before(time.Now())
	}
}

type Scheduler struct {
	ctx             context.Context
	controller      *RunnerController
	queue           []*Workload
	queueMtx        *sync.RWMutex
	queueSize       int
	onSchedulingErr func(work *Workload, err error)
	slots           map[uuid.UUID]*Slot // Maps slot ID to Slot details. Map because we want control over mutex
	slotsMtx        *sync.RWMutex
	modelStaleFunc  TimeoutFunc // Function to check if models are stale
	slotTimeoutFunc TimeoutFunc // Function to check if slots have timed out due to error
}

type Params struct {
	RunnerController  *RunnerController
	QueueSize         int
	OnSchedulingErr   func(work *Workload, err error)
	OnResponseHandler func(ctx context.Context, resp *types.RunnerLLMInferenceResponse) error
}

func NewScheduler(ctx context.Context, serverConfig *config.ServerConfig, params *Params) (*Scheduler, error) {
	if params == nil {
		params = &Params{}
	}
	if params.RunnerController == nil {
		return nil, fmt.Errorf("runner controller is required")
	}
	modelTTL := serverConfig.Providers.Helix.ModelTTL
	if modelTTL == 0 {
		modelTTL = 10 * time.Second
	}
	slotTTL := serverConfig.Providers.Helix.SlotTTL
	if slotTTL == 0 {
		slotTTL = 300 * time.Second
	}
	queueSize := 100
	if params.QueueSize > 0 {
		queueSize = params.QueueSize
	}

	log.Info().Dur("model_stale_time", modelTTL).Dur("slot_timeout", slotTTL).Msg("slot timeouts")

	s := &Scheduler{
		ctx:             ctx,
		controller:      params.RunnerController,
		queueSize:       queueSize,
		queue:           make([]*Workload, 0, queueSize),
		queueMtx:        &sync.RWMutex{},
		onSchedulingErr: params.OnSchedulingErr,
		slots:           make(map[uuid.UUID]*Slot),
		slotsMtx:        &sync.RWMutex{},
		modelStaleFunc:  NewTimeoutFunc(modelTTL),
		slotTimeoutFunc: NewTimeoutFunc(slotTTL),
	}

	// Start the queue processor
	go s.processQueue(ctx)

	// Start the slot reconciler
	go s.reconcileSlots(ctx)

	return s, nil
}

func (s *Scheduler) Enqueue(work *Workload) error {
	s.queueMtx.Lock()
	defer s.queueMtx.Unlock()

	// Check if the work is already in the queue.
	for _, w := range s.queue {
		if w.ID() == work.ID() {
			return fmt.Errorf("work already in queue")
		}
	}

	if len(s.queue) >= s.queueSize {
		return fmt.Errorf("queue is full")
	}

	// Check if the work is a session and has priority
	if work.WorkloadType == WorkloadTypeSession {
		if work.Session().Metadata.Priority {
			// Add the work to the front of the queue.
			// Ignoring the order of other priority sessions here to avoid complexity
			s.queue = append([]*Workload{work}, s.queue...)
			return nil
		}
	}

	// Queue the work
	s.queue = append(s.queue, work)

	return nil
}

func (s *Scheduler) Queue() ([]*types.WorkloadSummary, error) {
	s.queueMtx.RLock()
	defer s.queueMtx.RUnlock()
	queue := make([]*types.WorkloadSummary, 0, len(s.queue))
	for _, w := range s.queue {
		queue = append(queue, &types.WorkloadSummary{
			ID:        w.ID(),
			CreatedAt: w.Created(),
			UpdatedAt: w.Updated(),
		})
	}
	return queue, nil
}

func (s *Scheduler) RunnerStatus() ([]*types.RunnerStatus, error) {
	// Get a current list of runners
	runners := s.controller.RunnerIDs()

	// Get the current state of each runner
	runnerStates := make([]*types.RunnerStatus, 0, len(runners))
	for _, runnerID := range runners {
		runnerStatus, err := s.controller.getStatus(runnerID)
		if err != nil {
			return nil, err
		}
		runnerStates = append(runnerStates, runnerStatus)
	}

	return runnerStates, nil
}

func (s *Scheduler) RunnerSlots(runnerID string) ([]*types.RunnerSlot, error) {
	runnerSlots, err := s.controller.Slots(runnerID)
	if err != nil {
		return nil, err
	}
	return runnerSlots, nil
}

// processQueue runs in a goroutine to processes the queue of requests.
func (s *Scheduler) processQueue(ctx context.Context) {
	log.Debug().Msg("starting queue processor")
	for {
		select {
		case <-ctx.Done():
			return
		default:
			s.processQueueOnce(ctx)
			// Sleep for a while to allow others to access the queue
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// reconcileSlots runs in a goroutine to reconcile slots.
// The reason why we do this async is because we don't want to have to check the runner on the hot
// path. When a user makes a request we want to forward it to a warm runner as quickly as possible.
func (s *Scheduler) reconcileSlots(ctx context.Context) {
	log.Debug().Msg("starting slot reconciler")
	for {
		select {
		case <-ctx.Done():
			return
		default:
			s.reconcileSlotsOnce()
			// Sleep for a while to allow others to access the queue
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// reconcileSlotsOnce reconciles slots once.
func (s *Scheduler) reconcileSlotsOnce() {
	s.slotsMtx.Lock()
	defer s.slotsMtx.Unlock()

	// Get all runners
	runnerIDs := s.controller.RunnerIDs()

	// Build a complete map of all actual slots across all runners
	allActualSlots := make(map[uuid.UUID]string) // maps slot ID to runner ID
	for _, runnerID := range runnerIDs {
		actualSlots, err := s.controller.Slots(runnerID)
		if err != nil {
			log.Error().Err(err).Str("runner_id", runnerID).Msg("failed to get slots from runner")
			continue
		}

		for _, slot := range actualSlots {
			// If we find the same slot ID on multiple runners, delete from the duplicate runner
			if existingRunnerID, exists := allActualSlots[slot.ID]; exists {
				log.Warn().
					Str("slot_id", slot.ID.String()).
					Str("existing_runner", existingRunnerID).
					Str("duplicate_runner", runnerID).
					Msg("found duplicate slot ID on multiple runners, cleaning up duplicate")
				err = s.controller.DeleteSlot(runnerID, slot.ID)
				if err != nil {
					log.Error().Err(err).Str("runner_id", runnerID).Str("slot_id", slot.ID.String()).Msg("failed to delete duplicate slot")
				}
				continue
			}
			allActualSlots[slot.ID] = runnerID
		}
	}

	// Clean up scheduler slots that don't exist on any runner
	for slotID, slot := range s.slots {
		if runnerID, exists := allActualSlots[slotID]; !exists {
			log.Warn().
				Str("runner_id", slot.RunnerID).
				Str("slot_id", slotID.String()).
				Msg("found slot on the scheduler that doesn't exist on any runner, deleting...")
			delete(s.slots, slotID)
		} else if runnerID != slot.RunnerID {
			// The slot exists but on a different runner than we thought
			log.Warn().
				Str("scheduler_runner", slot.RunnerID).
				Str("actual_runner", runnerID).
				Str("slot_id", slotID.String()).
				Msg("slot exists on different runner than expected, updating runner ID")
			slot.RunnerID = runnerID
		}
	}

	// Clean up runner slots that don't exist in scheduler
	for slotID, runnerID := range allActualSlots {
		if _, exists := s.slots[slotID]; !exists {
			log.Warn().
				Str("runner_id", runnerID).
				Str("slot_id", slotID.String()).
				Msg("found slot on runner that doesn't exist in scheduler, deleting...")
			err := s.controller.DeleteSlot(runnerID, slotID)
			if err != nil {
				log.Error().Err(err).Str("runner_id", runnerID).Str("slot_id", slotID.String()).Msg("failed to delete slot")
			}
		}
	}
}

func (s *Scheduler) processQueueOnce(ctx context.Context) {
	s.queueMtx.Lock()
	defer s.queueMtx.Unlock()

	// Store jobs that weren't able to be scheduled to re-add to the queue later
	// This is important because there many be workloads that persistently fail to schedule
	// and we don't want to block workloads that can be scheduled from further down the queue
	unscheduledQueue := make([]*Workload, 0)

	// Schedule any requests that are currently in the queue.
	for _, work := range s.queue {
		err := s.start(ctx, work)
		if err != nil {
			retry, err := ErrorHandlingStrategy(err, work)

			// If we can retry, break out of the loop and try again later
			if retry {
				unscheduledQueue = append(unscheduledQueue, work)
				continue
			}

			// If we can't retry, write an error to the request and continue so it takes it off
			// the queue
			s.onSchedulingErr(work, err)
		}
	}
	// Clear processed queue
	s.queue = unscheduledQueue
}

func (s *Scheduler) start(ctx context.Context, work *Workload) error {
	s.slotsMtx.RLock()
	defer s.slotsMtx.RUnlock()

	if work == nil {
		return fmt.Errorf("workload is nil")
	}

	// Validate session mode.
	if work.Mode() == types.SessionModeNone {
		return fmt.Errorf("session mode isn't set")
	}

	// Try to find warm warmSlots, which are ready to take new work.
	warmSlots := s.warmSlots(work)

	// If warm slots are available, select one from the least loaded runner.
	if len(warmSlots) > 0 {
		// Slots grouped by runner
		runnerSlots := make(map[string][]*Slot)
		for _, slot := range warmSlots {
			runnerSlots[slot.RunnerID] = append(runnerSlots[slot.RunnerID], slot)
		}

		// Map of ALL active slots per runner (not just warm ones)
		activeSlots := make(map[string]int)
		// Get ALL slots from controller, not just warm ones
		for _, slot := range s.slots {
			if slot.IsActive() {
				activeSlots[slot.RunnerID]++
			}
		}

		// List of runners
		runnerIDs := make([]string, 0, len(runnerSlots))
		for runnerID := range runnerSlots {
			runnerIDs = append(runnerIDs, runnerID)
		}

		// Sort runners by:
		// 1. Active slot count (ascending)
		// 2. Number of warm slots available (descending)
		// 3. Random shuffle for ties
		slices.SortFunc(runnerIDs, func(a, b string) int {
			if activeSlots[a] != activeSlots[b] {
				return activeSlots[a] - activeSlots[b]
			}
			if len(runnerSlots[b]) != len(runnerSlots[a]) {
				return len(runnerSlots[b]) - len(runnerSlots[a])
			}
			return rand.Intn(3) - 1 // Introduces random shuffle for true ties
		})

		// Pick a random warm slot from the least loaded runner
		leastLoadedRunnerSlots := runnerSlots[runnerIDs[0]]
		slot := leastLoadedRunnerSlots[rand.Intn(len(leastLoadedRunnerSlots))]

		err := s.allocateSlot(slot.ID, work)
		if err != nil {
			// Return error if unable to allocate work to the warm model.
			return fmt.Errorf("unable to allocate work to a warm model slot (ID: %s, slot runner: %s): %w", slot.ID, slot.RunnerID, err)
		}
	} else {
		// If no warm slots are available, pick a runner to allocate a slot to.

		// Take time to find the best runner here, since it's not on the hot path.
		// Reach out to each runner and get an update on their current load.
		// Pick the first runner that has the least load and has enough memory to allocate the new
		// workload.

		// First get a list of all runners
		allRunners := s.controller.RunnerIDs()

		// Reach out to each runner and get their total memory
		runnerMemory := make(map[string]uint64)
		for _, runnerID := range allRunners {
			runnerMemory[runnerID] = s.controller.TotalMemory(runnerID)
		}

		// Filter out runners that don't have enough memory to allocate the new workload
		filteredRunners := make([]string, 0)
		for runnerID, memory := range runnerMemory {
			if memory >= work.Model().GetMemoryRequirements(work.Mode()) {
				filteredRunners = append(filteredRunners, runnerID)
			}
		}

		// Reach out to the remaining runners and get their current load
		runnerLoad := make(map[string]uint64)
		for _, runnerID := range filteredRunners {
			runnerLoad[runnerID] = s.controller.FreeMemory(runnerID)
		}

		// Sort the runners by load, increasing, with a random shuffle for ties
		slices.SortFunc(filteredRunners, func(a, b string) int {
			if runnerLoad[a] != runnerLoad[b] {
				return int(runnerLoad[a] - runnerLoad[b])
			}
			return rand.Intn(3) - 1 // Introduces random shuffle for true ties
		})

		// Error if there are no runners left
		if len(filteredRunners) == 0 {
			return fmt.Errorf("no runners available")
		}

		// Pick the first runner
		bestRunnerID := filteredRunners[0]
		log.Trace().Str("runner_id", bestRunnerID).Msg("chosen best runner")

		// While there isn't enough free memory, delete the most stale slot.
		err := s.deleteMostStaleStrategy(bestRunnerID, work.Model().GetMemoryRequirements(work.Mode()))
		if err != nil {
			return fmt.Errorf("unable to delete stale slots: %w", err)
		}

		// Create an allocated slot
		err = s.allocateNewSlot(ctx, bestRunnerID, work)
		if err != nil {
			// Return error if unable to allocate a new slot.
			return fmt.Errorf("unable to allocate new work on runner (ID: %s): %w", bestRunnerID, err)
		}
	}

	return nil
}

// DeleteMostStaleStrategy iteratively deletes allocated work from stale slots until there is enough
// memory to allocate the new workload.
func (s *Scheduler) deleteMostStaleStrategy(runnerID string, requiredMem uint64) error {
	for {
		var allSlots []*Slot
		for _, slot := range s.slots {
			if slot.RunnerID == runnerID {
				allSlots = append(allSlots, slot)
			}
		}
		staleSlots := Filter(allSlots, func(slot *Slot) bool {
			return slot.IsStale()
		})
		// If there is enough free space on the runner, break out of the loop.
		if requiredMem <= s.controller.FreeMemory(runnerID) {
			break
		}
		// Sort the slots by last activity time
		slices.SortFunc(staleSlots, func(i, j *Slot) int {
			return int(i.LastActivityTime.Sub(j.LastActivityTime))
		})
		if len(staleSlots) == 0 {
			return fmt.Errorf("unable to find any slots to free up memory, do you have something else taking up GPU memory?")
		}
		// Then delete the most stale slot
		log.Debug().Str("slot_id", staleSlots[0].ID.String()).Msg("deleting stale slot")
		err := s.controller.DeleteSlot(runnerID, staleSlots[0].ID)
		if err != nil {
			return fmt.Errorf("unable to delete stale slot: %w", err)
		}
		delete(s.slots, staleSlots[0].ID)
	}
	return nil
}

func (s *Scheduler) warmSlots(req *Workload) []*Slot {
	s.slotsMtx.RLock()
	defer s.slotsMtx.RUnlock()

	cosyWarm := make([]*Slot, 0, len(s.slots))
	for _, slot := range s.slots {
		l := log.With().
			Str("slot_id", slot.ID.String()).
			Str("req_model_name", req.ModelName().String()).
			Str("slot_model_name", slot.ModelName().String()).
			Str("req_inference_runtime", req.ModelName().InferenceRuntime().String()).
			Str("slot_inference_runtime", slot.ModelName().InferenceRuntime().String()).
			Str("req_lora_dir", req.LoraDir()).
			Str("slot_lora_dir", slot.LoraDir()).
			Logger()

		// If it's not the same model name, skip
		if slot.ModelName() != req.ModelName() {
			l.Trace().Msg("skipping warm slot, model name mismatch")
			continue
		}

		// If it's not the same runtime, skip
		if slot.ModelName().InferenceRuntime() != req.ModelName().InferenceRuntime() {
			l.Trace().Msg("skipping warm slot, inference runtime mismatch")
			continue
		}

		// If the slot is already running another job, skip
		if slot.IsActive() {
			l.Trace().Msg("skipping warm slot, already active")
			continue
		}

		// If the slot is scheduled to run another job, skip
		if slot.IsScheduled() {
			l.Trace().Msg("skipping warm slot, already scheduled")
			continue
		}

		// If it doesn't have the right LoraDir then skip
		if slot.LoraDir() != req.LoraDir() {
			l.Trace().Str("slot_lora_dir", slot.LoraDir()).Str("req_lora_dir", req.LoraDir()).Msg("skipping warm slot, LoraDir mismatch")
			continue
		}

		// Add available slots to the list.
		cosyWarm = append(cosyWarm, slot)
	}
	return cosyWarm
}

// AllocateSlot assigns a workload to a specific slot, validating the model and slot before scheduling.
func (s *Scheduler) allocateSlot(slotID uuid.UUID, req *Workload) error {
	// Validate slot
	slot, ok := s.slots[slotID]
	if !ok {
		return fmt.Errorf("slot not found: %s", slot.ID.String())
	}

	// Ensure the slot is not already scheduled or active.
	if slot.IsScheduled() {
		return fmt.Errorf("slot has scheduled work: %s", slot.ID.String())
	}
	if slot.IsActive() {
		return fmt.Errorf("slot already active: %s", slot.ID.String())
	}

	log.Debug().
		Str("runner_id", slot.RunnerID).
		Str("slot_id", slot.ID.String()).
		Str("model_name", slot.ModelName().String()).
		Uint64("total_memory", slot.Memory()).
		Str("request_id", req.ID()).
		Msg("allocating slot")

	// Schedule the slot.
	slot.Schedule()

	// Submit the work to the slot
	slot.Start()
	switch req.WorkloadType {
	case WorkloadTypeLLMInferenceRequest:
		err := s.controller.SubmitChatCompletionRequest(slot, req.LLMInferenceRequest())
		if err != nil {
			// TODO(Phil): Need to pass on the error to the session for all these cases
			log.Error().Err(err).Msg("error submitting chat completion request")
		}
	case WorkloadTypeSession:
		switch req.Session().Mode {
		case types.SessionModeInference:
			switch req.Session().Type {
			case types.SessionTypeImage:
				err := s.controller.SubmitImageGenerationRequest(slot, req.Session())
				if err != nil {
					log.Error().Err(err).Msg("error submitting text2image request")
				}
			case types.SessionTypeText:
				if req.Session().LoraDir != "" {
					// Overwrite the request model name with the helix lora model details
					convertedRequest := req.ToLLMInferenceRequest()
					convertedRequest.Request.Model = req.Session().LoraDir

					// Forward the request to the chat completion handler
					err := s.controller.SubmitChatCompletionRequest(slot, convertedRequest)
					if err != nil {
						return fmt.Errorf("error submitting text request: %w", err)
					}
				} else {
					panic(fmt.Sprintf("not implemented: %s and no lora dir", req.Session().Type))
				}
			default:
				panic(fmt.Sprintf("not implemented: %s", req.Session().Type))
			}
		case types.SessionModeFinetune:
			switch req.Session().Type {
			case types.SessionTypeText:
				err := s.controller.SubmitFinetuningRequest(slot, req.Session())
				if err != nil {
					log.Error().Err(err).Msg("error submitting finetuning request")
				}
			default:
				panic(fmt.Sprintf("not implemented: %s", req.Session().Type))
			}
		default:
			panic(fmt.Sprintf("not implemented: %s", req.Session().Mode))
		}
	}
	// TODO(Phil): This isn't right, we shouldn't release the slot here, because it's still running
	// a job. Probably should move this to the runner itself.
	slot.Release()

	return nil
}

// AllocateNewSlot creates a new slot for a workload and allocates it to the best available runner.
func (s *Scheduler) allocateNewSlot(ctx context.Context, runnerID string, req *Workload) error {
	// Create a new slot and schedule the workload.
	slot := NewSlot(runnerID, req, s.modelStaleFunc, s.slotTimeoutFunc)
	log.Debug().
		Str("runner_id", slot.RunnerID).
		Str("slot_id", slot.ID.String()).
		Str("model_name", slot.ModelName().String()).
		Uint64("total_memory", slot.Memory()).
		Str("request_id", req.ID()).
		Msg("creating new slot")

	err := s.controller.CreateSlot(slot)
	if err != nil {
		return err
	}

	// Wait for the slot to be ready
	slotReady := make(chan bool)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				slots, err := s.controller.Slots(slot.RunnerID)
				if err != nil {
					log.Error().Err(err).Msg("unable to get slots")
					return
				}
				for _, s := range slots {
					if s.ID == slot.ID {
						slotReady <- true
						return
					}
				}
			}
		}
	}()
	select {
	case <-slotReady:
	case <-time.After(120 * time.Second):
		return fmt.Errorf("slot not ready after 120 seconds")
	}

	log.Trace().Msg("slot created")

	// Ensure the slot is stored.
	s.slots[slot.ID] = slot

	// Schedule and store the new slot.
	return s.allocateSlot(slot.ID, req)
}
