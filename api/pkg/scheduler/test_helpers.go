package scheduler

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/model"
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

// MockRunnerClient implements RunnerClient for testing - always succeeds
type MockRunnerClient struct {
	TotalMemory uint64 // Total memory to report for each runner
	GPUCount    int    // Number of GPUs to report
}

func (m *MockRunnerClient) CreateSlot(runnerID string, slotID uuid.UUID, req *types.CreateRunnerSlotRequest) error {
	// In tests, always succeed with slot creation
	return nil
}

func (m *MockRunnerClient) DeleteSlot(runnerID string, slotID uuid.UUID) error {
	// In tests, always succeed with slot deletion
	return nil
}

func (m *MockRunnerClient) FetchSlots(runnerID string) (types.ListRunnerSlotsResponse, error) {
	// In tests, return empty slots list
	return types.ListRunnerSlotsResponse{Slots: []*types.RunnerSlot{}}, nil
}

func (m *MockRunnerClient) FetchStatus(runnerID string) (types.RunnerStatus, error) {
	// Use configured memory values, with sensible defaults if not set
	totalMemory := m.TotalMemory
	if totalMemory == 0 {
		totalMemory = 200 * 1024 * 1024 * 1024 // Default to 200GB
	}

	gpuCount := m.GPUCount
	if gpuCount == 0 {
		gpuCount = 2 // Default to 2 GPUs
	}

	memoryPerGPU := totalMemory / uint64(gpuCount)
	gpus := make([]*types.GPUStatus, gpuCount)
	for i := 0; i < gpuCount; i++ {
		gpus[i] = &types.GPUStatus{
			Index:       i,
			TotalMemory: memoryPerGPU,
			FreeMemory:  memoryPerGPU,
			UsedMemory:  0,
		}
	}

	return types.RunnerStatus{
		ID:          runnerID,
		TotalMemory: totalMemory,
		GPUCount:    gpuCount,
		GPUs:        gpus,
	}, nil
}

func (m *MockRunnerClient) SyncSystemSettings(runnerID string, settings *types.RunnerSystemConfigRequest) error {
	// In tests, always succeed with system settings sync
	return nil
}

// NewMockRunnerClient creates a MockRunnerClient with specified memory configuration
func NewMockRunnerClient(totalMemoryGB uint64, gpuCount int) *MockRunnerClient {
	return &MockRunnerClient{
		TotalMemory: totalMemoryGB * 1024 * 1024 * 1024, // Convert GB to bytes
		GPUCount:    gpuCount,
	}
}

// DefaultMockRunnerClient creates a MockRunnerClient with default configuration (200GB, 2 GPUs)
func DefaultMockRunnerClient() *MockRunnerClient {
	return NewMockRunnerClient(200, 2)
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
// This function reads the real models from models.go to ensure tests stay in sync
func GetDefaultTestModels() []*types.Model {
	var testModels []*types.Model

	// Get Ollama models
	ollamaModels, err := model.GetDefaultOllamaModels()
	if err == nil {
		for _, m := range ollamaModels {
			testModels = append(testModels, &types.Model{
				ID:            m.ID,
				Name:          m.Name,
				Memory:        m.Memory,
				ContextLength: m.ContextLength,
				Description:   m.Description,
				Hide:          m.Hide,
				Prewarm:       m.Prewarm,
				Runtime:       types.RuntimeOllama,
				Type:          types.ModelTypeChat,
			})
		}
	}

	// Get VLLM models
	vllmModels, err := model.GetDefaultVLLMModels()
	if err == nil {
		for _, m := range vllmModels {
			testModels = append(testModels, &types.Model{
				ID:            m.ID,
				Name:          m.Name,
				Memory:        m.Memory,
				ContextLength: m.ContextLength,
				Description:   m.Description,
				Hide:          m.Hide,
				Prewarm:       m.Prewarm,
				Runtime:       types.RuntimeVLLM,
				Type:          types.ModelTypeChat,
			})
		}
	}

	// Get Diffusers models
	diffusersModels, err := model.GetDefaultDiffusersModels()
	if err == nil {
		for _, m := range diffusersModels {
			testModels = append(testModels, &types.Model{
				ID:          m.ID,
				Name:        m.Name,
				Memory:      m.GetMemoryRequirements(types.SessionModeInference),
				Description: m.Description,
				Hide:        m.GetHidden(),
				Runtime:     types.RuntimeDiffusers,
				Type:        types.ModelTypeImage,
			})
		}
	}

	return testModels
}
