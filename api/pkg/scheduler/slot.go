package scheduler

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type Slot struct {
	ID               uuid.UUID // An ID representing this unique model on a runner
	RunnerID         string    // The runner that this slot is assigned to
	work             *Workload // The work that is currently assigned to this slot
	LastActivityTime time.Time // Private because I don't want people misinterpreting this
	isActive         bool      // Private because I don't want people misinterpreting this
	mu               *sync.RWMutex
	isStaleFunc      TimeoutFunc
	isErrorFunc      TimeoutFunc
}

// NewSlot creates a new slot with the given runnerID and work
// staleTimeout is a function that determines if a slot is stale
// errorTimeout is a function that determines if a slot has errored
func NewSlot(runnerID string, work *Workload, staleTimeout TimeoutFunc, errorTimeout TimeoutFunc) *Slot {
	return &Slot{
		ID:               uuid.New(),
		RunnerID:         runnerID,
		work:             work,
		LastActivityTime: time.Now(),
		isActive:         false,
		mu:               &sync.RWMutex{},
		isStaleFunc:      staleTimeout,
		isErrorFunc:      errorTimeout,
	}
}

// True if the model is not active and hasn't been active for at least ModelTTL
func (s *Slot) IsStale() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// First if the work is active, or is scheduled...
	if s.isActive {
		// ... then check if the slot has timed out due to an error
		if s.isErrorFunc(s.RunnerID, s.LastActivityTime) {
			log.Warn().Str("runner_id", s.RunnerID).Str("slot_id", s.ID.String()).Msg("slot has timed out due to an unknown error, releasing slot")
			s.mu.RUnlock()
			s.Release()
			s.mu.RLock()
			// If it has errored, then it is stale
			return true
		}
	}

	// If work is active, not stale
	if s.isActive {
		return false
	}

	// Now check if the slot is stale
	return s.isStaleFunc(s.RunnerID, s.LastActivityTime)
}

// True if this slot is currently active with work
func (s *Slot) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.isActive
}

// Sets a slot as no longer active
func (s *Slot) Release() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isActive = false
	s.LastActivityTime = time.Now()
}

// Marks the work as started
func (s *Slot) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.LastActivityTime = time.Now()
	s.isActive = true
}

func (s *Slot) Mode() types.SessionMode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.work.Mode()
}

func (s *Slot) ModelName() model.Name {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.work.ModelName()
}

func (s *Slot) Memory() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.work.Model().GetMemoryRequirements(s.Mode())
}

func (s *Slot) LoraDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.work.LoraDir()
}

func (s *Slot) Runtime() types.Runtime {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.work.Runtime()
}
