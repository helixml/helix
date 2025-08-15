// Package scheduler implements the Helix workload scheduler.
//
// CRITICAL SCHEDULING INVARIANT:
// ===============================
// The scheduler MUST NEVER over-allocate GPU memory. This means that the sum of all
// allocated memory on any GPU must never exceed the total memory available on that GPU.
//
// To maintain this invariant, ALL GPU allocation decisions MUST be based on allocated
// memory (memory already assigned to slots), NOT real-time memory usage from nvidia-smi.
// Real-time memory queries have a ~500ms delay which can cause race conditions where
// multiple allocation decisions are made before the memory tracking catches up.
//
// The scheduler is single-threaded, but allocation decisions are cached and reused,
// so we must ensure that:
// 1. calculateAllocatedMemoryPerGPU() always reflects the current scheduler state
// 2. All GPU selection (single and multi-GPU) uses allocated memory calculations
// 3. Memory allocations are immediately reflected in slot tracking
//
// This invariant is CRITICAL for system stability - violating it can cause GPU
// out-of-memory errors that crash model processes.
//
// STATE CONSISTENCY ARCHITECTURE:
// ================================
// To work well in a distributed system (where runners may get disconnected, be slow, etc),
// the scheduler operates on a single authoritative source of truth for scheduling decisions:
// LOCAL STATE (s.slots). Remote state is used only for reconciliation and monitoring.
//
// | Operation                     | State Source                    | Rationale                      |
// |-------------------------------|---------------------------------|--------------------------------|
// | 🔥 HOT PATH (Scheduling)     |                                 |                                |
// | Queue processing              | Local (s.slots)                | Fast decisions, no remote calls|
// | Warm slot detection           | Local (s.slots)                | Consistent with scheduling     |
// | Slot allocation               | Local (s.slots)                | Atomic local operations        |
// | Memory calculations           | Local (s.slots) + Remote (total)| Consistent allocated memory   |
// | Prewarming analysis           | Local (s.slots)                | Consistent with scheduling     |
// |                               |                                 |                                |
// | 🔄 RECONCILIATION             |                                 |                                |
// | Slot reconciliation           | Remote (fetchSlots)             | Sync local with remote reality |
// | Activity reconciliation       | Remote (fetchSlot)              | Check actual runner state      |
// |                               |                                 |                                |
// | 📊 MONITORING                 |                                 |                                |
// | Runner status API             | Remote (GetStatus)              | Live monitoring data           |
// | Runner slots API              | Remote (GetSlots)               | Live monitoring data           |
//
// KEY PRINCIPLES:
// 1. Hot path operations use LOCAL state for performance and consistency
// 2. Reconciliation processes sync local and remote state asynchronously
// 3. Unknown model memory is treated as a CRITICAL ERROR, not silently ignored
// 4. Memory calculations and scheduling decisions use the same source of truth
//
// MATHEMATICAL PROOF OF INVARIANT:
// =================================
// Let:
//   - G = set of all GPUs in a runner
//   - C_i = total memory capacity of GPU i ∈ G
//   - S = set of all active slots in the scheduler
//   - M_s = memory requirement of slot s ∈ S
//   - gpu(s) = GPU(s) assigned to slot s (for single-GPU slots)
//   - gpus(s) = set of GPUs assigned to slot s (for multi-GPU slots)
//   - |gpus(s)| = number of GPUs in multi-GPU slot s
//
// INVARIANT: ∀i ∈ G, ∑(s∈S: i∈allocated_gpus(s)) allocated_memory(s,i) ≤ C_i
//
// Where allocated_memory(s,i) = M_s if single-GPU slot on GPU i
//
//	= M_s/|gpus(s)| if multi-GPU slot using GPU i
//
// PROOF BY INDUCTION:
// Base case: Initially S = ∅, so ∑ = 0 ≤ C_i ✓
//
// Inductive step: Assume invariant holds for current state S.
// When adding new slot s_new with memory M_new:
//
// 1. Single-GPU allocation to GPU j:
//   - Current allocated: A_j = ∑(s∈S: j∈allocated_gpus(s)) allocated_memory(s,j)
//   - Free memory: F_j = C_j - A_j
//   - Allocation condition: F_j ≥ M_new ⟹ A_j + M_new ≤ C_j
//   - New total: A_j + M_new ≤ C_j ✓
//
// 2. Multi-GPU allocation to GPUs J = {j₁, j₂, ..., j_k}:
//
//   - For each j_i ∈ J: F_jᵢ = C_jᵢ - A_jᵢ
//
//   - Allocation condition: ∀j_i ∈ J, F_jᵢ ≥ M_new/k ⟹ A_jᵢ + M_new/k ≤ C_jᵢ
//
//   - New total for each GPU: A_jᵢ + M_new/k ≤ C_jᵢ ✓
//
//     3. Atomicity: The scheduler is single-threaded, so allocation decisions
//     and slot creation are atomic operations. No interleaving is possible.
//
//     4. Consistency: calculateAllocatedMemoryPerGPU() computes exact values:
//     A_i = ∑(s∈S: i∈allocated_gpus(s)) allocated_memory(s,i)
//     This is computed fresh for each allocation decision.
//
// Therefore, the invariant is maintained after each allocation. QED.
//
// COROLLARY: Over-allocation is mathematically impossible under this scheme.
package scheduler

import (
	"context"
	"fmt"
	"runtime/debug"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
	"golang.org/x/exp/rand"
)

const (
	pendingSlotsBufferSize         = 1 // The number of slot creation requests to buffer
	defaultRunnerReconcileInterval = 5 * time.Second
	activityReconcileInterval      = 100 * time.Millisecond
	queueReconcileInterval         = 100 * time.Millisecond
)

// TimeoutFunc defines a function type that determines if a runner has timed out based on the last activity.
type TimeoutFunc func(runnerID string, lastActivityTime time.Time) bool

func NewTimeoutFunc(ttl time.Duration) TimeoutFunc {
	return func(runnerID string, lastActivityTime time.Time) bool {
		elapsed := time.Since(lastActivityTime)
		isTimedOut := elapsed > ttl
		if isTimedOut {
			log.Debug().
				Str("runner_id", runnerID).
				Dur("elapsed", elapsed).
				Dur("ttl", ttl).
				Time("last_activity", lastActivityTime).
				Bool("timed_out", isTimedOut).
				Msg("Timeout function evaluation")
		}
		return isTimedOut
	}
}

type Scheduler struct {
	ctx                     context.Context
	controller              *RunnerController
	queue                   *WorkQueue
	onSchedulingErr         func(work *Workload, err error)
	slots                   *SlotStore                  // Database-backed slot storage
	modelStaleFunc          TimeoutFunc                 // Function to check if models are stale
	slotTimeoutFunc         TimeoutFunc                 // Function to check if slots have timed out due to error
	decisionsTracker        *SchedulingDecisionsTracker // Tracks scheduling decisions for dashboard
	runnerReconcileInterval time.Duration               // Configurable reconcile interval

	// GPU allocation tracking - maps workload+runner to specific GPU allocation
	gpuAllocations *xsync.MapOf[string, *GPUAllocation]

	// Channel to trigger immediate queue processing (e.g., when runners reconnect)
	queueTrigger chan struct{}

	// Heartbeat tracking for goroutines
	heartbeats *xsync.MapOf[string, *GoroutineHeartbeat]
}

// GoroutineHeartbeat tracks the health of scheduler goroutines
type GoroutineHeartbeat struct {
	Name          string    `json:"name"`
	LastBeat      time.Time `json:"last_beat"`
	RestartCount  int       `json:"restart_count"`
	IsHealthy     bool      `json:"is_healthy"`
	CurrentStatus string    `json:"current_status"`
}

// GPUAllocation represents a specific GPU allocation decision made by the scheduler
type GPUAllocation struct {
	WorkloadID         string // Workload identifier
	RunnerID           string // Runner identifier
	SingleGPU          *int   // For single-GPU models
	MultiGPUs          []int  // For multi-GPU models
	TensorParallelSize int    // Number of GPUs for tensor parallelism
}

type Params struct {
	RunnerController        *RunnerController
	Store                   store.Store // Required for slot persistence
	QueueSize               int
	OnSchedulingErr         func(work *Workload, err error)
	OnResponseHandler       func(ctx context.Context, resp *types.RunnerLLMInferenceResponse) error
	RunnerReconcileInterval *time.Duration // Optional: defaults to 5 seconds, set to 100ms for fast tests
}

func NewScheduler(ctx context.Context, serverConfig *config.ServerConfig, params *Params) (*Scheduler, error) {
	if params == nil {
		params = &Params{}
	}
	if params.RunnerController == nil {
		return nil, fmt.Errorf("runner controller is required")
	}
	if params.Store == nil {
		return nil, fmt.Errorf("store is required for slot persistence")
	}
	modelTTL := serverConfig.Providers.Helix.ModelTTL
	if modelTTL == 0 {
		modelTTL = 300 * time.Second
	}
	slotTTL := serverConfig.Providers.Helix.SlotTTL
	if slotTTL == 0 {
		slotTTL = 300 * time.Second
	}
	queueSize := 50
	if params.QueueSize > 0 {
		queueSize = params.QueueSize
	}

	// Set reconcile interval - default to 5 seconds, allow override for fast tests
	reconcileInterval := defaultRunnerReconcileInterval
	if params.RunnerReconcileInterval != nil {
		reconcileInterval = *params.RunnerReconcileInterval
	}

	log.Info().
		Dur("model_stale_time", modelTTL).
		Dur("slot_timeout", slotTTL).
		Dur("runner_reconcile_interval", reconcileInterval).
		Msg("slot timeouts configured")

	modelStaleFunc := NewTimeoutFunc(modelTTL)
	slotTimeoutFunc := NewTimeoutFunc(slotTTL)

	log.Info().
		Interface("runner_controller", params.RunnerController).
		Int("queue_size", params.QueueSize).
		Bool("has_scheduling_err_handler", params.OnSchedulingErr != nil).
		Bool("has_response_handler", params.OnResponseHandler != nil).
		Msg("Creating scheduler with parameters")

	if params.OnSchedulingErr == nil {
		params.OnSchedulingErr = func(work *Workload, err error) {
			log.Error().Err(err).Interface("work", work).Msg("scheduling error")
		}
	}

	s := &Scheduler{
		ctx:                     ctx,
		controller:              params.RunnerController,
		queue:                   NewWorkQueue(queueSize),
		onSchedulingErr:         params.OnSchedulingErr,
		slots:                   NewSlotStore(params.Store),
		modelStaleFunc:          modelStaleFunc,
		slotTimeoutFunc:         slotTimeoutFunc,
		decisionsTracker:        NewSchedulingDecisionsTracker(100), // Keep last 100 decisions
		runnerReconcileInterval: reconcileInterval,
		gpuAllocations:          xsync.NewMapOf[string, *GPUAllocation](),
		queueTrigger:            make(chan struct{}, 1), // Buffered channel to avoid blocking
		heartbeats:              xsync.NewMapOf[string, *GoroutineHeartbeat](),
	}

	// Start the queue processor
	go s.processQueue(ctx)

	// Start the slot reconciler
	go s.reconcileSlots(ctx)

	// Start the activity reconciler
	go s.reconcileActivity(ctx)

	// Start the runner reconciler
	go s.reconcileRunners(ctx)

	// Set up the model memory callback so RunnerController can get authoritative model memory
	s.controller.setModelMemoryCallback(s.getModelMemory)

	// Set up the scheduler slots callback so RunnerController can use desired state for scheduling decisions
	s.controller.setSchedulerSlotsCallback(s.getSchedulerSlots)

	return s, nil
}

// runGoroutineWithRestart runs a goroutine with automatic restart on both panic and normal exit
func (s *Scheduler) runGoroutineWithRestart(ctx context.Context, name string, fn func(context.Context)) {
	// Initialize heartbeat
	s.heartbeats.Store(name, &GoroutineHeartbeat{
		Name:          name,
		LastBeat:      time.Now(),
		RestartCount:  0,
		IsHealthy:     true,
		CurrentStatus: "initializing",
	})

	go func() {
		for {
			select {
			case <-ctx.Done():
				// Mark as unhealthy when context is cancelled
				if hb, ok := s.heartbeats.Load(name); ok {
					hb.IsHealthy = false
					s.heartbeats.Store(name, hb)
				}
				log.Info().Str("goroutine", name).Msg("goroutine shutting down due to context cancellation")
				return
			default:
				// Run the goroutine function with recovery
				func() {
					defer func() {
						if r := recover(); r != nil {
							// Update heartbeat on panic
							if hb, ok := s.heartbeats.Load(name); ok {
								hb.RestartCount++
								hb.LastBeat = time.Now()
								s.heartbeats.Store(name, hb)
							}
							log.Error().
								Str("goroutine", name).
								Interface("panic", r).
								Str("stack", string(debug.Stack())).
								Msg("goroutine panicked - restarting in 5 seconds")
						}
					}()

					// Update heartbeat before starting
					s.updateHeartbeat(name)

					// Run the actual function
					fn(ctx)

					// If we reach here, the function exited normally
					if hb, ok := s.heartbeats.Load(name); ok {
						hb.RestartCount++
						hb.LastBeat = time.Now()
						s.heartbeats.Store(name, hb)
					}
					log.Warn().
						Str("goroutine", name).
						Msg("goroutine exited normally - restarting in 5 seconds")
				}()

				// Wait 5 seconds before restarting (unless context is cancelled)
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
					// Continue to restart
				}
			}
		}
	}()
}

// updateHeartbeat updates the heartbeat for a goroutine
func (s *Scheduler) updateHeartbeat(name string) {
	if hb, ok := s.heartbeats.Load(name); ok {
		hb.LastBeat = time.Now()
		hb.IsHealthy = true
		s.heartbeats.Store(name, hb)
	}
}

// updateHeartbeatStatus updates the current status for a goroutine
func (s *Scheduler) updateHeartbeatStatus(name string, status string) {
	if hb, ok := s.heartbeats.Load(name); ok {
		hb.CurrentStatus = status
		s.heartbeats.Store(name, hb)
	}
}

// GetGoroutineHeartbeats returns the current heartbeat status of all scheduler goroutines
func (s *Scheduler) GetGoroutineHeartbeats() map[string]*GoroutineHeartbeat {
	result := make(map[string]*GoroutineHeartbeat)
	s.heartbeats.Range(func(name string, hb *GoroutineHeartbeat) bool {
		// Check if heartbeat is stale (no update in last 30 seconds)
		isStale := time.Since(hb.LastBeat) > 30*time.Second

		// Create a copy to avoid race conditions
		result[name] = &GoroutineHeartbeat{
			Name:          hb.Name,
			LastBeat:      hb.LastBeat,
			RestartCount:  hb.RestartCount,
			IsHealthy:     hb.IsHealthy && !isStale,
			CurrentStatus: hb.CurrentStatus,
		}
		return true
	})
	return result
}

// GetRunnerController returns the RunnerController for external access
func (s *Scheduler) GetRunnerController() *RunnerController {
	return s.controller
}

// storeGPUAllocation stores the GPU allocation decision for a workload-runner combination
func (s *Scheduler) storeGPUAllocation(work *Workload, runnerID string, singleGPU *int, multiGPUs []int) {
	var tensorParallelSize int
	if singleGPU != nil {
		tensorParallelSize = 1
	} else {
		tensorParallelSize = len(multiGPUs)
	}

	key := fmt.Sprintf("%s-%s", work.ID(), runnerID)
	allocation := &GPUAllocation{
		WorkloadID:         work.ID(),
		RunnerID:           runnerID,
		SingleGPU:          singleGPU,
		MultiGPUs:          multiGPUs,
		TensorParallelSize: tensorParallelSize,
	}

	s.gpuAllocations.Store(key, allocation)

	log.Trace().
		Str("workload_id", work.ID()).
		Str("runner_id", runnerID).
		Interface("single_gpu", singleGPU).
		Ints("multi_gpus", multiGPUs).
		Int("tensor_parallel_size", tensorParallelSize).
		Msg("Stored GPU allocation decision")
}

// getGPUAllocation retrieves the GPU allocation decision for a workload-runner combination
func (s *Scheduler) getGPUAllocation(workloadID, runnerID string) *GPUAllocation {
	key := fmt.Sprintf("%s-%s", workloadID, runnerID)
	if allocation, ok := s.gpuAllocations.Load(key); ok {
		return allocation
	}
	return nil
}

// clearGPUAllocation removes the GPU allocation for a workload when it's completed
func (s *Scheduler) clearGPUAllocation(workloadID string) {
	// Remove all allocations for this workload across all runners
	s.gpuAllocations.Range(func(key string, allocation *GPUAllocation) bool {
		if allocation.WorkloadID == workloadID {
			s.gpuAllocations.Delete(key)
			log.Debug().
				Str("workload_id", workloadID).
				Str("runner_id", allocation.RunnerID).
				Msg("Cleared GPU allocation for completed workload")
		}
		return true
	})
}

func (s *Scheduler) Enqueue(work *Workload) error {
	startTime := time.Now()

	err := s.queue.Add(work)
	if err != nil {
		// Log failed queuing - no specific runner/memory info available during queuing
		s.logSchedulingDecision(work, types.SchedulingDecisionTypeError, false,
			fmt.Sprintf("Failed to add to queue: %v", err), "", "", startTime, 0, work.model.Memory, 0)
		return err
	}

	// Log successful queuing - no specific runner/memory info available during queuing
	s.logSchedulingDecision(work, types.SchedulingDecisionTypeQueued, true,
		"Added to queue", "", "", startTime, 0, work.model.Memory, 0)

	return nil
}

func (s *Scheduler) Queue() ([]*types.WorkloadSummary, error) {
	currentQueue := s.queue.Queue()
	queue := make([]*types.WorkloadSummary, 0, len(currentQueue))
	for _, w := range currentQueue {
		summary := ""
		switch w.WorkloadType {
		case WorkloadTypeLLMInferenceRequest:
			req := w.LLMInferenceRequest()
			if req.Request != nil && len(req.Request.Messages) > 0 {
				summary = req.Request.Messages[len(req.Request.Messages)-1].Content
			} else if req.Embeddings && req.EmbeddingRequest.Input != nil {
				switch input := req.EmbeddingRequest.Input.(type) {
				case string:
					summary = input
				case []string:
					if len(input) > 0 {
						summary = input[0]
					}
				default:
					summary = "Embedding request"
				}
			}
		case WorkloadTypeSession:
			s := w.Session()
			summary = s.Interactions[len(s.Interactions)-1].PromptMessage
		}
		queue = append(queue, &types.WorkloadSummary{
			ID:        w.ID(),
			CreatedAt: w.Created(),
			UpdatedAt: w.Updated(),
			ModelName: string(w.ModelName()),
			Mode:      string(w.Mode()),
			Runtime:   string(w.Runtime()),
			LoraDir:   w.LoraDir(),
			Summary:   summary,
		})
	}
	return queue, nil
}

func (s *Scheduler) RunnerStatus() ([]*types.RunnerStatus, error) {
	var err error
	// Get a current list of runners
	runners := s.controller.RunnerIDs()

	// Get the current state of each runner
	runnerStates := make([]*types.RunnerStatus, 0, len(runners))
	for _, runnerID := range runners {
		var runnerStatus *types.RunnerStatus
		runnerStatus, err = s.controller.GetStatus(runnerID)
		if err != nil {
			log.Warn().Err(err).Str("runner_id", runnerID).Msg("error getting runner status, this shouldn't happen, please investigate this runner")
			runnerStatus = &types.RunnerStatus{
				ID: runnerID,
			}
		}
		runnerStates = append(runnerStates, runnerStatus)
	}

	return runnerStates, nil
}

func (s *Scheduler) RunnerSlots(runnerID string) ([]*types.RunnerSlot, error) {
	return s.controller.GetSlots(runnerID)
}

// processQueue runs in a goroutine to processes the queue of requests.
func (s *Scheduler) processQueue(ctx context.Context) {
	s.runGoroutineWithRestart(ctx, "processQueue", func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(queueReconcileInterval):
				s.updateHeartbeat("processQueue")
				s.processQueueOnce()
			case <-s.queueTrigger:
				s.updateHeartbeat("processQueue")
				s.processQueueOnce()
			}
		}
	})
}

// reconcileSlots runs in a goroutine to reconcile slots.
// The reason why we do this async is because we don't want to have to check the runner on the hot
// path. When a user makes a request we want to forward it to a warm runner as quickly as possible.
func (s *Scheduler) reconcileSlots(ctx context.Context) {
	s.runGoroutineWithRestart(ctx, "reconcileSlots", func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.runnerReconcileInterval):
				s.updateHeartbeat("reconcileSlots")
				s.updateHeartbeatStatus("reconcileSlots", "starting reconcile cycle")
				// log.Debug().
				//	Str("SLOT_RECONCILE_DEBUG", "starting_reconcile").
				//	Msg("SLOT_RECONCILE_DEBUG: Starting reconcileSlots cycle")
				s.reconcileSlotsOnce(ctx)
				// log.Debug().
				//	Str("SLOT_RECONCILE_DEBUG", "finished_reconcile").
				//	Msg("SLOT_RECONCILE_DEBUG: Finished reconcileSlots cycle")
				s.updateHeartbeatStatus("reconcileSlots", "idle - waiting for next cycle")
			}
		}
	})
}

// reconcileActivity runs in a goroutine to reconcile activity.
func (s *Scheduler) reconcileActivity(ctx context.Context) {
	s.runGoroutineWithRestart(ctx, "reconcileActivity", func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(activityReconcileInterval):
				s.updateHeartbeat("reconcileActivity")
				s.reconcileActivityOnce()
			}
		}
	})
}

func (s *Scheduler) reconcileActivityOnce() {
	if s.slots == nil {
		return
	}
	activeCount := 0
	checkedCount := 0
	releasedCount := 0

	// Check the status of all remaining slots to see if they have finished their work
	s.slots.Range(func(slotID uuid.UUID, slot *Slot) bool {
		checkedCount++
		if slot == nil {
			withSlotContext(&log.Logger, slot).Debug().Msg("slot is nil, releasing")
			slot.Release()
			releasedCount++
			return true
		}
		if slot.IsActive() {
			activeCount++
			// Get the live slot from the runner, don't use the cached copy
			remoteSlot, err := s.controller.fetchSlot(slot.RunnerID, slotID)
			if err != nil {
				withSlotContext(&log.Logger, slot).Error().
					Err(err).
					Msg("failed to get slot during activity reconciliation, assuming it's finished")
				slot.Release()
				releasedCount++
			} else if !remoteSlot.Active {
				withSlotContext(&log.Logger, slot).Debug().
					Bool("remote_active", remoteSlot.Active).
					Bool("remote_ready", remoteSlot.Ready).
					Msg("slot is not active according to remote, releasing")
				slot.Release()
				releasedCount++
			} else {
				withSlotContext(&log.Logger, slot).Trace().
					Bool("remote_active", remoteSlot.Active).
					Bool("remote_ready", remoteSlot.Ready).
					Msg("slot is still active according to remote")
			}
		}
		return true
	})

	// Only log when there's actual activity to report (skip empty reconciliations)
	if activeCount > 0 || releasedCount > 0 {
		log.Trace().
			Int("checked", checkedCount).
			Int("active", activeCount).
			Int("released", releasedCount).
			Msg("Slot activity reconciliation completed")
	}
}

// reconcileRunners runs in a goroutine to reconcile runners.
func (s *Scheduler) reconcileRunners(ctx context.Context) {
	s.runGoroutineWithRestart(ctx, "reconcileRunners", func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.runnerReconcileInterval):
				s.updateHeartbeat("reconcileRunners")
				s.reconcileRunnersOnce()
			}
		}
	})
}

func (s *Scheduler) reconcileRunnersOnce() {
	// Get all runners
	runnerIDs := s.controller.RunnerIDs()

	// Get the health of each runner
	for _, runnerID := range runnerIDs {
		err := s.controller.GetHealthz(runnerID)
		if err != nil {
			log.Warn().Err(err).Str("runner_id", runnerID).Msg("runner is not healthy, deleting...")
			s.controller.deleteRunner(runnerID)

			// Delete all slots belonging to this runner
			s.deleteRunnerSlots(runnerID)
			continue
		}

		// Set the models on the runner
		err = s.controller.SetModels(runnerID)
		if err != nil {
			log.Warn().Err(err).Str("runner_id", runnerID).Msg("failed to set models on runner, skipping...")
		}
	}
}

func (s *Scheduler) deleteRunnerSlots(runnerID string) {
	// First collect the slots to delete
	var slotsToDelete []uuid.UUID
	var workloadsToCleanup []string
	s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		if slot.RunnerID == runnerID {
			slotsToDelete = append(slotsToDelete, slot.ID)
			workloadsToCleanup = append(workloadsToCleanup, slot.InitialWork().ID())
		}
		return true
	})

	// Then delete them after the range is complete
	for _, slotID := range slotsToDelete {
		s.slots.Delete(slotID)
	}

	// Clean up GPU allocations for deleted workloads
	for _, workloadID := range workloadsToCleanup {
		s.clearGPUAllocation(workloadID)
	}
}

// reconcileSlotsOnce reconciles slots once.
func (s *Scheduler) reconcileSlotsOnce(ctx context.Context) {
	s.updateHeartbeatStatus("reconcileSlots", "getting runner list")
	// Get all runners
	runnerIDs := s.controller.RunnerIDs()
	// log.Info().Strs("runner_ids", runnerIDs).Msg("SLOT_RECONCILE_DEBUG: Starting slot reconciliation")

	s.updateHeartbeatStatus("reconcileSlots", "getting required slots from queue")

	// Ensure new slots are created and ready to take work
	requiredSlots := s.queue.GetRequiredSlots()
	// log.Info().Interface("required_slots", requiredSlots).Msg("SLOT_RECONCILE_DEBUG: Required slots for current workload")

	// Track slot stats
	existingSlotCount := 0
	slotsToCreate := 0
	duplicateSlotCount := 0
	orphanedSlotCount := 0
	mismatchedRunnerCount := 0

	s.updateHeartbeatStatus("reconcileSlots", "processing slot requirements")
	// Process slot requirements one at a time to prevent GPU allocation race conditions
	// This ensures each new slot sees the updated GPU state from previous allocations
	for _, req := range requiredSlots {
		// Check if we have enough slots for this work right now
		existingCount := 0
		// TODO(Phil): be careful about this, it's detached from the concept of a warm slot. Ideally
		// refactor so that warm and this are using the same logic.
		s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
			if slot.InitialWork().ModelName() == req.Model &&
				slot.InitialWork().Runtime() == req.Runtime &&
				slot.InitialWork().LoraDir() == req.LoraDir &&
				!slot.IsActive() {
				existingCount++
			}
			return true
		})

		existingSlotCount += existingCount

		// If we need more slots, create only ONE slot per reconciliation cycle
		// This prevents race conditions where multiple models see the same GPU state
		slotsNeeded := req.Count - existingCount
		if slotsNeeded > 0 {
			// Create only one slot per cycle to ensure proper GPU distribution
			slotsToCreateThisCycle := 1
			slotsToCreate += slotsToCreateThisCycle

			// log.Info().
			//	Str("model", req.Model.String()).
			//	Str("runtime", string(req.Runtime)).
			//	Int("existing", existingCount).
			//	Int("required", req.Count).
			//	Int("needed_total", slotsNeeded).
			//	Int("creating_this_cycle", slotsToCreateThisCycle).
			//	Msg("SLOT_RECONCILE_DEBUG: Creating one slot per cycle for proper GPU distribution")

			// log.Debug().
			//	Str("SLOT_RECONCILE_DEBUG", "calling_ensure_slots").
			//	Str("model", req.Model.String()).
			//	Int("slots_needed", slotsToCreateThisCycle).
			//	Msg("SLOT_RECONCILE_DEBUG: About to call ensureSlots")

			s.updateHeartbeatStatus("reconcileSlots", fmt.Sprintf("creating 1 slot for model %s (%d more needed)", req.Model.String(), slotsNeeded-1))
			s.ensureSlots(req, slotsToCreateThisCycle)

			// log.Debug().
			//	Str("SLOT_RECONCILE_DEBUG", "ensure_slots_returned").
			//	Str("model", req.Model.String()).
			//	Int("slots_created", slotsToCreateThisCycle).
			//	Int("remaining_needed", slotsNeeded-1).
			//	Msg("SLOT_RECONCILE_DEBUG: ensureSlots returned - remaining slots will be created in next cycle")

			// Exit early after creating one slot to allow GPU state to update
			// The reconciler runs every 5 seconds, so remaining slots will be created in subsequent cycles
			break
		}
		// log.Info().
		//	Str("model", req.Model.String()).
		//	Str("runtime", string(req.Runtime)).
		//	Int("existing", existingCount).
		//	Int("required", req.Count).
		//	Msg("SLOT_RECONCILE_DEBUG: No new slots needed for requirement")
	}

	// Build a complete map of all actual slots across all runners
	allActualSlots := make(map[uuid.UUID]string) // maps slot ID to runner ID
	for _, runnerID := range runnerIDs {
		// We need a live view of the slots here, otherwise we might get stale data
		actualSlots, err := s.controller.fetchSlots(runnerID)
		if err != nil {
			log.Error().Err(err).Str("runner_id", runnerID).Msg("failed to get slots from runner")
			continue
		}

		for _, slot := range actualSlots.Slots {
			// If we find the same slot ID on multiple runners, delete from the duplicate runner
			if existingRunnerID, exists := allActualSlots[slot.ID]; exists {
				duplicateSlotCount++
				log.Warn().
					Str("slot_id", slot.ID.String()).
					Str("existing_runner", existingRunnerID).
					Str("duplicate_runner", runnerID).
					Msg("found duplicate slot ID on multiple runners, cleaning up duplicate")
				err = s.controller.DeleteSlot(runnerID, slot.ID)
				if err != nil {
					log.Error().Err(err).Str("runner_id", runnerID).Str("slot_id", slot.ID.String()).Msg("failed to delete duplicate slot")
				}
				continue
			}
			allActualSlots[slot.ID] = runnerID
		}
	}

	// Delete removed slots -- important to delete first to create room for new slots
	unusedSlotCount := 0
	for slotID, runnerID := range allActualSlots {
		if _, exists := s.slots.Load(slotID); !exists {
			unusedSlotCount++
			// Look up the actual slot to get details for logging
			slotDetails, err := s.controller.fetchSlot(runnerID, slotID)
			if err == nil {
				log.Warn().
					Str("runner_id", runnerID).
					Str("slot_id", slotID.String()).
					Str("model", slotDetails.Model).
					Str("runtime", string(slotDetails.Runtime)).
					Bool("is_active", slotDetails.Active).
					Bool("is_ready", slotDetails.Ready).
					Str("reason", "orphaned_slot").
					Msg("deleting orphaned slot - exists on runner but not in scheduler")

				// Log as scheduling decision for visibility
				if slotDetails.Model != "" {
					// Create a dummy workload for logging purposes with minimal model info
					dummyWork := &Workload{
						WorkloadType: WorkloadTypeLLMInferenceRequest,
						llmInferenceRequest: &types.RunnerLLMInferenceRequest{
							RequestID: "orphaned-cleanup",
							Request:   &openai.ChatCompletionRequest{Model: slotDetails.Model},
						},
						model: &types.Model{
							ID:     slotDetails.Model,
							Name:   slotDetails.Model,
							Memory: 0, // Unknown memory for orphaned slots
						},
					}
					s.logSchedulingDecision(dummyWork, types.SchedulingDecisionTypeEvictStaleSlot, true,
						fmt.Sprintf("Deleted orphaned slot %s (model: %s) - existed on runner but not in scheduler",
							slotID.String(), slotDetails.Model), runnerID, slotID.String(),
						time.Now(), 0, 0, 0)
				}
			} else {
				log.Warn().
					Err(err).
					Str("runner_id", runnerID).
					Str("slot_id", slotID.String()).
					Str("reason", "orphaned_slot").
					Msg("deleting orphaned slot - exists on runner but not in scheduler, couldn't fetch details")
			}

			err = s.controller.DeleteSlot(runnerID, slotID)
			if err != nil {
				log.Error().Err(err).Str("runner_id", runnerID).Str("slot_id", slotID.String()).Msg("failed to delete slot")
			}
		}
	}

	// Create new slots - collect failed slots for deletion after Range completes to avoid deadlock
	type failedSlotInfo struct {
		slotID uuid.UUID
		work   *Workload
		err    error
	}
	var failedSlots []failedSlotInfo

	s.slots.Range(func(slotID uuid.UUID, slot *Slot) bool {
		if runnerID, exists := allActualSlots[slotID]; !exists {
			orphanedSlotCount++

			// Safely handle potential nil InitialWork for logging
			var modelName, runtime string
			if work := slot.InitialWork(); work != nil {
				modelName = work.ModelName().String()
				runtime = string(work.Runtime())
			} else {
				modelName = "unknown"
				runtime = "unknown"
			}

			log.Info().
				Str("runner_id", slot.RunnerID).
				Str("slot_id", slotID.String()).
				Str("model", modelName).
				Str("runtime", runtime).
				Bool("is_active", slot.IsActive()).
				Bool("is_running", slot.IsRunning()).
				Msg("found slot on the scheduler that doesn't exist on the runner, creating...")

			// log.Debug().
			//	Str("SLOT_RECONCILE_DEBUG", "calling_create_new_slot").
			//	Str("runner_id", slot.RunnerID).
			//	Str("slot_id", slot.ID.String()).
			//	Str("model", slot.InitialWork().ModelName().String()).
			//	Msg("SLOT_RECONCILE_DEBUG: About to call createNewSlot for orphaned slot")

			err := s.createNewSlot(ctx, slot)

			// log.Debug().
			//	Str("SLOT_RECONCILE_DEBUG", "create_new_slot_returned").
			//	Str("runner_id", slot.RunnerID).
			//	Str("slot_id", slot.ID.String()).
			//	Str("model", slot.InitialWork().ModelName().String()).
			//	Bool("had_error", err != nil).
			//	Msg("SLOT_RECONCILE_DEBUG: createNewSlot returned for orphaned slot")

			if err != nil {
				// Check for nil work before proceeding
				work := slot.InitialWork()
				if work == nil {
					log.Warn().Err(err).Str("slot_id", slot.ID.String()).Msg("failed to create slot with nil work, skipping error handling")
					return true
				}

				// Then see if we can retry
				retry, err := ErrorHandlingStrategy(err, work)
				if retry {
					withWorkContext(&log.Logger, work).Debug().Err(err).Msg("failed to create slot, but retrying later...")
				} else {
					withWorkContext(&log.Logger, work).Warn().Err(err).Msg("failed to create slot, calling error handler")

					// Don't delete immediately - collect for deletion after Range completes to avoid deadlock
					failedSlots = append(failedSlots, failedSlotInfo{
						slotID: slot.ID,
						work:   work,
						err:    err,
					})
				}
			}
		} else if runnerID != slot.RunnerID {
			// The slot exists but on a different runner than we thought
			mismatchedRunnerCount++
			log.Warn().
				Str("scheduler_runner", slot.RunnerID).
				Str("actual_runner", runnerID).
				Str("slot_id", slotID.String()).
				Msg("slot exists on different runner than expected, updating runner ID")
			slot.RunnerID = runnerID
		}
		return true
	})

	// Now safely delete the failed slots after Range has completed
	for _, failed := range failedSlots {
		// First remove that slot, since it was never created
		s.slots.Delete(failed.slotID)

		// Then remove the work from the queue if it exists
		s.queue.Remove(failed.work)

		// Then notify the error handler
		s.onSchedulingErr(failed.work, failed.err)
	}

	log.Trace().
		Int("existing_slots", existingSlotCount).
		Int("slots_to_create", slotsToCreate).
		Int("duplicate_slots", duplicateSlotCount).
		Int("orphaned_slots", orphanedSlotCount).
		Int("mismatched_runner_slots", mismatchedRunnerCount).
		Int("unused_slots", unusedSlotCount).
		Msg("Completed slot reconciliation")
}

// calculateRunnerMemory calculates the total, allocated, and free memory for a runner
// Returns error if runner status or memory information is unavailable - NO DEFAULTS
func (s *Scheduler) calculateRunnerMemory(runnerID string) (uint64, uint64, uint64, error) {
	// Get runner status
	runnerStatus, err := s.controller.GetStatus(runnerID)
	if err != nil {
		log.Error().
			Str("runner_id", runnerID).
			Err(err).
			Msg("CRITICAL: failed to get runner status for memory calculation - cannot proceed without real data")
		return 0, 0, 0, fmt.Errorf("runner %s status unavailable: %w", runnerID, err)
	}

	totalMemory := runnerStatus.TotalMemory
	if totalMemory == 0 {
		log.Error().
			Str("runner_id", runnerID).
			Msg("CRITICAL: runner reports zero total memory - cannot proceed without real memory data")
		return 0, 0, 0, fmt.Errorf("runner %s reports zero total memory", runnerID)
	}

	// Calculate allocated memory from all models using LOCAL state for consistency
	// Memory is allocated to slots regardless of whether they're currently active
	allocatedMemory := uint64(0)
	var memoryCalculationError error
	s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		// Only count slots on this specific runner
		if slot.RunnerID == runnerID {
			modelName := slot.InitialWork().ModelName().String()
			modelMemory, err := s.getModelMemory(modelName)
			if err != nil {
				memoryCalculationError = fmt.Errorf("failed to get model memory for slot %s with model %s on runner %s: %w", slot.ID.String(), modelName, runnerID, err)
				return false // Stop iteration
			}
			allocatedMemory += modelMemory
		}
		return true
	})

	if memoryCalculationError != nil {
		log.Error().
			Str("runner_id", runnerID).
			Err(memoryCalculationError).
			Msg("CRITICAL: Cannot calculate runner memory due to unknown model memory - this could lead to over-scheduling")
		return 0, 0, 0, memoryCalculationError
	}

	var freeMemory uint64
	if allocatedMemory < totalMemory {
		freeMemory = totalMemory - allocatedMemory
	} else {
		freeMemory = 0 // Over-allocated
	}

	return totalMemory, allocatedMemory, freeMemory, nil
}

// getModelMemory returns the memory requirement for a model from the model registry
func (s *Scheduler) getModelMemory(modelID string) (uint64, error) {
	// try to get from the model registry (the authoritative source)
	models, err := s.controller.store.ListModels(context.Background(), &store.ListModelsQuery{})
	if err != nil {
		log.Error().
			Str("model_id", modelID).
			Err(err).
			Msg("CRITICAL: failed to list models from store - cannot get model memory")
		return 0, fmt.Errorf("failed to list models from store for model %s: %w", modelID, err)
	}

	for _, model := range models {
		if model.ID == modelID {
			if model.Memory == 0 {
				log.Error().
					Str("model_id", modelID).
					Msg("CRITICAL: model has zero memory requirement - invalid configuration")
				return 0, fmt.Errorf("model %s has zero memory requirement - invalid configuration", modelID)
			}
			return model.Memory, nil
		}
	}

	log.Error().
		Str("model_id", modelID).
		Msg("CRITICAL: model not found in model registry - cannot determine memory requirement")
	return 0, fmt.Errorf("model %s not found in model registry", modelID)
}

// getSchedulerSlots returns the scheduler's desired state slots for use in memory calculations
func (s *Scheduler) getSchedulerSlots() map[uuid.UUID]*Slot {
	result := make(map[uuid.UUID]*Slot)
	s.slots.Range(func(key uuid.UUID, value *Slot) bool {
		result[key] = value
		return true
	})
	return result
}

func (s *Scheduler) ensureSlots(req SlotRequirement, count int) {
	// log.Info().
	//	Str("model", req.Model.String()).
	//	Str("runtime", string(req.Runtime)).
	//	Int("count", count).
	//	Msg("SLOT_RECONCILE_DEBUG: Starting ensureSlots")

	for i := 0; i < count; i++ {
		startTime := time.Now()

		// Get all runners sorted by preference
		sortedRunners, err := s.getSortedRunners(req.ExampleWorkload)
		if err != nil {
			// log.Error().Err(err).
			//	Str("model", req.Model.String()).
			//	Msg("SLOT_RECONCILE_DEBUG: Failed to get sorted runners")

			// Log scheduling rejection - no specific runner info available
			s.logSchedulingDecision(req.ExampleWorkload, types.SchedulingDecisionTypeRejected, false,
				fmt.Sprintf("Failed to get runners: %v", err), "", "", startTime, 0, req.ExampleWorkload.model.Memory, 0)

			retry, err := ErrorHandlingStrategy(err, req.ExampleWorkload)
			if retry {
				// log.Info().Err(err).Interface("requirement", req).Msg("SLOT_RECONCILE_DEBUG: failed to get runners for requirement, retrying...")
				return
			}
			// log.Warn().Err(err).Interface("requirement", req).Msg("SLOT_RECONCILE_DEBUG: failed to get runners for requirement, skipping...")
			s.onSchedulingErr(req.ExampleWorkload, err)
			s.queue.Remove(req.ExampleWorkload)
			return
		}

		// log.Info().
		//	Str("model", req.Model.String()).
		//	Strs("sorted_runners", sortedRunners).
		//	Int("runner_count", len(sortedRunners)).
		//	Msg("SLOT_RECONCILE_DEBUG: Got sorted runners for slot creation")

		// Try each runner in order of preference until one succeeds
		var lastErr error
		slotCreated := false

		for j, runnerID := range sortedRunners {
			withWorkContext(&log.Logger, req.ExampleWorkload).Trace().
				Str("runner_id", runnerID).
				Int("attempt", j+1).
				Int("total_runners", len(sortedRunners)).
				Msg("trying runner for slot creation")

			// Try to delete stale slots on this runner if required
			totalMemory, _, freeMemory, err := s.deleteMostStaleStrategy(runnerID, req.ExampleWorkload)
			if err != nil {
				lastErr = err
				withWorkContext(&log.Logger, req.ExampleWorkload).Debug().
					Err(err).
					Str("runner_id", runnerID).
					Msg("failed to free memory on runner, trying next")
				continue // Try next runner
			}

			// Success! Create slot on this runner with GPU allocation from scheduler
			gpuAllocation := s.getGPUAllocation(req.ExampleWorkload.ID(), runnerID)
			slot := NewSlot(runnerID, req.ExampleWorkload, s.modelStaleFunc, s.slotTimeoutFunc, gpuAllocation)
			s.slots.Store(slot.ID, slot)

			// Log successful new slot creation with memory info
			s.logSchedulingDecision(req.ExampleWorkload, types.SchedulingDecisionTypeCreateNewSlot, true,
				fmt.Sprintf("Created new slot on runner %s (attempt %d/%d)", runnerID, j+1, len(sortedRunners)),
				runnerID, slot.ID.String(), startTime, freeMemory, req.ExampleWorkload.model.Memory, totalMemory)
			slotCreated = true
			break // Success, no need to try more runners
		}

		// If we failed on all runners, handle the error
		if !slotCreated {
			// Log scheduling rejection due to all runners failing - no specific runner info available
			s.logSchedulingDecision(req.ExampleWorkload, types.SchedulingDecisionTypeRejected, false,
				fmt.Sprintf("Failed to create slot on any of %d runners, last error: %v", len(sortedRunners), lastErr),
				"", "", startTime, 0, req.ExampleWorkload.model.Memory, 0)

			retry, retryErr := ErrorHandlingStrategy(lastErr, req.ExampleWorkload)
			if retry {
				log.Info().Err(lastErr).Interface("requirement", req).Msg("failed to create slot on any runner, retrying...")
				return
			}
			log.Warn().Err(lastErr).Interface("requirement", req).Msg("failed to create slot on any runner, skipping...")
			if retryErr != nil {
				log.Warn().Err(retryErr).Msg("error handling strategy failed")
			}
			s.onSchedulingErr(req.ExampleWorkload, lastErr)
			s.queue.Remove(req.ExampleWorkload)
			return
		}
	}
}

// TriggerQueueProcessing triggers immediate queue processing without waiting for the next interval.
// This is useful when runners reconnect and queued work might now be schedulable.
func (s *Scheduler) TriggerQueueProcessing() {
	select {
	case s.queueTrigger <- struct{}{}:
		log.Debug().Msg("triggered immediate queue processing")
	default:
		// Channel is full, meaning a trigger is already pending
		log.Debug().Msg("queue processing trigger already pending")
	}
}

func (s *Scheduler) processQueueOnce() {
	// Try to take next work item that has a warm slot available
	work := s.queue.TakeNext(func(w *Workload) bool {
		warmSlots := s.warmSlots(w)
		return len(warmSlots) > 0
	})

	if work == nil {
		// Don't do expensive logging analysis in the hot path - it causes performance issues
		// and prevents actual slot reconciliation from working properly
		return // Nothing can be scheduled right now
	}

	startTime := time.Now()

	// We know we have a warm slot, so schedule the work
	warmSlots := s.warmSlots(work)
	slot := s.pickBestWarmSlot(warmSlots)

	// Calculate memory info for the runner
	totalMemory, _, freeMemory, memErr := s.calculateRunnerMemory(slot.RunnerID)
	if memErr != nil {
		log.Warn().Err(memErr).Str("runner_id", slot.RunnerID).Msg("failed to get runner memory info for logging")
		totalMemory, freeMemory = 0, 0 // Use defaults for logging
	}

	err := s.allocateSlot(slot.ID, work)
	if err != nil {
		// Log failed allocation decision
		s.logSchedulingDecision(work, types.SchedulingDecisionTypeError, false,
			fmt.Sprintf("Failed to allocate warm slot: %v", err), slot.RunnerID, slot.ID.String(), startTime, freeMemory, work.model.Memory, totalMemory)

		// If allocation fails, put work back in queue
		err = s.queue.Add(work)
		if err != nil {
			log.Warn().Err(err).Msg("failed to add work back to queue")
			s.onSchedulingErr(work, err)
		}
	} else {
		// Log successful warm slot reuse
		s.logSchedulingDecision(work, types.SchedulingDecisionTypeReuseWarmSlot, true,
			fmt.Sprintf("Reused warm slot on runner %s", slot.RunnerID), slot.RunnerID, slot.ID.String(), startTime, freeMemory, work.model.Memory, totalMemory)
	}
}

// Helper method to get all runners sorted by preference
func (s *Scheduler) getSortedRunners(work *Workload) ([]string, error) {
	// First get a list of all runners
	allRunners := s.controller.RunnerIDs()

	filteredRunners, err := s.filterRunners(work, allRunners)
	if err != nil {
		return nil, err
	}

	// Error if there are no runners left
	if len(filteredRunners) == 0 {
		return nil, ErrNoRunnersAvailable
	}

	// Check if workload has a preferred runner and if it's available
	preferredRunner := work.PreferredRunnerID()
	if preferredRunner != "" {
		// Check if preferred runner is in the filtered list
		for _, runnerID := range filteredRunners {
			if runnerID == preferredRunner {
				withWorkContext(&log.Logger, work).Debug().
					Str("preferred_runner", preferredRunner).
					Msg("using preferred runner for workload")
				// Return preferred runner first, followed by others
				result := []string{preferredRunner}
				for _, id := range filteredRunners {
					if id != preferredRunner {
						result = append(result, id)
					}
				}
				return result, nil
			}
		}
		withWorkContext(&log.Logger, work).Debug().
			Str("preferred_runner", preferredRunner).
			Msg("preferred runner not available, falling back to normal scheduling")
	}

	// Calculate the scheduled load on each runner according to their slots
	// Note: We discussed using real free memory by pinging the runner, but decided to use the
	// control-plane's view of free memory to avoid the overhead of pinging the runner. This also
	// has the added benefit of being able to over-commit memory slightly.
	runnerLoad := make(map[string]uint64)
	for _, runnerID := range filteredRunners {
		// Sum up all scheduled slots on the runner
		s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
			if slot.RunnerID == runnerID {
				runnerLoad[runnerID] += slot.Memory()
			}
			return true
		})
	}
	withWorkContext(&log.Logger, work).Trace().Interface("runner_load", runnerLoad).Msg("runner load")

	// Sort the runners by load, increasing, with a random shuffle for ties
	slices.SortFunc(filteredRunners, func(a, b string) int {
		if runnerLoad[a] != runnerLoad[b] {
			return int(runnerLoad[a] - runnerLoad[b])
		}
		return rand.Intn(3) - 1 // Introduces random shuffle for true ties
	})
	withWorkContext(&log.Logger, work).Trace().Interface("sorted_runners", filteredRunners).Msg("sorted runners")

	return filteredRunners, nil
}

// DeleteMostStaleStrategy iteratively deletes allocated work from stale slots until there is enough
// memory to allocate the new workload. Returns the final memory state used for the decision.
func (s *Scheduler) deleteMostStaleStrategy(runnerID string, work *Workload) (totalMemory, allocatedMemory, freeMemory uint64, err error) {
	// CRITICAL FIX: Use the same memory calculation as calculateRunnerMemory to avoid overscheduling
	// This properly accounts for both scheduler slots AND existing runner memory usage
	var finalAllocatedMem uint64
	var finalFreeMem uint64
	var totalMem, currentAllocatedMem, currentFreeMem uint64

	for {
		// Recalculate memory state after each potential slot deletion
		totalMem, currentAllocatedMem, currentFreeMem, err = s.calculateRunnerMemory(runnerID)
		if err != nil {
			return 0, 0, 0, err
		}

		var allSlots []*Slot
		s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
			if slot.RunnerID == runnerID {
				allSlots = append(allSlots, slot)
			}
			return true
		})

		requiredMem := work.model.Memory
		freeMem := int64(currentFreeMem) - int64(requiredMem)
		log.Trace().Interface("slots", allSlots).
			Uint64("current_allocated_mem", currentAllocatedMem).
			Uint64("current_free_mem", currentFreeMem).
			Uint64("required_mem", requiredMem).
			Int64("free_after_allocation", freeMem).
			Msg("checking if we can allocate using calculateRunnerMemory")

		// Store the final memory state used for the decision
		finalAllocatedMem = currentAllocatedMem
		finalFreeMem = currentFreeMem

		// If there is enough free space on the runner, break out of the loop.
		if freeMem > 0 {
			break
		}

		// Only keep slots that are not the same as the required workload
		// Since there's no point deleting slots that are already the same as the required workload
		notSameWorkload := Filter(allSlots, func(slot *Slot) bool {
			return slot.InitialWork().ModelName() != work.ModelName() ||
				slot.InitialWork().Runtime() != work.Runtime() ||
				slot.InitialWork().LoraDir() != work.LoraDir()
		})

		// Only keep the stale slots
		staleSlots := Filter(notSameWorkload, func(slot *Slot) bool {
			return slot.IsStale()
		})

		// Sort the slots by last activity time
		slices.SortFunc(staleSlots, func(i, j *Slot) int {
			return int(i.LastActivityTime.Sub(j.LastActivityTime))
		})
		log.Trace().Interface("stale_slots", staleSlots).Msg("stale slots")
		if len(staleSlots) == 0 {
			return totalMem, finalAllocatedMem, finalFreeMem, ErrRunnersAreFull
		}
		// Then delete the most stale slot, allow the reconciler to mop up
		evictedSlot := staleSlots[0]
		withSlotContext(&log.Logger, evictedSlot).Info().
			Str("reason", "memory_pressure").
			Uint64("required_memory_mb", requiredMem/1024/1024).
			Uint64("total_memory_mb", totalMem/1024/1024).
			Uint64("allocated_memory_mb", finalAllocatedMem/1024/1024).
			Int64("free_memory_mb", freeMem/1024/1024).
			Dur("slot_age", time.Since(evictedSlot.LastActivityTime)).
			Int("stale_slots_available", len(staleSlots)).
			Msg("evicting stale slot due to memory pressure")

		// Log as scheduling decision for the dashboard
		s.logSchedulingDecision(work, types.SchedulingDecisionTypeEvictStaleSlot, true,
			fmt.Sprintf("Evicted stale slot %s (model: %s, age: %v) to free memory for new workload",
				evictedSlot.ID.String(), evictedSlot.InitialWork().ModelName(), time.Since(evictedSlot.LastActivityTime)),
			runnerID, evictedSlot.ID.String(), time.Now(), finalFreeMem/1024/1024, work.model.Memory/1024/1024, totalMem/1024/1024)

		s.slots.Delete(evictedSlot.ID)

		// Clean up GPU allocation for evicted workload
		s.clearGPUAllocation(evictedSlot.InitialWork().ID())
	}
	return totalMem, finalAllocatedMem, finalFreeMem, nil
}

func (s *Scheduler) warmSlots(req *Workload) []*Slot {
	return s.warmSlotsWithReason(req, nil)
}

func (s *Scheduler) warmSlotsWithReason(req *Workload, reasonOut *string) []*Slot {
	cosyWarm := make([]*Slot, 0, s.slots.Size())

	// Track counts for detailed rejection reasons
	totalSlots := 0
	modelMatchSlots := 0
	runtimeMatchSlots := 0
	loraMatchSlots := 0
	runningSlots := 0
	availableSlots := 0

	s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		totalSlots++

		// If it's not the same model name, skip
		if slot.InitialWork().ModelName() != req.ModelName() {
			withSlotContext(&log.Logger, slot).Trace().Msg("skipping warm slot, model name mismatch")
			return true
		}
		modelMatchSlots++

		// If it's not the same runtime, skip
		if slot.InitialWork().Runtime() != req.Runtime() {
			withSlotContext(&log.Logger, slot).Trace().Msg("skipping warm slot, inference runtime mismatch")
			return true
		}
		runtimeMatchSlots++

		// If it doesn't have the right LoraDir then skip
		if slot.InitialWork().LoraDir() != req.LoraDir() {
			withSlotContext(&log.Logger, slot).Trace().Str("slot_lora_dir", slot.InitialWork().LoraDir()).Str("req_lora_dir", req.LoraDir()).Msg("skipping warm slot, LoraDir mismatch")
			return true
		}
		loraMatchSlots++

		// If the slot isn't running yet, skip
		if !slot.IsRunning() {
			withSlotContext(&log.Logger, slot).Trace().Msg("skipping warm slot, not running yet")
			return true
		}
		runningSlots++

		// If the slot is already running another job, skip
		if slot.IsActive() {
			withSlotContext(&log.Logger, slot).Trace().Msg("skipping warm slot, already active")
			return true
		}
		availableSlots++

		// Add available slots to the list - this includes both regular and prewarming slots
		cosyWarm = append(cosyWarm, slot)

		return true
	})

	// Generate detailed reason if requested and no warm slots found
	if reasonOut != nil && len(cosyWarm) == 0 {
		availableRunners := s.controller.RunnerIDs()

		if len(availableRunners) == 0 {
			*reasonOut = "No runners available"
		} else if modelMatchSlots == 0 {
			*reasonOut = fmt.Sprintf("No slots for model %s (found %d total slots)", req.ModelName(), totalSlots)
		} else if runtimeMatchSlots == 0 {
			*reasonOut = fmt.Sprintf("No slots for runtime %s with model %s (found %d model slots)", req.Runtime(), req.ModelName(), modelMatchSlots)
		} else if loraMatchSlots == 0 {
			loraMsg := "no LoRA"
			if req.LoraDir() != "" {
				loraMsg = fmt.Sprintf("LoRA %s", req.LoraDir())
			}
			*reasonOut = fmt.Sprintf("No slots for %s with model %s (found %d runtime slots)", loraMsg, req.ModelName(), runtimeMatchSlots)
		} else if runningSlots == 0 {
			*reasonOut = fmt.Sprintf("All %d matching slots still starting for model %s", loraMatchSlots, req.ModelName())
		} else if availableSlots == 0 {
			*reasonOut = fmt.Sprintf("All %d running slots busy for model %s", runningSlots, req.ModelName())
		} else {
			*reasonOut = fmt.Sprintf("No warm slots available for model %s (%d total, %d running, %d available)",
				req.ModelName(), totalSlots, runningSlots, availableSlots)
		}
	}

	return cosyWarm
}

func (s *Scheduler) pickBestWarmSlot(warmSlots []*Slot) *Slot {
	// If we only have one slot, return it
	if len(warmSlots) == 1 {
		return warmSlots[0]
	}

	// Group slots by runner
	runnerSlots := make(map[string][]*Slot)
	for _, slot := range warmSlots {
		runnerSlots[slot.RunnerID] = append(runnerSlots[slot.RunnerID], slot)
	}

	// Count active slots per runner
	activeSlots := make(map[string]int)
	s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		if slot.IsActive() {
			activeSlots[slot.RunnerID]++
		}
		return true
	})

	// Sort slots considering:
	// 1. Runner load (prefer less loaded runners)
	// 2. Last activity time (prefer more recently used slots)
	// 3. Random factor for tie-breaking
	slices.SortFunc(warmSlots, func(i, j *Slot) int {
		// First compare runner load
		if activeSlots[i.RunnerID] != activeSlots[j.RunnerID] {
			return activeSlots[i.RunnerID] - activeSlots[j.RunnerID]
		}

		// Then prefer more recently used slots (reverse of current order)
		if !i.LastActivityTime.Equal(j.LastActivityTime) {
			return int(j.LastActivityTime.Sub(i.LastActivityTime))
		}

		// Random tie-breaker
		return rand.Intn(3) - 1
	})

	return warmSlots[0]
}

// AllocateSlot assigns a workload to a specific slot, validating the model and slot before scheduling.
func (s *Scheduler) allocateSlot(slotID uuid.UUID, req *Workload) error {
	// Validate slot
	slot, ok := s.slots.Load(slotID)
	if !ok {
		return fmt.Errorf("slot not found: %s", slot.ID.String())
	}

	// Ensure the slot is not already scheduled or active.
	if slot.IsActive() {
		return fmt.Errorf("slot already active: %s", slot.ID.String())
	}

	// Marks the slot as locally active. This is reset in the reconciliation process.
	withSlotAndWorkContext(&log.Logger, slot, req).Trace().Msg("starting slot")
	slot.Start()

	// Can do the rest in a goroutine, no need to wait for it to submit
	go func() {
		// Submit the work to the slot
		switch req.WorkloadType {
		case WorkloadTypeLLMInferenceRequest:
			llmReq := req.LLMInferenceRequest()
			if llmReq.Embeddings {
				withSlotAndWorkContext(&log.Logger, slot, req).Trace().Msg("submitting embedding request")
				err := s.controller.RunnerClient().SubmitEmbeddingRequest(slot, llmReq)
				if err != nil {
					s.onSchedulingErr(req, err)
					withSlotAndWorkContext(&log.Logger, slot, req).Warn().Err(err).Msg("error submitting embedding request")
				}
			} else {
				withSlotAndWorkContext(&log.Logger, slot, req).Trace().Msg("submitting chat completion request")
				err := s.controller.RunnerClient().SubmitChatCompletionRequest(slot, llmReq)
				if err != nil {
					s.onSchedulingErr(req, err)
					withSlotAndWorkContext(&log.Logger, slot, req).Warn().Err(err).Msg("error submitting chat completion request")
				}
			}
		case WorkloadTypeSession:
			switch req.Session().Mode {
			case types.SessionModeInference:
				switch req.Session().Type {
				case types.SessionTypeImage:
					withSlotAndWorkContext(&log.Logger, slot, req).Trace().Msg("submitting text2image request")
					err := s.controller.RunnerClient().SubmitImageGenerationRequest(slot, req.Session())
					if err != nil {
						s.onSchedulingErr(req, err)
						withSlotAndWorkContext(&log.Logger, slot, req).Warn().Err(err).Msg("error submitting text2image request")
					}
				case types.SessionTypeText:
					if req.Session().LoraDir != "" {
						withSlotAndWorkContext(&log.Logger, slot, req).Trace().Msg("submitting LORA text inference request")
						// Overwrite the request model name with the helix lora model details
						convertedRequest := req.ToLLMInferenceRequest()
						convertedRequest.Request.Model = req.Session().LoraDir

						// Forward the request to the chat completion handler
						err := s.controller.RunnerClient().SubmitChatCompletionRequest(slot, convertedRequest)
						if err != nil {
							s.onSchedulingErr(req, err)
							withSlotAndWorkContext(&log.Logger, slot, req).Warn().Err(err).Msg("error submitting LORA inference request")
						}
					} else {
						s.onSchedulingErr(req, fmt.Errorf("not implemented: %s and no lora dir", req.Session().Type))
						withSlotAndWorkContext(&log.Logger, slot, req).Warn().Msg("not implemented: no lora dir")
					}
				default:
					s.onSchedulingErr(req, fmt.Errorf("not implemented: %s", req.Session().Type))
					withSlotAndWorkContext(&log.Logger, slot, req).Warn().Msg("not implemented: session type")
				}
			default:
				s.onSchedulingErr(req, fmt.Errorf("not implemented: %s", req.Session().Mode))
				withSlotAndWorkContext(&log.Logger, slot, req).Warn().Msg("not implemented: session mode")
			}
		}

		withSlotAndWorkContext(&log.Logger, slot, req).Trace().Msg("finished submitting request")
	}()

	return nil
}

// createNewSlot creates a new slot for the given runner and workload.
func (s *Scheduler) createNewSlot(ctx context.Context, slot *Slot) error {
	withSlotContext(&log.Logger, slot).Info().
		Str("SLOT_CREATION", "starting").
		Msg("SLOT_CREATION: Starting createNewSlot")

	defer func() {
		withSlotContext(&log.Logger, slot).Info().
			Str("SLOT_CREATION", "finished").
			Msg("SLOT_CREATION: Finished createNewSlot")
	}()

	s.updateHeartbeatStatus("reconcileSlots", fmt.Sprintf("creating slot %s on runner %s", slot.ID.String()[:8], slot.RunnerID))

	s.updateHeartbeatStatus("reconcileSlots", fmt.Sprintf("calling CreateSlot for slot %s on runner %s", slot.ID.String()[:8], slot.RunnerID))
	err := s.controller.CreateSlot(slot)
	if err != nil {
		s.updateHeartbeatStatus("reconcileSlots", fmt.Sprintf("CreateSlot failed for slot %s: %v", slot.ID.String()[:8], err))
		return err
	}
	s.updateHeartbeatStatus("reconcileSlots", fmt.Sprintf("CreateSlot succeeded for slot %s, starting wait loop", slot.ID.String()[:8]))

	// Wait for the slot to be ready
	slotReady := make(chan bool)

	// Add timeout variables for logging
	startTime := time.Now()
	readyTimeout := 120 * time.Minute
	lastLogTime := startTime
	attemptCount := 0

	s.updateHeartbeatStatus("reconcileSlots", fmt.Sprintf("waiting for slot %s to be ready (model: %s)", slot.ID.String()[:8], slot.InitialWork().ModelName().String()))

	withSlotContext(&log.Logger, slot).Info().
		Dur("timeout", readyTimeout).
		Str("model", slot.InitialWork().ModelName().String()).
		Msg("Starting wait for slot to be ready with timeout")

	go func() {
		for {
			select {
			case <-ctx.Done():
				withSlotContext(&log.Logger, slot).Warn().
					Dur("elapsed", time.Since(startTime)).
					Msg("Context canceled while waiting for slot to be ready")
				return
			case <-time.After(500 * time.Millisecond):
				attemptCount++
				elapsed := time.Since(startTime)

				// Log status every 10 seconds and update heartbeat status
				if time.Since(lastLogTime) > 10*time.Second {
					timeLeft := readyTimeout - elapsed
					s.updateHeartbeatStatus("reconcileSlots", fmt.Sprintf("waiting for slot %s (%s elapsed, %d attempts)", slot.ID.String()[:8], elapsed.Round(time.Second), attemptCount))
					withSlotContext(&log.Logger, slot).Debug().
						Dur("elapsed", elapsed).
						Dur("time_left", timeLeft).
						Int("attempt", attemptCount).
						Msg("Waiting for slot to be ready")
					lastLogTime = time.Now()
				}

				// First check if the runner is still connected - this is the key fix!
				// If runner is gone, we should exit immediately instead of waiting 2 hours
				runnerIDs := s.controller.RunnerIDs()
				runnerConnected := slices.Contains(runnerIDs, slot.RunnerID)

				withSlotContext(&log.Logger, slot).Debug().
					Str("SLOT_WAIT_CHECK", "runner_connection_check").
					Strs("connected_runners", runnerIDs).
					Str("target_runner", slot.RunnerID).
					Bool("runner_connected", runnerConnected).
					Dur("elapsed", elapsed).
					Msg("SLOT_WAIT_CHECK: Checking if runner is still connected")

				if !runnerConnected {
					withSlotContext(&log.Logger, slot).Warn().
						Str("SLOT_WAIT_CHECK", "runner_disconnected").
						Dur("elapsed", elapsed).
						Msg("SLOT_WAIT_CHECK: Runner is no longer connected while waiting for slot - aborting slot creation")
					close(slotReady)
					return
				}

				// Runner is healthy, now check if slot is ready
				withSlotContext(&log.Logger, slot).Debug().
					Str("SLOT_WAIT_CHECK", "fetching_slot_status").
					Str("runner_id", slot.RunnerID).
					Str("slot_id", slot.ID.String()).
					Dur("elapsed", elapsed).
					Msg("SLOT_WAIT_CHECK: About to fetch slot status from runner")

				if s, err := s.controller.fetchSlot(slot.RunnerID, slot.ID); err == nil {
					withSlotContext(&log.Logger, slot).Debug().
						Str("SLOT_WAIT_CHECK", "slot_status_received").
						Bool("slot_ready", s.Ready).
						Bool("slot_active", s.Active).
						Dur("elapsed", elapsed).
						Msg("SLOT_WAIT_CHECK: Received slot status from runner")

					if s.Ready {
						withSlotContext(&log.Logger, slot).Info().
							Str("SLOT_WAIT_CHECK", "slot_ready_success").
							Dur("elapsed", elapsed).
							Int("attempts", attemptCount).
							Msg("SLOT_WAIT_CHECK: Slot is now ready")
						slotReady <- true
					}
				} else {
					withSlotContext(&log.Logger, slot).Warn().
						Str("SLOT_WAIT_CHECK", "fetch_slot_error").
						Err(err).
						Dur("elapsed", elapsed).
						Msg("SLOT_WAIT_CHECK: Error checking if slot is ready - closing wait loop")
					close(slotReady)
					return
				}
			}
		}
	}()

	select {
	case <-slotReady:
		withSlotContext(&log.Logger, slot).Info().
			Dur("elapsed", time.Since(startTime)).
			Msg("Slot is ready")
	case <-time.After(readyTimeout):
		withSlotContext(&log.Logger, slot).Error().
			Dur("elapsed", time.Since(startTime)).
			Int("attempts", attemptCount).
			Msg("Timeout waiting for slot to be ready")
		return fmt.Errorf("slot not ready after 120 minutes")
	}

	// Mark the slot as running
	slot.SetRunning()
	withSlotContext(&log.Logger, slot).Info().Msg("slot created on runner")

	return nil
}

// AddWorkFields adds standard work-related fields to a log event
func withWorkContext(l *zerolog.Logger, w *Workload) *zerolog.Logger {
	nextLogger := l.With().
		Str("work_id", w.ID()).
		Str("model_name", w.ModelName().String()).
		Str("mode", string(w.Mode())).
		Str("runtime", string(w.Runtime())).
		Str("lora_dir", w.LoraDir()).
		Dur("duration_ms", time.Since(w.Created())).
		Logger()
	return &nextLogger
}

// AddSlotFields adds standard slot-related fields to a log event
func withSlotContext(l *zerolog.Logger, s *Slot) *zerolog.Logger {
	nextLogger := l.With().
		Str("runner_id", s.RunnerID).
		Str("slot_id", s.ID.String()).
		Str("model_name", s.InitialWork().ModelName().String()).
		Uint64("memory", s.Memory()).
		Logger()
	return &nextLogger
}

func withSlotAndWorkContext(l *zerolog.Logger, s *Slot, w *Workload) *zerolog.Logger {
	return withSlotContext(withWorkContext(l, w), s)
}

// GetSchedulingDecisions returns recent scheduling decisions for the dashboard
func (s *Scheduler) GetSchedulingDecisions(limit int) []*types.SchedulingDecision {
	return s.decisionsTracker.GetRecentDecisions(limit)
}

// logSchedulingDecision logs a scheduling decision with timing information and memory details
func (s *Scheduler) logSchedulingDecision(workload *Workload, decisionType types.SchedulingDecisionType, success bool, reason string, runnerID, slotID string, startTime time.Time, freeMemory uint64, modelMemory uint64, totalMemory uint64) {
	processingTime := time.Since(startTime).Milliseconds()

	// Get available runners for context
	availableRunners := s.controller.RunnerIDs()

	// Count warm slots for this workload
	warmSlots := s.warmSlots(workload)
	totalSlots := 0
	s.slots.Range(func(_ uuid.UUID, _ *Slot) bool {
		totalSlots++
		return true
	})

	// Get session ID based on workload type
	sessionID := ""
	switch workload.WorkloadType {
	case WorkloadTypeLLMInferenceRequest:
		sessionID = workload.LLMInferenceRequest().SessionID
	case WorkloadTypeSession:
		sessionID = workload.Session().ID
	}

	// Enhance reason with memory information
	memoryInfo := fmt.Sprintf("(free: %dMB, model: %dMB, total: %dMB)",
		freeMemory/(1024*1024), modelMemory/(1024*1024), totalMemory/(1024*1024))
	enhancedReason := fmt.Sprintf("%s %s", reason, memoryInfo)

	// Get memory required, handling case where workload.model might be nil
	var memoryRequired uint64
	if workload.model != nil {
		memoryRequired = workload.model.Memory
	}

	decision := &types.SchedulingDecision{
		WorkloadID:       workload.ID(),
		SessionID:        sessionID,
		ModelName:        string(workload.ModelName()),
		Mode:             workload.Mode(),
		DecisionType:     decisionType,
		RunnerID:         runnerID,
		SlotID:           slotID,
		Reason:           enhancedReason,
		Success:          success,
		ProcessingTimeMs: processingTime,
		AvailableRunners: availableRunners,
		MemoryRequired:   memoryRequired,
		WarmSlotCount:    len(warmSlots),
		TotalSlotCount:   totalSlots,
	}

	s.decisionsTracker.LogDecision(decision)

	// Log with structured logging for debugging
	log.Debug().
		Str("workload_id", workload.ID()).
		Str("session_id", sessionID).
		Str("model_name", string(workload.ModelName())).
		Str("decision_type", string(decisionType)).
		Bool("success", success).
		Str("reason", reason).
		Str("runner_id", runnerID).
		Str("slot_id", slotID).
		Int64("processing_time_ms", processingTime).
		Int("warm_slot_count", len(warmSlots)).
		Int("total_slot_count", totalSlots).
		Msg("Scheduling decision logged")
}

// PrewarmNewRunner creates prewarming workloads for newly connected or reconnected runners
func (s *Scheduler) PrewarmNewRunner(runnerID string) {
	// Safety checks to prevent panics
	if s == nil {
		log.Error().Str("runner_id", runnerID).Msg("scheduler is nil in PrewarmNewRunner")
		return
	}
	if s.controller == nil {
		log.Error().Str("runner_id", runnerID).Msg("scheduler controller is nil in PrewarmNewRunner")
		return
	}
	if runnerID == "" {
		log.Error().Msg("empty runner ID in PrewarmNewRunner")
		return
	}

	withContext := log.With().Str("runner_id", runnerID).Logger()
	withContext.Info().Msg("prewarming runner")

	// Get models that should be prewarmed on this runner
	prewarmModels, err := s.getPrewarmModelsSafely(runnerID)
	if err != nil {
		withContext.Error().Err(err).Msg("failed to get prewarm models")
		return
	}
	if len(prewarmModels) == 0 {
		withContext.Warn().Msg("no prewarm models configured or selected, skipping prewarming")
		return
	}

	withContext.Info().
		Int("model_count", len(prewarmModels)).
		Msg("starting prewarming for runner")

	successCount := 0
	for _, model := range prewarmModels {
		// Safety check: skip nil models or models with empty IDs
		if model == nil || model.ID == "" {
			withContext.Warn().
				Interface("model", model).
				Msg("skipping invalid model in prewarming - model is nil or has empty ID")
			continue
		}

		// Create a dummy workload for prewarming
		prewarmWorkload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: fmt.Sprintf("prewarm-%s-%s", runnerID, model.ID),
				CreatedAt: time.Now(),
				Priority:  false, // Low priority for prewarming
				Request: &openai.ChatCompletionRequest{
					Model: model.ID,
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "warmup"},
					},
				},
			},
			model: model,
		}

		// Set the preferred runner for this prewarming workload
		prewarmWorkload.SetPreferredRunner(runnerID)

		// Enqueue the prewarming workload
		err := s.Enqueue(prewarmWorkload)
		if err != nil {
			withContext.Warn().
				Err(err).
				Str("model", model.ID).
				Msg("failed to enqueue prewarming workload")
		} else {
			withContext.Debug().
				Str("model", model.ID).
				Msg("enqueued prewarming workload")
			successCount++
		}
	}

	withContext.Info().
		Int("total_models", len(prewarmModels)).
		Int("successful_enqueues", successCount).
		Msg("completed prewarming for runner")

	// Trigger immediate queue processing to handle any existing queued work
	// that might now be schedulable on this reconnected runner
	s.TriggerQueueProcessing()
}

// getPrewarmModelsSafely is a safe wrapper around getPrewarmModels that handles panics
func (s *Scheduler) getPrewarmModelsSafely(runnerID string) ([]*types.Model, error) {
	var models []*types.Model
	var err error

	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().
					Str("runner_id", runnerID).
					Interface("panic", r).
					Msg("panic recovered in getPrewarmModels")
				err = fmt.Errorf("panic in getPrewarmModels: %v", r)
			}
		}()
		models = s.getPrewarmModels(runnerID)
	}()

	return models, err
}

// getPrewarmModels returns models that should be prewarmed on the specified runner
// Uses memory-aware selection to maximize GPU utilization while improving distribution
func (s *Scheduler) getPrewarmModels(runnerID string) []*types.Model {
	// Get all models with Prewarm=true from configured models
	allPrewarmModels := s.getAllConfiguredPrewarmModels()
	if len(allPrewarmModels) == 0 {
		return nil
	}

	// Get available memory on the target runner
	totalMemory, allocatedMemory, freeMemory, err := s.calculateRunnerMemory(runnerID)
	if err != nil {
		log.Warn().Err(err).Str("runner_id", runnerID).Msg("failed to get runner memory for prewarming")
		return nil
	}

	log.Debug().
		Str("runner_id", runnerID).
		Uint64("total_memory", totalMemory).
		Uint64("allocated_memory", allocatedMemory).
		Uint64("free_memory", freeMemory).
		Msg("calculated runner memory for prewarming")

	// Filter models that can fit in available memory
	modelsWithMemory := s.selectModelsByMemory(allPrewarmModels, freeMemory)
	if len(modelsWithMemory) == 0 {
		log.Debug().
			Str("runner_id", runnerID).
			Uint64("free_memory", freeMemory).
			Msg("no prewarm models can fit in available memory")
		return nil
	}

	// Analyze current global distribution of these models
	modelCounts := s.analyzeGlobalModelDistribution(modelsWithMemory)

	// Select models for prewarming: fill memory while improving distribution
	selectedModels := s.selectModelsForMemoryAndDistribution(modelsWithMemory, modelCounts, freeMemory)

	log.Debug().
		Str("runner_id", runnerID).
		Int("prewarm_model_count", len(selectedModels)).
		Int("total_available", len(allPrewarmModels)).
		Int("memory_compatible", len(modelsWithMemory)).
		Uint64("free_memory", freeMemory).
		Interface("model_distribution", modelCounts).
		Msg("selected models for memory-aware prewarming")

	return selectedModels
}

// getAllConfiguredPrewarmModels gets all models with Prewarm=true from configuration
func (s *Scheduler) getAllConfiguredPrewarmModels() []*types.Model {
	var prewarmModels []*types.Model

	// Check VLLM models
	vllmModels, err := model.GetDefaultVLLMModels()
	if err != nil {
		log.Warn().Err(err).Msg("failed to get VLLM models for prewarming")
	} else {
		for _, m := range vllmModels {
			if m.Prewarm {
				prewarmModels = append(prewarmModels, &types.Model{
					ID:      m.ID,
					Memory:  m.Memory,
					Runtime: types.RuntimeVLLM,
					Prewarm: m.Prewarm,
				})
			}
		}
	}

	// Check Ollama models
	ollamaModels, err := model.GetDefaultOllamaModels()
	if err != nil {
		log.Warn().Err(err).Msg("failed to get Ollama models for prewarming")
	} else {
		for _, m := range ollamaModels {
			if m.Prewarm {
				prewarmModels = append(prewarmModels, &types.Model{
					ID:      m.ID,
					Memory:  m.Memory,
					Runtime: types.RuntimeOllama,
					Prewarm: m.Prewarm,
				})
			}
		}
	}

	// Check Diffusers models
	diffusersModels, err := model.GetDefaultDiffusersModels()
	if err != nil {
		log.Warn().Err(err).Msg("failed to get Diffusers models for prewarming")
	} else {
		for _, m := range diffusersModels {
			if m.Prewarm {
				prewarmModels = append(prewarmModels, &types.Model{
					ID:      m.ID,
					Memory:  m.Memory,
					Runtime: types.RuntimeDiffusers,
					Prewarm: m.Prewarm,
				})
			}
		}
	}

	log.Debug().
		Int("total_prewarm_models", len(prewarmModels)).
		Msg("loaded prewarm models from configuration")

	return prewarmModels
}

// analyzeGlobalModelDistribution counts how many instances of each model are currently running
func (s *Scheduler) analyzeGlobalModelDistribution(prewarmModels []*types.Model) map[string]int {
	modelCounts := make(map[string]int)

	// Initialize counts for all prewarm models
	for _, model := range prewarmModels {
		modelCounts[model.ID] = 0
	}

	// Count running instances across all runners
	runners := s.controller.RunnerIDs()
	availableRunners := 0

	// Count running instances using local state for consistency with scheduling decisions
	// Include both active and starting slots since they represent allocated models
	s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		// Only count runners that are in our runner list
		if slices.Contains(runners, slot.RunnerID) {
			modelName := slot.InitialWork().ModelName().String()
			if _, isPrewarmModel := modelCounts[modelName]; isPrewarmModel {
				modelCounts[modelName]++
			}
		}
		return true
	})

	// Count available runners from those that have slots
	availableRunnersSet := make(map[string]bool)
	s.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		if slices.Contains(runners, slot.RunnerID) {
			availableRunnersSet[slot.RunnerID] = true
		}
		return true
	})
	availableRunners = len(availableRunnersSet)

	log.Debug().
		Int("total_runners", len(runners)).
		Int("available_runners", availableRunners).
		Interface("model_counts", modelCounts).
		Msg("analyzed global model distribution for prewarming")

	return modelCounts
}

// selectModelsForMemoryAndDistribution chooses models to prewarm based on available memory and distribution
// Primary goal: Fill available GPU memory. Secondary goal: Improve global distribution.
func (s *Scheduler) selectModelsForMemoryAndDistribution(prewarmModels []*types.Model, modelCounts map[string]int, freeMemory uint64) []*types.Model {
	if len(prewarmModels) == 0 {
		return nil
	}

	// Calculate total memory needed for all compatible models
	totalMemoryNeeded := uint64(0)
	for _, model := range prewarmModels {
		totalMemoryNeeded += model.Memory
	}

	// If all models can fit, prewarm them all (prioritize memory utilization)
	if totalMemoryNeeded <= freeMemory {
		log.Debug().
			Int("model_count", len(prewarmModels)).
			Uint64("total_memory_needed", totalMemoryNeeded).
			Uint64("free_memory", freeMemory).
			Msg("all compatible models fit in memory, prewarming all")
		return prewarmModels
	}

	// Not all models can fit - use greedy selection prioritizing distribution
	log.Debug().
		Int("model_count", len(prewarmModels)).
		Uint64("total_memory_needed", totalMemoryNeeded).
		Uint64("free_memory", freeMemory).
		Msg("not all models fit, using greedy selection based on distribution")

	// Sort models by current count (ascending) to prioritize least-running models
	type modelWithCount struct {
		model *types.Model
		count int
	}

	modelsWithCounts := make([]modelWithCount, 0, len(prewarmModels))
	for _, model := range prewarmModels {
		modelsWithCounts = append(modelsWithCounts, modelWithCount{
			model: model,
			count: modelCounts[model.ID],
		})
	}

	// Sort by count (ascending), then by memory (ascending) for tie-breaking
	sort.Slice(modelsWithCounts, func(i, j int) bool {
		if modelsWithCounts[i].count != modelsWithCounts[j].count {
			return modelsWithCounts[i].count < modelsWithCounts[j].count
		}
		// If counts are equal, prefer smaller models (can fit more)
		return modelsWithCounts[i].model.Memory < modelsWithCounts[j].model.Memory
	})

	// Greedy selection: pack models into available memory, prioritizing better distribution
	selectedModels := []*types.Model{}
	remainingMemory := freeMemory

	for _, mwc := range modelsWithCounts {
		if mwc.model.Memory <= remainingMemory {
			selectedModels = append(selectedModels, mwc.model)
			remainingMemory -= mwc.model.Memory

			log.Debug().
				Str("model_id", mwc.model.ID).
				Int("current_count", mwc.count).
				Uint64("model_memory", mwc.model.Memory).
				Uint64("remaining_memory", remainingMemory).
				Msg("selected model for memory-aware prewarming")
		} else {
			log.Debug().
				Str("model_id", mwc.model.ID).
				Uint64("model_memory", mwc.model.Memory).
				Uint64("remaining_memory", remainingMemory).
				Msg("model too large for remaining memory, skipping")
		}
	}

	log.Debug().
		Int("selected_count", len(selectedModels)).
		Int("total_candidates", len(prewarmModels)).
		Uint64("memory_used", freeMemory-remainingMemory).
		Uint64("memory_available", freeMemory).
		Float64("memory_utilization", float64(freeMemory-remainingMemory)/float64(freeMemory)*100).
		Msg("completed memory-aware model selection")

	return selectedModels
}

// selectModelsByMemory filters models that can fit in the available memory
func (s *Scheduler) selectModelsByMemory(prewarmModels []*types.Model, freeMemory uint64) []*types.Model {
	filteredModels := []*types.Model{}
	for _, model := range prewarmModels {
		if model.Memory <= freeMemory {
			filteredModels = append(filteredModels, model)
		}
	}
	return filteredModels
}
