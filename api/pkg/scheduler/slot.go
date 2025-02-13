package scheduler

import (
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
		isStaleFunc:      staleTimeout,
		isErrorFunc:      errorTimeout,
	}
}

// True if the model is not active and hasn't been active for at least ModelTTL
func (s *Slot) IsStale() bool {
	// If work is active, check for error timeout
	if s.isActive {
		if s.isErrorFunc(s.RunnerID, s.LastActivityTime) {
			// Don't release the slot while holding the read lock
			// Instead, just return true and let the caller handle the release
			log.Warn().Str("runner_id", s.RunnerID).Str("slot_id", s.ID.String()).Msg("slot has timed out due to an unknown error")
			return true
		}
		return false
	}

	// Now check if the slot is stale
	return s.isStaleFunc(s.RunnerID, s.LastActivityTime)
}

// True if this slot is currently active with work
func (s *Slot) IsActive() bool {
	return s.isActive
}

// Sets a slot as no longer active
func (s *Slot) Release() {
	s.isActive = false
	s.LastActivityTime = time.Now()
}

// Marks the work as started
func (s *Slot) Start() {
	s.LastActivityTime = time.Now()
	s.isActive = true
}

func (s *Slot) Mode() types.SessionMode {
	return s.work.Mode()
}

func (s *Slot) ModelName() model.Name {
	return s.work.ModelName()
}

func (s *Slot) Memory() uint64 {
	return s.work.Model().GetMemoryRequirements(s.Mode())
}

func (s *Slot) LoraDir() string {
	return s.work.LoraDir()
}

func (s *Slot) Runtime() types.Runtime {
	return s.work.Runtime()
}
