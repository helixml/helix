package scheduler

import (
	"context"
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
	scheduler := NewScheduler(context.Background(), &config, nil)
	err := scheduleTestLLMWorkload(scheduler, "test-request-1", model.Model_Ollama_Llama3_8b)
	assert.ErrorContains(t, err, "no runners available")
}

func TestScheduler_TimeoutRunner(t *testing.T) {
	config, _ := config.LoadServerConfig()
	scheduler := NewScheduler(context.Background(), &config, nil)

	// Monkeypatch the scheduler's cluster
	timeoutRunner1Func := func(id string, t time.Time) bool {
		return id == "test-runner-1"
	}
	cluster := NewCluster(timeoutRunner1Func)
	scheduler.cluster = cluster

	m, _ := model.GetModel(string(model.Model_Ollama_Llama3_8b))
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-1",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference) * 2,
	})

	// Schedule a job
	err := scheduleTestLLMWorkload(scheduler, "test-request-1", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-2",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference) * 2,
	})

	// Simulate not updating the runner for a while so that subsequent jobs get rescheduled
	work, err := scheduler.WorkForRunner("test-runner-2", WorkloadTypeLLMInferenceRequest, false, model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Assert that the work, originally scheduled for runner-1 is now on runner-2
	assert.Equal(t, work.ID(), "test-request-1")
}

func TestScheduler_ThreeJobsOnSingleRunnerThatCanFitTwo(t *testing.T) {
	config, _ := config.LoadServerConfig()
	scheduler := NewScheduler(context.Background(), &config, nil)
	m, _ := model.GetModel(string(model.Model_Ollama_Llama3_8b))
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference) * 2,
	})

	// Test requests
	err := scheduleTestLLMWorkload(scheduler, "test-request-1", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	err = scheduleTestLLMWorkload(scheduler, "test-request-2", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	err = scheduleTestLLMWorkload(scheduler, "test-request-3", model.Model_Ollama_Llama3_8b)
	assert.ErrorContains(t, err, "full")
}

func TestScheduler_TestWarmSlot(t *testing.T) {
	config, _ := config.LoadServerConfig()
	scheduler := NewScheduler(context.Background(), &config, nil)
	m, _ := model.GetModel(model.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference) * 2,
	})

	// Test request
	err := scheduleTestLLMWorkload(scheduler, "test-request-1", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Simulate the runner starting the work
	scheduler.WorkForRunner("test-runner", WorkloadTypeLLMInferenceRequest, false, model.Model_Ollama_Llama3_8b)
	// Simulate the runner finishing the work
	err = scheduler.Release("test-request-1")
	assert.NoError(t, err)

	// Start request-2
	err = scheduleTestLLMWorkload(scheduler, "test-request-2", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Make sure there's only one slot
	assert.Equal(t, len(scheduler.allocator.RunnerSlots("test-runner")), 1)
}

func TestScheduler_TestRemoveStaleSlots(t *testing.T) {
	config, _ := config.LoadServerConfig()
	config.Providers.Helix.ModelTTL = 1 * time.Microsecond
	scheduler := NewScheduler(context.Background(), &config, nil)
	m, _ := model.GetModel(model.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: 2 * m.GetMemoryRequirements(types.SessionModeInference),
	})

	// Test request
	err := scheduleTestLLMWorkload(scheduler, "test-request-1", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Test request 2
	err = scheduleTestLLMWorkload(scheduler, "test-request-2", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Simulate the runner starting the work
	scheduler.WorkForRunner("test-runner", WorkloadTypeLLMInferenceRequest, false, model.Model_Ollama_Llama3_8b)
	scheduler.WorkForRunner("test-runner", WorkloadTypeLLMInferenceRequest, false, model.Model_Ollama_Llama3_8b)
	// Simulate the runner finishing the work
	err = scheduler.Release("test-request-1")
	assert.NoError(t, err)
	err = scheduler.Release("test-request-2")
	assert.NoError(t, err)

	// Start request-3, a new model type
	err = scheduleTestLLMWorkload(scheduler, "test-request-3", model.Model_Ollama_Phi3)
	assert.NoError(t, err)

	// Simulate the runner starting the work
	scheduler.WorkForRunner("test-runner", WorkloadTypeLLMInferenceRequest, false, model.Model_Ollama_Phi3)

	// Simulate runner updating control plane with removed models
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference),
		ModelInstances: []*types.ModelInstanceState{
			{
				ModelName: model.Model_Ollama_Llama3_8b,
				Mode:      types.SessionModeInference,
			}, {
				ModelName: model.Model_Ollama_Phi3,
				Mode:      types.SessionModeInference,
			},
		},
	})

	assert.Equal(t, len(scheduler.allocator.RunnerSlots("test-runner")), 2)
}

func TestScheduler_FullWhenJobsWarm(t *testing.T) {
	config, _ := config.LoadServerConfig()
	scheduler := NewScheduler(context.Background(), &config, nil)
	m, _ := model.GetModel(model.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference),
	})

	// Test request
	err := scheduleTestLLMWorkload(scheduler, "test-request-1", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Simulate runner doing work
	scheduler.WorkForRunner("test-runner", WorkloadTypeLLMInferenceRequest, false, model.Model_Ollama_Llama3_8b)
	err = scheduler.Release("test-request-1")
	assert.NoError(t, err)

	// Even though the work has finished, the slot is still warm, so it should report full when a
	// new model is requested
	err = scheduleTestLLMWorkload(scheduler, "test-request-2", model.Model_Ollama_Phi3)
	assert.ErrorContains(t, err, "full")
}

func TestScheduler_MaximiseUtilization(t *testing.T) {
	config, _ := config.LoadServerConfig()
	config.Providers.Helix.SchedulingStrategy = string(SchedulingStrategy_MaxUtilization)
	scheduler := NewScheduler(context.Background(), &config, nil)
	m, _ := model.GetModel(model.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-1",
		TotalMemory: 2 * m.GetMemoryRequirements(types.SessionModeInference),
	})

	// Add one request
	err := scheduleTestLLMWorkload(scheduler, "test-request-1", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Add a second runner
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-2",
		TotalMemory: 2 * m.GetMemoryRequirements(types.SessionModeInference),
	})
	assert.NoError(t, err)

	// When scheduling a second request, it should fill the first runner, not the second
	err = scheduleTestLLMWorkload(scheduler, "test-request-2", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)

	// Check that NO work has been scheduler's cluster
	work, err := scheduler.WorkForRunner("test-runner-2", WorkloadTypeLLMInferenceRequest, false, model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)
	if work != nil {
		t.Error("second runner should have no work because we're maximizing utilization (represented by nil)")
	}
}

// Session scheduling is largely the same
func TestScheduler_TestSessionScheduler(t *testing.T) {
	config, _ := config.LoadServerConfig()
	config.Providers.Helix.ModelTTL = 1 * time.Microsecond
	scheduler := NewScheduler(context.Background(), &config, nil)
	m, _ := model.GetModel(model.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference) * 2,
	})

	// Test request
	err := createTestSession(scheduler, "test-request-1", model.Model_Ollama_Llama3_8b, "")
	assert.NoError(t, err)
	err = createTestSession(scheduler, "test-request-2", model.Model_Ollama_Llama3_8b, "")
	assert.NoError(t, err)
	err = createTestSession(scheduler, "test-request-3", model.Model_Ollama_Phi3, "")
	assert.ErrorContains(t, err, "full")

	// Simulate runner taking and finishing work
	scheduler.WorkForRunner("test-runner", WorkloadTypeSession, false, model.Model_Ollama_Llama3_8b)
	err = scheduler.Release("test-request-1")
	assert.NoError(t, err)

	scheduler.WorkForRunner("test-runner", WorkloadTypeSession, false, model.Model_Ollama_Llama3_8b)
	err = scheduler.Release("test-request-2")
	assert.NoError(t, err)

	// Now work should fit, since the test is always stale
	err = scheduleTestLLMWorkload(scheduler, "test-request-4", model.Model_Ollama_Phi3)
	assert.NoError(t, err)
}

func TestScheduler_LoraDirSession(t *testing.T) {
	config, _ := config.LoadServerConfig()
	scheduler := NewScheduler(context.Background(), &config, nil)
	m, _ := model.GetModel(model.Model_Axolotl_Mistral7b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-1",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference),
	})

	// Test request
	err := createTestSession(scheduler, "test-request-1", model.Model_Axolotl_Mistral7b, "test")
	assert.NoError(t, err)

	// Simulate runner taking and finishing work
	scheduler.WorkForRunner("test-runner-1", WorkloadTypeSession, false, model.Model_Axolotl_Mistral7b)
	err = scheduler.Release("test-request-1")
	assert.NoError(t, err)

	// Add a second runner
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner-2",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference),
	})

	// Reschedule lora work, must always scheduler's cluster
	err = createTestSession(scheduler, "test-request-2", model.Model_Axolotl_Mistral7b, "test")
	assert.NoError(t, err)

	// Check that NO work has been scheduler's cluster
	work, err := scheduler.WorkForRunner("test-runner-2", WorkloadTypeSession, false, model.Model_Axolotl_Mistral7b)
	assert.NoError(t, err)
	if work != nil {
		t.Error("second runner should have no work because of the warm lora dir")
	}

	// Schedule a second lora dir, must scheduler's cluster
	err = createTestSession(scheduler, "test-request-3", model.Model_Axolotl_Mistral7b, "new")
	assert.NoError(t, err)
	work, err = scheduler.WorkForRunner("test-runner-2", WorkloadTypeSession, false, model.Model_Axolotl_Mistral7b)
	assert.NoError(t, err)
	if work == nil {
		t.Error("second runner should have work because of the new lora dir")
	}
}

func TestScheduler_RunnerWithWrongModel(t *testing.T) {
	config, _ := config.LoadServerConfig()
	config.Providers.Helix.ModelTTL = 1 * time.Microsecond
	scheduler := NewScheduler(context.Background(), &config, nil)
	m, _ := model.GetModel(model.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference) * 2,
	})

	// Test request
	err := createTestSession(scheduler, "test-request-1", model.Model_Ollama_Llama3_8b, "")
	assert.NoError(t, err)
	err = createTestSession(scheduler, "test-request-2", "gemma2:2b-instruct-q8_0", "")
	assert.NoError(t, err)

	// Simulate runner taking and finishing work
	w, err := scheduler.WorkForRunner("test-runner", WorkloadTypeSession, false, model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)
	assert.Equal(t, w.ID(), "test-request-1")
	err = scheduler.Release("test-request-1")
	assert.NoError(t, err)

	w, err = scheduler.WorkForRunner("test-runner", WorkloadTypeSession, false, "gemma2:2b-instruct-q8_0")
	assert.NoError(t, err)
	assert.Equal(t, w.ID(), "test-request-2")
	err = scheduler.Release("test-request-2")
	assert.NoError(t, err)

	// Test any work will do
	err = createTestSession(scheduler, "test-request-1", model.Model_Ollama_Llama3_8b, "")
	assert.NoError(t, err)
	w, err = scheduler.WorkForRunner("test-runner", WorkloadTypeSession, false, "")
	assert.NoError(t, err)
	assert.NotNil(t, w)

	// Test any new work will do part 2 -- new work only, ignore filter
	err = createTestSession(scheduler, "test-request-2", "adrienbrault/nous-hermes2pro:Q5_K_S", "")
	assert.NoError(t, err)
	w, err = scheduler.WorkForRunner("test-runner", WorkloadTypeSession, true, "gemma2:2b-instruct-q8_0")
	assert.NoError(t, err)
	assert.NotNil(t, w)
}

func TestScheduler_SlotTimeoutTest(t *testing.T) {
	config, _ := config.LoadServerConfig()
	config.Providers.Helix.SlotTTL = 1 * time.Microsecond
	config.Providers.Helix.ModelTTL = 1 * time.Microsecond
	scheduler := NewScheduler(context.Background(), &config, nil)
	m, _ := model.GetModel(model.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference) * 1,
	})

	// Test request
	err := createTestSession(scheduler, "test-request-1", model.Model_Ollama_Llama3_8b, "")
	assert.NoError(t, err)

	// Wait for the model to timeout
	time.Sleep(2 * time.Millisecond)

	// Since the model has timed out, the slot should be stale
	err = createTestSession(scheduler, "test-request-2", model.Model_Ollama_Llama3_8b, "")
	assert.NoError(t, err)
}

func TestScheduler_EnqueueLLMRequest(t *testing.T) {
	// Create the server and helper function to test if the queue is empty
	config, _ := config.LoadServerConfig()
	config.Providers.Helix.QueueSize = 1
	scheduler := NewScheduler(context.Background(), &config, nil)
	emptyQueueFunc := func() bool {
		return len(scheduler.queue) == 0
	}

	// Add a runner, otherwise we will get an error saying no runners available
	m, _ := model.GetModel(model.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference) * 1,
	})

	// Start some work on the runner, so that subsequent requests must queue
	err := enqueueTestLLMWorkload(scheduler, "request-1", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)
	WaitFor(t, emptyQueueFunc, time.Second) // This waits for the queue to be processed
	err = scheduler.Begin("request-1")      // This marks the slot as started
	assert.NoError(t, err)

	// Now runners are busy, add work to queue
	err = enqueueTestLLMWorkload(scheduler, "request-2", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)
	assert.Len(t, scheduler.queue, 1)

	// Can't requeue work already in queue
	err = enqueueTestLLMWorkload(scheduler, "request-2", model.Model_Ollama_Llama3_8b)
	assert.Error(t, err)
	assert.Len(t, scheduler.queue, 1)

	// Finish original work, queue should now run (in the goroutine, might need to wait a minute)
	err = scheduler.Release("request-1")
	assert.NoError(t, err)
	WaitFor(t, emptyQueueFunc, time.Second)
	assert.Len(t, scheduler.queue, 0)

	// Now add too many things to the queue
	err = enqueueTestLLMWorkload(scheduler, "request-3", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)
	err = enqueueTestLLMWorkload(scheduler, "request-4", model.Model_Ollama_Llama3_8b)
	assert.Error(t, err)
}

func TestScheduler_EnqueueSessionRequest(t *testing.T) {
	// Create the server and helper function to test if the queue is empty
	config, _ := config.LoadServerConfig()
	config.Providers.Helix.QueueSize = 2
	scheduler := NewScheduler(context.Background(), &config, nil)
	emptyQueueFunc := func() bool {
		return len(scheduler.queue) == 0
	}

	// Add a runner, otherwise we will get an error saying no runners available
	m, _ := model.GetModel(model.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference) * 1,
	})

	// Start some work on the runner, so that subsequent requests must queue
	err := enqueueTestLLMWorkload(scheduler, "request-1", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)
	WaitFor(t, emptyQueueFunc, time.Second) // This waits for the queue to be processed
	err = scheduler.Begin("request-1")      // This marks the slot as started
	assert.NoError(t, err)

	// Test Priority item entering the queue after a non-priority item
	err = enqueueTestSession(scheduler, "request-2", model.Model_Ollama_Llama3_8b, "", false)
	assert.NoError(t, err)
	assert.Len(t, scheduler.queue, 1)

	err = enqueueTestSession(scheduler, "request-3", model.Model_Ollama_Llama3_8b, "", true)
	assert.NoError(t, err)
	assert.Len(t, scheduler.queue, 2)

	// request-3 should be earlier in the queue than request-2
	assert.Equal(t, scheduler.queue[0].ID(), "request-3")
}

func TestScheduler_RunnerLifecycle(t *testing.T) {
	config, _ := config.LoadServerConfig()
	scheduler := NewScheduler(context.Background(), &config, nil)
	emptyQueueFunc := func() bool {
		return len(scheduler.queue) == 0
	}

	// Runner shows up
	m, _ := model.GetModel(model.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "test-runner",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference) * 1,
	})

	// Runner asks for slots, no work yet
	slots := scheduler.SlotsForRunner("test-runner")
	assert.Len(t, slots, 0)

	// Enqueue and schedule some work
	err := enqueueTestLLMWorkload(scheduler, "request-1", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)
	WaitFor(t, emptyQueueFunc, time.Second) // This waits for the queue to be processed

	// Runner asks for slots, now there is work
	slots = scheduler.SlotsForRunner("test-runner")
	assert.Len(t, slots, 1)
}

func TestScheduler_ProcessQueue(t *testing.T) {
	hasErr := false
	errorFunc := func(*Workload, error) {
		hasErr = true
	}

	// Manually start a scheduler so that the goroutine doesn't start
	config, _ := config.LoadServerConfig()
	scheduler := newSchedulerWithoutQueue(&config, errorFunc)

	// Without a runner, adding to the queue and processing should result in an error on the work
	err := enqueueTestLLMWorkload(scheduler, "request-1", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)
	assert.Len(t, scheduler.queue, 1)

	// Process the queue and the job should error and be removed from the queue
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	scheduler.processQueue(ctx)
	assert.Len(t, scheduler.queue, 0)
	assert.True(t, hasErr)

	// Add a two runner, one big one small
	m, _ := model.GetModel(model.Model_Ollama_Llama3_8b)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "runner-1",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference) * 1,
	})
	m, _ = model.GetModel(model.Model_Ollama_Phi3)
	scheduler.UpdateRunner(&types.RunnerState{
		ID:          "runner-2",
		TotalMemory: m.GetMemoryRequirements(types.SessionModeInference) * 1,
	})

	// Now enqueue work. Fill up the big runner.
	err = enqueueTestLLMWorkload(scheduler, "request-1", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)
	err = enqueueTestLLMWorkload(scheduler, "request-2", model.Model_Ollama_Llama3_8b)
	assert.NoError(t, err)
	err = enqueueTestLLMWorkload(scheduler, "request-3", model.Model_Ollama_Phi3)
	assert.NoError(t, err)

	ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	scheduler.processQueue(ctx)

	// That final phi request should have been scheduled to the small runner, so there should be one
	// job left. This failed in a previous version.
	assert.Len(t, scheduler.queue, 1)
}

func enqueueTestLLMWorkload(scheduler Scheduler, name string, model string) error {
	req := &types.RunnerLLMInferenceRequest{
		RequestID: name,
		Request: &openai.ChatCompletionRequest{
			Model: model,
		},
	}
	work, err := NewLLMWorkload(req)
	if err != nil {
		return err
	}
	return scheduler.Enqueue(work)
}

func enqueueTestSession(scheduler Scheduler, name string, model string, loraDir string, priority bool) error {
	req := &types.Session{
		ID:        name,
		ModelName: model,
		Mode:      types.SessionModeInference,
		LoraDir:   loraDir,
		Metadata: types.SessionMetadata{
			Priority: priority,
		},
	}
	work, err := NewSessionWorkload(req)
	if err != nil {
		return err
	}
	return scheduler.Enqueue(work)
}

func scheduleTestLLMWorkload(scheduler Scheduler, name string, model string) error {
	req := &types.RunnerLLMInferenceRequest{
		RequestID: name,
		Request: &openai.ChatCompletionRequest{
			Model: model,
		},
	}
	work, err := NewLLMWorkload(req)
	if err != nil {
		return err
	}
	return scheduler.Schedule(work)
}

func createTestSession(scheduler Scheduler, name string, model string, loraDir string) error {
	req := &types.Session{
		ID:        name,
		ModelName: model,
		Mode:      types.SessionModeInference,
		LoraDir:   loraDir,
	}
	work, err := NewSessionWorkload(req)
	if err != nil {
		return err
	}
	return scheduler.Schedule(work)
}

func WaitFor(t *testing.T, successFunc func() bool, d time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if successFunc() {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}
}
