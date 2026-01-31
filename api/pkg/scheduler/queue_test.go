package scheduler

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
)

func TestGetRequiredSlots_PreservesFIFOOrder(t *testing.T) {
	queue := NewWorkQueue(10)

	// Create test models in specific order: Ollama first, then vLLM
	ollamaModel1 := &types.Model{
		ID:      "ollama-model-1",
		Runtime: types.RuntimeOllama,
		Memory:  1024 * 1024 * 1024, // 1GB
	}

	ollamaModel2 := &types.Model{
		ID:      "ollama-model-2",
		Runtime: types.RuntimeOllama,
		Memory:  2 * 1024 * 1024 * 1024, // 2GB
	}

	vllmModel1 := &types.Model{
		ID:      "vllm-model-1",
		Runtime: types.RuntimeVLLM,
		Memory:  4 * 1024 * 1024 * 1024, // 4GB
	}

	vllmModel2 := &types.Model{
		ID:      "vllm-model-2",
		Runtime: types.RuntimeVLLM,
		Memory:  8 * 1024 * 1024 * 1024, // 8GB
	}

	// Create workloads and add them in FIFO order: Ollama first, then vLLM
	workload1, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
		RequestID: "req-1",
		Request:   &openai.ChatCompletionRequest{Model: ollamaModel1.ID},
	}, ollamaModel1)
	require.NoError(t, err)

	workload2, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
		RequestID: "req-2",
		Request:   &openai.ChatCompletionRequest{Model: ollamaModel2.ID},
	}, ollamaModel2)
	require.NoError(t, err)

	workload3, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
		RequestID: "req-3",
		Request:   &openai.ChatCompletionRequest{Model: vllmModel1.ID},
	}, vllmModel1)
	require.NoError(t, err)

	workload4, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
		RequestID: "req-4",
		Request:   &openai.ChatCompletionRequest{Model: vllmModel2.ID},
	}, vllmModel2)
	require.NoError(t, err)

	// Add duplicate of first model to test deduplication preserves first occurrence order
	workload5, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
		RequestID: "req-5",
		Request:   &openai.ChatCompletionRequest{Model: ollamaModel1.ID},
	}, ollamaModel1)
	require.NoError(t, err)

	// Enqueue in order: Ollama models first, then vLLM models
	err = queue.Add(workload1) // ollama-model-1 (first occurrence)
	require.NoError(t, err)

	err = queue.Add(workload2) // ollama-model-2
	require.NoError(t, err)

	err = queue.Add(workload3) // vllm-model-1
	require.NoError(t, err)

	err = queue.Add(workload4) // vllm-model-2
	require.NoError(t, err)

	err = queue.Add(workload5) // ollama-model-1 (duplicate - should increment count)
	require.NoError(t, err)

	// Get required slots
	requirements := queue.GetRequiredSlots()

	// Should have 4 unique slot requirements (4 unique models)
	require.Len(t, requirements, 4, "Should have 4 unique slot requirements")

	// Verify FIFO order is preserved (order of first occurrence)
	expectedOrder := []string{
		"ollama-model-1", // First in queue
		"ollama-model-2", // Second in queue
		"vllm-model-1",   // Third in queue
		"vllm-model-2",   // Fourth in queue
	}

	for i, req := range requirements {
		require.Equal(t, expectedOrder[i], req.Model.String(),
			"Slot requirement %d should be for model %s, got %s", i, expectedOrder[i], req.Model.String())
	}

	// Verify runtime types are preserved and in correct order
	require.Equal(t, types.RuntimeOllama, requirements[0].Runtime, "First requirement should be Ollama")
	require.Equal(t, types.RuntimeOllama, requirements[1].Runtime, "Second requirement should be Ollama")
	require.Equal(t, types.RuntimeVLLM, requirements[2].Runtime, "Third requirement should be vLLM")
	require.Equal(t, types.RuntimeVLLM, requirements[3].Runtime, "Fourth requirement should be vLLM")

	// Verify count is correct for the duplicated model
	require.Equal(t, 2, requirements[0].Count, "ollama-model-1 should have count=2 due to duplicate")
	require.Equal(t, 1, requirements[1].Count, "ollama-model-2 should have count=1")
	require.Equal(t, 1, requirements[2].Count, "vllm-model-1 should have count=1")
	require.Equal(t, 1, requirements[3].Count, "vllm-model-2 should have count=1")
}

func TestGetRequiredSlots_EmptyQueue(t *testing.T) {
	queue := NewWorkQueue(10)
	requirements := queue.GetRequiredSlots()
	require.Empty(t, requirements, "Empty queue should return no requirements")
}

func TestGetRequiredSlots_SingleModel(t *testing.T) {
	queue := NewWorkQueue(10)

	model := &types.Model{
		ID:      "test-model",
		Runtime: types.RuntimeOllama,
		Memory:  1024 * 1024 * 1024,
	}

	workload, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
		RequestID: "req-1",
		Request:   &openai.ChatCompletionRequest{Model: model.ID},
	}, model)
	require.NoError(t, err)

	err = queue.Add(workload)
	require.NoError(t, err)

	requirements := queue.GetRequiredSlots()
	require.Len(t, requirements, 1, "Should have exactly one requirement")
	require.Equal(t, "test-model", requirements[0].Model.String())
	require.Equal(t, types.RuntimeOllama, requirements[0].Runtime)
	require.Equal(t, 1, requirements[0].Count)
}
