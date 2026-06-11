package services

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestSpecDrivenTaskService_StartSpecGeneration_NoDoubleStartDesktopOnConcurrency
// pins the fix for the orchestrator double-fire race that ships two dev containers
// against the same spec-task workspace and corrupts its git clone.
//
// In production, SpecTaskOrchestrator has two dispatch paths: a 10s ticker that
// iterates active tasks, and a subscription that fires on every status change.
// Both can land on a task in QueuedSpecGeneration within milliseconds of each
// other and each spawn a goroutine that calls StartSpecGeneration. The old guard
// at spec_driven_task_service.go:414-423 was a read-then-write TOCTOU — both
// callers read PlanningSessionID=="", both passed, both went on to CreateSession
// and StartDesktop. Real-world evidence of this failure: planning session
// ses_01kts7zgv54067bgkdyjv3kj2x for task spt_01kts7zed3fe0wmadwb5wv0xzy on
// 2026-06-10, where two containers (01kts7zgv0znfte5dr3nq6stbs +
// 01kts7zgv54067bgkdyjv3kj2x) raced the same workspace volume and one ended up
// with a half-written /home/retro/work/helix whose `git fetch origin --prune`
// returned only `* branch HEAD -> FETCH_HEAD`.
//
// The fix: SetPlanningSessionIDIfEmpty does the claim atomically at the store
// layer; the loser deletes its orphan session and returns BEFORE StartDesktop.
func TestSpecDrivenTaskService_StartSpecGeneration_NoDoubleStartDesktopOnConcurrency(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockExecutor := external_agent.NewMockExecutor(ctrl)

	project := &types.Project{
		ID:                "project-race",
		OrganizationID:    "org-1",
		DefaultHelixAppID: "app-race",
	}
	app := &types.App{ID: "app-race"}
	baseTask := &types.SpecTask{
		ID:         "task-race",
		ProjectID:  "project-race",
		HelixAppID: "app-race",
		Status:     types.TaskStatusQueuedSpecGeneration,
		CreatedBy:  "user-1",
		BranchMode: types.BranchModeNew,
		BaseBranch: "main",
		Name:       "racing task",
	}

	// All read-side store calls happen on both goroutines; allow any number.
	mockStore.EXPECT().GetProject(gomock.Any(), "project-race").Return(project, nil).AnyTimes()
	mockStore.EXPECT().GetApp(gomock.Any(), "app-race").Return(app, nil).AnyTimes()
	mockStore.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	// The TOCTOU read at spec_driven_task_service.go:417. We force it to ALWAYS
	// return an empty planning_session_id so both concurrent callers pass the
	// read-then-write guard. This is the canonical setup for the race.
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "task-race").Return(&types.SpecTask{
		ID:                "task-race",
		PlanningSessionID: "",
	}, nil).AnyTimes()

	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// CreateSession echoes the input (preserves any pre-generated ID).
	mockStore.EXPECT().CreateSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, s types.Session) (*types.Session, error) {
			sCopy := s
			return &sCopy, nil
		}).AnyTimes()

	// The orphan-cleanup path the fix uses on the losing branch. Without the
	// fix this is never called (the loser proceeds to StartDesktop instead).
	mockStore.EXPECT().DeleteSession(gomock.Any(), gomock.Any()).
		Return(&types.Session{}, nil).AnyTimes()

	// The atomic gate that replaces the TOCTOU guard. The DoAndReturn body is
	// the in-test simulation of the postgres single-statement UPDATE: the
	// first caller wins, the rest lose. Without the fix this is never called.
	var claimAttempts int32
	mockStore.EXPECT().SetPlanningSessionIDIfEmpty(gomock.Any(), "task-race", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string) (bool, error) {
			return atomic.AddInt32(&claimAttempts, 1) == 1, nil
		}).AnyTimes()

	mockStore.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, i *types.Interaction) (*types.Interaction, error) {
			return i, nil
		}).AnyTimes()

	mockStore.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).
		Return([]*types.GitRepository{}, nil).AnyTimes()

	mockStore.EXPECT().ListSpecTaskAttachments(gomock.Any(), gomock.Any()).
		Return([]*types.SpecTaskAttachment{}, nil).AnyTimes()

	// The assertion this whole test exists for: even with two concurrent
	// StartSpecGeneration callers seeing the same empty PlanningSessionID,
	// exactly one of them must reach StartDesktop. The other must bail.
	var startDesktopCount int32
	mockExecutor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *types.DesktopAgent) (*types.DesktopAgentResponse, error) {
			atomic.AddInt32(&startDesktopCount, 1)
			return &types.DesktopAgentResponse{
				DevContainerID: "dev-test",
				ContainerName:  "container-test",
			}, nil
		}).AnyTimes()

	// Real GitRepositoryService — SyncBaseBranchForTask iterates zero repos
	// (we mock ListGitRepositories to []), so no remote calls happen.
	gitRepoService := NewGitRepositoryService(mockStore, t.TempDir(), "http://test", "test", "test@test")

	service := NewSpecDrivenTaskService(
		mockStore,
		nil, // notifier
		"test-helix-agent",
		[]string{"test-zed-agent"},
		nil, // pubsub
		mockExecutor,
		gitRepoService,
		nil, // RegisterRequestMapping
		NewDisabledKoditService(),
	)
	service.SetTestMode(true)

	ctx := context.Background()

	// Two callers, mirroring orchestrator ticker + subscription firing within
	// milliseconds of each other on the same task.
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine gets an independent task copy so they don't
			// trample shared in-memory fields. (Same as production: the
			// orchestrator passes a fresh *types.SpecTask per dispatch.)
			taskCopy := *baseTask
			service.StartSpecGeneration(ctx, &taskCopy)
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), atomic.LoadInt32(&startDesktopCount),
		"StartDesktop must fire exactly once per task even when two StartSpecGeneration goroutines race; got %d calls",
		atomic.LoadInt32(&startDesktopCount))
}
