package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func TestSendToRunner(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctrl, err := NewRunnerController(context.Background(), &RunnerControllerConfig{
		PubSub:        ps,
		HealthChecker: &MockHealthChecker{},
		RunnerClient:  DefaultMockRunnerClient(),
	})
	require.NoError(t, err)

	mockRunnerID := "test"

	mockRunner, err := ps.SubscribeWithCtx(context.Background(), pubsub.GetRunnerQueue(mockRunnerID), func(_ context.Context, msg *nats.Msg) error {
		response := &types.Response{
			StatusCode: 200,
			Body:       []byte("test"),
		}
		responseBytes, err := json.Marshal(response)
		require.NoError(t, err)
		err = msg.Respond(responseBytes)
		require.NoError(t, err)
		return nil
	})
	require.NoError(t, err)
	defer func() {
		err := mockRunner.Unsubscribe()
		require.NoError(t, err)
	}()

	// Simulate runner connection by publishing to the runner.connected.{runnerID} subject
	err = ps.Publish(context.Background(), pubsub.GetRunnerConnectedQueue(mockRunnerID), []byte("connected"))
	require.NoError(t, err)

	// Give the controller a moment to process the connection event
	time.Sleep(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	response, err := ctrl.Send(ctx, mockRunnerID, map[string]string{}, &types.Request{
		Method: "GET",
		URL:    "https://example.com",
		Body:   []byte("{}"),
	}, 1*time.Second)
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, 200, response.StatusCode)
	require.Equal(t, []byte("test"), response.Body)
}

func TestSendNoRunner(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctrl, err := NewRunnerController(context.Background(), &RunnerControllerConfig{
		PubSub:        ps,
		HealthChecker: &MockHealthChecker{},
		RunnerClient:  DefaultMockRunnerClient(),
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err = ctrl.Send(ctx, "snowman", map[string]string{}, &types.Request{
		Method: "GET",
		URL:    "https://example.com",
		Body:   []byte("{}"),
	}, 1*time.Second)
	require.Error(t, err)
}

func TestCalculateVLLMMemoryUtilizationRatio(t *testing.T) {
	ctrl := &RunnerController{
		statusCache: NewLockingRunnerMap[types.RunnerStatus](),
	}

	runnerID := "test-runner"

	// Test case 1: Small model on large GPU (should calculate actual ratio)
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory:     80 * 1024 * 1024 * 1024, // 80GB GPU
			AllocatedMemory: 0,                       // No existing allocations
			GPUCount:        1,                       // Single GPU
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))
	ratio := ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, 8*1024*1024*1024) // 8GB model
	require.InDelta(t, 0.10, ratio, 0.01)                                         // Should be 8/80 = 10%

	// Test case 2: Very tiny model (should hit minimum ratio of 1%)
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory:     80 * 1024 * 1024 * 1024, // 80GB GPU
			AllocatedMemory: 0,                       // No existing allocations
			GPUCount:        1,                       // Single GPU
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))
	ratio = ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, 1*1024*1024*1024) // 1GB model
	require.InDelta(t, 0.0125, ratio, 0.001)                                     // Should be 1/80 = 1.25% (not clamped since > 1%)

	// Test case 3: Medium model on GPU (reasonable ratio)
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory:     24 * 1024 * 1024 * 1024, // 24GB GPU
			AllocatedMemory: 0,                       // No existing allocations
			GPUCount:        1,                       // Single GPU
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))
	ratio = ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, 16*1024*1024*1024) // 16GB model
	require.Greater(t, ratio, 0.01)
	require.Less(t, ratio, 0.99)
	require.InDelta(t, 0.67, ratio, 0.01) // Should be ~67% (16 / 24 = ~0.67)

	// Test case 4: Large model on small GPU
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory:     24 * 1024 * 1024 * 1024, // 24GB GPU
			AllocatedMemory: 0,                       // No existing allocations
			GPUCount:        1,                       // Single GPU
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))
	ratio = ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, 20*1024*1024*1024) // 20GB model
	require.Greater(t, ratio, 0.05)
	require.InDelta(t, 0.833, ratio, 0.01) // Should be 20/24 = 0.833

	// Test case 5: No GPU memory info (fallback)
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory:     0,
			AllocatedMemory: 0,
			GPUCount:        0, // No GPU count info
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))
	ratio = ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, 8*1024*1024*1024)
	require.Equal(t, 0.8, ratio) // Should return default fallback

	// Test case 6: Model larger than GPU (should hit maximum ratio)
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory:     8 * 1024 * 1024 * 1024, // 8GB GPU
			AllocatedMemory: 0,                      // No existing allocations
			GPUCount:        1,                      // Single GPU
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))
	ratio = ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, 10*1024*1024*1024) // 10GB model
	require.Equal(t, 0.99, ratio)                                                 // Should hit maximum ratio

	// Test case 7: Model requiring exact ratio calculation
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory:     24 * 1024 * 1024 * 1024, // 24GB GPU
			AllocatedMemory: 8 * 1024 * 1024 * 1024,  // 8GB already allocated (not used in calculation)
			GPUCount:        1,                       // Single GPU
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))
	ratio = ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, 8*1024*1024*1024) // 8GB new model

	// Calculate expected: 8GB / 24GB = ~0.33 (above minimum, so not clamped)
	require.InDelta(t, 0.33, ratio, 0.01) // Should be ~33% (8/24 = 0.333)

	// Test case 8: Model larger than 95% of GPU
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory:     24 * 1024 * 1024 * 1024, // 24GB GPU
			AllocatedMemory: 20 * 1024 * 1024 * 1024, // 20GB already allocated (not used in calculation)
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))
	ratio = ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, 24*1024*1024*1024) // 24GB new model (100% of GPU)
	require.Equal(t, 0.99, ratio)                                                 // Should hit maximum ratio (24/24 = 1.0, clamped to 0.99)
}

func TestSubstituteVLLMArgsPlaceholders(t *testing.T) {
	ctrl := &RunnerController{
		statusCache: NewLockingRunnerMap[types.RunnerStatus](),
	}

	runnerID := "test-runner"
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory: 24 * 1024 * 1024 * 1024, // 24GB GPU
			GPUCount:    1,                       // Single GPU for test
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))

	// Test case 1: Replace placeholder in args
	originalArgs := []string{
		"--trust-remote-code",
		"--max-model-len", "32768",
		"--gpu-memory-utilization", "{{.DynamicMemoryUtilizationRatio}}",
		"--limit-mm-per-prompt", "image=10",
	}

	substitutedArgs := ctrl.substituteVLLMArgsPlaceholders(originalArgs, runnerID, 8*1024*1024*1024)

	require.Len(t, substitutedArgs, len(originalArgs))
	require.Equal(t, "--trust-remote-code", substitutedArgs[0])
	require.Equal(t, "--max-model-len", substitutedArgs[1])
	require.Equal(t, "32768", substitutedArgs[2])
	require.Equal(t, "--gpu-memory-utilization", substitutedArgs[3])
	require.NotEqual(t, "{{.DynamicMemoryUtilizationRatio}}", substitutedArgs[4]) // Should be substituted
	require.Regexp(t, `^0\.\d{2}$`, substitutedArgs[4])                           // Should be a ratio like "0.25"
	require.Equal(t, "--limit-mm-per-prompt", substitutedArgs[5])
	require.Equal(t, "image=10", substitutedArgs[6])

	// Test case 2: No placeholders (should automatically add --gpu-memory-utilization)
	argsWithoutPlaceholder := []string{
		"--trust-remote-code",
		"--max-model-len", "32768",
	}

	substitutedArgs = ctrl.substituteVLLMArgsPlaceholders(argsWithoutPlaceholder, runnerID, 8*1024*1024*1024)
	require.Len(t, substitutedArgs, 5) // Original 3 + 2 for --gpu-memory-utilization and its value
	require.Equal(t, "--trust-remote-code", substitutedArgs[0])
	require.Equal(t, "--max-model-len", substitutedArgs[1])
	require.Equal(t, "32768", substitutedArgs[2])
	require.Equal(t, "--gpu-memory-utilization", substitutedArgs[3])
	require.Regexp(t, `^0\.\d{2}$`, substitutedArgs[4]) // Should be a ratio like "0.33"

	// Test case 3: Empty args (should automatically add --gpu-memory-utilization)
	emptyArgs := []string{}
	substitutedArgs = ctrl.substituteVLLMArgsPlaceholders(emptyArgs, runnerID, 8*1024*1024*1024)
	require.Len(t, substitutedArgs, 2) // Should add --gpu-memory-utilization and its value
	require.Equal(t, "--gpu-memory-utilization", substitutedArgs[0])
	require.Regexp(t, `^0\.\d{2}$`, substitutedArgs[1]) // Should be a ratio like "0.33"

	// Test case 4: Multiple placeholders (should replace all)
	multiPlaceholderArgs := []string{
		"--gpu-memory-utilization", "{{.DynamicMemoryUtilizationRatio}}",
		"--another-flag", "{{.DynamicMemoryUtilizationRatio}}",
	}

	substitutedArgs = ctrl.substituteVLLMArgsPlaceholders(multiPlaceholderArgs, runnerID, 8*1024*1024*1024)
	require.Len(t, substitutedArgs, len(multiPlaceholderArgs))
	require.NotEqual(t, "{{.DynamicMemoryUtilizationRatio}}", substitutedArgs[1])
	require.NotEqual(t, "{{.DynamicMemoryUtilizationRatio}}", substitutedArgs[3])
	require.Equal(t, substitutedArgs[1], substitutedArgs[3]) // Both should be the same calculated value
}

func TestVLLMMemoryUtilizationRealWorldScenarios(t *testing.T) {
	ctrl := &RunnerController{
		statusCache: NewLockingRunnerMap[types.RunnerStatus](),
	}

	runnerID := "test-runner"

	scenarios := []struct {
		name             string
		gpuMemoryGB      uint64
		modelMemoryGB    uint64
		existingMemoryGB uint64 // NEW: existing allocated memory
		expectedRatioMin float64
		expectedRatioMax float64
		description      string
	}{
		// Simplified scenarios: ratio = model_memory / total_memory, clamped between 0.01 and 0.99
		{"1GB model on 80GB GPU (A100)", 80, 1, 0, 0.0125, 0.0125, "1/80 = 1.25%"},
		{"2GB model on 80GB GPU (A100)", 80, 2, 0, 0.025, 0.025, "2/80 = 2.5%"},
		{"4GB model on 80GB GPU (A100)", 80, 4, 0, 0.05, 0.05, "4/80 = 5%"},
		{"8GB model on 80GB GPU (A100)", 80, 8, 0, 0.10, 0.10, "8/80 = 10%"},
		{"16GB model on 80GB GPU (A100)", 80, 16, 0, 0.20, 0.20, "16/80 = 20%"},
		{"30GB model on 80GB GPU (A100)", 80, 30, 0, 0.37, 0.38, "30/80 = 37.5%"},
		{"8GB model on 24GB GPU (RTX 4090)", 24, 8, 0, 0.33, 0.34, "8/24 = 33%"},
		{"16GB model on 24GB GPU (RTX 4090)", 24, 16, 0, 0.66, 0.67, "16/24 = 67%"},
		{"20GB model on 24GB GPU (RTX 4090)", 24, 20, 0, 0.83, 0.84, "20/24 = 83%"},
		{"32GB model on 40GB GPU (A100 40GB)", 40, 32, 0, 0.80, 0.80, "32/40 = 80%"},
		{"60GB model on 80GB GPU (A100 80GB)", 80, 60, 0, 0.75, 0.75, "60/80 = 75%"},
		{"12GB model on 12GB GPU (RTX 3060)", 12, 12, 0, 0.99, 0.99, "12/12 = 100%, clamped to 99%"},
		{"16GB model on 12GB GPU (impossible)", 12, 16, 0, 0.99, 0.99, "16/12 = 133%, clamped to 99%"},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Set up GPU memory with existing allocations
			ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
				return types.RunnerStatus{
					TotalMemory:     scenario.gpuMemoryGB * 1024 * 1024 * 1024,
					AllocatedMemory: scenario.existingMemoryGB * 1024 * 1024 * 1024,
					GPUCount:        1, // Single GPU for all test scenarios
				}, nil
			}, CacheConfig{updateInterval: 5 * time.Second}))

			// Calculate ratio
			ratio := ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, scenario.modelMemoryGB*1024*1024*1024)

			// Verify ratio is in expected range
			require.GreaterOrEqual(t, ratio, scenario.expectedRatioMin,
				"Ratio %.3f should be >= %.3f for %s", ratio, scenario.expectedRatioMin, scenario.name)
			require.LessOrEqual(t, ratio, scenario.expectedRatioMax,
				"Ratio %.3f should be <= %.3f for %s", ratio, scenario.expectedRatioMax, scenario.name)

			// Calculate what this means in actual memory usage
			actualMemoryUsageGB := float64(scenario.gpuMemoryGB) * ratio

			t.Logf("%s: %.1f%% utilization = %.1fGB GPU allocation (%.1fGB model ratio) - %s",
				scenario.name,
				ratio*100,
				actualMemoryUsageGB,
				float64(scenario.modelMemoryGB),
				scenario.description)
		})
	}
}
