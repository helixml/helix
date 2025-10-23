# GPU Allocation Architecture Analysis

## Current Allocation Method Landscape

The scheduler has evolved to have **multiple overlapping allocation methods** that create confusion and potential bugs. Here's the current landscape:

### 1. Legacy Method: `GetOptimalGPUAllocation()` (runner.go:647)
**Purpose**: Original simple allocation logic
**Input**: `(runnerID, modelMemoryRequirement, runtime)`
**Output**: `(singleGPU *int, multiGPUs []int, tensorParallelSize int)`

```
GetOptimalGPUAllocation()
├── calculateAllocatedMemoryPerGPU()
├── Try single GPU: CanFitModelOnAnyGPUAllocated()
│   └── Select GPU with most free memory
└── Try multi-GPU: CanFitModelOnMultipleGPUsAllocated()
    └── Find enough GPUs for tensor parallelism
```

**Characteristics**:
- Simple, direct allocation
- No eviction logic
- Returns specific GPU indices
- Used by older code paths

### 2. Options-Based Method: `GetAllPossibleGPUAllocations()` (runner.go:734)
**Purpose**: Generate all viable allocation strategies
**Input**: `(runnerID, modelMemoryRequirement, runtime)`
**Output**: `[]AllocationOption`

```
GetAllPossibleGPUAllocations()
├── Try single GPU allocation
│   └── Find best GPU, create AllocationOption
└── Try all multi-GPU combinations (2, 3, 4... GPUs)
    └── Create AllocationOption for each viable combination
```

**AllocationOption Structure**:
```go
type AllocationOption struct {
    GPUCount            int      // 1, 2, 4, 8...
    GPUs                []int    // [0], [0,1], [0,1,2,3]
    MemoryPerGPU        uint64   // Memory needed per GPU
    TotalMemoryRequired uint64   // Total memory across all GPUs
    TensorParallelSize  int      // Tensor parallel group size
}
```

### 3. Eviction-Aware Method: `GetAllPossibleGPUAllocationsWithEviction()` (runner.go:866)
**Purpose**: Same as above, but considers evictable memory
**Input**: `(runnerID, modelMemoryRequirement, runtime, evictableMemoryPerGPU)`
**Output**: `[]AllocationOption`

```
GetAllPossibleGPUAllocationsWithEviction()
├── For each GPU: availableMemory = totalMemory - allocated + evictable
├── Try single GPU with expanded available memory
└── Try multi-GPU with expanded available memory
```

### 4. Eviction-Based Legacy: `getOptimalGPUAllocationWithEviction()` (scheduler.go:1910)
**Purpose**: Legacy method with eviction awareness
**Input**: `(runnerID, modelMemoryRequirement, runtime, evictableMemoryPerGPU)`
**Output**: `(singleGPU *int, multiGPUs []int, tensorParallelSize int)`

```
getOptimalGPUAllocationWithEviction()
├── Try single GPU with eviction potential
└── Try multi-GPU with eviction potential (VLLM only)
```

### 5. Comprehensive Strategy: `tryAllAllocationsWithEviction()` (scheduler.go:2015)
**Purpose**: New architecture - tries all options with actual eviction
**Input**: `(runnerID, work *Workload)`
**Output**: `*AllocationResult`

```
tryAllAllocationsWithEviction()
├── GetAllPossibleGPUAllocations() (try without eviction)
├── If no options: GetAllPossibleGPUAllocationsWithEviction()
├── For each option: tryEvictionForAllocation()
│   └── Actually evict slots until memory available
└── Return best successful allocation
```

## Architecture Problems

### Problem 1: Method Proliferation
**Issue**: 5 different allocation methods with overlapping responsibilities
- `GetOptimalGPUAllocation` vs `GetAllPossibleGPUAllocations`
- `getOptimalGPUAllocationWithEviction` vs `GetAllPossibleGPUAllocationsWithEviction`
- Different input/output formats

### Problem 2: Inconsistent Memory Calculations
**Issue**: Different methods use different memory validation approaches
- Single GPU: Uses per-GPU memory checks ✅
- Multi-GPU: Uses total memory checks ❌ (BUG!)

### Problem 3: Model Configuration State Confusion
**Issue**: Methods expect different model configuration states
- Some expect unconfigured models (with `model.Memory`)
- Some expect configured models (with `model.GetMemoryForAllocation()`)
- Ollama models have `Memory = 0`, requiring GGUF estimation

### Problem 4: Allocation vs Execution Split
**Issue**: Some methods plan allocations, others execute them
- `GetAllPossible*` methods: Return options
- `tryEvictionForAllocation`: Actually performs eviction
- `getOptimal*` methods: Return final decisions

## Current Flow Analysis

### Main Flow (ensureSlot)
```
ensureSlot(req)
├── tryAllAllocationsWithEviction(runnerID, work)  ← NEW METHOD
│   ├── GetAllPossibleGPUAllocations() ← OPTIONS METHOD
│   ├── GetAllPossibleGPUAllocationsWithEviction() ← OPTIONS METHOD  
│   └── tryEvictionForAllocation() ← EXECUTION METHOD
```

### Alternative Flow (Legacy)
```
Some code paths still use:
├── GetOptimalGPUAllocation() ← LEGACY METHOD
└── getOptimalGPUAllocationWithEviction() ← LEGACY METHOD
```

## Root Cause of Current Bug

Based on the logs and code analysis:

### The Memory Value Bug
```go
// In tryAllAllocationsWithEviction:
allocationOptions, err = s.controller.GetAllPossibleGPUAllocations(runnerID, work.model.Memory, work.Runtime())
//                                                                          ^^^^^^^^^^^^ 
//                                                                          For Ollama: 0
//                                                                          Should be: GetMemoryForAllocation()
```

**For Ollama models**:
- `work.model.Memory` = 0 (database value)
- `work.model.GetMemoryForAllocation()` = 74GB (GGUF estimate)
- Code uses `Memory` → passes 0 to allocation logic → validation always passes

### The Architecture Issue
The confusion stems from having **two different model states**:

1. **Unconfigured Model** (from database/queue):
   - VLLM: `Memory` = admin value (e.g., 30GB)
   - Ollama: `Memory` = 0, need GGUF estimation

2. **Configured Model** (after allocation decision):
   - Both runtimes: `AllocatedMemory` = computed value
   - Access via `GetMemoryForAllocation()`

The new architecture expects **unconfigured models**, but the memory calculation logic assumes **configured models**.

## Recommended Architecture Simplification

### Phase 1: Fix Immediate Bug
```go
// In tryAllAllocationsWithEviction, calculate proper memory requirement:
var memoryRequirement uint64
if work.model.IsAllocationConfigured() {
    memoryRequirement = work.model.GetMemoryForAllocation()
} else {
    // For unconfigured models, estimate memory upfront
    switch work.Runtime() {
    case types.RuntimeVLLM:
        memoryRequirement = work.model.Memory
    case types.RuntimeOllama:
        // Get GGUF estimate
        result, err := s.memoryEstimationService.EstimateModelMemory(ctx, work.model.ID, options)
        memoryRequirement = result.SingleGPU.TotalSize
    }
}
```

### Phase 2: Unify Allocation Methods
Replace the 5 methods with **2 clear methods**:

1. **Planning**: `GetAllViableAllocations(runnerID, memoryReq, runtime, withEviction bool)`
2. **Execution**: `AllocateWithEviction(runnerID, work, allocationOption)`

### Phase 3: Consistent Memory Validation
Use **per-GPU memory checks** for both single and multi-GPU allocations:

```go
func validateAllocation(option AllocationOption, allocatedMemoryPerGPU map[int]uint64, gpuCapacities map[int]uint64) error {
    memoryPerGPU := option.TotalMemoryRequired / uint64(option.GPUCount)
    for _, gpu := range option.GPUs {
        if allocatedMemoryPerGPU[gpu] + memoryPerGPU > gpuCapacities[gpu] {
            return ErrGPUOverallocation{GPU: gpu}
        }
    }
    return nil
}
```

## Immediate Action Items

1. **Fix the memory value bug** in `tryAllAllocationsWithEviction`
2. **Add consistent memory validation** for multi-GPU allocations  
3. **Add comprehensive logging** to trace allocation decisions
4. **Test the fix** with the current overscheduling scenario

This architecture analysis reveals that the overscheduling bug is caused by **passing 0 memory requirement** for Ollama models, combined with **inconsistent validation logic** for multi-GPU allocations.