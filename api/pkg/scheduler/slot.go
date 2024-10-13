package scheduler

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
)

type Slot struct {
	ID               uuid.UUID // An ID representing this unique model on a runner
	RunnerID         string    // The runner that this slot is assigned to
	work             *Workload // The work that is currently assigned to this slot
	lastActivityTime time.Time // Private because I don't want people misinterpreting this
	isActive         bool      // Private because I don't want people misinterpreting this
	isScheduled      bool      // Private because I don't want people misinterpreting this
	mu               *sync.RWMutex
	timeoutFunc      TimeoutFunc
	isNew            bool
}

func NewSlot(runnerID string, work *Workload, timeoutFunc TimeoutFunc) *Slot {
	return &Slot{
		ID:               uuid.New(),
		RunnerID:         runnerID,
		work:             work,
		lastActivityTime: time.Now(),
		isActive:         false,
		isScheduled:      false,
		isNew:            true, // Is new when slot is created
		mu:               &sync.RWMutex{},
		timeoutFunc:      timeoutFunc,
	}
}

// True if the model is not active and hasn't been active for at least ModelTTL
func (s *Slot) IsStale() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// If work is active, not stale
	if s.isActive {
		return false
	}

	// If work is scheduled, it is not stale
	if s.isScheduled {
		return false
	}

	// Now run the timeout function check
	return s.timeoutFunc(s.RunnerID, s.lastActivityTime)
}

// True if this slot is currently active with work
func (s *Slot) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.isActive
}

// Schedule sets a slot ready for scheduling
func (s *Slot) Schedule() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isScheduled = true
	s.lastActivityTime = time.Now()
}

// True if work is scheduled on this slot
func (s *Slot) IsScheduled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.isScheduled
}

// Sets a slot as no longer active
func (s *Slot) Release() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isActive = false
	s.lastActivityTime = time.Now()
}

// Marks the work as started
func (s *Slot) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isScheduled = false
	s.lastActivityTime = time.Now()
	s.isActive = true
	s.isNew = false
}

func (s *Slot) Mode() types.SessionMode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.work.Mode()
}

func (s *Slot) ModelName() model.ModelName {
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

func (s *Slot) IsNew() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.isNew
}
