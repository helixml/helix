# GPU Allocation Methods Dead Code Analysis

## Current Method Landscape

The scheduler has **5 different GPU allocation methods** that have accumulated over time. Here's the usage analysis:

### Method Usage Summary

| Method | Production Usage | Test Usage | Status |
|--------|-----------------|------------|--------|
| `GetOptimalGPUAllocation()` | ✅ scheduler_filters.go | ✅ Multiple tests | **ACTIVE (Legacy)** |
| `getOptimalGPUAllocationWithEviction()` | ✅ scheduler_filters.go | ✅ Limited tests | **ACTIVE (Legacy)** |
| `GetAllPossibleGPUAllocations()` | ✅ tryAllAllocationsWithEviction | ✅ Multiple tests | **ACTIVE (New)** |
| `GetAllPossibleGPUAllocationsWithEviction()` | ✅ tryAllAllocationsWithEviction | ❌ No direct tests | **ACTIVE (New)** |
| `tryAllAllocationsWithEviction()` | ✅ ensureSlot (MAIN PATH) | ✅ Limited tests | **ACTIVE (New)** |

## Production Code Paths

### Main Scheduling Path (NEW ARCHITECTURE)
```
ensureSlot() [scheduler.go:1448]
└── tryAllAllocationsWithEviction() [scheduler.go:2015]
    ├── GetAllPossibleGPUAllocations() [runner.go:781]
    └── GetAllPossibleGPUAllocationsWithEviction() [runner.go:902]
```

### Runner Filtering Path (LEGACY ARCHITECTURE)
```
filterRunnersByMemory() [scheduler_filters.go:208,300,337]
├── GetOptimalGPUAllocation() [runner.go:647]
└── getOptimalGPUAllocationWithEviction() [scheduler.go:1910]
```

### Prewarming Path
```
PrewarmNewRunner() [scheduler.go:2991]
└── ensureSlot() → NEW ARCHITECTURE
```

## Dead Code Analysis

### ❌ NO DEAD CODE FOUND
**All 5 methods are actively used in production**, but they serve different purposes:

#### **Legacy Methods** (Still Used)
1. **`GetOptimalGPUAllocation()`**
   - **Used by**: `scheduler_filters.go` (runner filtering)
   - **Purpose**: Quick allocation check for runner filtering
   - **Returns**: Direct GPU assignment

2. **`getOptimalGPUAllocationWithEviction()`** 
   - **Used by**: `scheduler_filters.go` (runner filtering with eviction)
   - **Purpose**: Check if runner could handle workload with eviction
   - **Returns**: Direct GPU assignment

#### **New Methods** (Primary)
3. **`GetAllPossibleGPUAllocations()`**
   - **Used by**: `tryAllAllocationsWithEviction()` (main scheduling)
   - **Purpose**: Generate allocation options without eviction
   - **Returns**: Array of viable allocation strategies

4. **`GetAllPossibleGPUAllocationsWithEviction()`**
   - **Used by**: `tryAllAllocationsWithEviction()` (main scheduling)  
   - **Purpose**: Generate allocation options considering eviction potential
   - **Returns**: Array of viable allocation strategies

5. **`tryAllAllocationsWithEviction()`**
   - **Used by**: `ensureSlot()` (main scheduling path)
   - **Purpose**: Comprehensive allocation with actual eviction execution
   - **Returns**: Final allocation result with evicted slots

## Architecture Issues

### Issue 1: Two Parallel Architectures
The system runs **two different allocation architectures simultaneously**:

**Legacy Architecture** (scheduler_filters.go):
- Simple allocation methods
- Used for runner filtering
- Direct GPU assignment results
- No comprehensive eviction

**New Architecture** (ensureSlot):
- Options-based allocation methods
- Used for actual scheduling
- Multiple strategies evaluated
- Comprehensive eviction with validation

### Issue 2: Inconsistent Interfaces
```go
// Legacy interface
(singleGPU *int, multiGPUs []int, tensorParallelSize int)

// New interface  
[]AllocationOption{GPUCount, GPUs, MemoryPerGPU, TotalMemoryRequired, TensorParallelSize}
```

### Issue 3: Code Path Duplication
Both architectures implement similar logic:
- GPU selection algorithms
- Memory calculations
- Multi-GPU support
- Eviction awareness

## Consolidation Opportunities

### Phase 1: Immediate (Keep Both Architectures)
**NO DEAD CODE TO REMOVE** - all methods are used

**Fix Current Bug**:
- Fix memory value passing in `tryAllAllocationsWithEviction()`
- Fix multi-GPU memory validation in both architectures

### Phase 2: Medium Term (Unify Interfaces)
**Migrate legacy calls to use new architecture**:
```go
// Replace in scheduler_filters.go:
singleGPU, multiGPUs, _ := GetOptimalGPUAllocation(...)

// With:
options, _ := GetAllPossibleGPUAllocations(...)
bestOption := options[0] // Select first/best option
```

### Phase 3: Long Term (Single Architecture)
**Remove legacy methods after migration**:
1. Migrate `scheduler_filters.go` to use options-based methods
2. Remove `GetOptimalGPUAllocation()` and `getOptimalGPUAllocationWithEviction()`
3. Standardize on `AllocationOption` interface

## Current Bug Root Cause

The bug is NOT in dead code, but in the **memory value passing** between the two architectures:

```go
// NEW ARCHITECTURE (tryAllAllocationsWithEviction):
allocationOptions, err = s.controller.GetAllPossibleGPUAllocations(runnerID, work.model.Memory, work.Runtime())
//                                                                          ^^^^^^^^^^^^ 
//                                                                          BUG: For Ollama = 0
```

Both architectures are alive and well, but they have inconsistent memory handling for Ollama models.

## Recommendation

**DO NOT remove any methods yet** - they're all actively used. Instead:

1. **Fix the memory bug** in the new architecture first
2. **Add validation** to prevent overscheduling in both architectures  
3. **Plan migration** from legacy to new architecture
4. **Remove legacy methods** only after full migration and testing

The complexity is intentional - the system supports two allocation strategies for different use cases (filtering vs. scheduling).