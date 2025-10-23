# Critical Memory Calculation Inconsistency Bug

## Summary

The GPU scheduler has a critical bug where it uses **different memory calculation methods** for single-GPU vs multi-GPU allocations. This inconsistency can lead to overscheduling where more memory is allocated to a GPU than it physically has.

**Current Issue**: 128.175 GiB allocated to an 80 GiB GPU (60% overscheduling)

## The Bug

### Location: `scheduler.go:tryEvictionForAllocation()` lines 2051-2102

The function uses **inconsistent memory validation logic**:

#### Single GPU Allocations (Lines 2051-2078)
```go
if option.GPUCount == 1 {
    // ✅ CORRECT: Uses per-GPU memory calculation
    allocatedMemoryPerGPU, err := s.controller.calculateAllocatedMemoryPerGPU(runnerID)
    
    // ✅ CORRECT: Checks individual GPU capacity
    targetGPUIndex := option.GPUs[0]
    allocatedOnTargetGPU := allocatedMemoryPerGPU[targetGPUIndex]
    freeOnTargetGPU := targetGPUTotal - allocatedOnTargetGPU
    hasEnoughMemory = freeOnTargetGPU >= option.MemoryPerGPU
}
```

#### Multi-GPU Allocations (Lines 2093-2102)
```go
else {
    // ❌ WRONG: Uses total runner memory calculation
    totalMem, currentAllocatedMem, currentFreeMem, err := s.calculateRunnerMemory(runnerID)
    
    // ❌ WRONG: Checks total memory across ALL GPUs
    memoryNeeded = option.TotalMemoryRequired
    hasEnoughMemory = currentFreeMem >= memoryNeeded
}
```

## Memory Calculation Methods

### Method 1: `calculateAllocatedMemoryPerGPU()` (Per-GPU)
```go
// Returns: map[gpuIndex]allocatedMemory
allocatedMemoryPerGPU := make(map[int]uint64)
for each slot {
    if slot.MultiGPUs:
        memoryPerGPU := modelMemory / numGPUs
        for each gpu in MultiGPUs:
            allocatedMemoryPerGPU[gpu] += memoryPerGPU
    else:
        allocatedMemoryPerGPU[singleGPU] += modelMemory
}
```

### Method 2: `calculateRunnerMemory()` (Total)
```go
// Returns: totalMemory, totalAllocated, totalFree
totalMemory := runnerStatus.TotalMemory  // Sum of ALL GPU memory
allocatedMemory := 0
for each slot {
    allocatedMemory += slot.model.GetMemoryForAllocation()  // Total across all GPUs
}
freeMemory := totalMemory - allocatedMemory
```

## Why This Causes Overscheduling

### Scenario: Runner with 2×80GB GPUs

**Current Broken Logic:**
1. **Single GPU models**: Use per-GPU calculations (✅ correct)
   - Check: `allocatedOnGPU + modelMemory <= 80GB`
   
2. **Multi-GPU models**: Use total calculations (❌ wrong)
   - Check: `totalAllocated + modelMemory <= 160GB`
   - **Missing check**: Each individual GPU constraint

**The Problem:**
A model marked as "multi-GPU" can pass the total memory check while violating individual GPU constraints.

### Example Overscheduling Sequence

```
Initial State: 2×80GB GPUs, 160GB total
├── GPU 0: 0GB allocated  
└── GPU 1: 0GB allocated

Step 1: Allocate qwen3:30b (74.49GB) as single GPU
├── Uses per-GPU calculation: 74.49GB < 80GB ✅
├── Selects GPU 1 (arbitrary choice)
└── Result: GPU 0: 0GB, GPU 1: 74.49GB

Step 2: Allocate qwen3:8b (14.685GB) 
├── Algorithm tries single GPU first
├── GPU 0: 0GB + 14.685GB = 14.685GB < 80GB ✅
├── GPU 1: 74.49GB + 14.685GB = 89.175GB > 80GB ❌
├── BUT: If somehow GPU 1 is selected anyway...
└── Result: GPU 0: 0GB, GPU 1: 89.175GB

Step 3: Allocate Qwen2.5-VL-7B (39GB)
├── If algorithm somehow selects GPU 1 again
└── Result: GPU 0: 0GB, GPU 1: 128.175GB (OVERSCHEDULED!)
```

## Root Cause Analysis

The overscheduling suggests **one of these scenarios**:

### Scenario A: GPU Selection Bug
The GPU selection algorithm (`GetOptimalGPUAllocation`) consistently selects the same GPU despite memory constraints.

### Scenario B: Memory State Inconsistency
The `calculateAllocatedMemoryPerGPU()` function doesn't immediately see newly created slots, causing stale memory calculations.

### Scenario C: Multi-GPU Logic Error
Models are incorrectly being allocated as multi-GPU when they should be single-GPU, and the broken multi-GPU validation logic allows overscheduling.

## Investigation Steps

### 1. Check GPU Selection Logic
```bash
# Look for logs showing GPU selection decisions
grep "Selected GPU\|selected_gpu" /var/log/helix/scheduler.log
```

### 2. Verify Memory Calculation Consistency
```bash
# Check if calculateAllocatedMemoryPerGPU sees new slots immediately
grep "allocated_memory_per_gpu\|GPU.*allocated" /var/log/helix/scheduler.log
```

### 3. Trace Allocation Decisions
```bash
# Look for allocation option decisions
grep "allocation.*option\|AllocationOption" /var/log/helix/scheduler.log
```

## The Fix

### Immediate Fix: Consistent Memory Validation

**Change multi-GPU validation to use per-GPU checks:**

```go
// In tryEvictionForAllocation(), replace lines 2093-2102:
else {
    // ✅ FIX: Use per-GPU memory calculation for multi-GPU too
    allocatedMemoryPerGPU, err := s.controller.calculateAllocatedMemoryPerGPU(runnerID)
    if err != nil {
        return nil, fmt.Errorf("failed to calculate per-GPU memory: %w", err)
    }

    status, err := s.controller.GetStatus(runnerID)
    if err != nil {
        return nil, fmt.Errorf("failed to get runner status: %w", err)
    }

    // Check EACH GPU individually
    memoryPerGPU := option.TotalMemoryRequired / uint64(option.GPUCount)
    hasEnoughMemory = true
    
    for _, gpuIndex := range option.GPUs {
        for _, gpu := range status.GPUs {
            if gpu.Index == gpuIndex {
                allocatedOnGPU := allocatedMemoryPerGPU[gpuIndex]
                freeOnGPU := gpu.TotalMemory - allocatedOnGPU
                if freeOnGPU < memoryPerGPU {
                    hasEnoughMemory = false
                    break
                }
            }
        }
        if !hasEnoughMemory { break }
    }
}
```

### Long-term Fix: Unified Memory Validation

Create a single `validateGPUAllocation()` function that:
1. Takes an allocation option and memory requirement
2. Validates ALL GPUs in the allocation individually  
3. Is used by both single-GPU and multi-GPU code paths
4. Includes extensive logging for debugging

## Verification

After implementing the fix:

1. **Test with the current scenario**: Deploy 3 models to a 2×80GB runner
2. **Verify GPU distribution**: Models should be distributed across GPUs
3. **Check logs**: Memory calculations should show per-GPU validation
4. **Monitor alerts**: No overscheduling warnings should occur

## Critical Questions

1. **Why are all 3 models on GPU 1?** Is the GPU selection algorithm broken?
2. **Are these single-GPU or multi-GPU allocations?** Check the `AllocationOption.GPUCount`
3. **When do memory calculations update?** Is there a timing issue with slot creation?

The fix above addresses the inconsistent validation logic, but the root cause of why all models selected the same GPU needs separate investigation.