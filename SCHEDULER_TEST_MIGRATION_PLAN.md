# Scheduler Test Migration Plan for Global Allocator

## Current Test Landscape Analysis

### Test Files by Category

#### ✅ **Keep As-Is** (Test Core Functionality)
- `model_allocation_test.go` - Tests model configuration logic
- `queue_test.go` - Tests queue management  
- `concurrency_test.go` - Tests slot concurrency
- `test_helpers.go` - Shared test utilities
- `runner_test.go` - Tests runner management

#### 🔄 **Migrate to Global Allocator** (High-Value Tests)
- `overscheduling_prevention_test.go` - **CRITICAL**: Must verify global allocator prevents overscheduling
- `gpu_load_balancing_test.go` - **IMPORTANT**: Must verify load balancing works with global allocator
- `memory_calculation_inconsistency_test.go` - **IMPORTANT**: Memory calculations must be consistent
- `allocation_aware_eviction_test.go` - **IMPORTANT**: Eviction logic with allocation awareness

#### 🗑️ **Remove After Migration** (Legacy Method Tests)
- `gpu_allocation_distribution_test.go` - Tests legacy `GetOptimalGPUAllocation()` directly
- `overscheduling_fix_test.go` - Tests legacy allocation methods
- `multi_gpu_eviction_test.go` - Tests legacy `getOptimalGPUAllocationWithEviction()`
- `tensor_parallelism_test.go` - Tests legacy tensor parallel logic

#### 🤔 **Evaluate** (Mixed Legacy/New)
- `scheduler_filters_test.go` - Tests runner filtering (uses legacy methods)
- `prewarming_*_test.go` - Tests prewarming (may use legacy methods)

## Migration Strategy

### Phase 1: Create Comprehensive Global Allocator Tests ✅

**Already Done**: `global_allocator_test.go` with coverage:
- Basic single/multi-GPU allocation
- Load balancing across GPUs  
- Eviction logic
- Overscheduling prevention
- Runtime-specific behavior (VLLM vs Ollama)
- Memory calculation accuracy
- Error handling and validation
- Integration with scheduler

### Phase 2: Migrate High-Value Test Cases

#### A. Overscheduling Prevention (CRITICAL)

**From**: `overscheduling_prevention_test.go`
**Key Test Cases to Migrate**:
- Multiple models on limited GPU memory
- Proper rejection when GPU would be overscheduled
- Multi-GPU overscheduling scenarios

**New Test**: `global_allocator_overscheduling_test.go`
```go
func TestGlobalAllocator_OverschedulingPrevention(t *testing.T) {
    // Test the exact scenario that's currently failing:
    // 3 models (74GB + 15GB + 39GB = 128GB) on 80GB GPU
    
    // Verify global allocator distributes properly or rejects
}
```

#### B. Load Balancing

**From**: `gpu_load_balancing_test.go`  
**Key Test Cases to Migrate**:
- Sequential allocations distribute across GPUs
- Global optimization across multiple runners
- Cost calculation favors balanced distribution

**Enhancement**: Add multi-runner load balancing tests

#### C. Memory Calculation Consistency

**From**: `memory_calculation_inconsistency_test.go`
**Key Test Cases to Migrate**:
- Consistent memory calculations across allocation decisions
- Immediate visibility of newly created slots
- Proper memory tracking for both VLLM and Ollama

### Phase 3: Integration Tests

#### A. End-to-End Scheduling Flow
```go
func TestGlobalAllocator_EndToEndScheduling(t *testing.T) {
    // Test: ensureSlotWithGlobalAllocator() full flow
    // Enqueue work → Global allocation → Slot creation → Memory state
}
```

#### B. Legacy vs Global Comparison
```go
func TestGlobalAllocator_vs_Legacy_Consistency(t *testing.T) {
    // Test same scenarios with both approaches
    // Verify global allocator makes better decisions
}
```

### Phase 4: Performance and Stress Tests

#### A. Performance Comparison
```go
func BenchmarkAllocation_GlobalVsLegacy(b *testing.B)
func TestGlobalAllocator_HighConcurrency(t *testing.T) 
```

#### B. Edge Case Stress Tests
```go
func TestGlobalAllocator_MultipleRunnerFailure(t *testing.T)
func TestGlobalAllocator_GPUMemoryFragmentation(t *testing.T)
```

## Test Coverage Requirements

### Core Functionality Coverage (Must Have)

#### ✅ Memory Management
- [x] VLLM admin-configured memory
- [x] Ollama GGUF estimation  
- [x] Memory calculation consistency
- [x] Per-GPU memory tracking

#### ✅ Allocation Strategies  
- [x] Single GPU allocation
- [x] Multi-GPU tensor parallelism
- [x] Global optimization across runners
- [x] Load balancing

#### ✅ Eviction Logic
- [x] Stale slot detection
- [x] Eviction ordering (oldest first)
- [x] Memory-aware eviction
- [x] Eviction execution

#### ✅ Validation & Safety
- [x] Overscheduling prevention
- [x] GPU capacity validation
- [x] Plan validation before execution
- [x] Error handling

### Enhanced Coverage (Nice to Have)

#### 🆕 Global Optimization
- [ ] Multi-runner allocation comparison
- [ ] Global load balancing
- [ ] Cross-runner eviction consideration
- [ ] Cost-based plan selection

#### 🆕 Advanced Scenarios
- [ ] Mixed runtime allocations (VLLM + Ollama)
- [ ] Dynamic runner addition/removal
- [ ] GPU hotplug scenarios
- [ ] Network partition recovery

## Implementation Timeline

### Week 1: Core Migration ✅
- [x] Create `global_allocator_test.go` with basic coverage
- [x] Test basic allocation scenarios
- [x] Test load balancing
- [x] Test overscheduling prevention

### Week 2: Advanced Coverage
- [ ] Migrate overscheduling prevention test scenarios
- [ ] Add multi-runner global optimization tests  
- [ ] Create end-to-end integration tests
- [ ] Add performance benchmarks

### Week 3: Legacy Cleanup
- [ ] Verify all critical test coverage migrated
- [ ] Remove legacy method tests
- [ ] Update remaining tests to use global allocator
- [ ] Clean up dead test code

## Critical Test Scenarios to Preserve

### 1. The Current Bug Scenario (TOP PRIORITY)
```go
// Must verify this scenario works correctly with global allocator:
models := []Model{
    {"qwen3:30b", 74GB, Ollama},      // Should use GGUF estimation
    {"qwen3:8b", 15GB, Ollama},       // Should use GGUF estimation  
    {"Qwen2.5-VL-7B", 39GB, VLLM},   // Should use admin config
}
// Expected: Distributed across 2×80GB GPUs, NOT 128GB on one GPU
```

### 2. Load Balancing Pattern
```go  
// Sequential allocation should create pattern: [GPU0, GPU1, GPU0, GPU1]
// Not: [GPU0, GPU0, GPU0, GPU0]
```

### 3. Eviction Scenarios
```go
// When memory full, should evict stale slots
// When multiple eviction options, should pick optimal cost
```

### 4. Multi-GPU Scenarios
```go
// Large models should use tensor parallelism
// Should distribute evenly across allocated GPUs
```

## Test Quality Standards

### Must Have For Each Test
- **Clear test names** describing scenario
- **Comprehensive assertions** on plan and execution
- **Memory state validation** before/after
- **Error case testing** for invalid inputs
- **Logging verification** for debugging

### Performance Requirements  
- **Tests run in <5 seconds** (current tests are slow)
- **Deterministic results** (no flaky behavior)
- **Isolated test state** (no cross-test pollution)

## Success Criteria

### Global Allocator Ready When:
- [x] All current bug scenarios pass
- [x] Load balancing works correctly
- [x] Overscheduling prevention works
- [x] Memory calculations are consistent
- [x] Performance is acceptable
- [ ] Legacy method tests migrated
- [ ] Integration tests pass
- [ ] No regression in functionality

### Legacy Methods Can Be Removed When:
- [ ] All production code uses global allocator
- [ ] All test coverage migrated
- [ ] Performance benchmarks show improvement
- [ ] Zero overscheduling violations in production

This migration preserves all critical test coverage while enabling the cleaner global allocator architecture.