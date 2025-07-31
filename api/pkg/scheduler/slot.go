package scheduler

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type Slot struct {
	ID               uuid.UUID // An ID representing this unique model on a runner
	RunnerID         string    // The runner that this slot is assigned to
	initialWork      *Workload // The work that is currently assigned to this slot
	LastActivityTime time.Time // Private because I don't want people misinterpreting this
	isActive         bool      // Private because I don't want people misinterpreting this
	isStaleFunc      TimeoutFunc
	isErrorFunc      TimeoutFunc
	isRunning        bool
	isCreating       bool         // True while the slot is being created on the runner
	mu               sync.RWMutex // Protects slot state changes
}

// NewSlot creates a new slot with the given runnerID and work
// staleTimeout is a function that determines if a slot is stale
// errorTimeout is a function that determines if a slot has errored
func NewSlot(runnerID string, work *Workload, staleTimeout TimeoutFunc, errorTimeout TimeoutFunc) *Slot {
	return &Slot{
		ID:               uuid.New(),
		RunnerID:         runnerID,
		initialWork:      work,
		LastActivityTime: time.Now(),
		isActive:         false,
		isStaleFunc:      staleTimeout,
		isErrorFunc:      errorTimeout,
		isRunning:        false,
		isCreating:       false,
	}
}

// True if the model is not active and hasn't been active for at least ModelTTL
func (s *Slot) IsStale() bool {
	// If work is not running yet, check for error timeout (it might never have started)
	if !s.IsRunning() {
		elapsed := time.Since(s.LastActivityTime)
		isError := s.isErrorFunc(s.RunnerID, s.LastActivityTime)
		if isError {
			// Don't release the slot while holding the read lock
			// Instead, just return true and let the caller handle the release
			log.Warn().
				Str("runner_id", s.RunnerID).
				Str("slot_id", s.ID.String()).
				Dur("elapsed_since_activity", elapsed).
				Str("model", s.initialWork.ModelName().String()).
				Msg("slot has timed out during creation (not running)")
			return true
		}
		return false
	}

	// If work is active, check for error timeout
	if s.isActive {
		elapsed := time.Since(s.LastActivityTime)
		isError := s.isErrorFunc(s.RunnerID, s.LastActivityTime)
		if isError {
			// Don't release the slot while holding the read lock
			// Instead, just return true and let the caller handle the release
			log.Warn().
				Str("runner_id", s.RunnerID).
				Str("slot_id", s.ID.String()).
				Dur("elapsed_since_activity", elapsed).
				Str("model", s.initialWork.ModelName().String()).
				Msg("slot has timed out due to an unknown error (active slot)")
			return true
		}
		return false
	}

	// Now check if the slot is stale
	elapsed := time.Since(s.LastActivityTime)
	isStale := s.isStaleFunc(s.RunnerID, s.LastActivityTime)
	if isStale {
		log.Debug().
			Str("runner_id", s.RunnerID).
			Str("slot_id", s.ID.String()).
			Dur("elapsed_since_activity", elapsed).
			Str("model", s.initialWork.ModelName().String()).
			Msg("slot is considered stale (inactive too long)")
	}
	return isStale
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

func (s *Slot) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning && !s.isCreating
}

func (s *Slot) SetRunning() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isRunning = true
	s.isCreating = false
}

func (s *Slot) IsCreating() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isCreating
}

func (s *Slot) SetCreating() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isCreating = true
	s.isRunning = false
}

func (s *Slot) SetCreationFailed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isCreating = false
	s.isRunning = false
}

// TrySetCreating atomically checks if the slot is already creating
// and sets it to creating if not. Returns true if successful.
func (s *Slot) TrySetCreating() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isCreating {
		return false
	}
	s.isCreating = true
	s.isRunning = false
	return true
}

func (s *Slot) Memory() uint64 {
	return s.initialWork.model.Memory
}

func (s *Slot) InitialWork() *Workload {
	return s.initialWork
}
