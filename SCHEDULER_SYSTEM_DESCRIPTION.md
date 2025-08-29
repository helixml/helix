# GPU Scheduler System Description (As-Is)

## System Overview

The Helix GPU scheduler is a single-threaded system that manages the allocation of GPU resources across multiple models and runners. It operates on a reconciliation pattern, where a main loop periodically examines the desired state (queue of work) against the actual state (existing slots) and takes actions to converge them.

### High-Level Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   API Layer     │    │   Scheduler     │    │ Runner Controller│
│                 │───►│   (scheduler.go)│◄──►│   (runner.go)   │
│ Enqueue Work    │    │                 │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                              │                        │
                              ▼                        ▼
                    ┌─────────────────┐    ┌─────────────────┐
                    │     Queue       │    │   Runners       │
                    │   (queue.go)    │    │  (Physical GPU  │
                    │                 │    │   Hardware)     │
                    └─────────────────┘    └─────────────────┘
```

## Core Components

### 1. Scheduler (`scheduler.go`)

**Purpose**: Central orchestrator that manages the lifecycle of GPU slots and workload assignment.

**Key Responsibilities**:
- Runs reconciliation loops in separate goroutines
- Maintains the authoritative state of slots (`s.slots`)
- Processes the work queue and creates slots as needed
- Handles GPU allocation decisions
- Manages slot lifecycle (creation, assignment, cleanup)

**Main Goroutines**:
- `reconcileSlots()`: Creates new slots based on queue requirements
- `processQueue()`: Assigns queued work to existing warm slots
- `reconcileActivity()`: Cleans up stale/inactive slots
- `reconcileRunners()`: Manages runner connections

### 2. Runner Controller (`runner.go`)

**Purpose**: Interface to physical GPU runners, providing allocation logic and runner state management.

**Key Responsibilities**:
- Maintains connections to physical GPU runners
- Provides GPU allocation algorithms
- Calculates memory usage across GPUs
- Manages runner status and health
- Handles slot CRUD operations on runners

### 3. Queue (`queue.go`)

**Purpose**: Manages incoming work requests and provides slot requirements.

**Key Responsibilities**:
- Stores pending workloads waiting for execution
- Calculates required slots based on queued work
- Provides work selection logic for slot assignment

### 4. Model Allocation (`model_allocation.go`)

**Purpose**: Configures models with specific GPU allocations.

**Key Responsibilities**:
- Creates configured models from base models + allocation decisions
- Handles runtime-specific memory calculation (VLLM vs Ollama)
- Ensures models have proper GPU allocation metadata

## Core Data Structures

### Slot (`slot.go`)
```go
type Slot struct {
    ID               uuid.UUID        // Unique slot identifier
    RunnerID         string          // Which runner hosts this slot
    initialWork      *Workload       // The configured workload
    LastActivityTime time.Time       // For staleness detection
    activeRequests   int64           // Current concurrent requests
    maxConcurrency   int64           // Maximum allowed concurrency
    GPUAllocation    *GPUAllocation  // GPU assignment info
}
```

**Key Characteristics**:
- Immutable after creation (work doesn't change)
- Thread-safe request counting via atomics
- Contains both model info and GPU allocation

### Workload (`workload.go`)
```go
type Workload struct {
    WorkloadType        WorkloadType
    llmInferenceRequest *types.RunnerLLMInferenceRequest
    session            *types.Session
    model              *types.Model    // Can be configured or unconfigured
}
```

**Key Characteristics**:
- Represents a unit of work to be executed
- Model can be in configured or unconfigured state
- Contains runtime-specific information

### Model (`types/models.go`)
```go
type Model struct {
    ID       string
    Memory   uint64           // Database/admin-configured memory (VLLM only)
    Runtime  Runtime          // VLLM, Ollama, etc.
    
    // Allocation-specific fields (set by NewModelForGPUAllocation)
    AllocationConfigured    bool
    AllocatedMemory        uint64    // Total memory for this allocation
    AllocatedGPUCount      int       // Number of GPUs
    AllocatedSpecificGPUs  []int     // Which GPU indices
    AllocatedPerGPUMemory  []uint64  // Memory per GPU
}
```

**Key Characteristics**:
- Has two states: unconfigured (from database) and configured (for specific allocation)
- Memory sources differ by runtime (VLLM uses `Memory`, Ollama uses GGUF estimation)
- Configured models have complete allocation metadata

### GPUAllocation (`scheduler.go`)
```go
type GPUAllocation struct {
    WorkloadID         string
    RunnerID           string
    SingleGPU          *int     // For single GPU allocations
    MultiGPUs          []int    // For multi-GPU allocations
    TensorParallelSize int      // Size of tensor parallel group
}
```

## Scheduling Flow

### Main Reconciliation Loop

The scheduler operates on a timer-based reconciliation pattern:

```
reconcileSlots() [every 5 seconds]
├── Get required slots from queue
├── FOR each slot requirement:
│   ├── Count existing slots for this model
│   ├── IF more slots needed:
│   │   ├── Call ensureSlot(requirement)
│   │   └── BREAK (only create one slot per cycle)
│   └── Continue to next requirement
└── Clean up orphaned/mismatched slots
```

### ensureSlot() Process

The core slot creation logic:

```
ensureSlot(requirement)
├── getSortedRunners() // Get runners ordered by preference
├── FOR each runner:
│   ├── tryAllAllocationsWithEviction()
│   │   ├── GetAllPossibleGPUAllocations() // Try without eviction
│   │   ├── IF no options: calculateEvictableMemoryPerGPU()
│   │   └── GetAllPossibleGPUAllocationsWithEviction()
│   ├── FOR each allocation option:
│   │   └── tryEvictionForAllocation() // Evict until memory available
│   ├── NewModelForGPUAllocation() // Configure model for chosen allocation
│   ├── NewSlot() // Create slot with configured model
│   └── s.slots.Store() // Store in scheduler state
└── Continue to next runner (⚠️ POTENTIAL BUG)
```

## Memory Management

### Memory Calculation Methods

The system uses multiple methods to calculate memory usage:

#### 1. Total Runner Memory (`scheduler.calculateRunnerMemory()`)
```go
// Calculates total memory across all GPUs on a runner
totalMemory := runnerStatus.TotalMemory
allocatedMemory := 0
s.slots.Range(func(slot *Slot) {
    allocatedMemory += slot.InitialWork().model.GetMemoryForAllocation()
})
freeMemory := totalMemory - allocatedMemory
```

#### 2. Per-GPU Memory (`runner.calculateAllocatedMemoryPerGPU()`)
```go
// Calculates memory allocated to each individual GPU
allocatedMemoryPerGPU := make(map[int]uint64)
for each slot in schedulerSlots {
    if slot.GPUAllocation.MultiGPUs:
        memoryPerGPU := modelMemory / numGPUs
        for each gpu in MultiGPUs:
            allocatedMemoryPerGPU[gpu] += memoryPerGPU
    else if slot.GPUAllocation.SingleGPU:
        allocatedMemoryPerGPU[*SingleGPU] += modelMemory
}
```

#### 3. Runner Status Memory (`runner status API`)
```go
// Real-time memory from GPU drivers
status.TotalMemory  // Sum of all GPU memory
status.FreeMemory   // Sum of free memory across GPUs
status.UsedMemory   // Sum of used memory across GPUs
status.GPUs[i].TotalMemory // Individual GPU capacity
status.GPUs[i].FreeMemory  // Individual GPU free memory
```

### Memory Sources by Runtime

#### VLLM Models
- Source: `model.Memory` (admin-configured in database)
- Allocation: `configuredModel.AllocatedMemory = baseModel.Memory`
- Per-GPU: `AllocatedPerGPUMemory[i] = Memory / GPUCount`

#### Ollama Models
- Source: GGUF estimation service (`memoryEstimationService.EstimateModelMemory()`)
- Allocation: `configuredModel.AllocatedMemory = estimationResult.TotalSize`
- Per-GPU: `AllocatedPerGPUMemory = estimationResult.GPUSizes`

## GPU Allocation Logic

### Allocation Decision Process

The system tries allocations in order of preference:

```
GetOptimalGPUAllocation()
├── calculateAllocatedMemoryPerGPU()
├── Try single GPU allocation:
│   ├── FOR each GPU:
│   │   ├── freeMemory = gpu.TotalMemory - allocatedMemory[gpu]
│   │   └── IF freeMemory >= modelMemory: consider this GPU
│   └── SELECT GPU with most free memory
├── IF single GPU fails AND runtime supports tensor parallelism:
│   └── Try multi-GPU allocation (2, 3, 4... GPUs)
```

### Allocation Options (`GetAllPossibleGPUAllocations()`)

Generates all viable allocation strategies:

```go
type AllocationOption struct {
    GPUCount            int      // 1, 2, 4, 8...
    GPUs                []int    // [0], [0,1], [0,1,2,3]
    MemoryPerGPU        uint64   // Memory needed per GPU
    TotalMemoryRequired uint64   // Total memory across all GPUs
    TensorParallelSize  int      // Tensor parallel group size
}
```

### Eviction Logic

When allocation fails due to insufficient memory:

```
tryEvictionForAllocation()
├── LOOP until memory available:
│   ├── calculateRunnerMemory() // Get current state
│   ├── IF single GPU allocation:
│   │   ├── Check target GPU has enough memory
│   │   └── memoryNeeded = option.MemoryPerGPU
│   ├── IF multi-GPU allocation:
│   │   ├── Check total memory across all GPUs
│   │   └── memoryNeeded = option.TotalMemoryRequired
│   ├── IF insufficient memory:
│   │   ├── Find stale slots on this runner
│   │   ├── Sort by staleness (oldest first)
│   │   ├── Delete most stale slot
│   │   └── CONTINUE loop
│   └── ELSE: return success
```

## State Management

### Slot Storage

The scheduler maintains the authoritative slot state:

```go
type Scheduler struct {
    slots sync.Map[uuid.UUID, *Slot]  // Thread-safe slot storage
}
```

### Runner State Synchronization

The runner controller manages runner connections:

```go
type RunnerController struct {
    runners      []string                    // List of connected runners
    statusCache  map[string]*Cache          // Cached runner status
    slotCache    map[string]*Cache          // Cached runner slots
}
```

### State Consistency Mechanism

The system uses a callback mechanism for state consistency:

```go
// RunnerController gets scheduler state via callback
c.getSchedulerSlotsFn = s.getSchedulerSlots

// This allows memory calculations to use scheduler's desired state
// instead of potentially stale runner state
schedulerSlots := c.getSchedulerSlotsFn()
```

## Key Algorithms

### 1. Runner Selection (`getSortedRunners()`)

Prioritizes runners based on available resources and model affinity.

### 2. GPU Load Balancing

Selects GPU with most free memory for single-GPU allocations:
```go
for _, gpu := range status.GPUs {
    freeMemory := gpu.TotalMemory - allocatedMemory[gpu.Index]
    if freeMemory > maxFreeMemory {
        bestGPU = gpu.Index
    }
}
```

### 3. Staleness Detection

Slots become stale based on inactivity:
```go
func (s *Slot) IsStale() bool {
    if s.IsActive() { return false }  // Active slots never stale
    return s.isStaleFunc(s.RunnerID, s.LastActivityTime)
}
```

### 4. Workload Matching

Existing slots are matched to new work based on:
- Model name
- Runtime 
- LoRA directory
- Available capacity

## Configuration and Timing

### Reconciliation Intervals
- `reconcileSlots`: Every 5 seconds (or `runnerReconcileInterval`)
- `processQueue`: Every 30 seconds + trigger-based
- `reconcileActivity`: Every 30 seconds
- `reconcileRunners`: Every 5 seconds

### Slot Lifecycle
- Creation: Via `ensureSlot()` in reconciliation loop
- Assignment: Via `processQueue()` matching warm slots to work
- Cleanup: Via `reconcileActivity()` removing stale slots

### Memory Estimation
- VLLM: Uses admin-configured values from database
- Ollama: Uses GGUF file analysis with configurable context length and concurrency

## Critical Design Characteristics

1. **Single-threaded scheduling**: All allocation decisions happen sequentially in reconciliation loops
2. **Desired state architecture**: Memory calculations use scheduler's intended state, not real-time runner state
3. **Lazy slot creation**: Only one slot created per reconciliation cycle to prevent resource conflicts
4. **Eviction-based allocation**: When memory insufficient, evicts stale slots rather than rejecting requests
5. **Runtime-agnostic allocation**: GPU allocation logic works across VLLM, Ollama, and future runtimes
6. **Eventual consistency**: System converges to desired state over multiple reconciliation cycles

This architecture prioritizes simplicity and eventual consistency over immediate response times, with the trade-off being that workloads may wait several reconciliation cycles before getting scheduled.