package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

// implTasks builds n spec tasks that are in the implementation status, i.e.
// already holding a sandbox slot.
func implTasks(n int) []*types.SpecTask {
	tasks := make([]*types.SpecTask, n)
	for i := range tasks {
		tasks[i] = &types.SpecTask{Status: types.TaskStatusImplementation}
	}
	return tasks
}

func onlineRunner(max int) *types.SandboxInstance {
	return &types.SandboxInstance{Status: "online", LastSeen: time.Now(), MaxSandboxes: max}
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_UnlimitedRunnerNeverGates() {
	ctx := context.Background()
	// A -1 runner is unlimited: we must not even count in-flight tasks.
	s.store.EXPECT().ListSandboxInstances(ctx).Return([]*types.SandboxInstance{
		onlineRunner(10), onlineRunner(-1),
	}, nil)

	blocked, reason, err := s.orchestrator.sandboxCapacityQueueReason(ctx)
	s.NoError(err)
	s.False(blocked)
	s.Empty(reason)
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_SlotFreePromotes() {
	ctx := context.Background()
	s.store.EXPECT().ListSandboxInstances(ctx).Return([]*types.SandboxInstance{onlineRunner(10)}, nil)
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{Status: types.TaskStatusImplementation}).
		Return(implTasks(9), nil)

	blocked, _, err := s.orchestrator.sandboxCapacityQueueReason(ctx)
	s.NoError(err)
	s.False(blocked)
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_FullQueues() {
	ctx := context.Background()
	s.store.EXPECT().ListSandboxInstances(ctx).Return([]*types.SandboxInstance{onlineRunner(10)}, nil)
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{Status: types.TaskStatusImplementation}).
		Return(implTasks(10), nil)

	blocked, reason, err := s.orchestrator.sandboxCapacityQueueReason(ctx)
	s.NoError(err)
	s.True(blocked)
	s.Contains(reason, "10 of 10")
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_SummedAcrossRunners() {
	ctx := context.Background()
	// Two finite runners: capacity 3 + 7 = 10; 10 in use -> full.
	s.store.EXPECT().ListSandboxInstances(ctx).Return([]*types.SandboxInstance{
		onlineRunner(3), onlineRunner(7),
	}, nil)
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{Status: types.TaskStatusImplementation}).
		Return(implTasks(10), nil)

	blocked, _, err := s.orchestrator.sandboxCapacityQueueReason(ctx)
	s.NoError(err)
	s.True(blocked)
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_OfflineAndStaleIgnored() {
	ctx := context.Background()
	// Offline and stale runners contribute no capacity; with no usable online
	// runner we do not gate (fail open) rather than wedge the queue.
	s.store.EXPECT().ListSandboxInstances(ctx).Return([]*types.SandboxInstance{
		{Status: "offline", LastSeen: time.Now(), MaxSandboxes: 10},
		{Status: "online", LastSeen: time.Now().Add(-10 * time.Minute), MaxSandboxes: 10},
	}, nil)

	blocked, _, err := s.orchestrator.sandboxCapacityQueueReason(ctx)
	s.NoError(err)
	s.False(blocked)
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_StoreErrorFailsOpen() {
	ctx := context.Background()
	s.store.EXPECT().ListSandboxInstances(ctx).Return(nil, errors.New("db down"))

	blocked, _, err := s.orchestrator.sandboxCapacityQueueReason(ctx)
	s.Error(err)
	s.False(blocked) // caller treats an error as "do not gate"
}

// Guard against an accidental gomock import prune.
var _ = gomock.Any

func TestSandboxCapacityHelperCompiles(t *testing.T) {}
