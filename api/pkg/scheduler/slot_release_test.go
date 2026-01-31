package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
)

// TestSlotReleaseOnError verifies that slot.Release() is called even when
// the goroutine in allocateSlot() encounters an error path that doesn't
// call any Submit method. This is a regression test for the slot counter leak bug.
func TestSlotReleaseOnError(t *testing.T) {
	ctx := context.Background()

	model := &types.Model{
		ID:      "test-model",
		Runtime: types.RuntimeOllama,
		Memory:  1024 * 1024 * 1024,
	}

	// Create a session workload with no LoraDir - this will trigger error path
	// on line 2066-2068 that doesn't call any Submit method
	session := &types.Session{
		ID:        "test-session",
		Type:      types.SessionTypeText,
		Mode:      types.SessionModeInference,
		ModelName: "test-model",
		// LoraDir is empty - this triggers the error path
	}

	workload, err := NewSessionWorkload(session, model)
	require.NoError(t, err)

	slot := &Slot{
		ID:               uuid.New(),
		RunnerID:         "test-runner",
		initialWork:      workload,
		isRunning:        true,
		maxConcurrency:   1,
		activeRequests:   0,
		LastActivityTime: time.Now(),
	}

	slotStore := &SlotStore{
		cache: make(map[uuid.UUID]*Slot),
	}
	slotStore.cache[slot.ID] = slot

	scheduler := &Scheduler{
		ctx:        ctx,
		controller: &RunnerController{},
		slots:      slotStore,
		onSchedulingErr: func(work *Workload, err error) {
			// Expected to be called for the error case
		},
	}

	// Verify initial state
	require.Equal(t, int64(0), slot.GetActiveRequests(), "slot should start with 0 active requests")

	// Call allocateSlot - this will hit the error path that doesn't call Submit
	err = scheduler.allocateSlot(slot.ID, workload)
	require.NoError(t, err, "allocateSlot should not return error")

	// Wait for goroutine to complete
	time.Sleep(100 * time.Millisecond)

	// Verify the slot was released despite the error path
	activeRequests := slot.GetActiveRequests()
	require.Equal(t, int64(0), activeRequests,
		"slot should have 0 active requests after error path completes")
}

// TestSlotReleaseOnSuccess verifies that slot.Release() is called when
// a normal request completes successfully (or at least calls the Submit method)
func TestSlotReleaseOnSuccess(t *testing.T) {
	ctx := context.Background()

	model := &types.Model{
		ID:      "test-model",
		Runtime: types.RuntimeOllama,
		Memory:  1024 * 1024 * 1024,
	}

	llmReq := &types.RunnerLLMInferenceRequest{
		RequestID: "test-req",
		OwnerID:   "test-owner",
		Request:   &openai.ChatCompletionRequest{Model: model.ID},
	}

	workload, err := NewLLMWorkload(llmReq, model)
	require.NoError(t, err)

	slot := &Slot{
		ID:               uuid.New(),
		RunnerID:         "test-runner",
		initialWork:      workload,
		isRunning:        true,
		maxConcurrency:   1,
		activeRequests:   0,
		LastActivityTime: time.Now(),
	}

	// Create a mock RunnerClient that returns success immediately
	mockClient := &mockSuccessRunnerClient{}

	controller := &RunnerController{}
	controller.runnerClient = mockClient

	slotStore := &SlotStore{
		cache: make(map[uuid.UUID]*Slot),
	}
	slotStore.cache[slot.ID] = slot

	scheduler := &Scheduler{
		ctx:        ctx,
		controller: controller,
		slots:      slotStore,
		onSchedulingErr: func(work *Workload, err error) {
			t.Errorf("Unexpected error: %v", err)
		},
	}

	// Verify initial state
	require.Equal(t, int64(0), slot.GetActiveRequests())

	// Call allocateSlot
	err = scheduler.allocateSlot(slot.ID, workload)
	require.NoError(t, err)

	// Wait for goroutine to complete
	time.Sleep(100 * time.Millisecond)

	// Verify the slot was released after successful completion
	activeRequests := slot.GetActiveRequests()
	require.Equal(t, int64(0), activeRequests,
		"slot should have 0 active requests after successful completion")
}

// mockSuccessRunnerClient is a simple mock that returns success for all operations
type mockSuccessRunnerClient struct{}

func (m *mockSuccessRunnerClient) CreateSlot(runnerID string, slotID uuid.UUID, req *types.CreateRunnerSlotRequest) error {
	return nil
}

func (m *mockSuccessRunnerClient) DeleteSlot(runnerID string, slotID uuid.UUID) error {
	return nil
}

func (m *mockSuccessRunnerClient) FetchSlot(runnerID string, slotID uuid.UUID) (types.RunnerSlot, error) {
	return types.RunnerSlot{}, nil
}

func (m *mockSuccessRunnerClient) FetchSlots(runnerID string) (types.ListRunnerSlotsResponse, error) {
	return types.ListRunnerSlotsResponse{}, nil
}

func (m *mockSuccessRunnerClient) FetchStatus(runnerID string) (types.RunnerStatus, error) {
	return types.RunnerStatus{}, nil
}

func (m *mockSuccessRunnerClient) SyncSystemSettings(runnerID string, settings *types.RunnerSystemConfigRequest) error {
	return nil
}

func (m *mockSuccessRunnerClient) SubmitChatCompletionRequest(slot *Slot, req *types.RunnerLLMInferenceRequest) error {
	// Simulate successful submission - just return nil
	return nil
}

func (m *mockSuccessRunnerClient) SubmitEmbeddingRequest(slot *Slot, req *types.RunnerLLMInferenceRequest) error {
	return nil
}

func (m *mockSuccessRunnerClient) SubmitImageGenerationRequest(slot *Slot, session *types.Session) error {
	return nil
}
