package scheduler

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/rs/zerolog/log"
)

// Scheduler is is the main entrypoint to the scheduler package.
// It provides methods to schedule, release, and assign workloads to runners.
// The underlying workload management is handled by the WorkloadAllocator interface.
type Scheduler interface {
	Schedule(request *Workload) error
	Release(id string) error
	WorkForRunner(id string, workType WorkloadType, newWorkOnly bool, model string) (*Workload, error)
	UpdateRunner(props *types.RunnerState)

	// New queueing scheduler methods
	Begin(requestID string) error
	SlotsForRunner(runnerID string) []types.DesiredRunnerSlot
	Enqueue(work *Workload) error
	DashboardData() ([]*types.SessionSummary, error)
	DashboardSlotsData() []types.DesiredSlots
}

// scheduler is a struct implementing the Scheduler interface.
// It includes a workload allocator (allocator) and a thread-safe map (workStore) to store scheduled workloads.
type scheduler struct {
	allocator         WorkloadAllocator                  // Interface to allocate workload to different slots/models.
	workStore         *xsync.MapOf[uuid.UUID, *Workload] // Map to store the work associated with a slot.
	cluster           Cluster                            // Cluster to manage runner state.
	placementStrategy SchedulingStrategyFunc
	queue             []*Workload
	queueMtx          *sync.Mutex
	queueSize         int
	onSchedulingErr   func(work *Workload, err error)
}

var _ Scheduler = &scheduler{}

// NewScheduler creates a new scheduler with a workload allocator.
// This also starts a goroutine to process the queue in the background.
//
// NOTE(milosgajdos): we really should make sure we return exported types.
// If we want the type fields to be inaccessible we should make them unexported.
// nolint:revive
func NewScheduler(ctx context.Context, cfg *config.ServerConfig, onSchedulingErr func(work *Workload, err error)) *scheduler {
	scheduler := newSchedulerWithoutGoroutines(cfg, onSchedulingErr)

	// Start a goroutine to process the buffered queue
	go func() {
		scheduler.processQueue(ctx)
	}()

	// Start a goroutine that will periodically check for dead runners and reschedule their work
	go func() {
		scheduler.checkForDeadRunners(ctx)
	}()

	return scheduler
}

func newSchedulerWithoutGoroutines(cfg *config.ServerConfig, onSchedulingErr func(work *Workload, err error)) *scheduler {
	modelTTL := cfg.Providers.Helix.ModelTTL
	if modelTTL == 0 {
		modelTTL = 10 * time.Second
	}
	slotTTL := cfg.Providers.Helix.SlotTTL
	if slotTTL == 0 {
		slotTTL = 300 * time.Second
	}
	log.Info().Dur("model_stale_time", modelTTL).Dur("slot_timeout", slotTTL).Msg("slot timeouts")
	allocator := NewWorkloadAllocator(
		NewTimeoutFunc(modelTTL),
		NewTimeoutFunc(slotTTL),
	)
	cluster := NewCluster(
		NewTimeoutFunc(cfg.Providers.Helix.RunnerTTL),
	)

	queueSize := 100
	if cfg.Providers.Helix.QueueSize > 0 {
		queueSize = cfg.Providers.Helix.QueueSize
	}
	if onSchedulingErr == nil {
		onSchedulingErr = func(work *Workload, err error) {
			log.Warn().Err(err).Str("id", work.ID()).Msg("error scheduling work")
		}
	}

	schedStratFunc := MaxSpreadStrategy
	switch SchedulingStrategy(cfg.Providers.Helix.SchedulingStrategy) {
	case SchedulingstrategyMaxutilization:
		log.Info().Str("strategy", cfg.Providers.Helix.SchedulingStrategy).Msg("scheduling strategy with spread work across all runners")
		schedStratFunc = MaxUtilizationStrategy
	case SchedulingstrategyMaxspread:
		log.Info().Str("strategy", cfg.Providers.Helix.SchedulingStrategy).Msg("scheduling strategy will maximize utilization on individual runners")
		schedStratFunc = MaxSpreadStrategy
	default:
		log.Warn().Str("strategy", cfg.Providers.Helix.SchedulingStrategy).Msg("unknown scheduling strategy, defaulting to max utilization")
	}
	scheduler := &scheduler{
		allocator:         allocator,
		cluster:           cluster,
		workStore:         xsync.NewMapOf[uuid.UUID, *Workload](),
		placementStrategy: schedStratFunc,
		queue:             make([]*Workload, 0, queueSize),
		queueMtx:          &sync.Mutex{},
		queueSize:         queueSize,
		onSchedulingErr:   onSchedulingErr,
	}

	return scheduler
}

// NewTimeoutFunc returns a function to check if a runner has been idle for a specified timeout duration.
func NewTimeoutFunc(timeout time.Duration) TimeoutFunc {
	return func(_ string, lastActivity time.Time) bool {
		// Check if the model has been unused for more than the specified timeout duration.
		return time.Since(lastActivity) > timeout
	}
}

// Schedule assigns work based on the current workload and available slots.
// It attempts to allocate the workload to a warm slot or creates a new slot if none are available.
func (s *scheduler) Schedule(work *Workload) (err error) {
	if work == nil {
		return fmt.Errorf("workload is nil")
	}
	// Validate model.
	if _, err := model.GetModel(work.ModelName().String()); err != nil {
		return fmt.Errorf("unable to get model (%s): %v", work.ModelName(), err)
	}

	// Validate session mode.
	if work.Mode() == types.SessionModeNone {
		return fmt.Errorf("session mode isn't set")
	}

	var slot *Slot // Holds the slot where the work will be scheduled.

	// Try to find warm slots, which are ready to take new work.
	slots := s.allocator.WarmSlots(work)

	// If warm slots are available, select a random one.
	if len(slots) > 0 {
		// TODO(PHIL): This doesn't use the scheduling strategy. That is only used for new models.
		// I should probably refactor this to use the strategy for all scheduling.
		// Randomly select one warm slot from the available warm slots.
		slot = slots[rand.Intn(len(slots))]

		// Allocate work to the selected warm slot.
		err = s.allocator.AllocateSlot(slot.ID, work)
		if err != nil {
			// Return error if unable to allocate work to the warm model.
			return fmt.Errorf("unable to allocate work to a warm model slot (ID: %s, slot runner: %s): %w", slot.ID, slot.RunnerID, err)
		}
	} else {
		// If no warm slots are available, pick a runner to allocate a slot to.
		bestRunnerID, err := s.placementStrategy(s.cluster, s.allocator, work)
		if err != nil {
			return fmt.Errorf("unable to place work on any runner: %w", err)
		}

		// Figure out if we have to kill a slot to make room for the new one.
		err = DeleteMostStaleStrategy(s.allocator, bestRunnerID, s.cluster.TotalMemory(bestRunnerID), work.Model().GetMemoryRequirements(work.Mode()))
		if err != nil {
			return fmt.Errorf("unable to delete stale slots: %w", err)
		}

		// Create an allocate slot
		slot, err = s.allocator.AllocateNewSlot(bestRunnerID, work)
		if err != nil {
			// Return error if unable to allocate a new slot.
			return fmt.Errorf("unable to allocate new work on runner (ID: %s): %w", bestRunnerID, err)
		}
	}

	// Store the work associated with the slot for future deallocation.
	if slot == nil {
		// If the slot is nil, return an error.
		return fmt.Errorf("slot is nil")
	}

	s.workStore.Store(slot.ID, work)

	return nil
}

// Release frees the resources associated with a specific scheduled request.
// It finds the request by its ID, releases the allocated slot, and removes the associated work from the store.
func (s *scheduler) Release(id string) error {
	// Find the slot ID associated with the request.
	slotID, ok := s.find(id)
	if !ok {
		// If the request is not found, return an error.
		return fmt.Errorf("request not found: %s", id)
	}

	// Release the resources allocated to the slot.
	err := s.allocator.ReleaseSlot(slotID)
	if err != nil {
		// If there is an error during deallocation, return it.
		return fmt.Errorf("problem deallocating: %w", err)
	}

	// Remove the work associated with the slot from the store.
	s.workStore.Delete(slotID)

	return nil
}

// WorkForRunner retrieves work for a specific runner by its ID.
// It checks the runner's slots and assigns the work if any slot is ready.
// If newWorkOnly is set, it will only return work from new slots
func (s *scheduler) WorkForRunner(id string, workType WorkloadType, newWorkOnly bool, model string) (*Workload, error) {
	// Iterate through the slots assigned to the runner.
	for _, slot := range s.allocator.RunnerSlots(id) {
		// If the slot is ready for scheduling, retrieve the associated work.
		if slot.IsScheduled() {
			work, ok := s.workStore.Load(slot.ID)
			if !ok {
				continue // Work not owned by this scheduler, ignore it.
			}
			// Check if request model type matches the model type of the slot.
			if !newWorkOnly && model != "" && work.ModelName().String() != model {
				continue // Work is not of the requested model type, ignore it.
			}
			if work.WorkloadType != workType {
				continue // Work is not of the requested type, ignore it.
			}
			if newWorkOnly && !slot.IsNew() {
				continue // Work is not new, ignore it.
			}
			slot.Start() // Mark the work in the slot as started.
			return work, nil
		}
	}
	// If no work is found for the runner, return nil.
	return nil, nil
}

// UpdateRunner updates the state of a runner and reconciles its slots with the allocator's records.
func (s *scheduler) UpdateRunner(props *types.RunnerState) {
	// Update the runner's state in the cluster.
	s.cluster.UpdateRunner(props)
	// TODO: Reconcile the runner's slots with the allocator's records.
	// s.allocator.ReconcileSlots(props)
}

// find searches for the slot ID associated with a given workload ID.
func (s *scheduler) find(id string) (uuid.UUID, bool) {
	var result uuid.UUID
	// Iterate through the workStore to find the matching workload ID.
	s.workStore.Range(func(slotID uuid.UUID, w *Workload) bool {
		if w.ID() == id {
			result = slotID
			return false
		}
		return true
	})
	return result, result != uuid.Nil
}

// Enqueue adds a workload to the scheduler's queue.
// TODO(Phil): Previous implementations saved the session queue in the database to requeue on
// restarts. I don't think this is a particularly useful feature ATM. But may be in the future.
func (s *scheduler) Enqueue(work *Workload) error {
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

// TODO(PHIL): Deprecate in preference of a new dashboard API
// DashboardData returns the queue of work for the scheduler in old SessionSummary format
func (s *scheduler) DashboardData() ([]*types.SessionSummary, error) {
	s.queueMtx.Lock()
	defer s.queueMtx.Unlock()

	// Convert the queue of work to a list of SessionSummary objects.
	sessionSummaries := make([]*types.SessionSummary, 0, len(s.queue))
	for _, w := range s.queue {
		switch w.WorkloadType {
		case WorkloadTypeSession:
			summary, err := data.GetSessionSummary(w.Session())
			if err != nil {
				return nil, err
			}
			sessionSummaries = append(sessionSummaries, summary)
		case WorkloadTypeLLMInferenceRequest:
			sessionSummaries = append(sessionSummaries, &types.SessionSummary{
				SessionID:     w.ID(),
				Name:          w.LLMInferenceRequest().Request.Model,
				InteractionID: w.LLMInferenceRequest().InteractionID,
				Mode:          types.SessionModeInference,
				Type:          types.SessionTypeText,
				ModelName:     w.LLMInferenceRequest().Request.Model,
				Owner:         w.LLMInferenceRequest().OwnerID,
				Created:       w.LLMInferenceRequest().CreatedAt,
				Updated:       w.LLMInferenceRequest().CreatedAt,
				Scheduled:     w.LLMInferenceRequest().CreatedAt,
				Completed:     w.LLMInferenceRequest().CreatedAt,
				Summary:       "LLM Inference Request",
				Priority:      w.LLMInferenceRequest().Priority,
			})
		}
	}

	return sessionSummaries, nil
}

// DashboardSlotsData returns the queue of work for the scheduler in new DesiredSlots format
// TODO(PHIL): We shouldn't pass implementation details about types of work to the dashboard.
func (s *scheduler) DashboardSlotsData() []types.DesiredSlots {
	s.queueMtx.Lock()
	defer s.queueMtx.Unlock()

	// Convert the queue of work to a list of SessionSummary objects.
	runnerIDs := s.cluster.RunnerIDs()
	desiredSlots := make([]types.DesiredSlots, 0, len(runnerIDs))
	for _, runnerID := range runnerIDs {
		desiredSlots = append(desiredSlots, types.DesiredSlots{
			ID:   runnerID,
			Data: s.SlotsForRunner(runnerID),
		})
	}

	return desiredSlots
}

func (s *scheduler) Begin(requestID string) error {
	// Find the slot ID associated with the request.
	slotID, ok := s.find(requestID)
	if !ok {
		// If the request is not found, it has probably already been released
		return nil
	}

	// Set the slot as started
	err := s.allocator.StartSlot(slotID)
	if err != nil {
		return fmt.Errorf("problem starting slot: %w", err)
	}

	return nil
}

func (s *scheduler) SlotsForRunner(runnerID string) []types.DesiredRunnerSlot {
	slots := s.allocator.RunnerSlots(runnerID)
	desiredRunnerSlots := make([]types.DesiredRunnerSlot, 0, len(slots))
	for _, slot := range slots {
		attr := types.DesiredRunnerSlotAttributes{
			Mode:  string(slot.Mode()),
			Model: string(slot.ModelName()),
		}
		slotWork, ok := s.workStore.Load(slot.ID)
		if ok {
			attr.Workload = slotWork.ToRunnerWorkload()
		}
		desiredRunnerSlot := types.DesiredRunnerSlot{
			ID:         slot.ID,
			Attributes: attr,
		}
		desiredRunnerSlots = append(desiredRunnerSlots, desiredRunnerSlot)
	}

	return desiredRunnerSlots
}

// processQueue runs in a goroutine to processes the queue of requests.
func (s *scheduler) processQueue(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			s.processQueueOnce()
			// Sleep for a while to allow others to access the queue
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (s *scheduler) processQueueOnce() {
	s.queueMtx.Lock()
	defer s.queueMtx.Unlock()

	// Store jobs that weren't able to be scheduled to re-add to the queue later
	// This is important because there many be workloads that persistently fail to schedule
	// and we don't want to block workloads that can be scheduled from further down the queue
	unscheduledQueue := make([]*Workload, 0)

	// Schedule any requests that are currently in the queue.
	for _, work := range s.queue {
		err := s.Schedule(work)
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

func (s *scheduler) checkForDeadRunners(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			s.checkForDeadRunnersOnce()

			// Sleep for a while to allow others to access the scheduler
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (s *scheduler) checkForDeadRunnersOnce() {
	deadRunnerIDs := s.cluster.DeadRunnerIDs()
	for _, id := range deadRunnerIDs {
		deadSlots := s.allocator.DeadSlots([]string{id})
		for _, dead := range deadSlots {
			// Load and delete that work from the store
			work, ok := s.workStore.LoadAndDelete(dead.ID)
			if !ok {
				continue // No work to reschedule
			}

			// Attempt to reschedule the work.
			log.Trace().
				Str("runner_id", id).
				Str("slot_id", dead.ID.String()).
				Msg("rescheduling work for dead slot")
			err := s.Enqueue(work)
			if err != nil {
				log.Error().
					Err(err).
					Str("runner_id", id).
					Str("slot_id", dead.ID.String()).
					Msg("failed to reschedule work for dead slot")
				continue
			}
		}
	}
}
