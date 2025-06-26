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
		PubSub: ps,
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
		PubSub: ps,
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

	// Test case 1: Small model on large GPU (should hit minimum ratio of 35%)
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory: 80 * 1024 * 1024 * 1024, // 80GB GPU
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))
	ratio := ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, 8*1024*1024*1024) // 8GB model
	require.Equal(t, 0.35, ratio)                                                 // Should hit minimum ratio of 35%

	// Test case 2: Very tiny model (should hit minimum ratio)
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory: 80 * 1024 * 1024 * 1024, // 80GB GPU
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))
	ratio = ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, 1*1024*1024*1024) // 1GB model
	require.Equal(t, 0.35, ratio)                                                // Should hit minimum ratio

	// Test case 3: Medium model on GPU (reasonable ratio)
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory: 24 * 1024 * 1024 * 1024, // 24GB GPU
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))
	ratio = ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, 16*1024*1024*1024) // 16GB model
	require.Greater(t, ratio, 0.35)
	require.Less(t, ratio, 0.95)
	require.InDelta(t, 0.6667, ratio, 0.01) // Should be exactly 66.67% (16/24)

	// Test case 4: Large model on small GPU
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory: 24 * 1024 * 1024 * 1024, // 24GB GPU
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))
	ratio = ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, 20*1024*1024*1024) // 20GB model
	require.Greater(t, ratio, 0.35)
	require.Less(t, ratio, 0.95)
	require.InDelta(t, 0.8333, ratio, 0.01) // Should be exactly 83.33% (20/24)

	// Test case 5: No GPU memory info (fallback)
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory: 0,
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))
	ratio = ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, 8*1024*1024*1024)
	require.Equal(t, 0.8, ratio) // Should return default fallback

	// Test case 6: Model larger than GPU (should hit maximum ratio)
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory: 8 * 1024 * 1024 * 1024, // 8GB GPU
		}, nil
	}, CacheConfig{updateInterval: 5 * time.Second}))
	ratio = ctrl.calculateVLLMMemoryUtilizationRatio(runnerID, 10*1024*1024*1024) // 10GB model
	require.Equal(t, 0.95, ratio)                                                 // Should hit maximum ratio
}

func TestSubstituteVLLMArgsPlaceholders(t *testing.T) {
	ctrl := &RunnerController{
		statusCache: NewLockingRunnerMap[types.RunnerStatus](),
	}

	runnerID := "test-runner"
	ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory: 24 * 1024 * 1024 * 1024, // 24GB GPU
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

	// Test case 2: No placeholders (should return unchanged)
	argsWithoutPlaceholder := []string{
		"--trust-remote-code",
		"--max-model-len", "32768",
	}

	substitutedArgs = ctrl.substituteVLLMArgsPlaceholders(argsWithoutPlaceholder, runnerID, 8*1024*1024*1024)
	require.Equal(t, argsWithoutPlaceholder, substitutedArgs)

	// Test case 3: Empty args (should return unchanged)
	emptyArgs := []string{}
	substitutedArgs = ctrl.substituteVLLMArgsPlaceholders(emptyArgs, runnerID, 8*1024*1024*1024)
	require.Equal(t, emptyArgs, substitutedArgs)

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
		expectedRatioMin float64
		expectedRatioMax float64
	}{
		// Tiny models on large GPUs - should hit minimum of 35%
		{"1GB model on 80GB GPU (A100)", 80, 1, 0.35, 0.35}, // Should hit minimum
		{"2GB model on 80GB GPU (A100)", 80, 2, 0.35, 0.35}, // Should hit minimum
		{"4GB model on 80GB GPU (A100)", 80, 4, 0.35, 0.35}, // Should hit minimum

		// Small models on large GPUs - should hit minimum of 35%
		{"8GB model on 80GB GPU (A100)", 80, 8, 0.35, 0.35},   // 8/80 = 10% but clamped to 35%
		{"16GB model on 80GB GPU (A100)", 80, 16, 0.35, 0.35}, // 16/80 = 20% but clamped to 35%

		// Medium models on medium GPUs - exact ratios
		{"8GB model on 24GB GPU (RTX 4090)", 24, 8, 0.35, 0.35},   // 8/24 = 33.33% but clamped to 35%
		{"16GB model on 24GB GPU (RTX 4090)", 24, 16, 0.65, 0.68}, // 16/24 = 66.67%

		// Large models - exact ratios
		{"20GB model on 24GB GPU (RTX 4090)", 24, 20, 0.82, 0.85},  // 20/24 = 83.33%
		{"32GB model on 40GB GPU (A100 40GB)", 40, 32, 0.79, 0.81}, // 32/40 = 80%
		{"60GB model on 80GB GPU (A100 80GB)", 80, 60, 0.74, 0.76}, // 60/80 = 75%

		// Edge cases
		{"12GB model on 12GB GPU (RTX 3060)", 12, 12, 0.95, 0.95},   // Should hit maximum (100% would be 1.0)
		{"16GB model on 12GB GPU (impossible)", 12, 16, 0.95, 0.95}, // Should hit maximum
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Set up GPU memory
			ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
				return types.RunnerStatus{
					TotalMemory: scenario.gpuMemoryGB * 1024 * 1024 * 1024,
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

			t.Logf("%s: %.1f%% utilization = %.1fGB actual usage (%.1fGB model + %.1fGB overhead)",
				scenario.name,
				ratio*100,
				actualMemoryUsageGB,
				float64(scenario.modelMemoryGB),
				actualMemoryUsageGB-float64(scenario.modelMemoryGB))
		})
	}
}
