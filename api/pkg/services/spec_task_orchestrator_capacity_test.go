package services

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"
)

// desktopTask is a queued spec task on the default (ubuntu-desktop) runtime,
// so placement needs a render-capable host advertising the ubuntu image.
func desktopTask() *types.SpecTask {
	return &types.SpecTask{ID: "task-cap", ProjectID: "project-cap", Status: types.TaskStatusQueuedImplementation}
}

func headlessTask() *types.SpecTask {
	t := desktopTask()
	t.SandboxRuntime = types.SandboxRuntimeHeadlessUbuntu
	return t
}

func versions(keys ...string) datatypes.JSON {
	m := map[string]string{}
	for _, k := range keys {
		m[k] = "sha-" + k
	}
	b, _ := json.Marshal(m)
	return datatypes.JSON(b)
}

// eligibleRunner is an online, render-capable runner advertising the ubuntu
// image whose cap the runner itself confirmed (MaxSandboxesFromRunner).
func eligibleRunner(max, active int) *types.SandboxInstance {
	return &types.SandboxInstance{
		Status: "online", LastSeen: time.Now(),
		MaxSandboxes: max, ActiveSandboxes: active, MaxSandboxesFromRunner: true,
		DesktopVersions: versions("ubuntu"),
	}
}

// unconfirmedRunner is eligible but its cap is the API's
// HELIX_SANDBOX_MAX_DEV_CONTAINERS fallback, not the runner's real limit.
func unconfirmedRunner(max int) *types.SandboxInstance {
	r := eligibleRunner(max, 0)
	r.MaxSandboxesFromRunner = false
	return r
}

func (s *SpecTaskOrchestratorTestSuite) budget(task *types.SpecTask, runners ...*types.SandboxInstance) (int, bool) {
	ctx := context.Background()
	s.store.EXPECT().ListSandboxInstances(ctx).Return(runners, nil)
	budget, gated, err := s.orchestrator.sandboxCapacityBudget(ctx, task)
	s.Require().NoError(err)
	return budget, gated
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_ConfirmedUnlimitedDisablesGate() {
	_, gated := s.budget(desktopTask(), eligibleRunner(10, 10), eligibleRunner(-1, 50))
	s.False(gated)
}

// Review finding 1: in-use comes from the runner's ActiveSandboxes, which
// includes planning desktops and any other live session, not only tasks in the
// implementation status.
func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_InUseCountsEveryLiveSandbox() {
	budget, gated := s.budget(desktopTask(), eligibleRunner(10, 7))
	s.True(gated)
	s.Equal(3, budget)
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_FullRunnerHasNoBudget() {
	budget, gated := s.budget(desktopTask(), eligibleRunner(10, 10))
	s.True(gated)
	s.Equal(0, budget)
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_SummedAcrossEligibleRunners() {
	budget, gated := s.budget(desktopTask(), eligibleRunner(10, 8), eligibleRunner(5, 1))
	s.True(gated)
	s.Equal(6, budget)
}

// Review finding 2: only runners placement could actually select count.
func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_IneligibleRunnersDoNotInflate() {
	noImage := eligibleRunner(50, 0)
	noImage.DesktopVersions = versions("sway")
	noRender := eligibleRunner(50, 0)
	noRender.GPUVendor = "none"

	budget, gated := s.budget(desktopTask(), eligibleRunner(4, 3), noImage, noRender)
	s.True(gated)
	s.Equal(1, budget, "runners without the image or a render node must not add desktop capacity")
}

// A headless task has no display requirement, so a CPU-only host advertising
// the ubuntu image is eligible for it.
func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_HeadlessCountsCPUOnlyHosts() {
	cpuOnly := eligibleRunner(5, 1)
	cpuOnly.GPUVendor = "none"
	budget, gated := s.budget(headlessTask(), cpuOnly)
	s.True(gated)
	s.Equal(4, budget)
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_OfflineAndStaleIgnored() {
	offline := eligibleRunner(50, 0)
	offline.Status = "offline"
	stale := eligibleRunner(50, 0)
	stale.LastSeen = time.Now().Add(-time.Hour)
	budget, gated := s.budget(desktopTask(), eligibleRunner(2, 1), offline, stale)
	s.True(gated)
	s.Equal(1, budget)
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_UnconfirmedCapDoesNotInflate() {
	budget, gated := s.budget(desktopTask(), eligibleRunner(2, 2), unconfirmedRunner(100))
	s.True(gated)
	s.Equal(0, budget)
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_UnconfirmedUnlimitedDoesNotDisableGate() {
	budget, gated := s.budget(desktopTask(), eligibleRunner(2, 2), unconfirmedRunner(-1))
	s.True(gated)
	s.Equal(0, budget)
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_OnlyUnconfirmedRunnersFailOpen() {
	_, gated := s.budget(desktopTask(), unconfirmedRunner(10), unconfirmedRunner(-1))
	s.False(gated)
}

func (s *SpecTaskOrchestratorTestSuite) TestSandboxCapacity_StoreErrorIsReturned() {
	ctx := context.Background()
	s.store.EXPECT().ListSandboxInstances(ctx).Return(nil, errors.New("db down"))
	_, gated, err := s.orchestrator.sandboxCapacityBudget(ctx, desktopTask())
	s.Error(err)
	s.False(gated)
}

// expectPromotionPrelude sets up the store calls handleQueuedImplementation
// makes before the sandbox-capacity gate.
func (s *SpecTaskOrchestratorTestSuite) expectPromotionPrelude(ctx context.Context, task *types.SpecTask) {
	s.store.EXPECT().GetSpecTask(ctx, task.ID).DoAndReturn(func(context.Context, string) (*types.SpecTask, error) {
		c := *task
		return &c, nil
	})
	s.store.EXPECT().GetProject(ctx, task.ProjectID).Return(&types.Project{ID: task.ProjectID}, nil)
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{ProjectID: task.ProjectID, WithDependsOn: true}).
		DoAndReturn(func(context.Context, *types.SpecTaskFilters) ([]*types.SpecTask, error) {
			c := *task
			return []*types.SpecTask{&c}, nil
		})
	s.store.EXPECT().GetSystemSettings(ctx).Return(&types.SystemSettings{}, nil)
}

// Review finding 3: when gated, the promotion goes through the store's atomic,
// cross-replica admission check with the measured free-slot budget.
func (s *SpecTaskOrchestratorTestSuite) TestHandleQueuedImplementation_GatedClaimIsAtomic() {
	ctx := context.Background()
	task := desktopTask()
	workflow := &countingSpecTaskWorkflowService{}
	orchestrator := &SpecTaskOrchestrator{store: s.store, specTaskService: workflow}

	s.expectPromotionPrelude(ctx, task)
	s.store.EXPECT().ListSandboxInstances(ctx).Return([]*types.SandboxInstance{eligibleRunner(10, 8)}, nil)
	s.store.EXPECT().TransitionSpecTaskStatusWithinCapacity(
		ctx, task.ID,
		[]types.SpecTaskStatus{types.TaskStatusQueuedImplementation},
		types.TaskStatusImplementation,
		gomock.Any(), 2,
	).Return(true, false, nil)

	s.Require().NoError(orchestrator.handleQueuedImplementation(ctx, task))
	orchestrator.wg.Wait()
	s.Equal(int32(1), workflow.justDoItStarts.Load())
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleQueuedImplementation_AtomicClaimRefusedStaysQueued() {
	ctx := context.Background()
	task := desktopTask()
	workflow := &countingSpecTaskWorkflowService{}
	orchestrator := &SpecTaskOrchestrator{store: s.store, specTaskService: workflow}

	s.expectPromotionPrelude(ctx, task)
	s.store.EXPECT().ListSandboxInstances(ctx).Return([]*types.SandboxInstance{eligibleRunner(10, 9)}, nil)
	// Another replica took the last slot first: the store refuses.
	s.store.EXPECT().TransitionSpecTaskStatusWithinCapacity(
		ctx, task.ID, gomock.Any(), types.TaskStatusImplementation, gomock.Any(), 1,
	).Return(false, true, nil)

	s.Require().NoError(orchestrator.handleQueuedImplementation(ctx, task))
	orchestrator.wg.Wait()
	s.Equal(int32(0), workflow.justDoItStarts.Load())
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleQueuedImplementation_NoBudgetSkipsClaim() {
	ctx := context.Background()
	task := desktopTask()
	workflow := &countingSpecTaskWorkflowService{}
	orchestrator := &SpecTaskOrchestrator{store: s.store, specTaskService: workflow}

	s.expectPromotionPrelude(ctx, task)
	s.store.EXPECT().ListSandboxInstances(ctx).Return([]*types.SandboxInstance{eligibleRunner(10, 10)}, nil)
	// No TransitionSpecTaskStatus* expectation: gomock fails the test if one runs.

	s.Require().NoError(orchestrator.handleQueuedImplementation(ctx, task))
	s.Equal(int32(0), workflow.justDoItStarts.Load())
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleQueuedImplementation_CapacityErrorFailsOpen() {
	ctx := context.Background()
	task := desktopTask()
	workflow := &countingSpecTaskWorkflowService{}
	orchestrator := &SpecTaskOrchestrator{store: s.store, specTaskService: workflow}

	s.expectPromotionPrelude(ctx, task)
	s.store.EXPECT().ListSandboxInstances(ctx).Return(nil, errors.New("db down"))
	s.store.EXPECT().TransitionSpecTaskStatus(
		ctx, task.ID, gomock.Any(), types.TaskStatusImplementation, nil,
	).Return(true, nil)

	s.Require().NoError(orchestrator.handleQueuedImplementation(ctx, task))
	orchestrator.wg.Wait()
	s.Equal(int32(1), workflow.justDoItStarts.Load())
}
