package scheduler

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
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
	WorkForRunner(id string, workType WorkloadType, newWorkOnly bool) (*Workload, error)
	UpdateRunner(props *types.RunnerState)
}

// scheduler is a struct implementing the Scheduler interface.
// It includes a workload allocator (allocator) and a thread-safe map (workStore) to store scheduled workloads.
type scheduler struct {
	allocator         WorkloadAllocator                  // Interface to allocate workload to different slots/models.
	workStore         *xsync.MapOf[uuid.UUID, *Workload] // Map to store the work associated with a slot.
	cluster           Cluster                            // Cluster to manage runner state.
	placementStrategy SchedulingStrategyFunc
}

var _ Scheduler = &scheduler{}

// NewScheduler creates a new scheduler with a workload allocator.
// It returns a Scheduler instance for managing workloads.
func NewScheduler(cfg *config.ServerConfig) *scheduler {
	allocator := NewWorkloadAllocator(
		NewTimeoutFunc(cfg.Providers.Helix.ModelTTL),
	)
	cluster := NewCluster(
		NewTimeoutFunc(cfg.Providers.Helix.RunnerTTL),
	)

	schedStratFunc := MaxSpreadStrategy
	switch SchedulingStrategy(cfg.Providers.Helix.SchedulingStrategy) {
	case SchedulingStrategy_MaxUtilization:
		log.Info().Str("strategy", cfg.Providers.Helix.SchedulingStrategy).Msg("scheduling strategy with spread work across all runners")
		schedStratFunc = MaxUtilizationStrategy
	case SchedulingStrategy_MaxSpread:
		log.Info().Str("strategy", cfg.Providers.Helix.SchedulingStrategy).Msg("scheduling strategy will maximize utilization on individual runners")
		schedStratFunc = MaxSpreadStrategy
	default:
		log.Warn().Str("strategy", cfg.Providers.Helix.SchedulingStrategy).Msg("unknown scheduling strategy, defaulting to max utilization")
	}
	scheduler := &scheduler{
		allocator:         allocator,
		cluster:           cluster,
		workStore:         xsync.NewMapOf[uuid.UUID, *Workload](),
		placementStrategy: schedStratFunc, // TODO: Make this configurable.
	}

	// Start a goroutine to log the current state of the scheduler.
	ticker := time.NewTicker(time.Minute * 1)
	go func() {
		for range ticker.C {
			scheduler.logState()
		}
	}()

	return scheduler
}

// NewTimeoutFunc returns a function to check if a runner has been idle for a specified timeout duration.
func NewTimeoutFunc(timeout time.Duration) TimeoutFunc {
	return func(runnerID string, lastActivity time.Time) bool {
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
	log.Trace().
		Int("warm_slots", len(slots)).
		Str("work_id", work.ID()).
		Msg("finding warm slots")

	// If warm slots are available, select a random one.
	if len(slots) > 0 {
		// Randomly select one warm slot from the available warm slots.
		slot = slots[rand.Intn(len(slots))]

		// Allocate work to the selected warm slot.
		err = s.allocator.AllocateSlot(slot.ID, work)
		if err != nil {
			// Return error if unable to allocate work to the warm model.
			return fmt.Errorf("unable to allocate work to a warm model: %w", err)
		}
	} else {
		// If no warm slots are available, pick a runner to allocate a slot to.
		bestRunnerID, err := s.placementStrategy(s.cluster, s.allocator, work)
		if err != nil {
			return fmt.Errorf("unable to place work on any runner: %w", err)
		}

		// Create an allocate slot
		slot, err = s.allocator.AllocateNewSlot(bestRunnerID, work)
		if err != nil {
			// Return error if unable to allocate a new slot.
			return fmt.Errorf("unable to allocate new work: %w", err)
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
// If newWorkOnly is FALSE, it means SEEKING WARM WORK, it will only return work from warm slots
// If newWorkOnly is TRUE, it means SEEKING NEW WORK, it will only return work from new slots
func (s *scheduler) WorkForRunner(id string, workType WorkloadType, newWorkOnly bool) (*Workload, error) {
	// Before retrieving work, check for dead runners and attempt to reschedule their work.
	deadSlots := s.allocator.DeadSlots(s.cluster.DeadRunnerIDs())
	for _, dead := range deadSlots {
		// Get work associated with the dead slot.
		work, ok := s.workStore.Load(dead.ID)
		if !ok {
			continue // Work not owned by this scheduler, ignore it.
		}

		// Attempt to reschedule the work.
		log.Trace().
			Str("runner_id", id).
			Str("slot_id", dead.ID.String()).
			Msg("rescheduling work for dead slot")
		err := s.Schedule(work)
		if err != nil {
			log.Error().
				Err(err).
				Str("runner_id", id).
				Str("slot_id", dead.ID.String()).
				Msg("failed to reschedule work for dead slot")
			continue
		}
	}

	// Iterate through the slots assigned to the runner.
	for _, slot := range s.allocator.RunnerSlots(id) {
		// If the slot is ready for scheduling, retrieve the associated work.
		if slot.IsScheduled() {
			work, ok := s.workStore.Load(slot.ID)
			if !ok {
				continue // Work not owned by this scheduler, ignore it.
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
	// Reconcile the runner's slots with the allocator's records.
	s.allocator.ReconcileSlots(props)
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

func (s *scheduler) logState() {
	for _, runnerID := range s.cluster.RunnerIDs() {
		currentSlots := s.allocator.RunnerSlots(runnerID)
		activeSlots := Filter(currentSlots, func(slot *Slot) bool {
			return slot.IsActive()
		})
		log.Trace().
			Str("runner_id", runnerID).
			Int("active_slots", len(activeSlots)).
			Int("total_slots", len(currentSlots)).
			Msg("runner state")
	}

}
