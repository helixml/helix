package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

func TestController_GetSlots(t *testing.T) {
	// Create a httptest server to test the getSlots method
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"slots": 10}`))
	}))
	defer server.Close()

	// Create a new controller with the server URL
	runner, err := NewRunner(context.Background(), RunnerOptions{
		ApiHost:     server.URL,
		ID:          "test",
		ApiToken:    "test",
		MemoryBytes: 1,
	}, nil)
	assert.NoError(t, err)

	// Test the getSlots method
	_, err = runner.getSlots()
	assert.NoError(t, err)
}

func TestController_SlotLifecycle(t *testing.T) {
	testSlotID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	apiSlots := &types.PatchRunnerSlots{
		Data: []types.RunnerSlot{},
	}
	// Create a httptest server to test the getSlots method
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := json.Marshal(apiSlots)
		assert.NoError(t, err)
		w.Write(data)
	}))
	defer server.Close()

	// Create a new controller with the server URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Cancel when we finish the test to kill all the started runtimes
	runner, err := NewRunner(ctx, RunnerOptions{
		ApiHost:     server.URL,
		ID:          "test",
		ApiToken:    "test",
		MemoryBytes: 1,
	}, nil)
	assert.NoError(t, err)

	// Test no slots
	err = runner.pollSlots(context.Background())
	assert.NoError(t, err)

	// Simulate the control plan adding a new slot
	apiSlots = &types.PatchRunnerSlots{
		Data: []types.RunnerSlot{
			{
				ID: testSlotID,
				Attributes: types.RunnerSlotAttributes{
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

	// Delete the slot
	apiSlots = &types.PatchRunnerSlots{
		Data: []types.RunnerSlot{},
	}
	err = runner.pollSlots(context.Background())
	assert.NoError(t, err)

	assert.Len(t, runner.slots, 0)
}
