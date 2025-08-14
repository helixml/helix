package scheduler

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// MockHealthChecker implements HealthChecker for testing - always reports healthy
type MockHealthChecker struct{}

func (m *MockHealthChecker) GetHealthz(runnerID string) error {
	// In tests, always return healthy
	return nil
}

func (m *MockHealthChecker) SetModels(runnerID string) error {
	// In tests, always return success for model setting
	return nil
}

// MockRunnerSetup configures a test runner controller to behave like a healthy runner
// without requiring NATS communication
func MockRunnerSetup(mockCtrl *RunnerController, runnerID string, gpus []*types.GPUStatus) {
	// Mock the runner as connected using the proper method
	mockCtrl.OnConnectedHandler(runnerID)

	// Mock status to return proper GPU configuration
	status := &types.RunnerStatus{
		ID:   runnerID,
		GPUs: gpus,
	}

	// Set up the status cache directly
	mockCtrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return *status, nil
	}, CacheConfig{
		updateInterval: 1 * time.Second,
	}))

	// Mock slots to return empty initially
	mockCtrl.slotsCache.Set(runnerID, NewCache(context.Background(), func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{
			Slots: []*types.RunnerSlot{},
		}, nil
	}, CacheConfig{
		updateInterval: 1 * time.Second,
	}))
}

// CreateTestGPUs creates a slice of test GPUs with specified memory
func CreateTestGPUs(memoryPerGPU uint64, count int) []*types.GPUStatus {
	gpus := make([]*types.GPUStatus, count)
	for i := 0; i < count; i++ {
		gpus[i] = &types.GPUStatus{
			Index:       i,
			TotalMemory: memoryPerGPU,
			FreeMemory:  memoryPerGPU,
			UsedMemory:  0,
			ModelName:   "Test GPU",
		}
	}
	return gpus
}

// GetDefaultTestModels returns a set of test models with known memory requirements
func GetDefaultTestModels() []*types.Model {
	return []*types.Model{
		{ID: "MrLight/dse-qwen2-2b-mrl-v1", Memory: 4 * 1024 * 1024 * 1024, Runtime: types.RuntimeOllama, Prewarm: true},         // 4GB
		{ID: "microsoft/DialoGPT-medium", Memory: 8 * 1024 * 1024 * 1024, Runtime: types.RuntimeOllama, Prewarm: true},           // 8GB
		{ID: "NousResearch/Hermes-3-Llama-3.1-8B", Memory: 12 * 1024 * 1024 * 1024, Runtime: types.RuntimeOllama, Prewarm: true}, // 12GB
	}
}
