package scheduler

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

const (
	testModelStr = model.ModelOllamaLlama38b
)

var (
	dummyTimeout = func(_ string, _ time.Time) bool {
		return false
	}
	testModel, _ = model.GetModel(testModelStr)
)

func TestPlacement_MaxSpread_Simple(t *testing.T) {
	c := NewCluster(dummyTimeout)
	c.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-1",
		TotalMemory: testModel.GetMemoryRequirements(types.SessionModeInference) * 2,
	})
	a := NewWorkloadAllocator(dummyTimeout, dummyTimeout)

	req := createPlacementWork("test", model.NewModel(testModelStr))

	runnerID, err := MaxSpreadStrategy(c, a, req)
	assert.NoError(t, err)
	assert.Equal(t, "test-runner-1", runnerID)

	_, err = a.AllocateNewSlot(runnerID, req)
	assert.NoError(t, err)

	runnerID, err = MaxSpreadStrategy(c, a, req)
	assert.NoError(t, err)
	assert.Equal(t, "test-runner-1", runnerID)
}

func TestPlacement_MaxSpread_MultiMachine(t *testing.T) {
	c := NewCluster(dummyTimeout)
	c.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-1",
		TotalMemory: 2 * testModel.GetMemoryRequirements(types.SessionModeInference),
	})
	a := NewWorkloadAllocator(dummyTimeout, dummyTimeout)
	req := createPlacementWork("test", model.NewModel(testModelStr))

	_, err := a.AllocateNewSlot("test-runner-1", req)
	assert.NoError(t, err)

	// Add a second runner
	c.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-2",
		TotalMemory: 2 * testModel.GetMemoryRequirements(types.SessionModeInference),
	})

	runnerID, err := MaxSpreadStrategy(c, a, req)
	assert.NoError(t, err)
	assert.Equal(t, "test-runner-2", runnerID)
}

func createPlacementWork(name string, model model.Name) *Workload {
	req := &types.RunnerLLMInferenceRequest{
		RequestID: name,
		Request: &openai.ChatCompletionRequest{
			Model: model.String(),
		},
	}
	work, _ := NewLLMWorkload(req)
	return work
}
