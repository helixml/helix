package scheduler

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/memory"
	"github.com/helixml/helix/api/pkg/types"
)

// AllocationPlan represents a complete plan to allocate a workload to specific hardware
type AllocationPlan struct {
	// Target allocation
	RunnerID   string
	GPUs       []int
	GPUCount   int
	IsMultiGPU bool

	// Memory details
	TotalMemoryRequired uint64
	MemoryPerGPU        uint64

	// Cost and feasibility
	Cost             int // Lower is better (evictions + load balancing penalty)
	RequiresEviction bool
	EvictionsNeeded  []*Slot // Slots that need to be evicted

	// Runtime details
	TensorParallelSize int
	Runtime            types.Runtime

	// Validation
	IsValid         bool
	ValidationError error
}

// AllocationCost represents the cost components of an allocation plan
type AllocationCost struct {
	EvictionCount    int // Number of slots to evict
	LoadPenalty      int // Penalty for uneven load distribution
	GPUFragmentation int // Penalty for inefficient GPU usage
	Total            int // Combined cost score
}

// GlobalAllocator handles global GPU allocation decisions across all runners
type GlobalAllocator struct {
	controller              *RunnerController
	memoryEstimationService MemoryEstimationService
	slots                   *SlotStore
	globalDecisionsTracker  *GlobalAllocationDecisionsTracker
}

// NewGlobalAllocator creates a new global allocator
func NewGlobalAllocator(controller *RunnerController, memoryService MemoryEstimationService, slots *SlotStore, globalDecisionsTracker *GlobalAllocationDecisionsTracker) *GlobalAllocator {
	return &GlobalAllocator{
		controller:              controller,
		memoryEstimationService: memoryService,
		slots:                   slots,
		globalDecisionsTracker:  globalDecisionsTracker,
	}
}

// PlanAllocation creates an optimal allocation plan for a workload across all available resources
func (ga *GlobalAllocator) PlanAllocation(work *Workload) (*AllocationPlan, error) {
	if work == nil || work.model == nil {
		return nil, fmt.Errorf("invalid workload or model")
	}

	// External agents don't need GPU allocation - they run in containers
	// Return a dummy allocation plan to satisfy the interface
	if work.model.ID == "external_agent" {
		return &AllocationPlan{
			RunnerID:            "", // No runner needed
			GPUs:                []int{},
			GPUCount:            0,
			IsMultiGPU:          false,
			TotalMemoryRequired: 0,
			MemoryPerGPU:        0,
			Cost:                0,
			RequiresEviction:    false,
			EvictionsNeeded:     nil,
			TensorParallelSize:  1,
			Runtime:             types.Runtime("external"),
			IsValid:             true,
			ValidationError:     nil,
		}, nil
	}

	// Calculate memory requirement for this workload
	memoryRequirement, err := ga.calculateMemoryRequirement(work)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate memory requirement for %s: %w", work.ModelName(), err)
	}

	log.Debug().
		Str("model", work.ModelName().String()).
		Str("runtime", string(work.Runtime())).
		Uint64("memory_requirement_gb", memoryRequirement/(1024*1024*1024)).
		Msg("🌍 GLOBAL: Planning allocation for workload")

	// Get all viable allocation plans across all runners and GPUs
	allPlans, err := ga.generateAllocationPlans(work, memoryRequirement)
	if err != nil {
		return nil, fmt.Errorf("failed to generate allocation plans: %w", err)
	}

	if len(allPlans) == 0 {
		return nil, fmt.Errorf("no viable allocation plans found for model %s (%d GB)",
			work.ModelName(), memoryRequirement/(1024*1024*1024))
	}

	// Select the optimal plan
	bestPlan := ga.selectOptimalPlan(allPlans)

	log.Info().
		Str("model", work.ModelName().String()).
		Str("selected_runner", bestPlan.RunnerID).
		Ints("selected_gpus", bestPlan.GPUs).
		Int("gpu_count", bestPlan.GPUCount).
		Bool("requires_eviction", bestPlan.RequiresEviction).
		Int("evictions_needed", len(bestPlan.EvictionsNeeded)).
		Int("cost", bestPlan.Cost).
		Uint64("memory_per_gpu_gb", bestPlan.MemoryPerGPU/(1024*1024*1024)).
		Msg("🌍 GLOBAL: Selected optimal allocation plan")

	return bestPlan, nil
}

// ExecuteAllocationPlan executes an allocation plan by performing evictions and creating the slot
func (ga *GlobalAllocator) ExecuteAllocationPlan(plan *AllocationPlan, work *Workload) (*Slot, error) {
	if plan == nil {
		return nil, fmt.Errorf("allocation plan is nil")
	}
	if !plan.IsValid {
		return nil, fmt.Errorf("cannot execute invalid allocation plan: %w", plan.ValidationError)
	}

	log.Info().
		Str("runner_id", plan.RunnerID).
		Str("model", work.ModelName().String()).
		Ints("gpus", plan.GPUs).
		Int("evictions_needed", len(plan.EvictionsNeeded)).
		Msg("🚀 GLOBAL: Executing allocation plan")

	// External agents don't need GPU allocation - create simple slot and return
	if work.model.ID == "external_agent" {
		slot := NewSlot("", work, nil, nil, nil)
		log.Info().
			Str("slot_id", slot.ID.String()).
			Str("model", work.ModelName().String()).
			Msg("🎯 GLOBAL: Successfully created external agent slot (no GPU allocation)")
		return slot, nil
	}

	// Step 1: Perform evictions if needed
	if len(plan.EvictionsNeeded) > 0 {
		if err := ga.performEvictions(plan.RunnerID, plan.EvictionsNeeded); err != nil {
			return nil, fmt.Errorf("failed to perform evictions: %w", err)
		}
	}

	// Step 2: Create allocation config
	allocation := GPUAllocationConfig{
		GPUCount:     plan.GPUCount,
		SpecificGPUs: plan.GPUs,
	}

	// Step 3: Configure model for this specific allocation
	configuredModel, err := NewModelForGPUAllocation(work.model, allocation, ga.memoryEstimationService)
	if err != nil {
		return nil, fmt.Errorf("failed to configure model for allocation: %w", err)
	}

	// Step 4: Create configured workload
	configuredWork := &Workload{
		WorkloadType:        work.WorkloadType,
		llmInferenceRequest: work.llmInferenceRequest,
		session:             work.session,
		model:               configuredModel,
	}

	// Step 5: Create GPU allocation metadata
	var singleGPU *int
	var multiGPUs []int

	if plan.GPUCount == 1 {
		singleGPU = &plan.GPUs[0]
	} else {
		multiGPUs = plan.GPUs
	}

	gpuAllocation := &GPUAllocation{
		WorkloadID:         work.ID(),
		RunnerID:           plan.RunnerID,
		SingleGPU:          singleGPU,
		MultiGPUs:          multiGPUs,
		TensorParallelSize: plan.TensorParallelSize,
	}

	// Step 6: Create the slot
	slot := NewSlot(plan.RunnerID, configuredWork, nil, nil, gpuAllocation)

	log.Info().
		Str("slot_id", slot.ID.String()).
		Str("runner_id", plan.RunnerID).
		Str("model", work.ModelName().String()).
		Ints("gpus", plan.GPUs).
		Uint64("memory_gb", plan.TotalMemoryRequired/(1024*1024*1024)).
		Msg("🎯 GLOBAL: Successfully created slot from allocation plan")

	return slot, nil
}

// calculateMemoryRequirement determines the memory requirement for a workload
func (ga *GlobalAllocator) calculateMemoryRequirement(work *Workload) (uint64, error) {
	// If model is already configured, use that
	if work.model.IsAllocationConfigured() {
		return work.model.GetMemoryForAllocation(), nil
	}

	// For unconfigured models, calculate based on runtime
	switch work.Runtime() {
	case types.RuntimeVLLM:
		if work.model.Memory == 0 {
			return 0, fmt.Errorf("VLLM model %s has no admin-configured memory", work.model.ID)
		}
		return work.model.Memory, nil

	case types.RuntimeOllama:
		if work.model.Memory != 0 {
			return 0, fmt.Errorf("Ollama model %s should have Memory=0, found %d", work.model.ID, work.model.Memory)
		}

		// Use GGUF estimation
		estimateOptions := memory.CreateAutoEstimateOptions(work.model.ContextLength)
		if work.model.Concurrency > 0 {
			estimateOptions.NumParallel = int(work.model.Concurrency)
		} else {
			estimateOptions.NumParallel = memory.DefaultOllamaParallelSequences
		}

		result, err := ga.memoryEstimationService.EstimateModelMemory(context.Background(), work.model.ID, estimateOptions)
		if err != nil {
			return 0, fmt.Errorf("failed to estimate memory for Ollama model %s: %w", work.model.ID, err)
		}

		if result.SingleGPU != nil {
			return result.SingleGPU.TotalSize, nil
		}
		return 0, fmt.Errorf("no memory estimate available for Ollama model %s", work.model.ID)

	default:
		return 0, fmt.Errorf("unsupported runtime %s", work.Runtime())
	}
}

// generateAllocationPlans creates all viable allocation plans across all runners
func (ga *GlobalAllocator) generateAllocationPlans(work *Workload, memoryRequirement uint64) ([]AllocationPlan, error) {
	var allPlans []AllocationPlan

	runnerIDs := ga.controller.RunnerIDs()
	if len(runnerIDs) == 0 {
		return nil, fmt.Errorf("no runners available")
	}

	for _, runnerID := range runnerIDs {
		// Get runner status
		status, err := ga.controller.GetStatus(runnerID)
		if err != nil {
			log.Warn().Err(err).Str("runner_id", runnerID).Msg("🌍 GLOBAL: Skipping runner due to status error")
			continue
		}

		// Check if runner has the model (for Ollama)
		if work.Runtime() == types.RuntimeOllama {
			if !ga.runnerHasModel(runnerID, work.model.ID) {
				log.Debug().
					Str("runner_id", runnerID).
					Str("model", work.model.ID).
					Msg("🌍 GLOBAL: Skipping runner - model not available")
				continue
			}
		}

		// Generate allocation plans for this runner
		runnerPlans, err := ga.generateRunnerAllocationPlans(runnerID, status, work, memoryRequirement)
		if err != nil {
			log.Debug().
				Err(err).
				Str("runner_id", runnerID).
				Msg("🌍 GLOBAL: No viable plans for this runner")
			continue
		}

		allPlans = append(allPlans, runnerPlans...)
	}

	log.Debug().
		Int("total_plans", len(allPlans)).
		Int("runners_considered", len(runnerIDs)).
		Uint64("memory_requirement_gb", memoryRequirement/(1024*1024*1024)).
		Msg("🌍 GLOBAL: Generated all allocation plans")

	return allPlans, nil
}

// generateRunnerAllocationPlans creates all viable allocation plans for a specific runner
func (ga *GlobalAllocator) generateRunnerAllocationPlans(runnerID string, status *types.RunnerStatus, work *Workload, memoryRequirement uint64) ([]AllocationPlan, error) {
	var plans []AllocationPlan

	// Calculate current allocations on this runner
	allocatedMemoryPerGPU, err := ga.controller.calculateAllocatedMemoryPerGPU(runnerID)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate allocated memory: %w", err)
	}

	// Option 1: Try single GPU allocations
	singleGPUPlans := ga.generateSingleGPUPlans(runnerID, status, allocatedMemoryPerGPU, work, memoryRequirement)
	plans = append(plans, singleGPUPlans...)

	log.Debug().
		Str("runner_id", runnerID).
		Str("model", work.ModelName().String()).
		Int("single_gpu_plans", len(singleGPUPlans)).
		Msg("🌍 GLOBAL: Generated single GPU plans")

	// Option 2: Try multi-GPU allocations (if runtime supports it)
	if work.Runtime() == types.RuntimeVLLM || work.Runtime() == types.RuntimeOllama {
		multiGPUPlans := ga.generateMultiGPUPlans(runnerID, status, allocatedMemoryPerGPU, work, memoryRequirement)
		plans = append(plans, multiGPUPlans...)

		log.Debug().
			Str("runner_id", runnerID).
			Str("model", work.ModelName().String()).
			Int("multi_gpu_plans", len(multiGPUPlans)).
			Int("total_plans", len(plans)).
			Msg("🌍 GLOBAL: Generated multi-GPU plans")
	}

	return plans, nil
}

// generateSingleGPUPlans creates single GPU allocation plans
func (ga *GlobalAllocator) generateSingleGPUPlans(runnerID string, status *types.RunnerStatus, allocatedMemoryPerGPU map[int]uint64, work *Workload, memoryRequirement uint64) []AllocationPlan {
	var plans []AllocationPlan

	log.Debug().
		Str("runner_id", runnerID).
		Str("model", work.ModelName().String()).
		Uint64("memory_requirement_gb", memoryRequirement/(1024*1024*1024)).
		Interface("allocated_per_gpu_gb", func() map[string]uint64 {
			result := make(map[string]uint64)
			for gpu, mem := range allocatedMemoryPerGPU {
				result[fmt.Sprintf("gpu_%d", gpu)] = mem / (1024 * 1024 * 1024)
			}
			return result
		}()).
		Msg("🌍 GLOBAL: Starting single GPU plan generation")

	for _, gpu := range status.GPUs {
		// Check if model can physically fit on this GPU
		if memoryRequirement > gpu.TotalMemory {
			log.Debug().
				Str("runner_id", runnerID).
				Int("gpu", gpu.Index).
				Uint64("required_gb", memoryRequirement/(1024*1024*1024)).
				Uint64("gpu_capacity_gb", gpu.TotalMemory/(1024*1024*1024)).
				Msg("🌍 GLOBAL: Model too large for GPU - skipping")
			continue
		}

		log.Debug().
			Str("runner_id", runnerID).
			Int("gpu", gpu.Index).
			Uint64("required_gb", memoryRequirement/(1024*1024*1024)).
			Uint64("gpu_capacity_gb", gpu.TotalMemory/(1024*1024*1024)).
			Msg("🌍 GLOBAL: GPU can physically fit model")

		allocatedOnGPU := allocatedMemoryPerGPU[gpu.Index]
		freeOnGPU := gpu.TotalMemory - allocatedOnGPU

		log.Debug().
			Str("runner_id", runnerID).
			Int("gpu", gpu.Index).
			Uint64("gpu_total_gb", gpu.TotalMemory/(1024*1024*1024)).
			Uint64("gpu_allocated_gb", allocatedOnGPU/(1024*1024*1024)).
			Uint64("gpu_free_gb", freeOnGPU/(1024*1024*1024)).
			Uint64("required_gb", memoryRequirement/(1024*1024*1024)).
			Bool("can_fit_without_eviction", freeOnGPU >= memoryRequirement).
			Msg("🌍 GLOBAL: Evaluating GPU for single allocation")

		plan := AllocationPlan{
			RunnerID:            runnerID,
			GPUs:                []int{gpu.Index},
			GPUCount:            1,
			IsMultiGPU:          false,
			TotalMemoryRequired: memoryRequirement,
			MemoryPerGPU:        memoryRequirement,
			TensorParallelSize:  1,
			Runtime:             work.Runtime(),
			IsValid:             true,
		}

		if freeOnGPU >= memoryRequirement {
			// Can fit without eviction
			plan.RequiresEviction = false
			plan.Cost = ga.calculatePlanCost(runnerID, gpu.Index, 0, freeOnGPU, gpu.TotalMemory)
			plans = append(plans, plan)

			log.Debug().
				Str("runner_id", runnerID).
				Int("gpu", gpu.Index).
				Uint64("free_gb", freeOnGPU/(1024*1024*1024)).
				Uint64("required_gb", memoryRequirement/(1024*1024*1024)).
				Int("cost", plan.Cost).
				Msg("🌍 GLOBAL: Generated single GPU plan without eviction")
		} else {
			// Check if can fit with eviction
			evictableSlots := ga.findEvictableSlots(runnerID, gpu.Index, work)
			evictableMemory := ga.calculateEvictableMemory(evictableSlots)

			if freeOnGPU+evictableMemory >= memoryRequirement {
				plan.RequiresEviction = true
				plan.EvictionsNeeded = ga.selectSlotsForEviction(evictableSlots, memoryRequirement-freeOnGPU)
				plan.Cost = ga.calculatePlanCost(runnerID, gpu.Index, len(plan.EvictionsNeeded), freeOnGPU, gpu.TotalMemory)
				plans = append(plans, plan)

				log.Debug().
					Str("runner_id", runnerID).
					Int("gpu", gpu.Index).
					Uint64("free_gb", freeOnGPU/(1024*1024*1024)).
					Uint64("evictable_gb", evictableMemory/(1024*1024*1024)).
					Int("evictions_needed", len(plan.EvictionsNeeded)).
					Int("cost", plan.Cost).
					Msg("🌍 GLOBAL: Generated single GPU plan with eviction")
			}
		}
	}

	log.Debug().
		Str("runner_id", runnerID).
		Str("model", work.ModelName().String()).
		Int("single_gpu_plans_generated", len(plans)).
		Msg("🌍 GLOBAL: Completed single GPU plan generation")

	return plans
}

// generateMultiGPUPlans creates multi-GPU allocation plans
func (ga *GlobalAllocator) generateMultiGPUPlans(runnerID string, status *types.RunnerStatus, allocatedMemoryPerGPU map[int]uint64, work *Workload, memoryRequirement uint64) []AllocationPlan {
	var plans []AllocationPlan

	log.Debug().
		Str("runner_id", runnerID).
		Uint64("memory_requirement_gb", memoryRequirement/(1024*1024*1024)).
		Int("available_gpus", len(status.GPUs)).
		Msg("🌍 GLOBAL: Starting multi-GPU plan generation")

	// Try different numbers of GPUs (2, 3, 4...)
	for numGPUs := 2; numGPUs <= len(status.GPUs); numGPUs++ {
		memoryPerGPU := memoryRequirement / uint64(numGPUs)

		log.Debug().
			Str("runner_id", runnerID).
			Int("num_gpus", numGPUs).
			Uint64("memory_per_gpu_gb", memoryPerGPU/(1024*1024*1024)).
			Msg("🌍 GLOBAL: Trying multi-GPU configuration")

		// Find GPUs that can accommodate the per-GPU requirement
		var viableGPUs []int
		var totalEvictions []*Slot
		var totalEvictionCost int
		canFitWithEviction := true

		for _, gpu := range status.GPUs {
			allocatedOnGPU := allocatedMemoryPerGPU[gpu.Index]
			freeOnGPU := gpu.TotalMemory - allocatedOnGPU

			if freeOnGPU >= memoryPerGPU {
				// Can fit without eviction
				viableGPUs = append(viableGPUs, gpu.Index)
			} else {
				// Check if can fit with eviction
				evictableSlots := ga.findEvictableSlots(runnerID, gpu.Index, work)
				evictableMemory := ga.calculateEvictableMemory(evictableSlots)

				if freeOnGPU+evictableMemory >= memoryPerGPU {
					// Can fit with eviction
					needed := memoryPerGPU - freeOnGPU
					slotsToEvict := ga.selectSlotsForEviction(evictableSlots, needed)
					totalEvictions = append(totalEvictions, slotsToEvict...)
					totalEvictionCost += len(slotsToEvict)
					viableGPUs = append(viableGPUs, gpu.Index)
				} else {
					// Cannot fit even with eviction
					canFitWithEviction = false
					break
				}
			}

			if len(viableGPUs) >= numGPUs {
				break // Have enough GPUs
			}
		}

		// Create plan if we have enough viable GPUs
		if canFitWithEviction && len(viableGPUs) >= numGPUs {
			selectedGPUs := viableGPUs[:numGPUs]

			plan := AllocationPlan{
				RunnerID:            runnerID,
				GPUs:                selectedGPUs,
				GPUCount:            numGPUs,
				IsMultiGPU:          true,
				TotalMemoryRequired: memoryRequirement,
				MemoryPerGPU:        memoryPerGPU,
				RequiresEviction:    len(totalEvictions) > 0,
				EvictionsNeeded:     totalEvictions,
				TensorParallelSize:  numGPUs,
				Runtime:             work.Runtime(),
				Cost:                ga.calculateMultiGPUPlanCost(runnerID, selectedGPUs, totalEvictionCost),
				IsValid:             true,
			}

			plans = append(plans, plan)

			log.Debug().
				Str("runner_id", runnerID).
				Ints("gpus", selectedGPUs).
				Int("gpu_count", numGPUs).
				Uint64("memory_per_gpu_gb", memoryPerGPU/(1024*1024*1024)).
				Int("total_evictions", len(totalEvictions)).
				Int("cost", plan.Cost).
				Msg("🌍 GLOBAL: Generated multi-GPU allocation plan")
		}
	}

	log.Debug().
		Str("runner_id", runnerID).
		Int("generated_plans", len(plans)).
		Msg("🌍 GLOBAL: Completed multi-GPU plan generation")

	return plans
}

// selectOptimalPlan chooses the best allocation plan from all options
func (ga *GlobalAllocator) selectOptimalPlan(plans []AllocationPlan) *AllocationPlan {
	if len(plans) == 0 {
		return nil
	}

	// Sort plans by cost (lower is better)
	slices.SortFunc(plans, func(a, b AllocationPlan) int {
		return a.Cost - b.Cost
	})

	// Log all plans for debugging
	log.Debug().Int("total_plans", len(plans)).Msg("🌍 GLOBAL: All allocation plans")
	for i, p := range plans {
		log.Debug().
			Int("plan_index", i).
			Str("runner", p.RunnerID).
			Ints("gpus", p.GPUs).
			Int("gpu_count", p.GPUCount).
			Bool("is_multi_gpu", p.IsMultiGPU).
			Int("cost", p.Cost).
			Bool("requires_eviction", p.RequiresEviction).
			Msg("🌍 GLOBAL: Plan option")
	}

	// Log the decision process
	log.Info().
		Int("total_plans", len(plans)).
		Str("selected_runner", plans[0].RunnerID).
		Ints("selected_gpus", plans[0].GPUs).
		Int("selected_cost", plans[0].Cost).
		Bool("requires_eviction", plans[0].RequiresEviction).
		Bool("is_multi_gpu", plans[0].IsMultiGPU).
		Msg("🌍 GLOBAL: Selected optimal plan from candidates")

	return &plans[0]
}

// findEvictableSlots finds slots that can be evicted from a specific GPU
func (ga *GlobalAllocator) findEvictableSlots(runnerID string, gpuIndex int, currentWork *Workload) []*Slot {
	var evictableSlots []*Slot

	ga.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		// Only consider slots on this runner
		if slot.RunnerID != runnerID {
			return true
		}

		// Don't evict slots for the same model/runtime/lora
		if slot.InitialWork().ModelName() == currentWork.ModelName() &&
			slot.InitialWork().Runtime() == currentWork.Runtime() &&
			slot.InitialWork().LoraDir() == currentWork.LoraDir() {
			return true
		}

		// Only consider slots that use this specific GPU
		if slot.GPUAllocation != nil {
			usesThisGPU := false
			if slot.GPUAllocation.SingleGPU != nil && *slot.GPUAllocation.SingleGPU == gpuIndex {
				usesThisGPU = true
			}
			for _, gpu := range slot.GPUAllocation.MultiGPUs {
				if gpu == gpuIndex {
					usesThisGPU = true
					break
				}
			}

			if usesThisGPU && slot.IsStale() {
				evictableSlots = append(evictableSlots, slot)
			}
		}

		return true
	})

	// Sort by staleness (oldest first)
	slices.SortFunc(evictableSlots, func(a, b *Slot) int {
		return int(a.LastActivityTime.Sub(b.LastActivityTime))
	})

	return evictableSlots
}

// calculateEvictableMemory calculates total memory that can be freed by evicting slots
func (ga *GlobalAllocator) calculateEvictableMemory(slots []*Slot) uint64 {
	var total uint64
	for _, slot := range slots {
		if slot.InitialWork().model != nil && slot.InitialWork().model.IsAllocationConfigured() {
			total += slot.InitialWork().model.GetMemoryForAllocation()
		}
	}
	return total
}

// selectSlotsForEviction selects the minimum slots needed to free the required memory
func (ga *GlobalAllocator) selectSlotsForEviction(evictableSlots []*Slot, memoryNeeded uint64) []*Slot {
	var selected []*Slot
	var freedMemory uint64

	for _, slot := range evictableSlots {
		if freedMemory >= memoryNeeded {
			break
		}

		if slot.InitialWork().model != nil && slot.InitialWork().model.IsAllocationConfigured() {
			slotMemory := slot.InitialWork().model.GetMemoryForAllocation()
			selected = append(selected, slot)
			freedMemory += slotMemory
		}
	}

	return selected
}

// calculatePlanCost calculates the cost of a single GPU allocation plan
func (ga *GlobalAllocator) calculatePlanCost(runnerID string, gpuIndex int, evictionCount int, freeMemory uint64, totalMemory uint64) int {
	// Cost factors:
	// 1. Eviction cost (higher evictions = higher cost)
	evictionCost := evictionCount * 100

	// 2. Load balancing cost (prefer GPUs with more free memory)
	usedMemory := totalMemory - freeMemory
	memoryPenalty := int(usedMemory / (1024 * 1024 * 1024)) // Penalty based on used memory in GB

	// 3. Runner load balancing (prefer runners with less total load)
	runnerPenalty := ga.calculateRunnerLoadPenalty(runnerID)

	return evictionCost + memoryPenalty + runnerPenalty
}

// calculateMultiGPUPlanCost calculates the cost of a multi-GPU allocation plan
func (ga *GlobalAllocator) calculateMultiGPUPlanCost(runnerID string, gpus []int, evictionCount int) int {
	baseCost := evictionCount * 100

	// Multi-GPU penalty (strongly prefer single GPU when possible)
	multiGPUPenalty := len(gpus) * 1000 // High penalty to prefer single GPU

	// Runner load penalty
	runnerPenalty := ga.calculateRunnerLoadPenalty(runnerID)

	return baseCost + multiGPUPenalty + runnerPenalty
}

// calculateRunnerLoadPenalty calculates penalty based on runner's current load
func (ga *GlobalAllocator) calculateRunnerLoadPenalty(runnerID string) int {
	var totalLoad uint64
	ga.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		if slot.RunnerID == runnerID && slot.InitialWork().model != nil && slot.InitialWork().model.IsAllocationConfigured() {
			totalLoad += slot.InitialWork().model.GetMemoryForAllocation()
		}
		return true
	})

	// Return penalty based on total load (GB)
	return int(totalLoad / (1024 * 1024 * 1024))
}

// performEvictions executes the eviction of specified slots
func (ga *GlobalAllocator) performEvictions(runnerID string, slotsToEvict []*Slot) error {
	for _, slot := range slotsToEvict {
		log.Info().
			Str("runner_id", runnerID).
			Str("slot_id", slot.ID.String()).
			Str("model", slot.InitialWork().ModelName().String()).
			Dur("age", time.Since(slot.LastActivityTime)).
			Msg("🌍 GLOBAL: Evicting slot for new allocation")

		// Delete slot from runner
		err := ga.controller.DeleteSlot(runnerID, slot.ID)
		if err != nil {
			log.Error().
				Err(err).
				Str("runner_id", runnerID).
				Str("slot_id", slot.ID.String()).
				Msg("🌍 GLOBAL: Failed to delete slot from runner")
			return fmt.Errorf("failed to delete slot %s from runner %s: %w", slot.ID, runnerID, err)
		}

		// Remove from scheduler state
		ga.slots.Delete(slot.ID)
	}

	return nil
}

// runnerHasModel checks if a runner has a specific model available
func (ga *GlobalAllocator) runnerHasModel(runnerID, modelID string) bool {
	status, err := ga.controller.GetStatus(runnerID)
	if err != nil {

		return false
	}

	// Log available models for debugging
	availableModels := make([]string, len(status.Models))
	for i, model := range status.Models {
		availableModels[i] = model.ModelID
		if model.ModelID == modelID {

			return true
		}
	}

	return false
}

// validateAllocationPlan performs final validation on an allocation plan
func (ga *GlobalAllocator) validateAllocationPlan(plan *AllocationPlan) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}

	// Get current runner status
	status, err := ga.controller.GetStatus(plan.RunnerID)
	if err != nil {
		return fmt.Errorf("runner %s not available: %w", plan.RunnerID, err)
	}

	// Validate GPU indices exist
	for _, gpuIndex := range plan.GPUs {
		found := false
		for _, gpu := range status.GPUs {
			if gpu.Index == gpuIndex {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("GPU %d not found on runner %s", gpuIndex, plan.RunnerID)
		}
	}

	// Validate memory constraints
	allocatedMemoryPerGPU, err := ga.controller.calculateAllocatedMemoryPerGPU(plan.RunnerID)
	if err != nil {
		return fmt.Errorf("failed to validate memory constraints: %w", err)
	}

	for _, gpuIndex := range plan.GPUs {
		for _, gpu := range status.GPUs {
			if gpu.Index == gpuIndex {
				allocatedOnGPU := allocatedMemoryPerGPU[gpuIndex]

				// Account for evictions that will free memory
				evictableOnThisGPU := uint64(0)
				for _, slot := range plan.EvictionsNeeded {
					if slot.GPUAllocation != nil && ga.slotUsesGPU(slot, gpuIndex) {
						if slot.InitialWork().model != nil && slot.InitialWork().model.IsAllocationConfigured() {
							// For multi-GPU slots, only count the portion on this GPU
							slotMemory := slot.InitialWork().model.GetMemoryForAllocation()
							if len(slot.GPUAllocation.MultiGPUs) > 1 {
								evictableOnThisGPU += slotMemory / uint64(len(slot.GPUAllocation.MultiGPUs))
							} else {
								evictableOnThisGPU += slotMemory
							}
						}
					}
				}

				finalAllocated := allocatedOnGPU - evictableOnThisGPU + plan.MemoryPerGPU
				if finalAllocated > gpu.TotalMemory {
					return fmt.Errorf("GPU %d would be overscheduled: %d GB > %d GB capacity",
						gpuIndex, finalAllocated/(1024*1024*1024), gpu.TotalMemory/(1024*1024*1024))
				}
			}
		}
	}

	return nil
}

// slotUsesGPU checks if a slot uses a specific GPU
func (ga *GlobalAllocator) slotUsesGPU(slot *Slot, gpuIndex int) bool {
	if slot.GPUAllocation == nil {
		return false
	}

	if slot.GPUAllocation.SingleGPU != nil && *slot.GPUAllocation.SingleGPU == gpuIndex {
		return true
	}

	for _, gpu := range slot.GPUAllocation.MultiGPUs {
		if gpu == gpuIndex {
			return true
		}
	}

	return false
}

// AllocateWorkload is the main entry point for the global allocator
// This replaces the complex multi-method allocation logic in ensureSlot()
func (ga *GlobalAllocator) AllocateWorkload(work *Workload, modelStaleFunc, slotTimeoutFunc TimeoutFunc) (*Slot, error) {
	startTime := time.Now()

	if work == nil {
		return nil, fmt.Errorf("workload is nil")
	}

	log.Info().
		Str("model", work.ModelName().String()).
		Str("runtime", string(work.Runtime())).
		Str("workload_id", work.ID()).
		Msg("🌍 GLOBAL: Starting global allocation for workload")

	// Capture before state for visualization
	beforeState := ga.captureGlobalState()

	// Phase 1: Create allocation plan
	plan, err := ga.PlanAllocation(work)
	if err != nil {
		log.Warn().
			Err(err).
			Str("model", work.ModelName().String()).
			Msg("🌍 GLOBAL: Failed to create allocation plan")

		// Log failed global decision
		ga.logGlobalDecision(work, nil, nil, beforeState, nil, time.Since(startTime).Milliseconds(), false, fmt.Sprintf("Failed to create allocation plan: %v", err))
		return nil, err
	}

	// Phase 2: Validate plan before execution
	// Skip validation for external agents - they don't use GPU runners
	if work.model.ID != "external_agent" {
		if err := ga.validateAllocationPlan(plan); err != nil {
			log.Error().
				Err(err).
				Str("model", work.ModelName().String()).
				Str("runner_id", plan.RunnerID).
				Ints("gpus", plan.GPUs).
				Msg("🌍 GLOBAL: Allocation plan validation failed")

			// Log failed global decision
			ga.logGlobalDecision(work, []*AllocationPlan{plan}, plan, beforeState, nil, time.Since(startTime).Milliseconds(), false, fmt.Sprintf("Allocation plan validation failed: %v", err))
			return nil, fmt.Errorf("allocation plan validation failed: %w", err)
		}
	}

	// Phase 3: Execute plan
	slot, err := ga.ExecuteAllocationPlan(plan, work)
	if err != nil {
		log.Error().
			Err(err).
			Str("model", work.ModelName().String()).
			Str("runner_id", plan.RunnerID).
			Msg("🌍 GLOBAL: Failed to execute allocation plan")

		// Log failed global decision
		ga.logGlobalDecision(work, []*AllocationPlan{plan}, plan, beforeState, nil, time.Since(startTime).Milliseconds(), false, fmt.Sprintf("Failed to execute allocation plan: %v", err))
		return nil, err
	}

	// Set timeout functions
	slot.isStaleFunc = modelStaleFunc
	slot.isErrorFunc = slotTimeoutFunc

	// Capture after state for visualization
	afterState := ga.captureGlobalState()

	log.Info().
		Str("model", work.ModelName().String()).
		Str("runner_id", plan.RunnerID).
		Ints("gpus", plan.GPUs).
		Str("slot_id", slot.ID.String()).
		Int("evictions_performed", len(plan.EvictionsNeeded)).
		Uint64("memory_allocated_gb", plan.TotalMemoryRequired/(1024*1024*1024)).
		Msg("🌍 GLOBAL: Successfully allocated workload")

	// Log successful global decision
	ga.logGlobalDecision(work, []*AllocationPlan{plan}, plan, beforeState, afterState, time.Since(startTime).Milliseconds(), true, "Successfully allocated workload")

	return slot, nil
}

// GetGlobalMemoryState returns memory allocation state across all runners for debugging
func (ga *GlobalAllocator) GetGlobalMemoryState() map[string]map[int]uint64 {
	result := make(map[string]map[int]uint64)

	for _, runnerID := range ga.controller.RunnerIDs() {
		allocatedPerGPU, err := ga.controller.calculateAllocatedMemoryPerGPU(runnerID)
		if err != nil {
			log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to get memory state for runner")
			continue
		}
		result[runnerID] = allocatedPerGPU
	}

	return result
}

// ValidateNoOverscheduling performs a comprehensive check that no GPU is overscheduled
func (ga *GlobalAllocator) ValidateNoOverscheduling() []string {
	var violations []string

	for _, runnerID := range ga.controller.RunnerIDs() {
		status, err := ga.controller.GetStatus(runnerID)
		if err != nil {
			violations = append(violations, fmt.Sprintf("Runner %s: status unavailable", runnerID))
			continue
		}

		allocatedPerGPU, err := ga.controller.calculateAllocatedMemoryPerGPU(runnerID)
		if err != nil {
			violations = append(violations, fmt.Sprintf("Runner %s: memory calculation failed", runnerID))
			continue
		}

		for _, gpu := range status.GPUs {
			allocated := allocatedPerGPU[gpu.Index]
			if allocated > gpu.TotalMemory {
				violations = append(violations, fmt.Sprintf(
					"Runner %s GPU %d: %d GB allocated > %d GB capacity",
					runnerID, gpu.Index,
					allocated/(1024*1024*1024),
					gpu.TotalMemory/(1024*1024*1024)))
			}
		}
	}

	return violations
}

// captureGlobalState captures the current state of all runners for visualization
func (ga *GlobalAllocator) captureGlobalState() map[string]*types.RunnerStateView {
	state := make(map[string]*types.RunnerStateView)

	for _, runnerID := range ga.controller.RunnerIDs() {
		runnerState := &types.RunnerStateView{
			RunnerID:    runnerID,
			GPUStates:   make(map[int]*types.GPUState),
			IsConnected: slices.Contains(ga.controller.RunnerIDs(), runnerID),
		}

		// Get runner status
		status, err := ga.controller.GetStatus(runnerID)
		if err != nil {
			state[runnerID] = runnerState
			continue
		}

		// Get allocated memory per GPU
		allocatedPerGPU, err := ga.controller.calculateAllocatedMemoryPerGPU(runnerID)
		if err != nil {
			allocatedPerGPU = make(map[int]uint64)
		}

		// Capture GPU states
		for i, gpu := range status.GPUs {
			allocated := allocatedPerGPU[i]
			free := gpu.TotalMemory - allocated
			utilization := float64(allocated) / float64(gpu.TotalMemory)

			// Get active slots for this GPU
			var activeSlots []string
			ga.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
				if slot.RunnerID != runnerID {
					return true
				}

				if slot.GPUAllocation != nil {
					usesThisGPU := false
					if slot.GPUAllocation.SingleGPU != nil && *slot.GPUAllocation.SingleGPU == i {
						usesThisGPU = true
					}
					for _, multiGPU := range slot.GPUAllocation.MultiGPUs {
						if multiGPU == i {
							usesThisGPU = true
							break
						}
					}
					if usesThisGPU {
						activeSlots = append(activeSlots, slot.ID.String())
					}
				}
				return true
			})

			runnerState.GPUStates[i] = &types.GPUState{
				Index:           i,
				TotalMemory:     gpu.TotalMemory,
				AllocatedMemory: allocated,
				FreeMemory:      free,
				ActiveSlots:     activeSlots,
				Utilization:     utilization,
			}
		}

		state[runnerID] = runnerState
	}

	return state
}

// convertPlanToView converts an AllocationPlan to AllocationPlanView for visualization
func (ga *GlobalAllocator) convertPlanToView(plan *AllocationPlan) *types.AllocationPlanView {
	if plan == nil {
		return nil
	}

	var evictionIDs []string
	for _, slot := range plan.EvictionsNeeded {
		evictionIDs = append(evictionIDs, slot.ID.String())
	}

	// Get runner memory state
	runnerMemoryState := make(map[int]uint64)
	runnerCapacity := make(map[int]uint64)

	allocatedPerGPU, err := ga.controller.calculateAllocatedMemoryPerGPU(plan.RunnerID)
	if err == nil {
		runnerMemoryState = allocatedPerGPU
	}

	status, err := ga.controller.GetStatus(plan.RunnerID)
	if err == nil {
		for i, gpu := range status.GPUs {
			runnerCapacity[i] = gpu.TotalMemory
		}
	}

	validationError := ""
	if plan.ValidationError != nil {
		validationError = plan.ValidationError.Error()
	}

	return &types.AllocationPlanView{
		ID:                  fmt.Sprintf("%s-%v", plan.RunnerID, plan.GPUs),
		RunnerID:            plan.RunnerID,
		GPUs:                plan.GPUs,
		GPUCount:            plan.GPUCount,
		IsMultiGPU:          plan.IsMultiGPU,
		TotalMemoryRequired: plan.TotalMemoryRequired,
		MemoryPerGPU:        plan.MemoryPerGPU,
		Cost:                plan.Cost,
		RequiresEviction:    plan.RequiresEviction,
		EvictionsNeeded:     evictionIDs,
		TensorParallelSize:  plan.TensorParallelSize,
		Runtime:             plan.Runtime,
		IsValid:             plan.IsValid,
		ValidationError:     validationError,
		RunnerMemoryState:   runnerMemoryState,
		RunnerCapacity:      runnerCapacity,
	}
}

// logGlobalDecision logs a comprehensive global allocation decision
func (ga *GlobalAllocator) logGlobalDecision(work *Workload, allPlans []*AllocationPlan, selectedPlan *AllocationPlan, beforeState, afterState map[string]*types.RunnerStateView, totalTimeMs int64, success bool, reason string) {
	if ga.globalDecisionsTracker == nil {
		return
	}

	// Convert all plans to view format
	var consideredPlans []*types.AllocationPlanView
	for _, plan := range allPlans {
		consideredPlans = append(consideredPlans, ga.convertPlanToView(plan))
	}

	var selectedPlanView *types.AllocationPlanView
	if selectedPlan != nil {
		selectedPlanView = ga.convertPlanToView(selectedPlan)
	}

	// Calculate optimization score
	optimizationScore := 0.0
	if success && selectedPlan != nil && len(allPlans) > 0 {
		minCost := selectedPlan.Cost
		maxCost := selectedPlan.Cost
		for _, plan := range allPlans {
			if plan.Cost < minCost {
				minCost = plan.Cost
			}
			if plan.Cost > maxCost {
				maxCost = plan.Cost
			}
		}
		if maxCost > minCost {
			optimizationScore = 1.0 - float64(selectedPlan.Cost-minCost)/float64(maxCost-minCost)
		} else {
			optimizationScore = 1.0
		}
	}

	sessionID := ""
	if work.session != nil {
		sessionID = work.session.ID
	}

	decision := &types.GlobalAllocationDecision{
		WorkloadID:      work.ID(),
		SessionID:       sessionID,
		ModelName:       work.ModelName().String(),
		Runtime:         work.Runtime(),
		ConsideredPlans: consideredPlans,
		SelectedPlan:    selectedPlanView,
		PlanningTimeMs:  totalTimeMs,
		ExecutionTimeMs: 0, // Will be updated if execution succeeds
		TotalTimeMs:     totalTimeMs,
		Success:         success,
		Reason:          reason,
		ErrorMessage: func() string {
			if !success {
				return reason
			}
			return ""
		}(),
		BeforeState:           beforeState,
		AfterState:            afterState,
		TotalRunnersEvaluated: len(ga.controller.RunnerIDs()),
		TotalPlansGenerated:   len(allPlans),
		OptimizationScore:     optimizationScore,
	}

	ga.globalDecisionsTracker.LogGlobalDecision(decision)
}

// DebugAllocationDecision logs detailed information about an allocation decision
func (ga *GlobalAllocator) DebugAllocationDecision(work *Workload, plan *AllocationPlan) {
	memoryState := ga.GetGlobalMemoryState()

	log.Info().
		Str("🐛 DEBUG", "allocation_decision").
		Str("model", work.ModelName().String()).
		Str("runtime", string(work.Runtime())).
		Interface("global_memory_state_gb", func() map[string]map[string]uint64 {
			result := make(map[string]map[string]uint64)
			for runner, gpus := range memoryState {
				result[runner] = make(map[string]uint64)
				for gpu, mem := range gpus {
					result[runner][fmt.Sprintf("gpu_%d", gpu)] = mem / (1024 * 1024 * 1024)
				}
			}
			return result
		}()).
		Interface("selected_plan", map[string]interface{}{
			"runner_id":         plan.RunnerID,
			"gpus":              plan.GPUs,
			"gpu_count":         plan.GPUCount,
			"memory_per_gpu_gb": plan.MemoryPerGPU / (1024 * 1024 * 1024),
			"total_memory_gb":   plan.TotalMemoryRequired / (1024 * 1024 * 1024),
			"requires_eviction": plan.RequiresEviction,
			"cost":              plan.Cost,
		}).
		Msg("🐛 GLOBAL: Allocation decision details")
}
