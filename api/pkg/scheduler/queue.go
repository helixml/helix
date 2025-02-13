package scheduler

import (
	"fmt"
	"sync"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type SlotRequirement struct {
	Runtime         types.Runtime
	Model           model.Name
	LoraDir         string
	Count           int
	ExampleWorkload *Workload
}

type WorkQueue struct {
	items    []*Workload
	capacity int
	mu       sync.RWMutex
}

func NewWorkQueue(capacity int) *WorkQueue {
	return &WorkQueue{
		items:    make([]*Workload, 0, capacity),
		capacity: capacity,
	}
}

// Add adds work to the queue
func (q *WorkQueue) Add(work *Workload) error {
	// Acquire a full lock to edit the queue
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if the work is already in the queue
	for _, w := range q.items {
		if w.ID() == work.ID() {
			return fmt.Errorf("work already in queue")
		}
	}

	if len(q.items) >= q.capacity {
		return fmt.Errorf("queue is full")
	}

	withWorkContext(&log.Logger, work).Trace().Msg("adding work item to queue")

	// Add with priority if requested
	priority := false
	if work.WorkloadType == WorkloadTypeSession {
		if work.Session().Metadata.Priority {
			priority = true
		}
	}
	if priority {
		q.items = append([]*Workload{work}, q.items...)
	} else {
		q.items = append(q.items, work)
	}

	return nil
}

// Queue returns a copy of the current queue, because the original queue might be modified after
// this call
func (q *WorkQueue) Queue() []*Workload {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return append([]*Workload{}, q.items...)
}

func (q *WorkQueue) TakeNext(hasWarmSlot func(*Workload) bool) *Workload {
	// Get a copy of the copy of the queue with an RLock to reduce contention
	items := q.Queue()

	// Check for warm slots without holding the queue lock
	for i, work := range items {
		// Its really important to not lock around hasWarmSlot because that function might call
		// another queue or slot function which might deadlock
		if hasWarmSlot(work) {
			// Acquire a full lock to edit the queue
			q.mu.Lock()
			// Verify the queue hasn't changed and the item is still at same position
			if i < len(q.items) && q.items[i].ID() == work.ID() {
				// Remove the item from the queue
				q.items = append(q.items[:i], q.items[i+1:]...)
				q.mu.Unlock()
				return work
			}
			q.mu.Unlock()
			// Queue changed while we were checking, try again
			return q.TakeNext(hasWarmSlot)
		}
	}
	return nil
}

// GetRequiredSlots analyzes the queue and returns the slot requirements
func (q *WorkQueue) GetRequiredSlots() []SlotRequirement {
	// Get a copy of the copy of the queue with an RLock to reduce contention
	items := q.Queue()

	// Map to accumulate requirements
	reqMap := make(map[string]*SlotRequirement)

	for _, work := range items {
		// Create a key that uniquely identifies this slot type
		key := fmt.Sprintf("%s:%s:%s",
			work.Runtime(),
			work.ModelName(),
			work.LoraDir(),
		)

		if req, exists := reqMap[key]; exists {
			req.Count++
		} else {
			reqMap[key] = &SlotRequirement{
				Runtime:         work.Runtime(),
				Model:           work.ModelName(),
				LoraDir:         work.LoraDir(),
				Count:           1,
				ExampleWorkload: work,
			}
		}
	}

	// Convert map to slice
	requirements := make([]SlotRequirement, 0, len(reqMap))
	for _, req := range reqMap {
		requirements = append(requirements, *req)
	}

	return requirements
}
