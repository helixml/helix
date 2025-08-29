package scheduler

import (
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlotConcurrency(t *testing.T) {
	t.Run("single request default behavior", func(t *testing.T) {
		// Test that slots with concurrency=1 behave like before
		model := &types.Model{
			ID:          "test-model",
			Runtime:     types.RuntimeOllama,
			Memory:      1000000,
			Concurrency: 1, // Explicitly set to 1 for this test
		}

		workload, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
			RequestID: "test-1",
			Request: &openai.ChatCompletionRequest{
				Model: "test-model",
			},
		}, model)
		require.NoError(t, err)

		slot := NewSlot("runner-1", workload,
			func(string, time.Time) bool { return false },
			func(string, time.Time) bool { return false },
			nil)

		assert.False(t, slot.IsActive())
		assert.True(t, slot.HasCapacity())
		assert.Equal(t, int64(0), slot.GetActiveRequests())

		// Start first request
		slot.Start()
		assert.True(t, slot.IsActive())
		assert.False(t, slot.HasCapacity()) // Should be at capacity with concurrency=1
		assert.Equal(t, int64(1), slot.GetActiveRequests())

		// Release request
		slot.Release()
		assert.False(t, slot.IsActive())
		assert.True(t, slot.HasCapacity())
		assert.Equal(t, int64(0), slot.GetActiveRequests())
	})

	t.Run("multiple concurrent requests", func(t *testing.T) {
		// Test that slots can handle multiple concurrent requests
		model := &types.Model{
			ID:      "test-model",
			Runtime: types.RuntimeOllama, // Use Ollama with default 2 concurrency
			Memory:  1000000,
		}

		workload, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
			RequestID: "test-1",
			Request: &openai.ChatCompletionRequest{
				Model: "test-model",
			},
		}, model)
		require.NoError(t, err)

		slot := NewSlot("runner-1", workload,
			func(string, time.Time) bool { return false },
			func(string, time.Time) bool { return false },
			nil)

		assert.False(t, slot.IsActive())
		assert.True(t, slot.HasCapacity())
		assert.Equal(t, int64(0), slot.GetActiveRequests())

		// Start multiple requests (Ollama defaults to 2)
		for i := 0; i < 1; i++ {
			slot.Start()
			assert.True(t, slot.IsActive())
			assert.True(t, slot.HasCapacity()) // Should still have capacity
			assert.Equal(t, int64(i+1), slot.GetActiveRequests())
		}

		// Start 2nd request - should reach capacity (Ollama default is 2)
		slot.Start()
		assert.True(t, slot.IsActive())
		assert.False(t, slot.HasCapacity()) // Should be at capacity
		assert.Equal(t, int64(2), slot.GetActiveRequests())

		// Release requests
		for i := 1; i >= 0; i-- {
			slot.Release()
			if i > 0 {
				assert.True(t, slot.IsActive())
				assert.True(t, slot.HasCapacity())
			} else {
				assert.False(t, slot.IsActive())
				assert.True(t, slot.HasCapacity())
			}
			assert.Equal(t, int64(i), slot.GetActiveRequests())
		}
	})

	t.Run("per-model concurrency configuration", func(t *testing.T) {
		// Test that model-specific concurrency overrides global config
		model := &types.Model{
			ID:          "custom-model",
			Runtime:     types.RuntimeVLLM,
			Memory:      1000000,
			Concurrency: 8, // Model-specific setting
		}

		workload, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
			RequestID: "test-1",
			Request: &openai.ChatCompletionRequest{
				Model: "custom-model",
			},
		}, model)
		require.NoError(t, err)

		slot := NewSlot("runner-1", workload,
			func(string, time.Time) bool { return false },
			func(string, time.Time) bool { return false },
			nil)

		// Start 8 requests - should all fit
		for i := 0; i < 8; i++ {
			slot.Start()
			assert.True(t, slot.IsActive())
			if i < 7 {
				assert.True(t, slot.HasCapacity())
			} else {
				assert.False(t, slot.HasCapacity()) // Should be at capacity on 8th
			}
			assert.Equal(t, int64(i+1), slot.GetActiveRequests())
		}
	})

	t.Run("thread safety", func(t *testing.T) {
		// Test that concurrent operations on slot are thread-safe
		model := &types.Model{
			ID:      "thread-test-model",
			Runtime: types.RuntimeVLLM,
			Memory:  1000000,
		}

		workload, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
			RequestID: "test-1",
			Request: &openai.ChatCompletionRequest{
				Model: "thread-test-model",
			},
		}, model)
		require.NoError(t, err)

		slot := NewSlot("runner-1", workload,
			func(string, time.Time) bool { return false },
			func(string, time.Time) bool { return false },
			nil)

		const numGoroutines = 50
		const operationsPerGoroutine = 10

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		// Start multiple goroutines that start and release requests
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < operationsPerGoroutine; j++ {
					slot.Start()
					time.Sleep(time.Millisecond) // Small delay to increase contention
					slot.Release()
				}
			}()
		}

		wg.Wait()

		// After all operations, active requests should be 0
		assert.Equal(t, int64(0), slot.GetActiveRequests())
		assert.False(t, slot.IsActive())
		assert.True(t, slot.HasCapacity())
	})

	t.Run("capacity checking", func(t *testing.T) {
		// Test HasCapacity method directly
		model := &types.Model{
			ID:      "capacity-test-model",
			Runtime: types.RuntimeOllama, // Use Ollama with default 2 concurrency
			Memory:  1000000,
		}

		workload, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
			RequestID: "test-1",
			Request: &openai.ChatCompletionRequest{
				Model: "capacity-test-model",
			},
		}, model)
		require.NoError(t, err)

		// Create a slot - Ollama gets natural default of 4
		slot := NewSlot("runner-1", workload,
			func(string, time.Time) bool { return false },
			func(string, time.Time) bool { return false },
			nil)

		// Initially should have capacity
		assert.True(t, slot.HasCapacity())

		// Start multiple requests - should have capacity (default is 2 for Ollama)
		for i := 0; i < 1; i++ {
			slot.Start()
			assert.True(t, slot.HasCapacity())
		}

		// Start 2nd request - should reach capacity
		slot.Start()
		assert.False(t, slot.HasCapacity())

		// Release one request - should have capacity again
		slot.Release()
		assert.True(t, slot.HasCapacity())
	})
}
