package scheduler

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SlotStore provides a database-backed slot storage for the scheduler
// while maintaining the same synchronous interface as xsync.MapOf
type SlotStore struct {
	store store.Store
	mu    sync.RWMutex
	cache map[uuid.UUID]*Slot // Keep cache for performance
}

// NewSlotStore creates a new database-backed slot store
func NewSlotStore(store store.Store) *SlotStore {
	ss := &SlotStore{
		store: store,
		cache: make(map[uuid.UUID]*Slot),
	}

	// Load existing slots from database
	ss.loadFromDatabase()

	return ss
}

// SetTimeoutFunctions sets the timeout functions on all cached slots
// This should be called by the scheduler after creating the SlotStore
func (ss *SlotStore) SetTimeoutFunctions(staleFunc TimeoutFunc, errorFunc TimeoutFunc) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	for _, slot := range ss.cache {
		if slot.isStaleFunc == nil {
			slot.isStaleFunc = staleFunc
		}
		if slot.isErrorFunc == nil {
			slot.isErrorFunc = errorFunc
		}
	}
}

// Store saves a slot to both cache and database
func (ss *SlotStore) Store(id uuid.UUID, slot *Slot) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Ensure timeout functions are set if they're nil
	// This can happen when slots are created programmatically
	if slot.isStaleFunc == nil || slot.isErrorFunc == nil {
		// Try to get timeout functions from an existing slot in cache
		for _, existingSlot := range ss.cache {
			if existingSlot.isStaleFunc != nil && existingSlot.isErrorFunc != nil {
				if slot.isStaleFunc == nil {
					slot.isStaleFunc = existingSlot.isStaleFunc
				}
				if slot.isErrorFunc == nil {
					slot.isErrorFunc = existingSlot.isErrorFunc
				}
				break
			}
		}
	}

	// Store in cache first
	ss.cache[id] = slot

	log.Info().
		Str("slot_id", slot.ID.String()).
		Str("runner_id", slot.RunnerID).
		Msg("APPLE: Storing new slot with RunnerID")

	// Save to database
	ss.saveToDatabase(slot)
}

// Load retrieves a slot from cache
func (ss *SlotStore) Load(id uuid.UUID) (*Slot, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	slot, exists := ss.cache[id]
	return slot, exists
}

// Delete removes a slot from cache and database
func (ss *SlotStore) Delete(id uuid.UUID) {
	log.Info().Str("slot_id", id.String()).Msg("DEBUG: SlotStore.Delete called")
	// Remove from cache first while holding the lock
	log.Info().Str("slot_id", id.String()).Msg("DEBUG: about to acquire mutex lock")
	ss.mu.Lock()
	log.Info().Str("slot_id", id.String()).Msg("DEBUG: mutex lock acquired, deleting from cache")
	delete(ss.cache, id)
	ss.mu.Unlock()
	log.Info().Str("slot_id", id.String()).Msg("DEBUG: mutex lock released, about to delete from database")

	// Remove from database without holding the lock to avoid deadlock
	ss.deleteFromDatabase(id)
	log.Info().Str("slot_id", id.String()).Msg("DEBUG: deleteFromDatabase completed, Delete method finished")
}

// Range iterates over all slots in cache
func (ss *SlotStore) Range(fn func(uuid.UUID, *Slot) bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	for id, slot := range ss.cache {
		if !fn(id, slot) {
			break
		}
	}
}

// Size returns the number of slots in cache
func (ss *SlotStore) Size() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	return len(ss.cache)
}

// loadFromDatabase loads all slots from database into cache on startup
func (ss *SlotStore) loadFromDatabase() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbSlots, err := ss.store.ListAllSlots(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to load slots from database on startup")
		return
	}

	log.Info().Int("slot_count", len(dbSlots)).Msg("APPLE: Loading slots from database")

	ss.mu.Lock()
	defer ss.mu.Unlock()

	for _, dbSlot := range dbSlots {
		log.Info().
			Str("slot_id", dbSlot.ID.String()).
			Str("runner_id", dbSlot.RunnerID).
			Str("model", dbSlot.Model).
			Msg("APPLE: Loading slot from database")

		// Convert types.RunnerSlot to scheduler.Slot
		slot := &Slot{
			ID:               dbSlot.ID,
			RunnerID:         dbSlot.RunnerID,
			LastActivityTime: dbSlot.Updated,
			activeRequests:   0,                     // Start with 0 active requests (runtime state)
			maxConcurrency:   dbSlot.MaxConcurrency, // Restore from database
			isStaleFunc:      nil,                   // Will be set by SetTimeoutFunctions
			isErrorFunc:      nil,                   // Will be set by SetTimeoutFunctions
			isRunning:        dbSlot.Ready,
		}

		log.Info().
			Str("slot_id", slot.ID.String()).
			Str("runner_id", slot.RunnerID).
			Msg("APPLE: Created scheduler slot with RunnerID")

		// Deserialize workload from JSONB if present
		if dbSlot.WorkloadData != nil {
			workloadBytes, err := json.Marshal(dbSlot.WorkloadData)
			if err == nil {
				var workload Workload
				if err := json.Unmarshal(workloadBytes, &workload); err == nil {
					slot.initialWork = &workload
				} else {
					log.Error().Err(err).Str("slot_id", dbSlot.ID.String()).Msg("failed to deserialize workload data")
				}
			}
		}

		// Deserialize GPU allocation from JSONB if present
		if dbSlot.GPUAllocationData != nil {
			gpuBytes, err := json.Marshal(dbSlot.GPUAllocationData)
			if err == nil {
				var gpuAlloc GPUAllocation
				if err := json.Unmarshal(gpuBytes, &gpuAlloc); err == nil {
					slot.GPUAllocation = &gpuAlloc
				} else {
					log.Error().Err(err).Str("slot_id", dbSlot.ID.String()).Msg("failed to deserialize GPU allocation data")
				}
			}
		}

		// Fallback to legacy fields if JSONB data is not available
		if slot.GPUAllocation == nil {
			slot.GPUAllocation = &GPUAllocation{
				WorkloadID:         "", // Will be set by reconciliation
				RunnerID:           dbSlot.RunnerID,
				SingleGPU:          dbSlot.GPUIndex,
				MultiGPUs:          dbSlot.GPUIndices,
				TensorParallelSize: dbSlot.TensorParallelSize,
			}
		}

		ss.cache[dbSlot.ID] = slot

		log.Debug().
			Str("slot_id", dbSlot.ID.String()).
			Str("runner_id", dbSlot.RunnerID).
			Str("model", dbSlot.Model).
			Bool("active", dbSlot.Active).
			Msg("loaded scheduler slot from database")
	}

	log.Info().
		Int("slot_count", len(dbSlots)).
		Msg("loaded scheduler slots from database")
}

// saveToDatabase saves a slot to the database asynchronously
func (ss *SlotStore) saveToDatabase(slot *Slot) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Validate RunnerID before saving
	if slot.RunnerID == "" {
		log.Error().
			Str("slot_id", slot.ID.String()).
			Msg("APPLE: refusing to save slot with empty RunnerID to database")
		return
	}

	log.Info().
		Str("slot_id", slot.ID.String()).
		Str("runner_id", slot.RunnerID).
		Msg("APPLE: Saving slot to database with RunnerID")

	// Convert scheduler.Slot to types.RunnerSlot
	dbSlot := &types.RunnerSlot{
		ID:             slot.ID,
		RunnerID:       slot.RunnerID,
		Active:         slot.IsActive(),
		Ready:          slot.isRunning,
		Status:         "scheduler_managed",
		Created:        slot.Created,
		ActiveRequests: slot.GetActiveRequests(),
		MaxConcurrency: atomic.LoadInt64(&slot.maxConcurrency),
	}

	// Serialize workload to JSONB if available
	if slot.initialWork != nil {
		workloadBytes, err := json.Marshal(slot.initialWork)
		if err == nil {
			var workloadData map[string]any
			if err := json.Unmarshal(workloadBytes, &workloadData); err == nil {
				dbSlot.WorkloadData = workloadData
			}
		}

		// Also populate legacy fields for compatibility
		dbSlot.Model = string(slot.initialWork.ModelName())
		dbSlot.Runtime = slot.initialWork.Runtime()
	} else {
		// Default values for legacy fields
		dbSlot.Runtime = types.RuntimeOllama
		dbSlot.Model = ""
	}

	// Serialize GPU allocation to JSONB
	if slot.GPUAllocation != nil {
		gpuBytes, err := json.Marshal(slot.GPUAllocation)
		if err == nil {
			var gpuData map[string]any
			if err := json.Unmarshal(gpuBytes, &gpuData); err == nil {
				dbSlot.GPUAllocationData = gpuData
			}
		}

		// Also populate legacy fields for compatibility
		dbSlot.GPUIndex = slot.GPUAllocation.SingleGPU
		dbSlot.GPUIndices = slot.GPUAllocation.MultiGPUs
		dbSlot.TensorParallelSize = slot.GPUAllocation.TensorParallelSize
	}

	_, err := ss.store.CreateSlot(ctx, dbSlot)
	if err != nil {
		log.Error().Err(err).
			Str("slot_id", slot.ID.String()).
			Str("runner_id", slot.RunnerID).
			Msg("failed to save scheduler slot to database")
	}
}

// deleteFromDatabase removes a slot from the database asynchronously
func (ss *SlotStore) deleteFromDatabase(id uuid.UUID) {
	log.Info().Str("slot_id", id.String()).Msg("DEBUG: deleteFromDatabase called")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Info().Str("slot_id", id.String()).Msg("DEBUG: about to call store.DeleteSlot")
	err := ss.store.DeleteSlot(ctx, id.String())
	if err != nil {
		log.Error().Err(err).
			Str("slot_id", id.String()).
			Msg("failed to delete scheduler slot from database")
	} else {
		log.Info().Str("slot_id", id.String()).Msg("DEBUG: store.DeleteSlot completed successfully")
	}
}

// UpdateSlotActivity updates a slot's activity state in both cache and database
func (ss *SlotStore) UpdateSlotActivity(id uuid.UUID, active, running bool) {
	ss.mu.Lock()
	slot, exists := ss.cache[id]
	if exists {
		// Only sync running state from runner - scheduler manages active requests internally
		slot.isRunning = running
		slot.LastActivityTime = time.Now()
	}
	ss.mu.Unlock()

	if exists {
		log.Info().
			Str("slot_id", id.String()).
			Str("runner_id", slot.RunnerID).
			Bool("active", active).
			Bool("running", running).
			Msg("APPLE: Updating slot activity with RunnerID")
		ss.saveToDatabase(slot)
	}
}
