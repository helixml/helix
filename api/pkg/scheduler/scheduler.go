package scheduler

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/exp/rand"
)

const (
	pendingSlotsBufferSize    = 1 // The number of slot creation requests to buffer
	runnerReconcileInterval   = 5 * time.Second
	activityReconcileInterval = 100 * time.Millisecond
	queueReconcileInterval    = 100 * time.Millisecond
)

// TimeoutFunc defines a function type that determines if a runner has timed out based on the last activity.
type TimeoutFunc func(runnerID string, lastActivityTime time.Time) bool

func NewTimeoutFunc(ttl time.Duration) TimeoutFunc {
	return func(runnerID string, lastActivityTime time.Time) bool {
		elapsed := time.Since(lastActivityTime)
		isTimedOut := elapsed > ttl
		if isTimedOut {
			log.Debug().
				Str("runner_id", runnerID).
				Dur("elapsed", elapsed).
				Dur("ttl", ttl).
				Time("last_activity", lastActivityTime).
				Bool("timed_out", isTimedOut).
				Msg("Timeout function evaluation")
		}
		return isTimedOut
	}
}

type Scheduler struct {
	ctx             context.Context
	controller      *RunnerController
	queue           *WorkQueue
	onSchedulingErr func(work *Workload, err error)
	slots           *xsync.MapOf[uuid.UUID, *Slot]
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
		modelTTL = 300 * time.Second
	}
	slotTTL := serverConfig.Providers.Helix.SlotTTL
	if slotTTL == 0 {
		slotTTL = 300 * time.Second
	}
	queueSize := 50
	if params.QueueSize > 0 {
		queueSize = params.QueueSize
	}

	log.Info().
		Dur("model_stale_time", modelTTL).
		Dur("slot_timeout", slotTTL).
		Msg("slot timeouts configured")

	modelStaleFunc := NewTimeoutFunc(modelTTL)
	slotTimeoutFunc := NewTimeoutFunc(slotTTL)

	log.Info().
		Interface("runner_controller", params.RunnerController).
		Int("queue_size", params.QueueSize).
		Bool("has_scheduling_err_handler", params.OnSchedulingErr != nil).
		Bool("has_response_handler", params.OnResponseHandler != nil).
		Msg("Creating scheduler with parameters")

	s := &Scheduler{
		ctx:             ctx,
		controller:      params.RunnerController,
		queue:           NewWorkQueue(queueSize),
		onSchedulingErr: params.OnSchedulingErr,
		slots:           xsync.NewMapOf[uuid.UUID, *Slot](),
		modelStaleFunc:  modelStaleFunc,
		slotTimeoutFunc: slotTimeoutFunc,
	}

	// Start the queue processor
	go s.processQueue(ctx)

	// Start the slot reconciler
	go s.reconcileSlots(ctx)

	// Start the activity reconciler
	go s.reconcileActivity(ctx)

	// Start the runner reconciler
	go s.reconcileRunners(ctx)

	return s, nil
}

func (s *Scheduler) Enqueue(work *Workload) error {
	return s.queue.Add(work)
}

func (s *Scheduler) Queue() ([]*types.WorkloadSummary, error) {
	currentQueue := s.queue.Queue()
	queue := make([]*types.WorkloadSummary, 0, len(currentQueue))
	for _, w := range currentQueue {
		summary := ""
		switch w.WorkloadType {
		case WorkloadTypeLLMInferenceRequest:
			req := w.LLMInferenceRequest()
			if req.Request != nil && len(req.Request.Messages) > 0 {
				summary = req.Request.Messages[len(req.Request.Messages)-1].Content
			} else if req.Embeddings && req.EmbeddingRequest.Input != nil {
				switch input := req.EmbeddingRequest.Input.(type) {
				case string:
					summary = input
				case []string:
					if len(input) > 0 {
						summary = input[0]
					}
				default:
					summary = "Embedding request"
				}
			}
		case WorkloadTypeSession:
			s := w.Session()
			interaction, _ := data.GetLastUserInteraction(s.Interactions)
			if interaction != nil {
				summary = interaction.Message
			}
		}
		queue = append(queue, &types.WorkloadSummary{
			ID:        w.ID(),
			CreatedAt: w.Created(),
			UpdatedAt: w.Updated(),
			ModelName: string(w.ModelName()),
			Mode:      string(w.Mode()),
			Runtime:   string(w.Runtime()),
			LoraDir:   w.LoraDir(),
			Summary:   summary,
		})
	}
	return queue, nil
}

func (s *Scheduler) RunnerStatus() ([]*types.RunnerStatus, error) {
	var err error
	// Get a current list of runners
	runners := s.controller.RunnerIDs()

	// Get the current state of each runner
	runnerStates := make([]*types.RunnerStatus, 0, len(runners))
	for _, runnerID := range runners {
		var runnerStatus *types.RunnerStatus
		runnerStatus, err = s.controller.GetStatus(runnerID)
		if err != nil {
			log.Warn().Err(err).Str("runner_id", runnerID).Msg("error getting runner status, this shouldn't happen, please investigate this runner")
			runnerStatus = &types.RunnerStatus{
				ID: runnerID,
			}
		}
		runnerStates = append(runnerStates, runnerStatus)
	}

	return runnerStates, nil
}

func (s *Scheduler) RunnerSlots(runnerID string) ([]*types.RunnerSlot, error) {
	return s.controller.GetSlots(runnerID)
}

// processQueue runs in a goroutine to processes the queue of requests.
func (s *Scheduler) processQueue(ctx context.Context) {
	log.Debug().Msg("starting queue processor")
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(queueReconcileInterval):
			s.processQueueOnce()
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
		case <-time.After(runnerReconcileInterval):
			s.reconcileSlotsOnce(ctx)
		}
	}
}

// reconcileActivity runs in a goroutine to reconcile activity.
func (s *Scheduler) reconcileActivity(ctx context.Context) {
	log.Debug().Msg("starting activity reconciler")
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(activityReconcileInterval):
			s.reconcileActivityOnce()
		}
	}
}

func (s *Scheduler) reconcileActivityOnce() {
	if s.slots == nil {
		return
	}
	activeCount := 0
	checkedCount := 0
	releasedCount := 0

	// Check the status of all remaining slots to see if they have finished their work
	s.slots.Range(func(slotID uuid.UUID, slot *Slot) bool {
		checkedCount++
		if slot == nil {
			withSlotContext(&log.Logger, slot).Debug().Msg("slot is nil, releasing")
			slot.Release()
			releasedCount++
			return true
		}
		if slot.IsActive() {
			activeCount++
			// Get the live slot from the runner, don't use the cached copy
			remoteSlot, err := s.controller.fetchSlot(slot.RunnerID, slotID)
			if err != nil {
				withSlotContext(&log.Logger, slot).Error().
					Err(err).
					Msg("failed to get slot during activity reconciliation, assuming it's finished")
				slot.Release()
				releasedCount++
			} else if !remoteSlot.Active {
				withSlotContext(&log.Logger, slot).Debug().
					Bool("remote_active", remoteSlot.Active).
					Bool("remote_ready", remoteSlot.Ready).
					Msg("slot is not active according to remote, releasing")
				slot.Release()
				releasedCount++
			} else {
				withSlotContext(&log.Logger, slot).Debug().
					Bool("remote_active", remoteSlot.Active).
					Bool("remote_ready", remoteSlot.Ready).
					Msg("slot is still active according to remote")
			}
		}
		return true
	})

	// Only log when there's actual activity to report (skip empty reconciliations)
	if activeCount > 0 || releasedCount > 0 {
		log.Debug().
			Int("checked", checkedCount).
			Int("active", activeCount).
			Int("released", releasedCount).
			Msg("Slot activity reconciliation completed")
	}
}

// reconcileRunners runs in a goroutine to reconcile runners.
func (s *Scheduler) reconcileRunners(ctx context.Context) {
	log.Debug().Msg("starting runner reconciler")
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(runnerReconcileInterval):
			s.reconcileRunnersOnce()
		}
	}
}

func (s *Scheduler) reconcileRunnersOnce() {
	// Get all runners
	runnerIDs := s.controller.RunnerIDs()

	// Get the health of each runner
	for _, runnerID := range runnerIDs {
		err := s.controller.GetHealthz(runnerID)
		if err != nil {
			log.Warn().Err(err).Str("runner_id", runnerID).Msg("runner is not healthy, deleting...")
			s.controller.deleteRunner(runnerID)

			// Delete all slots belonging to this runner
			s.deleteRunnerSlots(runnerID)
			continue
		}

		// Set the models on the runner
		err = s.controller.SetModels(runnerID)
		if err != nil {
			log.Warn().Err(err).Str("runner_id", runnerID).Msg("failed to set models on runner, skipping...")
		}
	}
}

func (s *Scheduler) deleteRunnerSlots(runnerID string) {
	// First collect the slots to delete
	var slotsToDelete []uuid.UUID
	s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		if slot.RunnerID == runnerID {
			slotsToDelete = append(slotsToDelete, slot.ID)
		}
		return true
	})

	// Then delete them after the range is complete
	for _, slotID := range slotsToDelete {
		s.slots.Delete(slotID)
	}
}

// reconcileSlotsOnce reconciles slots once.
func (s *Scheduler) reconcileSlotsOnce(ctx context.Context) {
	// Get all runners
	runnerIDs := s.controller.RunnerIDs()
	log.Trace().Strs("runner_ids", runnerIDs).Msg("Starting slot reconciliation")

	// Ensure new slots are created and ready to take work
	requiredSlots := s.queue.GetRequiredSlots()
	log.Trace().Interface("required_slots", requiredSlots).Msg("Required slots for current workload")

	// Track slot stats
	existingSlotCount := 0
	slotsToCreate := 0
	duplicateSlotCount := 0
	orphanedSlotCount := 0
	mismatchedRunnerCount := 0

	// For each requirement, ensure we have enough slots for that work right now
	for _, req := range requiredSlots {
		// Check if we have enough slots for this work right now
		existingCount := 0
		// TODO(Phil): be careful about this, it's detached from the concept of a warm slot. Ideally
		// refactor so that warm and this are using the same logic.
		s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
			if slot.InitialWork().ModelName() == req.Model &&
				slot.InitialWork().Runtime() == req.Runtime &&
				slot.InitialWork().LoraDir() == req.LoraDir &&
				!slot.IsActive() {
				existingCount++
			}
			return true
		})

		existingSlotCount += existingCount

		// If we need more slots, start creating them
		slotsNeeded := req.Count - existingCount
		if slotsNeeded > 0 {
			slotsToCreate += slotsNeeded
			log.Info().
				Str("model", req.Model.String()).
				Str("runtime", string(req.Runtime)).
				Int("existing", existingCount).
				Int("required", req.Count).
				Int("to_create", slotsNeeded).
				Msg("Creating new slots for requirement")
			s.ensureSlots(req, slotsNeeded)
		}
	}

	// Build a complete map of all actual slots across all runners
	allActualSlots := make(map[uuid.UUID]string) // maps slot ID to runner ID
	for _, runnerID := range runnerIDs {
		// We need a live view of the slots here, otherwise we might get stale data
		actualSlots, err := s.controller.fetchSlots(runnerID)
		if err != nil {
			log.Error().Err(err).Str("runner_id", runnerID).Msg("failed to get slots from runner")
			continue
		}

		for _, slot := range actualSlots.Slots {
			// If we find the same slot ID on multiple runners, delete from the duplicate runner
			if existingRunnerID, exists := allActualSlots[slot.ID]; exists {
				duplicateSlotCount++
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

	// Delete removed slots -- important to delete first to create room for new slots
	unusedSlotCount := 0
	for slotID, runnerID := range allActualSlots {
		if _, exists := s.slots.Load(slotID); !exists {
			unusedSlotCount++
			// Look up the actual slot to get details for logging
			slotDetails, err := s.controller.fetchSlot(runnerID, slotID)
			if err == nil {
				log.Warn().
					Str("runner_id", runnerID).
					Str("slot_id", slotID.String()).
					Str("model", slotDetails.Model).
					Str("runtime", string(slotDetails.Runtime)).
					Bool("is_active", slotDetails.Active).
					Bool("is_ready", slotDetails.Ready).
					Msg("found slot on runner that doesn't exist in scheduler, deleting...")
			} else {
				log.Warn().
					Err(err).
					Str("runner_id", runnerID).
					Str("slot_id", slotID.String()).
					Msg("found slot on runner that doesn't exist in scheduler but couldn't fetch details, deleting...")
			}

			err = s.controller.DeleteSlot(runnerID, slotID)
			if err != nil {
				log.Error().Err(err).Str("runner_id", runnerID).Str("slot_id", slotID.String()).Msg("failed to delete slot")
			}
		}
	}

	// Create new slots
	s.slots.Range(func(slotID uuid.UUID, slot *Slot) bool {
		if runnerID, exists := allActualSlots[slotID]; !exists {
			orphanedSlotCount++
			log.Info().
				Str("runner_id", slot.RunnerID).
				Str("slot_id", slotID.String()).
				Str("model", slot.InitialWork().ModelName().String()).
				Str("runtime", string(slot.InitialWork().Runtime())).
				Bool("is_active", slot.IsActive()).
				Bool("is_running", slot.IsRunning()).
				Msg("found slot on the scheduler that doesn't exist on the runner, creating...")

			err := s.createNewSlot(ctx, slot)
			if err != nil {
				// Then see if we can retry
				retry, err := ErrorHandlingStrategy(err, slot.InitialWork())
				if retry {
					withWorkContext(&log.Logger, slot.InitialWork()).Debug().Err(err).Msg("failed to create slot, but retrying later...")
				} else {
					withWorkContext(&log.Logger, slot.InitialWork()).Warn().Err(err).Msg("failed to create slot, calling error handler")

					// First remove that slot, since it was never created
					s.slots.Delete(slot.ID)

					// Then remove the work from the queue if it exists
					s.queue.Remove(slot.InitialWork())

					// Then notify the error handler
					s.onSchedulingErr(slot.InitialWork(), err)
				}
			}
		} else if runnerID != slot.RunnerID {
			// The slot exists but on a different runner than we thought
			mismatchedRunnerCount++
			log.Warn().
				Str("scheduler_runner", slot.RunnerID).
				Str("actual_runner", runnerID).
				Str("slot_id", slotID.String()).
				Msg("slot exists on different runner than expected, updating runner ID")
			slot.RunnerID = runnerID
		}
		return true
	})

	log.Trace().
		Int("existing_slots", existingSlotCount).
		Int("slots_to_create", slotsToCreate).
		Int("duplicate_slots", duplicateSlotCount).
		Int("orphaned_slots", orphanedSlotCount).
		Int("mismatched_runner_slots", mismatchedRunnerCount).
		Int("unused_slots", unusedSlotCount).
		Msg("Completed slot reconciliation")
}

func (s *Scheduler) ensureSlots(req SlotRequirement, count int) {
	for i := 0; i < count; i++ {
		// Find best runner for this slot
		runnerID, err := s.pickBestRunner(req.ExampleWorkload)
		if err != nil {
			retry, err := ErrorHandlingStrategy(err, req.ExampleWorkload)
			if retry {
				log.Info().Err(err).Interface("requirement", req).Msg("failed to pick best runner for requirement, retrying...")
				return
			}
			log.Warn().Err(err).Interface("requirement", req).Msg("failed to pick best runner for requirement, skipping...")
			s.onSchedulingErr(req.ExampleWorkload, err)
			s.queue.Remove(req.ExampleWorkload) // This only removes the one workload from the slot requirement, not the entire queue full of them. It should clean up on the next time around.
			return
		}

		// Delete any stale slots on this runner if required
		err = s.deleteMostStaleStrategy(runnerID, req.ExampleWorkload)
		if err != nil {
			retry, err := ErrorHandlingStrategy(err, req.ExampleWorkload)
			if retry {
				log.Info().Err(err).Interface("requirement", req).Msg("failed to delete any stale slots, retrying...")
				return
			}
			log.Warn().Err(err).Interface("requirement", req).Msg("failed to delete any stale slots, skipping...")
			s.onSchedulingErr(req.ExampleWorkload, err)
			s.queue.Remove(req.ExampleWorkload) // This only removes the one workload from the slit requirement, not the entire queue full of them. It should clean up on the next time around.
			return
		}

		// Create the control plane view of the slot
		slot := NewSlot(runnerID, req.ExampleWorkload, s.modelStaleFunc, s.slotTimeoutFunc)

		// Store the slot
		s.slots.Store(slot.ID, slot)
	}
}

func (s *Scheduler) processQueueOnce() {
	// Try to take next work item that has a warm slot available
	work := s.queue.TakeNext(func(w *Workload) bool {
		warmSlots := s.warmSlots(w)
		return len(warmSlots) > 0
	})

	if work == nil {
		return // Nothing can be scheduled right now
	}

	// We know we have a warm slot, so schedule the work
	warmSlots := s.warmSlots(work)
	slot := s.pickBestWarmSlot(warmSlots)

	err := s.allocateSlot(slot.ID, work)
	if err != nil {
		// If allocation fails, put work back in queue
		err = s.queue.Add(work)
		if err != nil {
			log.Warn().Err(err).Msg("failed to add work back to queue")
			s.onSchedulingErr(work, err)
		}
	}
}

// Add new helper method to find the best runner
func (s *Scheduler) pickBestRunner(work *Workload) (string, error) {
	// First get a list of all runners
	allRunners := s.controller.RunnerIDs()

	filteredRunners, err := s.filterRunners(work, allRunners)
	if err != nil {
		return "", err
	}

	// Error if there are no runners left
	if len(filteredRunners) == 0 {
		return "", ErrNoRunnersAvailable
	}

	// Calculate the scheduled load on each runner according to their slots
	// Note: We discussed using real free memory by pinging the runner, but decided to use the
	// control-plane's view of free memory to avoid the overhead of pinging the runner. This also
	// has the added benefit of being able to over-commit memory slightly.
	runnerLoad := make(map[string]uint64)
	for _, runnerID := range filteredRunners {
		// Sum up all scheduled slots on the runner
		s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
			if slot.RunnerID == runnerID {
				runnerLoad[runnerID] += slot.Memory()
			}
			return true
		})
	}
	withWorkContext(&log.Logger, work).Debug().Interface("runner_load", runnerLoad).Msg("runner load")

	// Sort the runners by load, increasing, with a random shuffle for ties
	slices.SortFunc(filteredRunners, func(a, b string) int {
		if runnerLoad[a] != runnerLoad[b] {
			return int(runnerLoad[a] - runnerLoad[b])
		}
		return rand.Intn(3) - 1 // Introduces random shuffle for true ties
	})
	withWorkContext(&log.Logger, work).Debug().Interface("sorted_runners", filteredRunners).Msg("sorted runners")

	// Pick the first runner
	bestRunnerID := filteredRunners[0]
	withWorkContext(&log.Logger, work).Debug().Str("runner_id", bestRunnerID).Msg("chosen best runner")

	// Return the bestRunnerID and any error
	return bestRunnerID, nil
}

// DeleteMostStaleStrategy iteratively deletes allocated work from stale slots until there is enough
// memory to allocate the new workload.
func (s *Scheduler) deleteMostStaleStrategy(runnerID string, work *Workload) error {
	totalMem := s.controller.TotalMemory(runnerID)
	for {
		var allSlots []*Slot
		allocatedMem := uint64(0)
		s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
			if slot.RunnerID == runnerID {
				allSlots = append(allSlots, slot)
				allocatedMem += slot.Memory()
			}
			return true
		})

		requiredMem := work.model.Memory
		freeMem := int64(totalMem) - int64(allocatedMem) - int64(requiredMem)
		log.Trace().Interface("slots", allSlots).Int64("freeMem", freeMem).Msg("checking if we can allocate")
		// If there is enough free space on the runner, break out of the loop.
		if freeMem > 0 {
			break
		}

		// Only keep slots that are not the same as the required workload
		// Since there's no point deleting slots that are already the same as the required workload
		notSameWorkload := Filter(allSlots, func(slot *Slot) bool {
			return slot.InitialWork().ModelName() != work.ModelName() ||
				slot.InitialWork().Runtime() != work.Runtime() ||
				slot.InitialWork().LoraDir() != work.LoraDir()
		})

		// Only keep the stale slots
		staleSlots := Filter(notSameWorkload, func(slot *Slot) bool {
			return slot.IsStale()
		})

		// Sort the slots by last activity time
		slices.SortFunc(staleSlots, func(i, j *Slot) int {
			return int(i.LastActivityTime.Sub(j.LastActivityTime))
		})
		log.Trace().Interface("stale_slots", staleSlots).Msg("stale slots")
		if len(staleSlots) == 0 {
			return ErrRunnersAreFull
		}
		// Then delete the most stale slot, allow the reconciler to mop up
		withSlotContext(&log.Logger, staleSlots[0]).Info().Msg("deleting stale slot")
		s.slots.Delete(staleSlots[0].ID)
	}
	return nil
}

func (s *Scheduler) warmSlots(req *Workload) []*Slot {
	cosyWarm := make([]*Slot, 0, s.slots.Size())
	s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		// If the slot isn't running yet, skip
		if !slot.IsRunning() {
			withSlotContext(&log.Logger, slot).Trace().Msg("skipping warm slot, not running yet")
			return true
		}

		// If it's not the same model name, skip
		if slot.InitialWork().ModelName() != req.ModelName() {
			withSlotContext(&log.Logger, slot).Trace().Msg("skipping warm slot, model name mismatch")
			return true
		}

		// If it's not the same runtime, skip
		if slot.InitialWork().Runtime() != req.Runtime() {
			withSlotContext(&log.Logger, slot).Trace().Msg("skipping warm slot, inference runtime mismatch")
			return true
		}

		// If the slot is already running another job, skip
		if slot.IsActive() {
			withSlotContext(&log.Logger, slot).Trace().Msg("skipping warm slot, already active")
			return true
		}

		// If it doesn't have the right LoraDir then skip
		if slot.InitialWork().LoraDir() != req.LoraDir() {
			withSlotContext(&log.Logger, slot).Trace().Str("slot_lora_dir", slot.InitialWork().LoraDir()).Str("req_lora_dir", req.LoraDir()).Msg("skipping warm slot, LoraDir mismatch")
			return true
		}

		// Add available slots to the list.
		cosyWarm = append(cosyWarm, slot)
		return true
	})
	return cosyWarm
}

func (s *Scheduler) pickBestWarmSlot(warmSlots []*Slot) *Slot {
	// If we only have one slot, return it
	if len(warmSlots) == 1 {
		return warmSlots[0]
	}

	// Group slots by runner
	runnerSlots := make(map[string][]*Slot)
	for _, slot := range warmSlots {
		runnerSlots[slot.RunnerID] = append(runnerSlots[slot.RunnerID], slot)
	}

	// Count active slots per runner
	activeSlots := make(map[string]int)
	s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		if slot.IsActive() {
			activeSlots[slot.RunnerID]++
		}
		return true
	})

	// Sort slots considering:
	// 1. Runner load (prefer less loaded runners)
	// 2. Last activity time (prefer more recently used slots)
	// 3. Random factor for tie-breaking
	slices.SortFunc(warmSlots, func(i, j *Slot) int {
		// First compare runner load
		if activeSlots[i.RunnerID] != activeSlots[j.RunnerID] {
			return activeSlots[i.RunnerID] - activeSlots[j.RunnerID]
		}

		// Then prefer more recently used slots (reverse of current order)
		if !i.LastActivityTime.Equal(j.LastActivityTime) {
			return int(j.LastActivityTime.Sub(i.LastActivityTime))
		}

		// Random tie-breaker
		return rand.Intn(3) - 1
	})

	return warmSlots[0]
}

// AllocateSlot assigns a workload to a specific slot, validating the model and slot before scheduling.
func (s *Scheduler) allocateSlot(slotID uuid.UUID, req *Workload) error {
	// Validate slot
	slot, ok := s.slots.Load(slotID)
	if !ok {
		return fmt.Errorf("slot not found: %s", slot.ID.String())
	}

	// Ensure the slot is not already scheduled or active.
	if slot.IsActive() {
		return fmt.Errorf("slot already active: %s", slot.ID.String())
	}

	// Marks the slot as locally active. This is reset in the reconciliation process.
	withSlotAndWorkContext(&log.Logger, slot, req).Trace().Msg("starting slot")
	slot.Start()

	// Can do the rest in a goroutine, no need to wait for it to submit
	go func() {
		// Submit the work to the slot
		switch req.WorkloadType {
		case WorkloadTypeLLMInferenceRequest:
			llmReq := req.LLMInferenceRequest()
			if llmReq.Embeddings {
				withSlotAndWorkContext(&log.Logger, slot, req).Trace().Msg("submitting embedding request")
				err := s.controller.SubmitEmbeddingRequest(slot, llmReq)
				if err != nil {
					s.onSchedulingErr(req, err)
					withSlotAndWorkContext(&log.Logger, slot, req).Warn().Err(err).Msg("error submitting embedding request")
				}
			} else {
				withSlotAndWorkContext(&log.Logger, slot, req).Trace().Msg("submitting chat completion request")
				err := s.controller.SubmitChatCompletionRequest(slot, llmReq)
				if err != nil {
					s.onSchedulingErr(req, err)
					withSlotAndWorkContext(&log.Logger, slot, req).Warn().Err(err).Msg("error submitting chat completion request")
				}
			}
		case WorkloadTypeSession:
			switch req.Session().Mode {
			case types.SessionModeInference:
				switch req.Session().Type {
				case types.SessionTypeImage:
					withSlotAndWorkContext(&log.Logger, slot, req).Trace().Msg("submitting text2image request")
					err := s.controller.SubmitImageGenerationRequest(slot, req.Session())
					if err != nil {
						s.onSchedulingErr(req, err)
						withSlotAndWorkContext(&log.Logger, slot, req).Warn().Err(err).Msg("error submitting text2image request")
					}
				case types.SessionTypeText:
					if req.Session().LoraDir != "" {
						withSlotAndWorkContext(&log.Logger, slot, req).Trace().Msg("submitting LORA text inference request")
						// Overwrite the request model name with the helix lora model details
						convertedRequest := req.ToLLMInferenceRequest()
						convertedRequest.Request.Model = req.Session().LoraDir

						// Forward the request to the chat completion handler
						err := s.controller.SubmitChatCompletionRequest(slot, convertedRequest)
						if err != nil {
							s.onSchedulingErr(req, err)
							withSlotAndWorkContext(&log.Logger, slot, req).Warn().Err(err).Msg("error submitting LORA inference request")
						}
					} else {
						s.onSchedulingErr(req, fmt.Errorf("not implemented: %s and no lora dir", req.Session().Type))
						withSlotAndWorkContext(&log.Logger, slot, req).Warn().Msg("not implemented: no lora dir")
					}
				default:
					s.onSchedulingErr(req, fmt.Errorf("not implemented: %s", req.Session().Type))
					withSlotAndWorkContext(&log.Logger, slot, req).Warn().Msg("not implemented: session type")
				}
			case types.SessionModeFinetune:
				switch req.Session().Type {
				case types.SessionTypeText:
					withSlotAndWorkContext(&log.Logger, slot, req).Trace().Msg("submitting finetuning request")
					err := s.controller.SubmitFinetuningRequest(slot, req.Session())
					if err != nil {
						s.onSchedulingErr(req, err)
						withSlotAndWorkContext(&log.Logger, slot, req).Warn().Err(err).Msg("error submitting finetuning request")
					}
				default:
					s.onSchedulingErr(req, fmt.Errorf("not implemented: %s", req.Session().Type))
					withSlotAndWorkContext(&log.Logger, slot, req).Warn().Msg("not implemented: session type")
				}
			default:
				s.onSchedulingErr(req, fmt.Errorf("not implemented: %s", req.Session().Mode))
				withSlotAndWorkContext(&log.Logger, slot, req).Warn().Msg("not implemented: session mode")
			}
		}

		withSlotAndWorkContext(&log.Logger, slot, req).Trace().Msg("finished submitting request")
	}()

	return nil
}

// createNewSlot creates a new slot for the given runner and workload.
func (s *Scheduler) createNewSlot(ctx context.Context, slot *Slot) error {
	withSlotContext(&log.Logger, slot).Info().Msg("creating new slot on runner")

	err := s.controller.CreateSlot(slot)
	if err != nil {
		return err
	}

	// Wait for the slot to be ready
	slotReady := make(chan bool)

	// Add timeout variables for logging
	startTime := time.Now()
	readyTimeout := 120 * time.Minute
	lastLogTime := startTime
	attemptCount := 0

	withSlotContext(&log.Logger, slot).Info().
		Dur("timeout", readyTimeout).
		Str("model", slot.InitialWork().ModelName().String()).
		Msg("Starting wait for slot to be ready with timeout")

	go func() {
		for {
			select {
			case <-ctx.Done():
				withSlotContext(&log.Logger, slot).Warn().
					Dur("elapsed", time.Since(startTime)).
					Msg("Context canceled while waiting for slot to be ready")
				return
			case <-time.After(500 * time.Millisecond):
				attemptCount++
				elapsed := time.Since(startTime)

				// Log status every 10 seconds
				if time.Since(lastLogTime) > 10*time.Second {
					timeLeft := readyTimeout - elapsed
					withSlotContext(&log.Logger, slot).Debug().
						Dur("elapsed", elapsed).
						Dur("time_left", timeLeft).
						Int("attempt", attemptCount).
						Msg("Waiting for slot to be ready")
					lastLogTime = time.Now()
				}

				if s, err := s.controller.fetchSlot(slot.RunnerID, slot.ID); err == nil {
					if s.Ready {
						withSlotContext(&log.Logger, slot).Info().
							Dur("elapsed", elapsed).
							Int("attempts", attemptCount).
							Msg("Slot is now ready")
						slotReady <- true
					}
				} else {
					withSlotContext(&log.Logger, slot).Warn().
						Err(err).
						Dur("elapsed", elapsed).
						Msg("Error checking if slot is ready")
					close(slotReady)
					return
				}
			}
		}
	}()

	select {
	case <-slotReady:
		withSlotContext(&log.Logger, slot).Info().
			Dur("elapsed", time.Since(startTime)).
			Msg("Slot is ready")
	case <-time.After(readyTimeout):
		withSlotContext(&log.Logger, slot).Error().
			Dur("elapsed", time.Since(startTime)).
			Int("attempts", attemptCount).
			Msg("Timeout waiting for slot to be ready")
		return fmt.Errorf("slot not ready after 120 minutes")
	}

	// Mark the slot as running
	slot.SetRunning()
	withSlotContext(&log.Logger, slot).Info().Msg("slot created on runner")

	return nil
}

// AddWorkFields adds standard work-related fields to a log event
func withWorkContext(l *zerolog.Logger, w *Workload) *zerolog.Logger {
	nextLogger := l.With().
		Str("work_id", w.ID()).
		Str("model_name", w.ModelName().String()).
		Str("mode", string(w.Mode())).
		Str("runtime", string(w.Runtime())).
		Str("lora_dir", w.LoraDir()).
		Dur("duration_ms", time.Since(w.Created())).
		Logger()
	return &nextLogger
}

// AddSlotFields adds standard slot-related fields to a log event
func withSlotContext(l *zerolog.Logger, s *Slot) *zerolog.Logger {
	nextLogger := l.With().
		Str("runner_id", s.RunnerID).
		Str("slot_id", s.ID.String()).
		Str("model_name", s.InitialWork().ModelName().String()).
		Uint64("memory", s.Memory()).
		Logger()
	return &nextLogger
}

func withSlotAndWorkContext(l *zerolog.Logger, s *Slot, w *Workload) *zerolog.Logger {
	return withSlotContext(withWorkContext(l, w), s)
}
