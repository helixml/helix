# GPU Scheduler Design Review: Overscheduling Analysis

## Executive Summary

**Status: CRITICAL OVERSCHEDULING BUG IDENTIFIED**

The GPU scheduler is experiencing overscheduling where models are allocated more memory than physically available on GPUs. Current report shows **128.175 GiB allocated on an 80 GiB GPU**, representing a 60% overallocation that will cause GPU OOM crashes.

**Root Cause Categories:**
1. **Control Flow Bug** in ensureSlot() not properly exiting after slot creation
2. **GPU Selection Logic Bug** causing all models to select the same GPU
3. **Memory Calculation Inconsistency** in per-GPU tracking
4. **Timing Issues** in scheduler state updates

## Architecture Overview

### Core Components

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Scheduler     │    │ RunnerController│    │ Model Allocation│
│   scheduler.go  │◄──►│    runner.go    │◄──►│model_allocation.│
│                 │    │                 │    │      go         │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│     Queue       │    │  Memory Calc    │    │  GPU Tracking   │
│    queue.go     │    │calculateAllocated│    │ GPUAllocation   │
└─────────────────┘    │MemoryPerGPU     │    │                 │
                       └─────────────────┘    └─────────────────┘
```

### Key Data Structures

#### 1. **Model Configuration** (`model_allocation.go`)
```go
type GPUAllocationConfig struct {
    GPUCount     int      // Number of GPUs (1, 2, 4, 8...)
    SpecificGPUs []int    // Which GPU indices [0, 1, 3]
}

// CRITICAL: Models must be configured before scheduling
configuredModel := NewModelForGPUAllocation(baseModel, allocation, memoryService)
```

#### 2. **Slot Tracking** (`slot.go`)
```go
type Slot struct {
    ID            uuid.UUID
    RunnerID      string
    initialWork   *Workload        // Contains configured model
    GPUAllocation *GPUAllocation   // Runtime allocation info
}
```

#### 3. **Allocation Options** (`runner.go`)
```go
type AllocationOption struct {
    GPUCount            int      // 1, 2, 4...
    GPUs                []int    // [0], [0,1], [0,1,2,3]
    MemoryPerGPU        uint64   // Memory per GPU
    TotalMemoryRequired uint64   // Total across all GPUs
    TensorParallelSize  int      // For multi-GPU models
}
```

## Critical Code Paths

### 1. **Main Scheduling Flow** (`scheduler.go:1370-1550`)

```
ensureSlot(req) 
├── getSortedRunners() 
├── FOR each runner:
│   ├── tryAllAllocationsWithEviction()
│   │   ├── GetAllPossibleGPUAllocations() // Try without eviction
│   │   ├── calculateEvictableMemoryPerGPU() 
│   │   └── GetAllPossibleGPUAllocationsWithEviction()
│   ├── tryEvictionForAllocation() // ⚠️ CRITICAL: Per-GPU checks
│   ├── NewModelForGPUAllocation() // Configure model
│   └── NewSlot() // Create with GPUAllocation
```

### 2. **Memory Calculation** (`runner.go:983-1076`)

```
calculateAllocatedMemoryPerGPU(runnerID)
├── getSchedulerSlotsFn() // Get scheduler's desired state
├── FOR each slot on runner:
│   ├── Check model.IsAllocationConfigured()
│   ├── modelMemory = model.GetMemoryForAllocation()
│   └── IF MultiGPU: distribute across GPUs
│       ELSE: allocate to SingleGPU
```

### 3. **GPU Allocation Logic** (`runner.go:647-730`)

```
GetOptimalGPUAllocation(runnerID, modelMemory, runtime)
├── calculateAllocatedMemoryPerGPU() 
├── CanFitModelOnAnyGPUAllocated() // Single GPU check
│   └── FOR each GPU: freeMemory = totalMemory - allocatedMemory
└── CanFitModelOnMultipleGPUsAllocated() // Multi-GPU check
```

### 4. **Eviction Logic** (`scheduler.go:2038-2176`)

```
tryEvictionForAllocation(runnerID, work, option)
├── LOOP until memory available:
│   ├── calculateRunnerMemory() 
│   ├── IF single GPU: check per-GPU memory ⚠️ CRITICAL
│   ├── IF multi GPU: check total memory
│   ├── IF insufficient: find stale slots to evict
│   └── DELETE stale slot and retry
```

## Identified Issues

### 🚨 **CRITICAL Issue 1: Control Flow Bug in ensureSlot()**

**Location:** `scheduler.go:1527-1547` 

**Problem:** The ensureSlot() function has a critical control flow bug where it doesn't exit after successfully creating a slot.

```go
// BUGGY CODE:
for j, runnerID := range sortedRunners {
    // ... try allocation ...
    if successful {
        s.slots.Store(slot.ID, slot)
        slotCreated = true
        
        if !slotCreated {  // ← This is inside the loop!
            // error handling
        }
        // ← BUG: Loop continues to next runner instead of breaking!
    }
}
```

**Evidence:** 
- Missing break/return after successful slot creation
- Loop continues to try additional runners for same workload
- Could create multiple slots for same model on different runners

### 🚨 **CRITICAL Issue 2: GPU Selection Logic Bug**

**Location:** `runner.go:GetOptimalGPUAllocation()` and `GetAllPossibleGPUAllocations()`

**Problem:** GPU selection logic may consistently choose the same GPU instead of distributing across available GPUs.

```go
// POTENTIAL BUG: Always selects "best" GPU
for _, gpu := range status.GPUs {
    if freeMemory >= modelMemoryRequirement && freeMemory > maxFreeMemory {
        maxFreeMemory = freeMemory
        bestGPU = &gpu.Index  // ← Always picks GPU with most free memory
    }
}
```

**Risk:** If multiple models are scheduled in sequence, they all see the same "best" GPU and get allocated there, causing overscheduling on that GPU while other GPUs remain empty.

### 🚨 **CRITICAL Issue 3: Scheduler Single-Threaded but State Update Timing**

**Location:** `scheduler.go:reconcileSlotsOnce()` + memory calculation timing

**Problem:** Although scheduler is single-threaded, there may be timing issues between slot creation and memory state updates.

```go
// TIMING ISSUE:
// 1. reconcileSlotsOnce() calls ensureSlot() for model A
// 2. ensureSlot() calculates GPU memory (GPU 1 = 80GB free)
// 3. ensureSlot() creates slot for model A on GPU 1
// 4. reconcileSlotsOnce() calls ensureSlot() for model B
// 5. calculateAllocatedMemoryPerGPU() still sees GPU 1 as free?
```

**Risk:** Memory calculations may not immediately reflect newly created slots.

### ⚠️ **Issue 4: Memory Calculation Inconsistency**

**Locations:** 
- `scheduler.go:calculateRunnerMemory()` (total memory)
- `runner.go:calculateAllocatedMemoryPerGPU()` (per-GPU memory)

**Problem:** Different methods calculate memory differently, and there may be timing issues in state updates.

```go
// Method 1: scheduler.calculateRunnerMemory() - uses total allocated
allocatedMemory += slot.InitialWork().model.GetMemoryForAllocation()

// Method 2: runner.calculateAllocatedMemoryPerGPU() - distributes per GPU
if multiGPU {
    memoryPerGPU := modelMemory / uint64(len(multiGPUs))
    for _, gpuIndex := range multiGPUs {
        allocatedMemoryPerGPU[gpuIndex] += memoryPerGPU
    }
}
```

**Risk:** Total memory calculations might pass while per-GPU calculations fail, or state may not be immediately consistent.

### ⚠️ **Issue 5: Complex Model Configuration**

**Location:** `model_allocation.go:NewModelForGPUAllocation()`

**Problem:** Models can have memory from multiple sources:
- VLLM: `baseModel.Memory` (admin-configured)
- Ollama: GGUF estimation service  
- Configured: `model.AllocatedMemory`

**Risk:** Wrong memory source used in calculations leads to incorrect allocations.

### ⚠️ **Issue 6: GPU Allocation Storage Issues**

**Location:** `scheduler.go:storeGPUAllocation()` and `runner.go` per-GPU tracking

**Problem:** GPU allocation info stored in multiple places:
- `Scheduler.gpuAllocations` map
- `Slot.GPUAllocation` 
- `Model.AllocatedSpecificGPUs`

**Risk:** Inconsistency between storage locations leads to wrong allocation tracking.

## Specific Overscheduling Scenarios

### **Scenario A: Sequential Single-GPU Allocations (Most Likely)**
```
Reconcile Cycle 1: qwen3:30b (74.49 GiB)
├── calculateAllocatedMemoryPerGPU() → GPU 0: 0GB, GPU 1: 0GB  
├── GetOptimalGPUAllocation() → selects GPU 1 (arbitrary choice)
├── Create slot with qwen3:30b on GPU 1
└── s.slots.Store() updates scheduler state

Reconcile Cycle 2: qwen3:8b (14.685 GiB) 
├── calculateAllocatedMemoryPerGPU() → should see GPU 1: 74.49GB
├── BUT: Still selects GPU 1 (bug in memory calculation?)
└── Create slot with qwen3:8b on GPU 1

Reconcile Cycle 3: Qwen2.5-VL-7B (39 GiB)
├── calculateAllocatedMemoryPerGPU() → should see GPU 1: 89.175GB  
├── BUT: Still selects GPU 1 (critical bug!)
└── Result: 128.175 GiB on 80 GiB GPU
```

### **Scenario B: Multi-GPU Model Miscalculation**
```
If qwen3:30b was intended for tensor parallelism:
├── Expected: 74.49 GiB / 2 GPUs = 37.245 GiB per GPU
├── Actual: 74.49 GiB allocated entirely to GPU 1
└── Bug in GPUAllocation.SingleGPU vs MultiGPUs logic
```

### **Scenario C: Stale Memory State**
```
├── Old slots not properly cleaned up in calculateAllocatedMemoryPerGPU
├── New allocations see more free memory than actually available  
└── Scheduler makes decisions based on stale data
```

## Root Cause Analysis

### **Primary Suspects**

1. **Control Flow Bug in ensureSlot()** (`scheduler.go:1527-1547`)
   - Missing break/return after successful slot creation
   - Loop continues to try additional runners unnecessarily
   - May create duplicate slots (though not the primary issue here)

2. **GPU Selection Algorithm Bug** (`runner.go:GetOptimalGPUAllocation`)
   - Always selects "best" GPU instead of distributing load
   - May not properly account for recently allocated memory
   - Load balancing logic not working correctly

3. **Memory State Update Timing** (`runner.go:calculateAllocatedMemoryPerGPU`)
   - Newly created slots may not be immediately visible
   - State consistency issues between slot creation and next calculation

### **Secondary Contributing Factors**

1. **Missing Final Validation** - No sanity checks before slot creation to verify GPU won't be overscheduled
2. **Insufficient Logging** - GPU allocation decisions not clearly logged with before/after state
3. **Complex Eviction Logic** - Too many code paths increase bug surface area
4. **Model Configuration Complexity** - Multiple memory sources create confusion

## Immediate Action Items

### 🔥 **URGENT: Stop the Bleeding**

1. **Fix Control Flow Bug in ensureSlot()**
   ```go
   // In scheduler.go:ensureSlot() after slot creation:
   s.slots.Store(slot.ID, slot)
   slotCreated = true
   return  // ← ADD THIS: Exit immediately after successful creation
   ```

2. **Add Final Validation Check**
   ```go
   // In ensureSlot() before NewSlot():
   func (s *Scheduler) validateGPUAllocation(runnerID string, allocation GPUAllocationConfig, memory uint64) error {
       currentAllocated, _ := s.controller.calculateAllocatedMemoryPerGPU(runnerID)
       for _, gpuIdx := range allocation.SpecificGPUs {
           perGPUMemory := memory / uint64(len(allocation.SpecificGPUs))
           if currentAllocated[gpuIdx] + perGPUMemory > s.getGPUCapacity(runnerID, gpuIdx) {
               return fmt.Errorf("GPU %d would be overscheduled", gpuIdx)
           }
       }
       return nil
   }
   ```

3. **Add Extensive Debug Logging**
   ```go
   // Before every allocation decision:
   log.Error().
       Str("🚨 OVERSCHEDULING_DEBUG", "before_allocation").
       Interface("current_allocated_per_gpu", allocatedMemoryPerGPU).
       Uint64("model_memory_gb", modelMemory/(1024*1024*1024)).
       Msg("🚨 GPU state before allocation decision")
   ```

### 🔧 **SHORT TERM: Fix Core Issues**

1. **Fix GPU Selection Logic** 
   - Implement proper load balancing instead of always picking "best" GPU
   - Add round-robin or least-loaded GPU selection
   - Ensure memory calculations are immediately consistent

2. **Improve Memory State Management**
   - Ensure slot creation immediately updates memory calculations
   - Add validation that calculateAllocatedMemoryPerGPU sees new slots
   - Single source of truth for allocated memory per GPU

3. **Add Comprehensive Validation**
   - Validate every allocation decision before execution
   - Add per-GPU capacity checks at multiple points
   - Fail fast on any overscheduling attempt

### 📋 **MEDIUM TERM: Architectural Improvements**

1. **Redesign GPU Selection Algorithm**
   - Implement proper load balancing across GPUs
   - Add GPU affinity and placement policies
   - Consider GPU memory, compute utilization, and model characteristics

2. **Improve State Management**
   - Ensure immediate consistency of memory calculations
   - Add state validation at every step
   - Implement proper error recovery

3. **Add Comprehensive Testing**
   - Unit tests for GPU selection logic
   - Integration tests with multiple models on limited GPU memory
   - Sequential scheduling tests to verify load distribution

## Recommended Investigation Steps

### **Phase 1: Immediate Diagnosis** (TODAY)

1. **Check Current State**
   ```bash
   # Get current runner status
   curl -s http://api:8080/api/v1/runners/{runner-id}/status | jq '.gpus'
   
   # Check scheduler state  
   curl -s http://api:8080/api/v1/scheduler/decisions | jq '.[] | select(.runner_id == "{runner-id}")'
   ```

2. **Enable Debug Logging**
   ```go
   // In scheduler.go, add to all allocation methods:
   log.Error().
       Str("🚨 DEBUG_OVERSCHEDULING", "allocation_decision").
       Str("runner_id", runnerID).
       Interface("allocated_memory_per_gpu", allocatedMemoryPerGPU).
       Uint64("model_memory_gb", modelMemory/(1024*1024*1024)).
       Msg("🚨 ALLOCATION DECISION DEBUG")
   ```

3. **Check for Stale Slots**
   ```bash
   # Look for slots that should have been cleaned up
   curl -s http://api:8080/api/v1/scheduler/slots | jq '.[] | select(.runner_id == "{runner-id}") | {id, model, gpu_allocation, created}'
   ```

### **Phase 2: Root Cause Confirmation** (THIS WEEK)

1. **Add Memory Tracking**
   - Instrument every allocation decision
   - Track memory state before/after each operation
   - Add validation at every step

2. **Test Race Conditions**
   - Create test that makes concurrent allocation requests
   - Verify current behavior reproduces overscheduling
   - Test with mutex protection

3. **Audit GPU Selection Logic**
   - Verify GetOptimalGPUAllocation always returns valid results
   - Check if load balancing distributes properly
   - Ensure multi-GPU models use correct GPU indices

### **Phase 3: Fix Validation** (NEXT WEEK)

1. **Implement Atomic Reservations**
2. **Add Final Validation Checks** 
3. **Deploy with Extensive Monitoring**
4. **Verify Fix Under Load**

## Code Quality Assessment

### **Strengths**
- ✅ Comprehensive eviction logic
- ✅ Good separation of concerns
- ✅ Extensive logging framework
- ✅ Strong type safety with model configuration

### **Weaknesses**  
- ❌ **No concurrency protection** for allocation decisions
- ❌ **Multiple memory calculation paths** create inconsistency
- ❌ **Complex GPU allocation logic** increases bug surface area  
- ❌ **Insufficient validation** before final allocation
- ❌ **Race condition vulnerability** in high-load scenarios

### **Technical Debt**
- Memory calculation logic spread across multiple files
- GPU allocation stored in multiple places
- Runtime-specific logic mixed into scheduler
- Missing integration tests for allocation scenarios

## Conclusion

The overscheduling bug is **CRITICAL** and requires **IMMEDIATE ACTION**. The primary cause appears to be a **control flow bug** in ensureSlot() combined with **GPU selection logic issues** that cause all models to be allocated to the same GPU.

**Recommended Priority:**
1. 🔥 **URGENT**: Fix ensureSlot() control flow and add final validation
2. 🔧 **HIGH**: Fix GPU selection algorithm to properly distribute load
3. 📋 **MEDIUM**: Improve state management and add comprehensive testing

The scheduler is single-threaded (good!), but has critical bugs in control flow and GPU selection logic. The fixes are straightforward once the root causes are identified.

---

**Next Steps:** Implement the immediate action items above and test with the current overscheduling scenario to confirm the fix.