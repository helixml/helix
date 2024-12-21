package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	gomock "go.uber.org/mock/gomock"
)

var _ SlotFactory = &mockRuntimeFactory{}

type mockRuntimeFactory struct {
	getRuntimeFunc func() *Slot
}

// nolint:revive
func (m *mockRuntimeFactory) NewSlot(ctx context.Context,
	slotID uuid.UUID,
	work *scheduler.Workload,
	inferenceResponseHandler func(res *types.RunnerLLMInferenceResponse) error,
	sessionResponseHandler func(res *types.RunnerTaskResponse) error,
	runnerOptions RunnerOptions,
) (*Slot, error) {
	return m.getRuntimeFunc(), nil
}

func TestController_GetSlots(t *testing.T) {
	// Create a httptest server to test the getSlots method
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(`{"slots": 10}`)); err != nil {
			t.Fatalf("error writing slots: %v", err)
		}
	}))
	defer server.Close()

	// Create a new controller with the server URL
	runner, err := NewRunner(context.Background(), RunnerOptions{
		ApiHost:     server.URL,
		ID:          "test",
		ApiToken:    "test",
		MemoryBytes: 1,
		RuntimeFactory: &mockRuntimeFactory{
			getRuntimeFunc: func() *Slot {
				return &Slot{}
			},
		},
	})
	assert.NoError(t, err)

	// Test the getSlots method
	_, err = runner.getSlots()
	assert.NoError(t, err)
}

func TestController_SlotLifecycle(t *testing.T) {
	testSlotID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	apiSlots := &types.GetDesiredRunnerSlotsResponse{
		Data: []types.DesiredRunnerSlot{},
	}
	// Create a httptest server to test the getSlots method
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		data, err := json.Marshal(apiSlots)
		assert.NoError(t, err)
		if _, err := w.Write(data); err != nil {
			t.Fatalf("error writing api slots: %v", err)
		}
	}))
	defer server.Close()

	// Create a new controller with the server URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Cancel when we finish the test to kill all the started runtimes
	ctrl := gomock.NewController(t)
	m := NewMockModelInstance(ctrl)
	m.EXPECT().ID().Return("test").AnyTimes()
	m.EXPECT().IsActive().Return(true).Times(1)
	m.EXPECT().Stop().Times(1)
	mockLLMWorkChan := make(chan *types.RunnerLLMInferenceRequest, 1)
	runner, err := NewRunner(ctx, RunnerOptions{
		ApiHost:     server.URL,
		ID:          "test",
		ApiToken:    "test",
		MemoryBytes: 1,
		RuntimeFactory: &mockRuntimeFactory{
			getRuntimeFunc: func() *Slot {
				return &Slot{
					modelInstance: m,
					llmWorkChan:   mockLLMWorkChan,
				}
			},
		},
	})
	assert.NoError(t, err)

	// Test no slots
	err = runner.pollSlots(context.Background())
	assert.NoError(t, err)

	// Simulate the control plan adding a new slot
	apiSlots = &types.GetDesiredRunnerSlotsResponse{
		Data: []types.DesiredRunnerSlot{
			{
				ID: testSlotID,
				Attributes: types.DesiredRunnerSlotAttributes{
					Workload: &types.RunnerWorkload{
						LLMInferenceRequest: &types.RunnerLLMInferenceRequest{
							RequestID: "test",
							Request: &openai.ChatCompletionRequest{
								Model: model.Model_Ollama_Llama3_8b,
							},
						},
					},
				},
			},
		},
	}

	// Runner should have a slot now
	err = runner.pollSlots(context.Background())
	assert.NoError(t, err)
	assert.Len(t, runner.slots, 1)

	// Check some more work does nothing, because the slot is being started
	err = runner.pollSlots(context.Background())
	assert.NoError(t, err)
	assert.Len(t, runner.slots, 1)

	// Simulate the slot not being active any more, which should mean it takes new work
	m.EXPECT().IsActive().Return(false).Times(1)
	assert.Len(t, mockLLMWorkChan, 0)
	err = runner.pollSlots(context.Background())
	assert.NoError(t, err)
	assert.Len(t, runner.slots, 1)
	assert.Len(t, mockLLMWorkChan, 1)

	// Delete the slot
	apiSlots = &types.GetDesiredRunnerSlotsResponse{
		Data: []types.DesiredRunnerSlot{},
	}
	err = runner.pollSlots(context.Background())
	assert.NoError(t, err)

	assert.Len(t, runner.slots, 0)

	// Simulate the control plan adding two slots at the same time
	apiSlots = &types.GetDesiredRunnerSlotsResponse{
		Data: []types.DesiredRunnerSlot{
			{
				ID: testSlotID,
				Attributes: types.DesiredRunnerSlotAttributes{
					Workload: &types.RunnerWorkload{
						LLMInferenceRequest: &types.RunnerLLMInferenceRequest{
							RequestID: "test-1",
							Request: &openai.ChatCompletionRequest{
								Model: model.Model_Ollama_Llama3_8b,
							},
						},
					},
				},
			},
			{
				ID: uuid.MustParse("00000000-0000-0000-0000-000000000002"),
				Attributes: types.DesiredRunnerSlotAttributes{
					Workload: &types.RunnerWorkload{
						LLMInferenceRequest: &types.RunnerLLMInferenceRequest{
							RequestID: "test-2",
							Request: &openai.ChatCompletionRequest{
								Model: model.Model_Ollama_Llama3_8b,
							},
						},
					},
				},
			},
		},
	}

	// Runner should have two slots now
	err = runner.pollSlots(context.Background())
	assert.NoError(t, err)
	assert.Len(t, runner.slots, 2)
}
