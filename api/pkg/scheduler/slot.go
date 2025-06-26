package scheduler

import (
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// SlotState represents the lifecycle state of a slot
type SlotState int

const (
	SlotStateCreating SlotState = iota // Slot creation request sent to runner, waiting for response
	SlotStateLoading                   // Slot exists on runner but model is still loading
	SlotStateReady                     // Slot is ready to accept work
	SlotStateActive                    // Slot is currently processing work
	SlotStateError                     // Slot creation or loading failed
)

func (s SlotState) String() string {
	switch s {
	case SlotStateCreating:
		return "creating"
	case SlotStateLoading:
		return "loading"
	case SlotStateReady:
		return "ready"
	case SlotStateActive:
		return "active"
	case SlotStateError:
		return "error"
	default:
		return "unknown"
	}
}

type Slot struct {
	ID               uuid.UUID // An ID representing this unique model on a runner
	RunnerID         string    // The runner that this slot is assigned to
	initialWork      *Workload // The work that is currently assigned to this slot
	LastActivityTime time.Time // Private because I don't want people misinterpreting this
	isActive         bool      // Private because I don't want people misinterpreting this
	isStaleFunc      TimeoutFunc
	isErrorFunc      TimeoutFunc
	isRunning        bool

	// New state management
	state        SlotState  // Current state of the slot
	createdAt    time.Time  // When slot creation was initiated
	readyAt      *time.Time // When slot became ready (nil if not ready yet)
	errorMessage string     // Error message if state is SlotStateError
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

		// Initialize state
		state:     SlotStateCreating,
		createdAt: time.Now(),
		readyAt:   nil,
	}
}

// State returns the current state of the slot
func (s *Slot) State() SlotState {
	return s.state
}

// SetState updates the slot state
func (s *Slot) SetState(state SlotState) {
	oldState := s.state
	s.state = state
	s.LastActivityTime = time.Now()

	// Track when slot becomes ready
	if state == SlotStateReady && s.readyAt == nil {
		now := time.Now()
		s.readyAt = &now
		log.Info().
			Str("runner_id", s.RunnerID).
			Str("slot_id", s.ID.String()).
			Str("model", s.initialWork.ModelName().String()).
			Dur("creation_time", now.Sub(s.createdAt)).
			Msg("slot became ready")
	}

	log.Debug().
		Str("runner_id", s.RunnerID).
		Str("slot_id", s.ID.String()).
		Str("old_state", oldState.String()).
		Str("new_state", state.String()).
		Msg("slot state changed")
}

// SetError sets the slot to error state with a message
func (s *Slot) SetError(err error) {
	s.errorMessage = err.Error()
	s.SetState(SlotStateError)
}

// IsReadyForWork returns true if the slot can accept work
func (s *Slot) IsReadyForWork() bool {
	return s.state == SlotStateReady
}

// IsCreating returns true if the slot is still being created
func (s *Slot) IsCreating() bool {
	return s.state == SlotStateCreating
}

// IsLoading returns true if the slot is loading the model
func (s *Slot) IsLoading() bool {
	return s.state == SlotStateLoading
}

// HasError returns true if the slot is in error state
func (s *Slot) HasError() bool {
	return s.state == SlotStateError
}

// GetError returns the error message if slot is in error state
func (s *Slot) GetError() string {
	return s.errorMessage
}

// CreationTime returns how long ago the slot creation was initiated
func (s *Slot) CreationTime() time.Duration {
	return time.Since(s.createdAt)
}

// ReadyTime returns how long it took for the slot to become ready (nil if not ready yet)
func (s *Slot) ReadyTime() *time.Duration {
	if s.readyAt == nil {
		return nil
	}
	duration := s.readyAt.Sub(s.createdAt)
	return &duration
}

// True if the model is not active and hasn't been active for at least ModelTTL
func (s *Slot) IsStale() bool {
	// If slot has an error, it's considered stale
	if s.HasError() {
		return true
	}

	// If slot is still creating or loading, check for timeout
	if s.IsCreating() || s.IsLoading() {
		elapsed := time.Since(s.createdAt)
		isError := s.isErrorFunc(s.RunnerID, s.createdAt)
		if isError {
			log.Warn().
				Str("runner_id", s.RunnerID).
				Str("slot_id", s.ID.String()).
				Dur("elapsed_since_creation", elapsed).
				Str("model", s.initialWork.ModelName().String()).
				Str("state", s.state.String()).
				Msg("slot has timed out during creation/loading")
			return true
		}
		return false
	}

	// If work is active, check for error timeout
	if s.isActive {
		elapsed := time.Since(s.LastActivityTime)
		isError := s.isErrorFunc(s.RunnerID, s.LastActivityTime)
		if isError {
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
	return s.isActive
}

// Sets a slot as no longer active
func (s *Slot) Release() {
	s.isActive = false
	s.LastActivityTime = time.Now()
	// If slot was active, it goes back to ready state
	if s.state == SlotStateActive {
		s.SetState(SlotStateReady)
	}
}

// Marks the work as started
func (s *Slot) Start() {
	s.LastActivityTime = time.Now()
	s.isActive = true
	s.SetState(SlotStateActive)
}

func (s *Slot) IsRunning() bool {
	return s.isRunning
}

func (s *Slot) SetRunning() {
	s.isRunning = true
}

func (s *Slot) Memory() uint64 {
	return s.initialWork.model.Memory
}

func (s *Slot) InitialWork() *Workload {
	return s.initialWork
}
