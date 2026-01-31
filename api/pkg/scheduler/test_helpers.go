package scheduler

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog"
	openai "github.com/sashabaranov/go-openai"
	"go.uber.org/mock/gomock"
)

// init runs before any tests and sets reasonable log level defaults
func init() {
	// Set a reasonable log level for tests if not already set
	if os.Getenv("LOG_LEVEL") == "" {
		os.Setenv("LOG_LEVEL", "error")
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	}
}

// TestMain sets up logging configuration for all scheduler tests
func TestMain(m *testing.M) {
	// Initialize logging with the configured level
	system.SetupLogging()

	// Run the tests
	code := m.Run()
	os.Exit(code)
}

// MockHealthChecker implements HealthChecker for testing - always reports healthy
type MockHealthChecker struct{}

func (m *MockHealthChecker) GetHealthz(_ string) error {
	// In tests, always return healthy
	return nil
}

func (m *MockHealthChecker) SetModels(_ string) error {
	// In tests, always return success for model setting
	return nil
}

// CreateMockSchedulerParams creates scheduler params with a mock store for testing
func CreateMockSchedulerParams(t *testing.T, runnerController *RunnerController) *Params {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)

	// Set up basic mock expectations for slot operations
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSpecTask(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	return &Params{
		RunnerController: runnerController,
		Store:            mockStore,
	}
}

// MockRunnerClient implements RunnerClient for testing - always succeeds
type MockRunnerClient struct {
	TotalMemory uint64                         // Total memory to report for each runner
	GPUCount    int                            // Number of GPUs to report
	Models      []*types.RunnerModelStatus     // Models to report as available on the runner
	slots       map[string][]*types.RunnerSlot // Track slots per runner for memory calculations
}

func (m *MockRunnerClient) CreateSlot(runnerID string, slotID uuid.UUID, req *types.CreateRunnerSlotRequest) error {
	// In tests, track the slot creation for memory calculations
	if m.slots == nil {
		m.slots = make(map[string][]*types.RunnerSlot)
	}

	slot := &types.RunnerSlot{
		ID:       slotID,
		Model:    req.Attributes.Model,
		Active:   true,
		Ready:    true, // In tests, slots are immediately ready
		Runtime:  req.Attributes.Runtime,
		GPUIndex: req.Attributes.GPUIndex,
	}

	m.slots[runnerID] = append(m.slots[runnerID], slot)

	// Debug logging to see what slots are being created (commented out for cleaner output)
	// fmt.Printf("🔧 MockRunnerClient.CreateSlot: runner=%s, model=%s, slots_count=%d\n",
	//	runnerID, req.Attributes.Model, len(m.slots[runnerID]))

	return nil
}

func (m *MockRunnerClient) DeleteSlot(runnerID string, slotID uuid.UUID) error {
	// In tests, remove the slot from tracking
	if m.slots == nil {
		return nil
	}

	slots := m.slots[runnerID]
	for i, slot := range slots {
		if slot.ID == slotID {
			// Remove slot from slice
			m.slots[runnerID] = append(slots[:i], slots[i+1:]...)
			break
		}
	}
	return nil
}

func (m *MockRunnerClient) FetchSlot(runnerID string, slotID uuid.UUID) (types.RunnerSlot, error) {
	// In tests, find the specific slot for this runner
	if m.slots == nil {
		return types.RunnerSlot{}, fmt.Errorf("slot not found: %s", slotID)
	}

	slots := m.slots[runnerID]
	if slots == nil {
		return types.RunnerSlot{}, fmt.Errorf("slot not found: %s", slotID)
	}

	// Find the slot with matching ID
	for _, slot := range slots {
		if slot.ID == slotID {
			return *slot, nil
		}
	}

	return types.RunnerSlot{}, fmt.Errorf("slot not found: %s", slotID)
}

func (m *MockRunnerClient) FetchSlots(runnerID string) (types.ListRunnerSlotsResponse, error) {
	// In tests, return the tracked slots for this runner
	if m.slots == nil {
		return types.ListRunnerSlotsResponse{Slots: []*types.RunnerSlot{}}, nil
	}

	slots := m.slots[runnerID]
	if slots == nil {
		slots = []*types.RunnerSlot{}
	}

	return types.ListRunnerSlotsResponse{Slots: slots}, nil
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

	// Use configured models if provided, otherwise use default models
	models := m.Models
	if models == nil {
		// Default models for backward compatibility
		models = []*types.RunnerModelStatus{
			{ModelID: "qwen3:8b", Runtime: types.RuntimeOllama, DownloadInProgress: false},
			{ModelID: "gpt-oss:20b", Runtime: types.RuntimeOllama, DownloadInProgress: false},
			{ModelID: "qwen2.5vl:32b", Runtime: types.RuntimeOllama, DownloadInProgress: false},
			{ModelID: "NousResearch/Hermes-3-Llama-3.1-8B", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
			{ModelID: "MrLight/dse-qwen2-2b-mrl-v1", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
		}
	}

	return types.RunnerStatus{
		ID:          runnerID,
		TotalMemory: totalMemory,
		GPUCount:    gpuCount,
		GPUs:        gpus,
		Models:      models,
	}, nil
}

func (m *MockRunnerClient) SyncSystemSettings(_ string, _ *types.RunnerSystemConfigRequest) error {
	// In tests, always succeed with system settings sync
	return nil
}

func (m *MockRunnerClient) SubmitChatCompletionRequest(_ *Slot, _ *types.RunnerLLMInferenceRequest) error {
	// In tests, always succeed with chat completion requests
	return nil
}

func (m *MockRunnerClient) SubmitEmbeddingRequest(_ *Slot, _ *types.RunnerLLMInferenceRequest) error {
	// In tests, always succeed with embedding requests
	return nil
}

func (m *MockRunnerClient) SubmitImageGenerationRequest(_ *Slot, _ *types.Session) error {
	// In tests, always succeed with image generation requests
	return nil
}

// NewMockRunnerClient creates a MockRunnerClient with specified memory configuration
func NewMockRunnerClient(totalMemoryGB uint64, gpuCount int) *MockRunnerClient {
	return &MockRunnerClient{
		TotalMemory: totalMemoryGB * 1024 * 1024 * 1024, // Convert GB to bytes
		GPUCount:    gpuCount,
		slots:       make(map[string][]*types.RunnerSlot),
	}
}

// DefaultMockRunnerClient creates a MockRunnerClient with default configuration (200GB, 2 GPUs)
func DefaultMockRunnerClient() *MockRunnerClient {
	return NewMockRunnerClient(200, 2)
}

// NewMockRunnerClientWithModels creates a MockRunnerClient with specific models
func NewMockRunnerClientWithModels(totalMemoryGB uint64, gpuCount int, models []*types.RunnerModelStatus) *MockRunnerClient {
	client := NewMockRunnerClient(totalMemoryGB, gpuCount)
	client.Models = models
	return client
}

// SetupMinimalSchedulerContext sets up a minimal scheduler context for unit tests that need
// to test RunnerController methods in isolation. This provides the scheduler slots callback
// without requiring a full scheduler instance.
func SetupMinimalSchedulerContext(runnerCtrl *RunnerController, testSlots map[uuid.UUID]*Slot) {
	// Set up the scheduler slots callback to return the test slots
	runnerCtrl.setSchedulerSlotsCallback(func() map[uuid.UUID]*Slot {
		return testSlots
	})
}

// CreateTestSlot creates a test slot for use in unit tests
func CreateTestSlot(runnerID string, configuredModel *types.Model, gpuIndex *int, gpuIndices []int) *Slot {
	// Create a minimal workload for the slot
	work := &Workload{
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			RequestID: uuid.New().String(),
			Request: &openai.ChatCompletionRequest{
				Model: configuredModel.ID,
				Messages: []openai.ChatCompletionMessage{
					{Role: "user", Content: "test"},
				},
			},
		},
		model: configuredModel,
	}

	// Create GPU allocation
	var gpuAllocation *GPUAllocation
	if len(gpuIndices) > 1 {
		gpuAllocation = &GPUAllocation{
			WorkloadID:         work.ID(),
			RunnerID:           runnerID,
			MultiGPUs:          gpuIndices,
			TensorParallelSize: len(gpuIndices),
		}
	} else if gpuIndex != nil {
		gpuAllocation = &GPUAllocation{
			WorkloadID:         work.ID(),
			RunnerID:           runnerID,
			SingleGPU:          gpuIndex,
			TensorParallelSize: 1,
		}
	}

	return NewSlot(runnerID, work, func(string, time.Time) bool { return false },
		func(string, time.Time) bool { return false }, gpuAllocation)
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
