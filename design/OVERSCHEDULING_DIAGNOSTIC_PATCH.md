# Overscheduling Diagnostic Patch

This patch adds extensive logging to help diagnose the overscheduling issue where 128.175 GiB is allocated to an 80 GiB GPU.

## Files to Modify

### 1. scheduler.go - Add memory state logging

```diff
--- a/api/pkg/scheduler/scheduler.go
+++ b/api/pkg/scheduler/scheduler.go
@@ -1430,6 +1430,19 @@ func (s *Scheduler) ensureSlot(req SlotRequirement) {
 		// Try all possible allocations with eviction to find the optimal solution
 		allocationResult, err := s.tryAllAllocationsWithEviction(runnerID, req.ExampleWorkload)
 		if err != nil {
+			// 🚨 DEBUG: Log allocation failure
+			log.Error().
+				Str("🚨 OVERSCHEDULING_DEBUG", "allocation_failed").
+				Str("runner_id", runnerID).
+				Str("model", req.ExampleWorkload.ModelName().String()).
+				Uint64("model_memory_gb", req.ExampleWorkload.model.Memory/(1024*1024*1024)).
+				Err(err).
+				Msg("🚨 FAILED to find allocation for model")
+			
+			// Log current GPU memory state when allocation fails
+			if allocatedPerGPU, memErr := s.controller.calculateAllocatedMemoryPerGPU(runnerID); memErr == nil {
+				log.Error().Interface("current_allocated_per_gpu_gb", mapToGB(allocatedPerGPU)).Msg("🚨 GPU memory state when allocation failed")
+			}
 			lastErr = err
 			withWorkContext(&log.Logger, req.ExampleWorkload).Debug().
 				Err(err).
@@ -1438,6 +1451,31 @@ func (s *Scheduler) ensureSlot(req SlotRequirement) {
 			continue // Try next runner
 		}
 
+		// 🚨 DEBUG: Log successful allocation decision
+		log.Error().
+			Str("🚨 OVERSCHEDULING_DEBUG", "allocation_success").
+			Str("runner_id", runnerID).
+			Str("model", req.ExampleWorkload.ModelName().String()).
+			Uint64("model_memory_gb", req.ExampleWorkload.model.Memory/(1024*1024*1024)).
+			Int("selected_gpu_count", allocationResult.AllocationOption.GPUCount).
+			Ints("selected_gpus", allocationResult.AllocationOption.GPUs).
+			Uint64("memory_per_gpu_gb", allocationResult.AllocationOption.MemoryPerGPU/(1024*1024*1024)).
+			Uint64("total_memory_required_gb", allocationResult.AllocationOption.TotalMemoryRequired/(1024*1024*1024)).
+			Msg("🚨 FOUND allocation for model")
+
+		// Log GPU memory state before creating slot
+		if allocatedPerGPU, err := s.controller.calculateAllocatedMemoryPerGPU(runnerID); err == nil {
+			log.Error().
+				Str("🚨 OVERSCHEDULING_DEBUG", "memory_before_slot_creation").
+				Interface("allocated_per_gpu_gb", mapToGB(allocatedPerGPU)).
+				Msg("🚨 GPU memory state BEFORE creating slot")
+		}
+
 		// Create GPU allocation config from the optimal allocation found
 		allocation := GPUAllocationConfig{
 			GPUCount:     allocationResult.AllocationOption.GPUCount,
@@ -1495,6 +1533,16 @@ func (s *Scheduler) ensureSlot(req SlotRequirement) {
 
 		s.slots.Store(slot.ID, slot)
 
+		// 🚨 DEBUG: Log GPU memory state after creating slot
+		if allocatedPerGPU, err := s.controller.calculateAllocatedMemoryPerGPU(runnerID); err == nil {
+			log.Error().
+				Str("🚨 OVERSCHEDULING_DEBUG", "memory_after_slot_creation").
+				Str("slot_id", slot.ID.String()).
+				Interface("allocated_per_gpu_gb", mapToGB(allocatedPerGPU)).
+				Msg("🚨 GPU memory state AFTER creating slot")
+		}
+
 		// Log successful slot creation with configured model memory
 		log.Debug().
 			Str("🐛 MODEL_MEMORY_DEBUG", "slot_creation_success").
@@ -2095,6 +2143,17 @@ func (s *Scheduler) tryEvictionForAllocation(runnerID string, work *Workload, o
 			freeMem = int64(freeOnTargetGPU) - int64(memoryNeeded)
 			hasEnoughMemory = freeMem >= 0
 
+			// 🚨 DEBUG: Log single GPU validation decision
+			log.Error().
+				Str("🚨 OVERSCHEDULING_DEBUG", "single_gpu_validation").
+				Str("runner_id", runnerID).
+				Int("target_gpu", targetGPUIndex).
+				Uint64("gpu_total_gb", targetGPUTotal/(1024*1024*1024)).
+				Uint64("gpu_allocated_gb", allocatedOnTargetGPU/(1024*1024*1024)).
+				Uint64("gpu_free_gb", freeOnTargetGPU/(1024*1024*1024)).
+				Uint64("required_gb", option.MemoryPerGPU/(1024*1024*1024)).
+				Bool("has_enough", hasEnoughMemory).
+				Int64("free_after_allocation_mb", freeMem/(1024*1024)).
+				Msg("🚨 Single GPU memory validation")
+
 			log.Debug().
 				Str("runner_id", runnerID).
 				Int("target_gpu", targetGPUIndex).
@@ -2106,6 +2165,16 @@ func (s *Scheduler) tryEvictionForAllocation(runnerID string, work *Workload, o
 				Msg("Checking single GPU memory for allocation")
 		} else {
 			// Multi-GPU allocation: check total memory across allocated GPUs
+			// 🚨 DEBUG: Log multi-GPU validation decision
+			log.Error().
+				Str("🚨 OVERSCHEDULING_DEBUG", "multi_gpu_validation").
+				Str("runner_id", runnerID).
+				Ints("target_gpus", option.GPUs).
+				Uint64("total_free_gb", currentFreeMem/(1024*1024*1024)).
+				Uint64("required_gb", option.TotalMemoryRequired/(1024*1024*1024)).
+				Bool("uses_total_memory_check", true).
+				Msg("🚨 Multi-GPU using TOTAL memory validation (potential bug!)")
+
 			memoryNeeded = option.TotalMemoryRequired
 			freeMem = int64(currentFreeMem) - int64(memoryNeeded)
 			hasEnoughMemory = freeMem >= 0
@@ -3259,6 +3328,15 @@ func (s *Scheduler) selectModelsByMemory(models []types.ModelName, availableMem
 	}
 	return selectedModels
 }
+
+// 🚨 DEBUG: Helper function to convert memory map to GB for logging
+func mapToGB(memoryPerGPU map[int]uint64) map[int]uint64 {
+	result := make(map[int]uint64)
+	for gpu, memory := range memoryPerGPU {
+		result[gpu] = memory / (1024 * 1024 * 1024)
+	}
+	return result
+}
```

### 2. runner.go - Add GPU selection logging

```diff
--- a/api/pkg/scheduler/runner.go  
+++ b/api/pkg/scheduler/runner.go
@@ -750,6 +750,14 @@ func (c *RunnerController) GetAllPossibleGPUAllocations(runnerID string, modelM
 		Str("runtime", string(runtime)).
 		Msg("DEBUG: GetAllPossibleGPUAllocations - checking single GPU options")
 
+	// 🚨 DEBUG: Log initial memory state
+	log.Error().
+		Str("🚨 OVERSCHEDULING_DEBUG", "gpu_allocation_start").
+		Str("runner_id", runnerID).
+		Uint64("model_memory_gb", modelMemoryRequirement/(1024*1024*1024)).
+		Interface("current_allocated_per_gpu_gb", mapToGB(allocatedMemoryPerGPU)).
+		Msg("🚨 Starting GPU allocation - current memory state")
+
 	for _, gpu := range status.GPUs {
 		allocatedMemory := allocatedMemoryPerGPU[gpu.Index]
 		freeMemory := gpu.TotalMemory - allocatedMemory
@@ -790,6 +798,15 @@ func (c *RunnerController) GetAllPossibleGPUAllocations(runnerID string, modelM
 			bestGPU = &idx
 
 			log.Debug().
+			// 🚨 DEBUG: Log GPU selection decision
+			log.Error().
+				Str("🚨 OVERSCHEDULING_DEBUG", "gpu_selected").
+				Str("runner_id", runnerID).
+				Int("selected_gpu", idx).
+				Uint64("gpu_free_gb", freeMemory/(1024*1024*1024)).
+				Uint64("model_requirement_gb", modelMemoryRequirement/(1024*1024*1024)).
+				Msg("🚨 Selected GPU for single GPU allocation")
+
 				Str("runner_id", runnerID).
 				Int("selected_gpu", idx).
 				Uint64("free_memory_gb", freeMemory/(1024*1024*1024)).
@@ -1010,6 +1027,15 @@ func (c *RunnerController) calculateAllocatedMemoryPerGPU(runnerID string) (map
 	log.Trace().
 		Str("runner_id", runnerID).
 		Int("total_scheduler_slots", len(schedulerSlots)).
+		
+	// 🚨 DEBUG: Log slot counting
+	log.Error().
+		Str("🚨 OVERSCHEDULING_DEBUG", "calculating_allocated_memory").
+		Str("runner_id", runnerID).
+		Int("total_scheduler_slots", len(schedulerSlots)).
+		Msg("🚨 Calculating allocated memory per GPU")
+
 		Msg("Using scheduler's desired state for memory calculation")
 
 	for slotID, slot := range schedulerSlots {
@@ -1048,6 +1074,18 @@ func (c *RunnerController) calculateAllocatedMemoryPerGPU(runnerID string) (map
 		// Allocate memory to the appropriate GPU(s) based on slot's GPU allocation
 		if slot.GPUAllocation != nil && len(slot.GPUAllocation.MultiGPUs) > 1 {
 			// Multi-GPU model: distribute memory across GPUs
+			// 🚨 DEBUG: Log multi-GPU allocation
+			log.Error().
+				Str("🚨 OVERSCHEDULING_DEBUG", "multi_gpu_slot_found").
+				Str("slot_id", slotID.String()).
+				Str("model", slot.InitialWork().ModelName().String()).
+				Uint64("total_memory_gb", modelMemory/(1024*1024*1024)).
+				Ints("allocated_gpus", slot.GPUAllocation.MultiGPUs).
+				Uint64("memory_per_gpu_gb", (modelMemory/uint64(len(slot.GPUAllocation.MultiGPUs)))/(1024*1024*1024)).
+				Msg("🚨 Found multi-GPU slot - distributing memory")
+
 			memoryPerGPU := modelMemory / uint64(len(slot.GPUAllocation.MultiGPUs))
 			for _, gpuIndex := range slot.GPUAllocation.MultiGPUs {
 				allocatedMemoryPerGPU[gpuIndex] += memoryPerGPU
@@ -1055,6 +1093,17 @@ func (c *RunnerController) calculateAllocatedMemoryPerGPU(runnerID string) (map
 		} else if slot.GPUAllocation != nil && slot.GPUAllocation.SingleGPU != nil {
 			// Single GPU model: allocate full memory to this GPU
 			allocatedMemoryPerGPU[*slot.GPUAllocation.SingleGPU] += modelMemory
+			
+			// 🚨 DEBUG: Log single-GPU allocation
+			log.Error().
+				Str("🚨 OVERSCHEDULING_DEBUG", "single_gpu_slot_found").
+				Str("slot_id", slotID.String()).
+				Str("model", slot.InitialWork().ModelName().String()).
+				Uint64("memory_gb", modelMemory/(1024*1024*1024)).
+				Int("allocated_gpu", *slot.GPUAllocation.SingleGPU).
+				Uint64("new_total_on_gpu_gb", allocatedMemoryPerGPU[*slot.GPUAllocation.SingleGPU]/(1024*1024*1024)).
+				Msg("🚨 Found single-GPU slot - allocating to GPU")
 		}
 		// CPU-only slots (no GPU allocation) don't count toward GPU memory
 	}
@@ -1062,6 +1111,12 @@ func (c *RunnerController) calculateAllocatedMemoryPerGPU(runnerID string) (map
 	log.Trace().
 		Str("runner_id", runnerID).
 		Interface("allocated_memory_per_gpu", allocatedMemoryPerGPU).
+		
+	// 🚨 DEBUG: Log final memory calculation result
+	log.Error().
+		Str("🚨 OVERSCHEDULING_DEBUG", "memory_calculation_result").
+		Interface("allocated_per_gpu_gb", mapToGB(allocatedMemoryPerGPU)).
+		Msg("🚨 Final allocated memory per GPU calculation")
+
 		Msg("Calculated allocated memory per GPU (skipped slots with unknown memory requirements)")
 
 	return allocatedMemoryPerGPU, nil
```

### 3. Add helper function at end of runner.go

```diff
+// 🚨 DEBUG: Helper function to convert memory map to GB for logging
+func mapToGB(memoryPerGPU map[int]uint64) map[int]uint64 {
+	result := make(map[int]uint64)
+	for gpu, memory := range memoryPerGPU {
+		result[gpu] = memory / (1024 * 1024 * 1024)
+	}
+	return result
+}
```

## How to Use This Patch

1. **Apply the patch** to your scheduler code
2. **Restart the scheduler** with increased log level (ERROR level to see the 🚨 debug messages)
3. **Trigger the overscheduling scenario** by enqueueing the same 3 models
4. **Collect the logs** and look for lines containing `🚨 OVERSCHEDULING_DEBUG`

## What This Will Tell Us

### Memory State Tracking
- **Before each allocation**: What the GPU memory state looks like
- **After each allocation**: How the memory state changes
- **Calculation details**: Which slots are counted and how

### GPU Selection Process
- **Which GPU is selected** for each model
- **Why that GPU was selected** (free memory amounts)
- **Single vs Multi-GPU decisions**

### Validation Logic
- **Which validation path** is taken (single-GPU vs multi-GPU)
- **Memory calculations** used for validation
- **Why validation passes** when it should fail

## Expected Output

With this patch, you should see a sequence like:

```
🚨 Starting GPU allocation - current memory state: {0: 0, 1: 0}
🚨 Selected GPU for single GPU allocation: GPU 1 
🚨 Single GPU memory validation: has_enough=true
🚨 GPU memory state BEFORE creating slot: {0: 0, 1: 0}
🚨 Found single-GPU slot - allocating to GPU: GPU 1
🚨 GPU memory state AFTER creating slot: {0: 0, 1: 74}

[Next model]
🚨 Starting GPU allocation - current memory state: {0: 0, 1: 74}  
🚨 Selected GPU for single GPU allocation: GPU 0  ← Should select GPU 0!
```

If instead you see GPU 1 being selected repeatedly despite high allocation, that's the bug.

## Key Questions This Answers

1. **Are all models actually selecting GPU 1?** Or is there a logging/reporting issue?
2. **Is the memory calculation seeing previous slots?** Check if allocated memory increases after each slot
3. **Single-GPU vs Multi-GPU?** Are models being allocated as single or multi-GPU?
4. **When does validation fail?** At what point should the system reject an allocation?

Run this and share the logs - it will pinpoint exactly where the overscheduling logic breaks down.