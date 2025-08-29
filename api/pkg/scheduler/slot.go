package scheduler

import (
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/memory"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type Slot struct {
	ID               uuid.UUID // An ID representing this unique model on a runner
	RunnerID         string    // The runner that this slot is assigned to
	initialWork      *Workload // The work that is currently assigned to this slot
	LastActivityTime time.Time // Private because I don't want people misinterpreting this
	Created          time.Time // When the slot was first created
	activeRequests   int64     // Number of concurrent active requests (atomic)
	maxConcurrency   int64     // Maximum concurrent requests allowed (atomic)
	isStaleFunc      TimeoutFunc
	isErrorFunc      TimeoutFunc
	isRunning        bool

	// GPU allocation from scheduler - authoritative allocation decision
	GPUAllocation *GPUAllocation
}

// NewSlot creates a new slot with the given runnerID and work
// staleTimeout is a function that determines if a slot is stale
// errorTimeout is a function that determines if a slot has errored
func NewSlot(runnerID string, work *Workload, staleTimeout TimeoutFunc, errorTimeout TimeoutFunc, gpuAllocation *GPUAllocation) *Slot {
	now := time.Now()

	// Determine concurrency limit: model-specific > runtime natural default
	maxConcurrency := int64(1) // Conservative default for unknown runtimes

	// First check if model has specific concurrency setting
	if work.model != nil && work.model.Concurrency > 0 {
		maxConcurrency = int64(work.model.Concurrency)
		log.Info().
			Str("model_id", work.model.ID).
			Int("model_concurrency", work.model.Concurrency).
			Int64("max_concurrency", maxConcurrency).
			Msg("🔧 CONCURRENCY_DEBUG: Using model-specific concurrency setting")
	} else {
		// Use natural runtime defaults
		if work.Runtime() == types.RuntimeVLLM {
			maxConcurrency = int64(types.DefaultVLLMParallelSequences)
			log.Info().
				Str("model_id", func() string {
					if work.model != nil {
						return work.model.ID
					}
					return "unknown"
				}()).
				Int("model_concurrency", func() int {
					if work.model != nil {
						return work.model.Concurrency
					}
					return 0
				}()).
				Int64("max_concurrency", maxConcurrency).
				Int("default_vllm_parallel", types.DefaultVLLMParallelSequences).
				Msg("🔧 CONCURRENCY_DEBUG: Using VLLM default concurrency")
		} else if work.Runtime() == types.RuntimeOllama {
			maxConcurrency = int64(memory.DefaultOllamaParallelSequences)
			log.Info().
				Str("model_id", func() string {
					if work.model != nil {
						return work.model.ID
					}
					return "unknown"
				}()).
				Int("model_concurrency", func() int {
					if work.model != nil {
						return work.model.Concurrency
					}
					return 0
				}()).
				Int64("max_concurrency", maxConcurrency).
				Int("default_ollama_parallel", memory.DefaultOllamaParallelSequences).
				Msg("🔧 CONCURRENCY_DEBUG: Using Ollama default concurrency")
		}
		// Other runtimes keep maxConcurrency = 1
	}

	return &Slot{
		ID:               uuid.New(),
		RunnerID:         runnerID,
		initialWork:      work,
		LastActivityTime: now,
		Created:          now,
		activeRequests:   0,
		maxConcurrency:   maxConcurrency,
		isStaleFunc:      staleTimeout,
		isErrorFunc:      errorTimeout,
		isRunning:        false,
		GPUAllocation:    gpuAllocation,
	}
}

// True if the model is not active and hasn't been active for at least ModelTTL
func (s *Slot) IsStale() bool {
	// Guard against completely corrupted slots - all nil fields should be considered stale for cleanup
	if s == nil {
		log.Warn().Msg("slot is nil - considering stale for cleanup")
		return true
	}

	// Guard against nil initialWork - corrupted slots should be considered stale for cleanup
	if s.initialWork == nil {
		log.Warn().
			Str("runner_id", s.RunnerID).
			Str("slot_id", s.ID.String()).
			Dur("elapsed_since_activity", time.Since(s.LastActivityTime)).
			Msg("slot has nil initialWork - considering stale for cleanup")
		return true
	}

	// Guard against nil function pointers - corrupted slots should be considered stale for cleanup
	if s.isErrorFunc == nil || s.isStaleFunc == nil {
		log.Warn().
			Str("runner_id", s.RunnerID).
			Str("slot_id", s.ID.String()).
			Bool("is_error_func_nil", s.isErrorFunc == nil).
			Bool("is_stale_func_nil", s.isStaleFunc == nil).
			Dur("elapsed_since_activity", time.Since(s.LastActivityTime)).
			Msg("slot has nil function pointers - considering stale for cleanup")
		return true
	}

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
				Str("model", func() string {
					if s.initialWork != nil {
						return s.initialWork.ModelName().String()
					}
					return "<nil_work>"
				}()).
				Msg("slot has timed out during creation (not running)")
			return true
		}
		return false
	}

	// If work is active, check for error timeout
	if s.IsActive() {
		elapsed := time.Since(s.LastActivityTime)
		isError := s.isErrorFunc(s.RunnerID, s.LastActivityTime)
		if isError {
			// Don't release the slot while holding the read lock
			// Instead, just return true and let the caller handle the release
			log.Warn().
				Str("runner_id", s.RunnerID).
				Str("slot_id", s.ID.String()).
				Dur("elapsed_since_activity", elapsed).
				Str("model", func() string {
					if s.initialWork != nil {
						return s.initialWork.ModelName().String()
					}
					return "<nil_work>"
				}()).
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
			Str("model", func() string {
				if s.initialWork != nil {
					return s.initialWork.ModelName().String()
				}
				return "<nil_work>"
			}()).
			Msg("slot is considered stale (inactive too long)")
	}
	return isStale
}

// True if this slot is currently active with work
func (s *Slot) IsActive() bool {
	return atomic.LoadInt64(&s.activeRequests) > 0
}

// True if this slot has capacity for more work
func (s *Slot) HasCapacity() bool {
	return atomic.LoadInt64(&s.activeRequests) < atomic.LoadInt64(&s.maxConcurrency)
}

// GetActiveRequests returns the current number of active requests
func (s *Slot) GetActiveRequests() int64 {
	return atomic.LoadInt64(&s.activeRequests)
}

// Sets a slot as no longer active (decrements active requests)
func (s *Slot) Release() {
	if atomic.AddInt64(&s.activeRequests, -1) < 0 {
		atomic.StoreInt64(&s.activeRequests, 0) // Prevent negative values
	}
	s.LastActivityTime = time.Now()
}

// Marks new work as started (increments active requests)
func (s *Slot) Start() {
	atomic.AddInt64(&s.activeRequests, 1)
	s.LastActivityTime = time.Now()
}

func (s *Slot) IsRunning() bool {
	return s.isRunning
}

func (s *Slot) SetRunning() {
	s.isRunning = true
}

func (s *Slot) Memory() uint64 {
	if s.initialWork == nil {
		return 0
	}
	return s.initialWork.model.Memory
}

func (s *Slot) InitialWork() *Workload {
	if s.initialWork == nil {
		log.Warn().
			Str("runner_id", s.RunnerID).
			Str("slot_id", s.ID.String()).
			Msg("slot has nil initialWork")
	}
	return s.initialWork
}
