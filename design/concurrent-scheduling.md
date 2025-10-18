# Concurrent Request Scheduling

Helix now supports concurrent request processing per model instance, allowing VLLM and Ollama runners to handle multiple inference requests simultaneously. This significantly improves throughput for models that support batched inference.

## Overview

Previously, Helix explicitly limited Ollama to `OLLAMA_NUM_PARALLEL=1` and didn't configure VLLM's concurrency limits. With concurrent scheduling, each slot can now process multiple requests concurrently, taking advantage of the native batching capabilities of both runtimes.

## Investigation Results

**Current State Before Changes:**
- **Ollama**: Hardcoded to `OLLAMA_NUM_PARALLEL=1` in Helix
- **VLLM**: No explicit `--max-num-seqs` parameter (defaults to 256)
- **Memory Estimation**: Used `NumParallel=1` for all calculations

**After Implementation:**
- **Ollama**: Configurable `OLLAMA_NUM_PARALLEL` based on model/global settings
- **VLLM**: Configurable `--max-num-seqs` parameter added to runtime args
- **Memory Estimation**: Uses actual concurrency values for accurate memory calculations

## Configuration

### Per-Model Configuration

Override the global setting for specific models by adding a `concurrency` field to the model configuration:

```yaml
apiVersion: helix.ml/v1alpha1
kind: Model
metadata:
  name: llama3-8b-high-concurrency
spec:
  id: llama3:8b-instruct-q4_0
  runtime: ollama
  memory: 8000000000
  concurrency: 8  # Allow up to 8 concurrent requests for this model
```

### Configuration Priority

The concurrency limit is determined in this order:
1. **Per-model setting** (highest priority)
2. **Runtime natural defaults** (256 for VLLM, 4 for Ollama, 1 for others)

## Runtime Implementation

### Ollama Configuration
- **Environment Variable**: `OLLAMA_NUM_PARALLEL=N` (passed to Ollama process)
- **Memory Impact**: Ollama's memory estimation accounts for `numParallel` in GPU layer calculations
- **Previous**: Hardcoded to 1
- **Now**: Configurable per-model or globally

### VLLM Configuration  
- **Command Line Argument**: `--max-num-seqs N` (added to vLLM startup args)
- **Memory Impact**: Higher concurrency may require more GPU memory for KV cache
- **Previous**: Used VLLM default (256, often too high for smaller GPUs)
- **Now**: Configurable per-model or globally with reasonable defaults

## Supported Runtimes

- **VLLM**: Fully supported with `--max-num-seqs` parameter (default: 256)
- **Ollama**: Fully supported with `OLLAMA_NUM_PARALLEL` environment variable (default: 4)
- **Other runtimes**: Default to 1 (no concurrency) for backward compatibility

**Memory Estimation Integration:**
- Both runtimes now pass the actual concurrency value to memory estimation
- Ollama uses `numParallel` in `EstimateGPULayers()` function
- VLLM concurrency affects KV cache memory requirements
- Scheduler considers concurrency when making placement decisions

## Dashboard Visibility

The runner dashboard now shows concurrent request information for each model slot:

- **Active Requests**: Current number of requests being processed
- **Max Concurrency**: Maximum concurrent requests allowed for this slot

Example dashboard display:
```
Runner: runner-gpu-01
├── llama3:8b-instruct-q4_0 [VLLM]
│   ├── Active Requests: 3/8
│   ├── Status: Ready
│   └── Memory: 8.0GB
└── phi3:3.8b-mini-instruct [Ollama]  
    ├── Active Requests: 1/4
    ├── Status: Ready
    └── Memory: 4.2GB
```

## Benefits

### Improved Throughput
- Multiple requests can be processed simultaneously by the same model instance
- Better utilization of GPU compute resources
- Reduced average response time under load

### Efficient Resource Usage
- No need to spin up multiple model instances for concurrency
- Lower memory overhead compared to running multiple separate model instances
- Better GPU memory utilization

### Load Distribution
- The scheduler automatically distributes requests to the least busy slots
- Prevents overloading of individual model instances
- Maintains backward compatibility with single-request-per-slot behavior

## Scheduling Behavior

### Slot Selection
When multiple slots are available for the same model, the scheduler prioritizes:
1. **Slot load**: Slots with fewer active requests
2. **Runner load**: Runners with lower overall utilization  
3. **Recent activity**: More recently used slots (for cache efficiency)

### Capacity Management
- Slots are considered "warm" and available as long as they have capacity
- Requests are queued if all slots for a model are at maximum capacity
- The scheduler automatically creates new slots when needed (subject to memory constraints)

### Thread Safety
All concurrent request tracking uses atomic operations to ensure thread-safe updates across multiple goroutines.

## Examples

### High-Throughput Chat Service
```yaml
# Configure a high-concurrency model for chat workloads
apiVersion: helix.ml/v1alpha1
kind: Model
metadata:
  name: chat-model-high-throughput
spec:
  id: llama3:8b-instruct-q4_0
  runtime: vllm
  memory: 12000000000
  concurrency: 16  # Handle up to 16 concurrent chat requests
```

**What happens:**
- VLLM gets `--max-num-seqs 16` added to startup args
- Memory estimation uses `NumParallel: 16` for accurate calculations
- Scheduler allows up to 16 concurrent requests per slot

### Balanced Configuration
**No global configuration needed!** Runtime defaults work well:
- **VLLM**: 256 concurrent requests (natural default)
- **Ollama**: 4 concurrent requests (reasonable default)

**Per-model tuning guidelines:**
- Large models (70B+): concurrency: 2-4
- Medium models (7B-30B): concurrency: 4-8  
- Small models (<7B): concurrency: 8-16

**Example Ollama Model:**
```yaml
apiVersion: helix.ml/v1alpha1
kind: Model
metadata:
  name: efficient-ollama-model
spec:
  id: phi3:3.8b-mini-instruct-q4_0
  runtime: ollama
  memory: 4200000000
  concurrency: 8  # Ollama gets OLLAMA_NUM_PARALLEL=8
```

### Development/Testing
```yaml
# Conservative setting for development - specify per model
spec:
  concurrency: 1  # Single request for predictable testing
```

## Monitoring

### Logs
The scheduler logs provide detailed information about concurrent request handling:

```
[INFO] Ollama: Added concurrency configuration to runtime args model=phi3:3.8b num_parallel=8
[INFO] VLLM: Added concurrency configuration to runtime args model=llama3:8b max_num_seqs=16
[INFO] Using concurrency setting from scheduler for Ollama model num_parallel=4
[DEBUG] Warm slot selected: runner-01/slot-abc3 (3 active, 5 capacity)
```

**Memory estimation logs:**
```
[DEBUG] GGUF memory estimation using NumParallel=8 for model phi3:3.8b
[INFO] Memory requirement increased due to concurrency: 4.2GB -> 5.1GB
```

### Metrics
Monitor these key metrics:
- **Active requests per slot**: Track utilization
- **Queue depth**: Monitor for capacity issues
- **Slot creation rate**: Indicates if more capacity is needed

## Troubleshooting

### High Queue Times
If requests are spending too long in the queue:
1. Increase `concurrency` for affected models
2. Add more runners with the same models  
3. Check memory constraints preventing new slot creation
4. **New**: Verify actual runtime concurrency vs configured values in logs

### Memory Issues  
If you see "insufficient memory" errors:
1. Reduce `concurrency` to free up memory for new slots (memory estimation now accounts for concurrency)
2. Consider using smaller model variants
3. Add runners with more GPU memory
4. **New**: Check if high concurrency is causing memory estimation to exceed GPU limits

### Performance Degradation
If concurrent requests are slower than expected:
1. Monitor GPU utilization - may be hitting compute limits
2. Consider reducing `concurrency` for better per-request performance  
3. Check if the model supports efficient batching at the current concurrency level
4. **New**: For Ollama, check if `OLLAMA_NUM_PARALLEL` matches your expectations in process environment
5. **New**: For VLLM, verify `--max-num-seqs` appears in the command line arguments

## Migration from Single-Request Mode

Existing deployments will continue to work without changes:
- **Ollama**: Will automatically use 4 concurrent requests instead of hardcoded 1
- **VLLM**: Will continue using natural default of 256 (now properly configured with `--max-num-seqs`)
- **Memory Estimation**: Now accounts for actual concurrency in calculations
- **Other runtimes**: Still default to 1 (backward compatible)

**Automatic Improvements:**
- Ollama models immediately benefit from 4x concurrency improvement
- VLLM models get explicit `--max-num-seqs` parameter for consistency
- Memory estimates become more accurate, preventing over-allocation

**No configuration required** - natural runtime defaults provide immediate benefits!