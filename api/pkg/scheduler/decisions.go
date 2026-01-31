package scheduler

import (
	"fmt"
	"strings"
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

	// Check for duplicate "unschedulable" decisions to avoid spam
	if decision.DecisionType == types.SchedulingDecisionTypeUnschedulable {
		// Look for a recent identical decision (same model, same reason core without memory info)
		duplicateKey := t.generateDuplicateKey(decision)

		// Check the last few decisions for duplicates
		for i := 0; i < min(t.count, 10); i++ {
			checkIndex := (t.index - 1 - i + t.size) % t.size
			if t.decisions[checkIndex] != nil &&
				t.decisions[checkIndex].DecisionType == types.SchedulingDecisionTypeUnschedulable &&
				t.generateDuplicateKey(t.decisions[checkIndex]) == duplicateKey {

				// Update the existing decision with new repeat count and timestamp
				t.decisions[checkIndex].RepeatCount++
				t.decisions[checkIndex].Created = time.Now()
				t.decisions[checkIndex].Reason = decision.Reason // Update with latest memory info
				return
			}
		}
	}

	// Add to circular buffer
	t.decisions[t.index] = decision
	t.index = (t.index + 1) % t.size
	if t.count < t.size {
		t.count++
	}
}

// generateDuplicateKey creates a key for identifying duplicate unschedulable decisions
func (t *SchedulingDecisionsTracker) generateDuplicateKey(decision *types.SchedulingDecision) string {
	// Extract the reason without memory info by finding the part before the first '('
	reasonCore := decision.Reason
	if idx := strings.Index(reasonCore, "("); idx > 0 {
		reasonCore = strings.TrimSpace(reasonCore[:idx])
	}

	return fmt.Sprintf("%s:%s:%s", decision.ModelName, string(decision.Mode), reasonCore)
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

// GlobalAllocationDecisionsTracker manages a circular buffer of global allocation decisions
type GlobalAllocationDecisionsTracker struct {
	mu        sync.RWMutex
	decisions []*types.GlobalAllocationDecision
	index     int
	size      int
	count     int
}

// NewGlobalAllocationDecisionsTracker creates a new tracker with the specified buffer size
func NewGlobalAllocationDecisionsTracker(size int) *GlobalAllocationDecisionsTracker {
	return &GlobalAllocationDecisionsTracker{
		decisions: make([]*types.GlobalAllocationDecision, size),
		size:      size,
	}
}

// LogGlobalDecision adds a new global allocation decision to the tracker
func (t *GlobalAllocationDecisionsTracker) LogGlobalDecision(decision *types.GlobalAllocationDecision) {
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

// GetRecentGlobalDecisions returns the most recent global decisions, newest first
func (t *GlobalAllocationDecisionsTracker) GetRecentGlobalDecisions(limit int) []*types.GlobalAllocationDecision {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.count == 0 {
		return []*types.GlobalAllocationDecision{}
	}

	if limit <= 0 || limit > t.count {
		limit = t.count
	}

	result := make([]*types.GlobalAllocationDecision, 0, limit)

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

// ClearGlobal removes all stored global allocation decisions
func (t *GlobalAllocationDecisionsTracker) ClearGlobal() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i := range t.decisions {
		t.decisions[i] = nil
	}
	t.index = 0
	t.count = 0
}
