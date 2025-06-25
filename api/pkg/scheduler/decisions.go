package scheduler

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
)

// SchedulingDecisionsTracker manages a circular buffer of scheduling decisions
type SchedulingDecisionsTracker struct {
	mu        sync.RWMutex
	decisions []*types.SchedulingDecision
	index     int
	size      int
	count     int
}

// NewSchedulingDecisionsTracker creates a new tracker with the specified buffer size
func NewSchedulingDecisionsTracker(size int) *SchedulingDecisionsTracker {
	return &SchedulingDecisionsTracker{
		decisions: make([]*types.SchedulingDecision, size),
		size:      size,
	}
}

// LogDecision adds a new scheduling decision to the tracker
func (t *SchedulingDecisionsTracker) LogDecision(decision *types.SchedulingDecision) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Generate ID if not provided
	if decision.ID == "" {
		decision.ID = uuid.New().String()
	}

	// Set created time if not provided
	if decision.Created.IsZero() {
		decision.Created = time.Now()
	}

	// Add to circular buffer
	t.decisions[t.index] = decision
	t.index = (t.index + 1) % t.size
	if t.count < t.size {
		t.count++
	}
}

// GetRecentDecisions returns the most recent decisions, newest first
func (t *SchedulingDecisionsTracker) GetRecentDecisions(limit int) []*types.SchedulingDecision {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.count == 0 {
		return []*types.SchedulingDecision{}
	}

	if limit <= 0 || limit > t.count {
		limit = t.count
	}

	result := make([]*types.SchedulingDecision, 0, limit)

	// Start from the most recent and work backwards
	currentIndex := t.index - 1
	if currentIndex < 0 {
		currentIndex = t.size - 1
	}

	for i := 0; i < limit; i++ {
		if t.decisions[currentIndex] != nil {
			result = append(result, t.decisions[currentIndex])
		}
		currentIndex--
		if currentIndex < 0 {
			currentIndex = t.size - 1
		}
	}

	return result
}

// Clear removes all stored decisions
func (t *SchedulingDecisionsTracker) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i := range t.decisions {
		t.decisions[i] = nil
	}
	t.index = 0
	t.count = 0
}
