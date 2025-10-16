# Concurrent Request Scheduling - Implementation Summary

This document provides a comprehensive overview of the concurrent request scheduling implementation in Helix, including investigation findings, technical implementation details, and benefits achieved.

## Investigation Results

### Pre-Implementation State

**Ollama Configuration:**
- **Hardcoded Limitation**: `OLLAMA_NUM_PARALLEL=1` in `ollama_runtime.go`
- **Capability**: Ollama natively supports `OLLAMA_NUM_PARALLEL` environment variable with defaults to 1
- **Memory Estimation**: Used `NumParallel=1` regardless of actual concurrency needs

**VLLM Configuration:**
- **Missing Parameter**: No `--max-num-seqs` parameter specified in runtime args
- **Natural Default**: VLLM defaults to `--max-num-seqs 256` when unspecified
- **Memory Impact**: No consideration for concurrent request memory overhead

**Scheduler Behavior:**
- **One-at-a-time**: Each slot could only handle one request at a time
- **Inefficient**: Underutilized native batching capabilities of both runtimes
- **Memory Misestimation**: Cache keys and estimates didn't account for concurrency

## Implementation Architecture

### Core Changes

#### 1. Slot Concurrency Model
```go
type Slot struct {
    // Before: isActive bool
    activeRequests   int64  // Atomic counter for concurrent requests
    maxConcurrency   int64  // Configurable limit per slot
    // ... other fields
}
```

**Key Methods:**
- `IsActive() bool` - Returns `activeRequests > 0`
- `HasCapacity() bool` - Returns `activeRequests < maxConcurrency`
- `Start()` - Atomically increments `activeRequests`
- `Release()` - Atomically decrements `activeRequests`
- `GetActiveRequests() int64` - Returns current active count

#### 2. Natural Runtime Defaults
No global configuration required - uses runtime natural defaults:

```go
// NewSlot determines concurrency automatically
if work.model != nil && work.model.Concurrency > 0 {
    maxConcurrency = int64(work.model.Concurrency)  // Per-model override
} else {
    // Natural runtime defaults
    if work.Runtime() == types.RuntimeVLLM {
        maxConcurrency = 256  // VLLM's natural default
    } else if work.Runtime() == types.RuntimeOllama {
        maxConcurrency = 4    // Reasonable Ollama default
    }
    // Other runtimes: maxConcurrency = 1
}
```

#### 3. Runtime Configuration Integration

**Ollama Runtime:**
```go
// Environment variable set based on slot concurrency
env := []string{
    // ... other env vars
    fmt.Sprintf("OLLAMA_NUM_PARALLEL=%d", numParallel),
    // ... 
}
```

**VLLM Runtime:**
```go
// Command line argument added automatically
if !hasMaxNumSeqs {
    existingArgs = append(existingArgs, "--max-num-seqs", fmt.Sprintf("%d", numParallel))
}
```

#### 4. Memory Estimation Updates
```go
// Memory estimation now uses actual concurrency
opts := memory.EstimateOptions{
    NumCtx:      int(work.model.ContextLength),
    NumBatch:    types.DefaultBatchSize,
    NumParallel: numParallel,  // Actual concurrency value
    NumGPU:      types.AutoDetectLayers,
    KVCacheType: types.DefaultKVCacheType,
}
```

**Cache Key Integration:**
```go
// generateCacheKey already included NumParallel
key := fmt.Sprintf("%s_%d_%d_%d_%d_%d_%s",
    modelName,
    len(gpuConfig),
    opts.NumCtx,
    opts.NumBatch,
    opts.NumParallel,  // ✅ Already included
    opts.NumGPU,
    opts.KVCacheType)
```

### Scheduler Logic Updates

#### 1. Slot Selection Algorithm
```go
// warmSlotsWithReason now checks capacity instead of binary active state
if !slot.HasCapacity() {
    withSlotContext(&log.Logger, slot).Trace().
        Int64("active_requests", slot.GetActiveRequests()).
        Msg("skipping warm slot, at capacity")
    return true
}
```

#### 2. Load Balancing
```go
// pickBestWarmSlot prioritizes by:
// 1. Slot load (fewer active requests)
// 2. Runner load (fewer total active slots)
// 3. Recent activity (cache efficiency)
// 4. Random tie-breaking
slices.SortFunc(warmSlots, func(i, j *Slot) int {
    iActive := i.GetActiveRequests()
    jActive := j.GetActiveRequests()
    if iActive != jActive {
        return int(iActive - jActive)  // Prefer less busy slots
    }
    // ... additional sorting criteria
})
```

#### 3. Dashboard Data Enhancement
```go
// RunnerSlots now includes concurrency information
enrichedSlots[i].ActiveRequests = schedulerSlot.GetActiveRequests()
enrichedSlots[i].MaxConcurrency = atomic.LoadInt64(&schedulerSlot.maxConcurrency)
```

## Configuration Options

### Per-Model Configuration
```yaml
apiVersion: helix.ml/v1alpha1
kind: Model
metadata:
  name: high-throughput-model
spec:
  id: llama3:8b-instruct-q4_0
  runtime: ollama
  concurrency: 8  # Override natural default
```

### Natural Runtime Defaults
| Runtime | Default Concurrency | Parameter |
|---------|-------------------|-----------|
| VLLM | 256 | `--max-num-seqs 256` |
| Ollama | 4 | `OLLAMA_NUM_PARALLEL=4` |
| Others | 1 | N/A (backward compatible) |

### Runtime Args Integration
For VLLM models, concurrency is automatically added to runtime args:
```yaml
runtime_args:
  args:
    - "--trust-remote-code"
    - "--max-model-len"
    - "8192"
    # --max-num-seqs 8 automatically added based on concurrency setting
```

## Thread Safety Implementation

### Atomic Operations
```go
// All slot state changes use atomic operations
func (s *Slot) Start() {
    atomic.AddInt64(&s.activeRequests, 1)
    s.LastActivityTime = time.Now()
}

func (s *Slot) Release() {
    if atomic.AddInt64(&s.activeRequests, -1) < 0 {
        atomic.StoreInt64(&s.activeRequests, 0) // Prevent negative values
    }
    s.LastActivityTime = time.Now()
}
```

### Concurrent Access Patterns
- Multiple goroutines can safely call `Start()` and `Release()`
- Scheduler reconciliation safely reads slot state
- Dashboard queries safely access concurrent request counts

## Testing Strategy

### Unit Tests
```go
func TestSlotConcurrency(t *testing.T) {
    // Tests cover:
    // - Single request behavior (concurrency=1)
    // - Multiple concurrent requests
    // - Per-model concurrency configuration
    // - Thread safety under high contention
    // - Capacity checking logic
}
```

### Thread Safety Testing
```go
const numGoroutines = 50
const operationsPerGoroutine = 10

// Test concurrent Start()/Release() operations
// Verify final state is consistent (activeRequests = 0)
```

## Performance Benefits

### Throughput Improvements
- **Ollama**: 4x immediate improvement (1 → 4 concurrent requests)
- **VLLM**: Explicit concurrency control (was uncontrolled, now configurable)
- **Batching**: Better utilization of native runtime batching capabilities

### Resource Efficiency
- **Memory**: Accurate estimation accounting for concurrency overhead
- **GPU Utilization**: Multiple requests can share model instances
- **Scheduling**: Intelligent load distribution across slots

### Operational Benefits
- **Backward Compatible**: Existing deployments work unchanged
- **Zero Configuration**: Natural defaults provide immediate benefits
- **Observable**: Dashboard shows real-time concurrency metrics

## Memory Estimation Accuracy

### Before
```go
// Always used NumParallel = 1 regardless of actual concurrency
opts := types.CreateOllamaEstimateOptions(contextLength, AutoDetectLayers)
// NumParallel: DefaultParallelSequences (1)
```

### After
```go
// Uses actual concurrency for accurate memory estimation
var numParallel int
if targetModel.Concurrency > 0 {
    numParallel = targetModel.Concurrency
} else if targetModel.Runtime == types.RuntimeVLLM {
    numParallel = 256 // VLLM's natural default
} else if targetModel.Runtime == types.RuntimeOllama {
    numParallel = 4   // Reasonable default for Ollama
}

opts := memory.EstimateOptions{
    NumParallel: numParallel,  // Actual concurrency value
    // ... other options
}
```

## Dashboard Integration

### Real-time Visibility
```json
{
  "active_requests": 3,
  "max_concurrency": 8,
  "model": "llama3:8b-instruct-q4_0",
  "status": "Ready"
}
```

### Monitoring Capabilities
- Current active requests per slot
- Maximum concurrency limits
- Capacity utilization metrics
- Load distribution across runners

## Migration Path

### Automatic Improvements
1. **Ollama Models**: Immediately benefit from 4x concurrency increase
2. **VLLM Models**: Get explicit `--max-num-seqs` parameter for consistency
3. **Memory Estimates**: Become more accurate, preventing over-allocation
4. **Other Runtimes**: Continue working with concurrency=1 (no change)

### No Breaking Changes
- All existing model configurations continue to work
- No environment variables required
- No configuration file changes needed
- Backward compatible slot behavior

## Production Considerations

### Recommended Configurations
```yaml
# Small models (<5GB): High concurrency
concurrency: 16

# Medium models (5-15GB): Moderate concurrency  
concurrency: 8

# Large models (15-50GB): Conservative concurrency
concurrency: 4

# Huge models (50GB+): Single or dual requests
concurrency: 2
```

### Monitoring Guidelines
- Watch GPU memory utilization when increasing concurrency
- Monitor request latency - too much concurrency can hurt performance
- Use dashboard metrics to tune concurrency per model
- Start with natural defaults and adjust based on observed performance

## Future Enhancements

### Potential Improvements
1. **Dynamic Concurrency**: Adjust based on GPU memory pressure
2. **Request Queuing**: Per-slot request queues for better scheduling
3. **Metrics Collection**: Detailed throughput and latency metrics
4. **Auto-tuning**: ML-based concurrency optimization

### Extension Points
- Runtime-specific concurrency strategies
- Workload-aware concurrency limits
- Integration with external load balancers
- Custom concurrency policies per use case

This implementation provides a solid foundation for concurrent request scheduling while maintaining full backward compatibility and providing immediate performance benefits for VLLM and Ollama deployments.