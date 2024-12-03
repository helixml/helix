package scheduler

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/rs/zerolog/log"
)

// WorkloadAllocator defines an interface for managing the allocation of workloads to runners.
type WorkloadAllocator interface {
	AllocateNewSlot(runnerID string, req *Workload) (*Slot, error)
	AllocateSlot(slotID uuid.UUID, req *Workload) error
	ReleaseSlot(slotID uuid.UUID) error
	DeadSlots(deadRunnerIDs []string) []*Slot
	WarmSlots(req *Workload) []*Slot
	RunnerSlots(id string) []*Slot
	ReconcileSlots(props *types.RunnerState) error

	// New queuing scheduler methods
	StartSlot(slotID uuid.UUID) error
	DeleteSlot(slotID uuid.UUID)
}

// TimeoutFunc defines a function type that determines if a runner has timed out based on the last activity.
type TimeoutFunc func(runnerID string, lastActivityTime time.Time) bool

// allocator implements the WorkloadAllocator interface, managing runners, slots, and workload allocation.
type allocator struct {
	slots           *xsync.MapOf[uuid.UUID, *Slot] // Maps slot ID to Slot details.
	modelStaleFunc  TimeoutFunc                    // Function to check if models are stale
	slotTimeoutFunc TimeoutFunc                    // Function to check if slots have timed out due to error
}

// NewWorkloadAllocator creates a new allocator instance with timeout functions for models and runners.
func NewWorkloadAllocator(staleFunc TimeoutFunc, slotTimeoutFunc TimeoutFunc) *allocator {
	return &allocator{
		slots:           xsync.NewMapOf[uuid.UUID, *Slot](),
		modelStaleFunc:  staleFunc,
		slotTimeoutFunc: slotTimeoutFunc,
	}
}

// AllocateSlot assigns a workload to a specific slot, validating the model and slot before scheduling.
func (a *allocator) AllocateSlot(slotID uuid.UUID, req *Workload) error {
	// Validate model
	if _, err := model.GetModel(req.ModelName().String()); err != nil {
		return fmt.Errorf("unable to get model (%s): %v", req.ModelName(), err)
	}

	// Validate slot
	slot, ok := a.slots.Load(slotID)
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

	log.Trace().
		Str("runner_id", slot.RunnerID).
		Str("slot_id", slot.ID.String()).
		Str("model_name", slot.ModelName().String()).
		Uint64("total_memory", slot.Memory()).
		Str("request_id", req.ID()).
		Msg("allocating slot")

	// Schedule the slot.
	slot.Schedule()

	return nil
}

// AllocateNewSlot creates a new slot for a workload and allocates it to the best available runner.
func (a *allocator) AllocateNewSlot(runnerID string, req *Workload) (*Slot, error) {
	// Create a new slot and schedule the workload.
	slot := NewSlot(runnerID, req, a.modelStaleFunc, a.slotTimeoutFunc)
	log.Trace().
		Str("runner_id", slot.RunnerID).
		Str("slot_id", slot.ID.String()).
		Str("model_name", slot.ModelName().String()).
		Uint64("total_memory", slot.Memory()).
		Str("request_id", req.ID()).
		Msg("creating new slot")

	// Ensure the slot is stored.
	a.slots.Store(slot.ID, slot)

	// Schedule and store the new slot.
	return slot, a.AllocateSlot(slot.ID, req)
}

// ReleaseSlot frees the resources allocated to a specific slot.
func (a *allocator) ReleaseSlot(slotID uuid.UUID) error {
	// Find the slot.
	slot, ok := a.slots.Load(slotID)
	if !ok {
		return fmt.Errorf("slot not found: %s", slotID.String())
	}

	log.Trace().
		Str("runner_id", slot.RunnerID).
		Str("slot_id", slot.ID.String()).
		Str("model_name", slot.ModelName().String()).
		Uint64("total_memory", slot.Memory()).
		Msg("releasing slot")

	// Release the slot.
	slot.Release()

	return nil
}

// ReconcileSlots updates the state of a runner and reconciles its slots with the allocator's records.
func (a *allocator) ReconcileSlots(props *types.RunnerState) error {
	// Log runner state update.
	l := log.With().
		Str("runner_id", props.ID).
		Int64("total_memory", int64(props.TotalMemory)).
		Int64("free_memory", int64(props.FreeMemory)).
		Logger()

	// Mark found slots.
	runnerSlots := a.RunnerSlots(props.ID)
	foundSlots := make(map[uuid.UUID]bool, len(runnerSlots))
	for _, s := range runnerSlots {
		foundSlots[s.ID] = false
	}

	// Reconcile the runner's view of running models with the allocator's.
	for _, m := range props.ModelInstances {
		for _, s := range runnerSlots {
			// If we've already seen this slot previously, skip. This might happen because there are
			// multiple instances of the same model running on a runner
			if foundSlots[s.ID] {
				continue
			}

			// If it's not the same model name, skip
			if m.ModelName != s.ModelName().String() {
				continue
			}

			// If it's not the same mode then skip
			if m.Mode != s.Mode() {
				continue
			}

			// If it's not the same LoraDir then skip
			if m.LoraDir != s.LoraDir() {
				continue
			}

			// Else we found it
			foundSlots[s.ID] = true

			// No need to keep searching the rest of the slots, move onto the next model
			break
		}
	}

	// Remove stale or unused slots.
	notFound := FilterMap(foundSlots, func(found bool) bool {
		return !found
	})
	if len(notFound) > 0 {
		l.Trace().
			Int("num_total_slots", a.slots.Size()).
			Int("num_slots_not_found", len(notFound)).
			Msg("reconciling slots with runner")
	}

	for id := range notFound {
		s, ok := a.slots.Load(id)
		if !ok {
			continue
		}

		// Check to make sure it's stale
		if !s.IsStale() {
			continue
		}

		if s.IsScheduled() {
			continue
		}

		// If we get here then it's ok to delete
		l.Trace().
			Str("slot_id", id.String()).
			Str("model_name", string(s.ModelName())).
			Bool("is_stale", s.IsStale()).
			Bool("is_scheduling", s.IsScheduled()).
			Msg("deleting slot")
		a.slots.Delete(id)
	}

	// Warn if the runner's state doesn't match the allocator's records.
	if len(props.ModelInstances) != len(a.RunnerSlots(props.ID)) {
		l.Trace().
			Int("runner_models_len", len(props.ModelInstances)).
			Int("controlplane_models_len", len(a.RunnerSlots(props.ID))).
			Msg("runner model mismatch, ignoring runner models")
	}

	return nil
}

// WarmSlots returns a list of available slots with warm models waiting for work.
func (a *allocator) WarmSlots(req *Workload) []*Slot {
	cosyWarm := make([]*Slot, 0, a.slots.Size())

	a.slots.Range(func(id uuid.UUID, slot *Slot) bool {
		l := log.With().
			Str("slot_id", id.String()).
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
			return true
		}

		// If it's not the same runtime, skip
		if slot.ModelName().InferenceRuntime() != req.ModelName().InferenceRuntime() {
			l.Trace().Msg("skipping warm slot, inference runtime mismatch")
			return true
		}

		// If the slot is already running another job, skip
		if slot.IsActive() {
			l.Trace().Msg("skipping warm slot, already active")
			return true
		}

		// If the slot is scheduled to run another job, skip
		if slot.IsScheduled() {
			l.Trace().Msg("skipping warm slot, already scheduled")
			return true
		}

		// If it doesn't have the right LoraDir then skip
		if slot.LoraDir() != req.LoraDir() {
			l.Trace().Msg("skipping warm slot, LoraDir mismatch")
			return true
		}

		// Add available slots to the list.
		cosyWarm = append(cosyWarm, slot)
		return true
	})
	return cosyWarm
}

// RunnerSlots returns all slots associated with a specific runner ID.
func (a *allocator) RunnerSlots(id string) []*Slot {
	allSlots := Values(a.slots)
	// Filter slots to include only those belonging to the specified runner.
	return Filter(allSlots, func(s *Slot) bool {
		return s.RunnerID == id
	})
}

// DeadSlots checks for any runners that have timed out and removes them.
// It returns the slots associated with the dead runners.
func (a *allocator) DeadSlots(deadRunnerIDs []string) []*Slot {
	deadSlots := make([]*Slot, 0)
	// Iterate through runners to check if any have timed out.
	for _, runnerID := range deadRunnerIDs {
		// Remove all slots from the allocator.
		slots := make([]*Slot, 0, a.slots.Size())
		runnerSlots := a.RunnerSlots(runnerID)
		for _, slot := range runnerSlots {
			log.Warn().
				Str("runner_id", runnerID).
				Str("slot_id", slot.ID.String()).
				Str("model_name", slot.ModelName().String()).
				Msg("deleting dead slot")
			a.slots.Delete(slot.ID)
			slots = append(slots, slot)
		}

		// Add these slots to the list of dead slots for rescheduling.
		deadSlots = append(deadSlots, slots...)
	}

	return deadSlots
}

// StartSlot marks scheduled work as in progress
func (a *allocator) StartSlot(slotID uuid.UUID) error {
	// Find the slot.
	slot, ok := a.slots.Load(slotID)
	if !ok {
		return fmt.Errorf("slot not found: %s", slotID.String())
	}

	// Log something when it first becomes active
	if slot.IsScheduled() {
		log.Trace().
			Str("runner_id", slot.RunnerID).
			Str("slot_id", slot.ID.String()).
			Str("model_name", slot.ModelName().String()).
			Uint64("total_memory", slot.Memory()).
			Msg("starting slot")
	}

	// Always mark the slot as active
	slot.Start()

	return nil
}

func (a *allocator) DeleteSlot(slotID uuid.UUID) {
	a.slots.Delete(slotID)
}
