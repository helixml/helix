package scheduler

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

func TestScheduler_NoRunnersAvailable(t *testing.T) {
	config, _ := config.LoadServerConfig()
	scheduler := NewScheduler(&config)
	err := createTestWork(scheduler, "test-request-1", types.Model_Ollama_Llama3_8b)
	assert.ErrorContains(t, err, "no runners available")
}

func TestScheduler_TimeoutRunner(t *testing.T) {
	config, _ := config.LoadServerConfig()
	scheduler := NewScheduler(&config)

	// Monkeypatch the scheduler's cluster
	timeoutRunner1Func := func(id string, t time.Time) bool {
		return id == "test-runner-1"
	}
	cluster := NewCluster(timeoutRunner1Func)
	scheduler.cluster = cluster

	model, _ := model.GetModel(types.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-1",
		TotalMemory: model.GetMemoryRequirements(types.SessionModeInference) * 2,
	})

	// Schedule a job
	err := createTestWork(scheduler, "test-request-1", types.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-2",
		TotalMemory: model.GetMemoryRequirements(types.SessionModeInference) * 2,
	})

	// Simulate not updating the runner for a while so that subsequent jobs get rescheduled
	work, err := scheduler.WorkForRunner("test-runner-2", WorkloadTypeLLMInferenceRequest, false)
	assert.NoError(t, err)

	// Assert that the work, originally scheduled for runner-1 is now on runner-2
	assert.Equal(t, work.ID(), "test-request-1")
}

func TestScheduler_ThreeJobsOnSingleRunnerThatCanFitTwo(t *testing.T) {
	config, _ := config.LoadServerConfig()
	scheduler := NewScheduler(&config)
	model, _ := model.GetModel(types.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: model.GetMemoryRequirements(types.SessionModeInference) * 2,
	})

	// Test requests
	err := createTestWork(scheduler, "test-request-1", types.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	err = createTestWork(scheduler, "test-request-2", types.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	err = createTestWork(scheduler, "test-request-3", types.Model_Ollama_Llama3_8b)
	assert.ErrorContains(t, err, "full")
}

func TestScheduler_TestWarmSlot(t *testing.T) {
	config, _ := config.LoadServerConfig()
	scheduler := NewScheduler(&config)
	model, _ := model.GetModel(types.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: model.GetMemoryRequirements(types.SessionModeInference) * 2,
	})

	// Test request
	err := createTestWork(scheduler, "test-request-1", types.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Simulate the runner starting the work
	scheduler.WorkForRunner("test-runner", WorkloadTypeLLMInferenceRequest, false)
	// Simulate the runner finishing the work
	err = scheduler.Release("test-request-1")
	assert.NoError(t, err)

	// Start request-2
	err = createTestWork(scheduler, "test-request-2", types.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Make sure there's only one slot
	assert.Equal(t, len(scheduler.allocator.RunnerSlots("test-runner")), 1)
}

func TestScheduler_TestRemoveStaleSlots(t *testing.T) {
	config, _ := config.LoadServerConfig()
	config.Providers.Helix.ModelTTL = 1 * time.Microsecond
	scheduler := NewScheduler(&config)
	model, _ := model.GetModel(types.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: 2 * model.GetMemoryRequirements(types.SessionModeInference),
	})

	// Test request
	err := createTestWork(scheduler, "test-request-1", types.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Test request 2
	err = createTestWork(scheduler, "test-request-2", types.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Simulate the runner starting the work
	scheduler.WorkForRunner("test-runner", WorkloadTypeLLMInferenceRequest, false)
	scheduler.WorkForRunner("test-runner", WorkloadTypeLLMInferenceRequest, false)
	// Simulate the runner finishing the work
	err = scheduler.Release("test-request-1")
	assert.NoError(t, err)
	err = scheduler.Release("test-request-2")
	assert.NoError(t, err)

	// Start request-3, a new model type
	err = createTestWork(scheduler, "test-request-3", types.Model_Ollama_Phi3)
	assert.NoError(t, err)

	// Simulate the runner starting the work
	scheduler.WorkForRunner("test-runner", WorkloadTypeLLMInferenceRequest, false)

	// Simulate runner updating control plane with removed models
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: model.GetMemoryRequirements(types.SessionModeInference),
		ModelInstances: []*types.ModelInstanceState{
			{
				ModelName: types.Model_Ollama_Llama3_8b,
				Mode:      types.SessionModeInference,
			}, {
				ModelName: types.Model_Ollama_Phi3,
				Mode:      types.SessionModeInference,
			},
		},
	})

	assert.Equal(t, len(scheduler.allocator.RunnerSlots("test-runner")), 2)
}

func TestScheduler_FullWhenJobsWarm(t *testing.T) {
	config, _ := config.LoadServerConfig()
	scheduler := NewScheduler(&config)
	model, _ := model.GetModel(types.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: model.GetMemoryRequirements(types.SessionModeInference),
	})

	// Test request
	err := createTestWork(scheduler, "test-request-1", types.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Simulate runner doing work
	scheduler.WorkForRunner("test-runner", WorkloadTypeLLMInferenceRequest, false)
	err = scheduler.Release("test-request-1")
	assert.NoError(t, err)

	// Even though the work has finished, the slot is still warm, so it should report full when a
	// new model is requested
	err = createTestWork(scheduler, "test-request-2", types.Model_Ollama_Phi3)
	assert.ErrorContains(t, err, "full")
}

func TestScheduler_MaximiseUtilization(t *testing.T) {
	config, _ := config.LoadServerConfig()
	scheduler := NewScheduler(&config)
	model, _ := model.GetModel(types.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-1",
		TotalMemory: 2 * model.GetMemoryRequirements(types.SessionModeInference),
	})

	// Add one request
	err := createTestWork(scheduler, "test-request-1", types.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Add a second runner
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-2",
		TotalMemory: 2 * model.GetMemoryRequirements(types.SessionModeInference),
	})
	assert.NoError(t, err)

	// When scheduling a second request, it should fill the first runner, not the second
	err = createTestWork(scheduler, "test-request-2", types.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Check that NO work has been scheduler's cluster
	work, err := scheduler.WorkForRunner("test-runner-2", WorkloadTypeLLMInferenceRequest, false)
	assert.NoError(t, err)
	if work != nil {
		t.Error("second runner should have no work because we're maximizing utilization (represented by nil)")
	}
}

// Session scheduling is largely the same
func TestScheduler_TestSessionScheduler(t *testing.T) {
	config, _ := config.LoadServerConfig()
	config.Providers.Helix.ModelTTL = 1 * time.Microsecond
	scheduler := NewScheduler(&config)
	model, _ := model.GetModel(types.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: model.GetMemoryRequirements(types.SessionModeInference) * 2,
	})

	// Test request
	err := createTestSession(scheduler, "test-request-1", types.Model_Ollama_Llama3_8b, "")
	assert.NoError(t, err)
	err = createTestSession(scheduler, "test-request-2", types.Model_Ollama_Llama3_8b, "")
	assert.NoError(t, err)
	err = createTestSession(scheduler, "test-request-3", types.Model_Ollama_Phi3, "")
	assert.ErrorContains(t, err, "full")

	// Simulate runner taking and finishing work
	scheduler.WorkForRunner("test-runner", WorkloadTypeSession, false)
	err = scheduler.Release("test-request-1")
	assert.NoError(t, err)

	scheduler.WorkForRunner("test-runner", WorkloadTypeSession, false)
	err = scheduler.Release("test-request-2")
	assert.NoError(t, err)

	// Now work should fit, since the test is always stale
	err = createTestWork(scheduler, "test-request-4", types.Model_Ollama_Phi3)
	assert.NoError(t, err)
}

func TestScheduler_LoraDirSession(t *testing.T) {
	config, _ := config.LoadServerConfig()
	scheduler := NewScheduler(&config)
	model, _ := model.GetModel(types.Model_Axolotl_Mistral7b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-1",
		TotalMemory: model.GetMemoryRequirements(types.SessionModeInference),
	})

	// Test request
	err := createTestSession(scheduler, "test-request-1", types.Model_Axolotl_Mistral7b, "test")
	assert.NoError(t, err)

	// Simulate runner taking and finishing work
	scheduler.WorkForRunner("test-runner-1", WorkloadTypeSession, false)
	err = scheduler.Release("test-request-1")
	assert.NoError(t, err)

	// Add a second runner
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-2",
		TotalMemory: model.GetMemoryRequirements(types.SessionModeInference),
	})

	// Reschedule lora work, must always scheduler's cluster
	err = createTestSession(scheduler, "test-request-2", types.Model_Axolotl_Mistral7b, "test")
	assert.NoError(t, err)

	// Check that NO work has been scheduler's cluster
	work, err := scheduler.WorkForRunner("test-runner-2", WorkloadTypeSession, false)
	assert.NoError(t, err)
	if work != nil {
		t.Error("second runner should have no work because of the warm lora dir")
	}

	// Schedule a second lora dir, must scheduler's cluster
	err = createTestSession(scheduler, "test-request-3", types.Model_Axolotl_Mistral7b, "new")
	assert.NoError(t, err)
	work, err = scheduler.WorkForRunner("test-runner-2", WorkloadTypeSession, false)
	assert.NoError(t, err)
	if work == nil {
		t.Error("second runner should have work because of the new lora dir")
	}
}

func createTestWork(scheduler Scheduler, name string, model types.ModelName) error {
	req := &types.RunnerLLMInferenceRequest{
		RequestID: name,
		Request: &openai.ChatCompletionRequest{
			Model: model.String(),
		},
	}
	work, err := NewLLMWorkload(req)
	if err != nil {
		return err
	}
	return scheduler.Schedule(work)
}

func createTestSession(scheduler Scheduler, name string, model types.ModelName, loraDir string) error {
	req := &types.Session{
		ID:        name,
		ModelName: model,
		Mode:      types.SessionModeInference,
		LoraDir:   loraDir,
	}
	work, err := NewSessonWorkload(req)
	if err != nil {
		return err
	}
	return scheduler.Schedule(work)
}
