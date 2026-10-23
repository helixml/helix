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

// onlineRunner is an online runner whose cap the runner itself confirmed
// (MaxSandboxesFromRunner), i.e. an authoritative cap the gate must trust.
func onlineRunner(max int) *types.SandboxInstance {
	return &types.SandboxInstance{Status: "online", LastSeen: time.Now(), MaxSandboxes: max, MaxSandboxesFromRunner: true}
}

// unconfirmedRunner is an online runner whose cap was NOT reported by the
// runner — it is the API's HELIX_SANDBOX_MAX_DEV_CONTAINERS fallback, which may
// not match what the runner actually enforces. The gate must not trust it.
func unconfirmedRunner(max int) *types.SandboxInstance {
	return &types.SandboxInstance{Status: "online", LastSeen: time.Now(), MaxSandboxes: max, MaxSandboxesFromRunner: false}
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

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_UnconfirmedCapDoesNotInflate() {
	ctx := context.Background()
	// A confirmed runner caps the fleet at 2. An unconfirmed runner advertises
	// an inflated fallback of 20 (e.g. HELIX_SANDBOX_MAX_DEV_CONTAINERS) while
	// really enforcing far less. The gate must ignore the unconfirmed cap, so
	// capacity is 2 (not 22) and 2 in-flight tasks fill it.
	s.store.EXPECT().ListSandboxInstances(ctx).Return([]*types.SandboxInstance{
		onlineRunner(2), unconfirmedRunner(20),
	}, nil)
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{Status: types.TaskStatusImplementation}).
		Return(implTasks(2), nil)

	blocked, reason, err := s.orchestrator.sandboxCapacityQueueReason(ctx)
	s.NoError(err)
	s.True(blocked)
	s.Contains(reason, "2 of 2")
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_UnconfirmedUnlimitedDoesNotDisableGate() {
	ctx := context.Background()
	// The auto-register default is MaxSandboxes=-1 with MaxSandboxesFromRunner
	// =false. That unconfirmed -1 must NOT be read as "unlimited" and disable
	// the gate; the confirmed runner's cap of 2 still governs.
	s.store.EXPECT().ListSandboxInstances(ctx).Return([]*types.SandboxInstance{
		onlineRunner(2), unconfirmedRunner(-1),
	}, nil)
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{Status: types.TaskStatusImplementation}).
		Return(implTasks(2), nil)

	blocked, reason, err := s.orchestrator.sandboxCapacityQueueReason(ctx)
	s.NoError(err)
	s.True(blocked)
	s.Contains(reason, "2 of 2")
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_OnlyUnconfirmedRunnersFailOpen() {
	ctx := context.Background()
	// No runner has confirmed a cap (the live 20/null box). We cannot verify
	// real capacity, so the gate fails open rather than trusting the inflated
	// fallback or wedging the queue. (Correctness for this box needs the runner
	// to report its real MAX_SANDBOXES.) No ListSpecTasks call is expected
	// because the gate returns before counting in-flight tasks.
	s.store.EXPECT().ListSandboxInstances(ctx).Return([]*types.SandboxInstance{
		unconfirmedRunner(20),
	}, nil)

	blocked, reason, err := s.orchestrator.sandboxCapacityQueueReason(ctx)
	s.NoError(err)
	s.False(blocked)
	s.Empty(reason)
}

// Guard against an accidental gomock import prune.
var _ = gomock.Any

func TestSandboxCapacityHelperCompiles(t *testing.T) {}
