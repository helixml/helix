package scheduler

import (
	"context"
	"encoding/json"
	"sync"
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

// Store saves a slot to both cache and database
func (ss *SlotStore) Store(id uuid.UUID, slot *Slot) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Store in cache first
	ss.cache[id] = slot

	// Save to database
	go ss.saveToDatabase(slot)
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
	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Remove from cache
	delete(ss.cache, id)

	// Remove from database
	go ss.deleteFromDatabase(id)
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

	ss.mu.Lock()
	defer ss.mu.Unlock()

	for _, dbSlot := range dbSlots {
		// Convert types.RunnerSlot to scheduler.Slot
		slot := &Slot{
			ID:               dbSlot.ID,
			RunnerID:         dbSlot.RunnerID,
			LastActivityTime: dbSlot.Updated,
			isActive:         dbSlot.Active,
			isStaleFunc:      nil, // Will be set by scheduler
			isErrorFunc:      nil, // Will be set by scheduler
			isRunning:        dbSlot.Ready,
		}

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

	// Convert scheduler.Slot to types.RunnerSlot
	dbSlot := &types.RunnerSlot{
		ID:       slot.ID,
		RunnerID: slot.RunnerID,
		Active:   slot.isActive,
		Ready:    slot.isRunning,
		Status:   "scheduler_managed",
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := ss.store.DeleteSlot(ctx, id.String())
	if err != nil {
		log.Error().Err(err).
			Str("slot_id", id.String()).
			Msg("failed to delete scheduler slot from database")
	}
}

// UpdateSlotActivity updates a slot's activity state in both cache and database
func (ss *SlotStore) UpdateSlotActivity(id uuid.UUID, active, running bool) {
	ss.mu.Lock()
	slot, exists := ss.cache[id]
	if exists {
		slot.isActive = active
		slot.isRunning = running
		slot.LastActivityTime = time.Now()
	}
	ss.mu.Unlock()

	if exists {
		go ss.saveToDatabase(slot)
	}
}
