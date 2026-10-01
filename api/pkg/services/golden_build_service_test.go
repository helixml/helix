package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type GoldenBuildServiceSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *MockContainerExecutor
	service  *GoldenBuildService
}

func TestGoldenBuildServiceSuite(t *testing.T) {
	suite.Run(t, new(GoldenBuildServiceSuite))
}

func (s *GoldenBuildServiceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = NewMockContainerExecutor(s.ctrl)
	s.service = NewGoldenBuildService(s.store, s.executor, nil)
}

func (s *GoldenBuildServiceSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *GoldenBuildServiceSuite) TestTriggerSkipsWhenAutoWarmDisabled() {
	project := &types.Project{
		ID: "prj_test",
		Metadata: types.ProjectMetadata{
			AutoWarmDockerCache: false,
		},
	}

	// Should not call ListSandboxes — early return
	s.service.TriggerGoldenBuild(context.Background(), project)
}

func (s *GoldenBuildServiceSuite) TestFanOutQueuesPendingWhenBuildRunning() {
	project := &types.Project{
		ID: "prj_test",
		Metadata: types.ProjectMetadata{
			AutoWarmDockerCache: true,
		},
	}
	sandbox := &types.SandboxInstance{ID: "sb_1", Status: "online"}
	key := buildKey(project.ID, sandbox.ID)

	// Simulate a running build
	s.service.mu.Lock()
	s.service.building[key] = time.Now()
	s.service.mu.Unlock()

	// Trigger while build is running
	s.store.EXPECT().ListSandboxInstances(gomock.Any()).Return([]*types.SandboxInstance{sandbox}, nil)
	s.service.TriggerGoldenBuild(context.Background(), project)

	// Should have queued a pending rebuild
	s.service.mu.Lock()
	pending, ok := s.service.pendingRebuild[key]
	s.service.mu.Unlock()
	assert.True(s.T(), ok, "should have queued a pending rebuild")
	assert.Equal(s.T(), project.ID, pending.ID)
}

func (s *GoldenBuildServiceSuite) TestMultipleTriggersCoalesceToOnePending() {
	project1 := &types.Project{
		ID: "prj_test",
		Metadata: types.ProjectMetadata{
			AutoWarmDockerCache: true,
		},
	}
	project2 := &types.Project{
		ID:   "prj_test",
		Name: "updated-name",
		Metadata: types.ProjectMetadata{
			AutoWarmDockerCache: true,
		},
	}
	sandbox := &types.SandboxInstance{ID: "sb_1", Status: "online"}
	key := buildKey(project1.ID, sandbox.ID)

	// Simulate a running build
	s.service.mu.Lock()
	s.service.building[key] = time.Now()
	s.service.mu.Unlock()

	// Trigger 3 times with different project states
	s.store.EXPECT().ListSandboxInstances(gomock.Any()).Return([]*types.SandboxInstance{sandbox}, nil).Times(3)
	s.service.TriggerGoldenBuild(context.Background(), project1)
	s.service.TriggerGoldenBuild(context.Background(), project1)
	s.service.TriggerGoldenBuild(context.Background(), project2) // latest

	// Should only have one pending rebuild (latest wins)
	s.service.mu.Lock()
	assert.Len(s.T(), s.service.pendingRebuild, 1)
	pending := s.service.pendingRebuild[key]
	s.service.mu.Unlock()
	assert.Equal(s.T(), "updated-name", pending.Name, "should keep latest project state")
}

func (s *GoldenBuildServiceSuite) TestManualTriggerAlsoQueuesPending() {
	project := &types.Project{
		ID: "prj_test",
	}
	sandbox := &types.SandboxInstance{ID: "sb_1", Status: "online"}
	key := buildKey(project.ID, sandbox.ID)

	// Simulate a running build
	s.service.mu.Lock()
	s.service.building[key] = time.Now()
	s.service.mu.Unlock()

	s.store.EXPECT().ListSandboxInstances(gomock.Any()).Return([]*types.SandboxInstance{sandbox}, nil)
	err := s.service.TriggerManualGoldenBuild(context.Background(), project)
	// Returns error because no new builds started (all queued)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "already running")

	// Should still have queued the pending rebuild
	s.service.mu.Lock()
	_, ok := s.service.pendingRebuild[key]
	s.service.mu.Unlock()
	assert.True(s.T(), ok, "manual trigger should queue pending rebuild")
}

func (s *GoldenBuildServiceSuite) TestPendingRebuildTriggersAfterCompletion() {
	project := &types.Project{
		ID:     "prj_test",
		UserID: "user_1",
		Metadata: types.ProjectMetadata{
			AutoWarmDockerCache: true,
		},
	}
	sandbox := &types.SandboxInstance{ID: "sb_1", Status: "online"}
	key := buildKey(project.ID, sandbox.ID)

	// Set up: build running + pending rebuild queued
	s.service.mu.Lock()
	s.service.building[key] = time.Now()
	s.service.pendingRebuild[key] = project
	s.service.mu.Unlock()

	// Expectations for build completion (container gone, result success)
	s.executor.EXPECT().HasRunningContainer(gomock.Any(), "ses_original").Return(false)
	s.executor.EXPECT().GetGoldenBuildResult(gomock.Any(), "sb_1", "prj_test").Return(
		&hydra.GoldenBuildResult{Success: true, CacheSizeBytes: 1000}, nil,
	)
	s.store.EXPECT().GetProject(gomock.Any(), "prj_test").Return(project, nil).AnyTimes()
	s.store.EXPECT().UpdateProject(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// The defer will call fanOutBuilds which calls ListSandboxes.
	// This proves the pending rebuild was triggered.
	fanOutCalled := make(chan struct{})
	s.store.EXPECT().ListSandboxInstances(gomock.Any()).DoAndReturn(
		func(_ context.Context) ([]*types.SandboxInstance, error) {
			close(fanOutCalled)
			return []*types.SandboxInstance{sandbox}, nil
		},
	)
	// runGoldenBuildOnSandbox will be spawned and fail on nil specTaskService — that's OK.
	// We only care that fanOutBuilds was called (proving the pending rebuild triggered).
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return([]*types.GitRepository{
		{ID: "repo_1"},
	}, nil).AnyTimes()
	s.store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(&types.Session{ID: "ses_rebuild"}, nil).AnyTimes()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.service.waitForGoldenBuildCompletion(ctx, project.ID, sandbox.ID, "ses_original")
	}()

	select {
	case <-fanOutCalled:
		// fanOutBuilds was called — pending rebuild triggered
	case <-time.After(20 * time.Second):
		s.T().Fatal("pending rebuild did not trigger fanOutBuilds within timeout")
	}

	cancel()
	wg.Wait()

	// Pending should be cleared
	s.service.mu.Lock()
	_, stillPending := s.service.pendingRebuild[key]
	s.service.mu.Unlock()
	assert.False(s.T(), stillPending, "pending rebuild should be cleared after triggering")
}

func (s *GoldenBuildServiceSuite) TestNoPendingDoesNotRetrigger() {
	project := &types.Project{
		ID:     "prj_test",
		UserID: "user_1",
	}
	sandbox := &types.SandboxInstance{ID: "sb_1", Status: "online"}
	key := buildKey(project.ID, sandbox.ID)

	// Build running, NO pending
	s.service.mu.Lock()
	s.service.building[key] = time.Now()
	s.service.mu.Unlock()

	s.executor.EXPECT().HasRunningContainer(gomock.Any(), "ses_1").Return(false)
	s.executor.EXPECT().GetGoldenBuildResult(gomock.Any(), "sb_1", "prj_test").Return(
		&hydra.GoldenBuildResult{Success: true}, nil,
	)
	s.store.EXPECT().GetProject(gomock.Any(), "prj_test").Return(project, nil).AnyTimes()
	s.store.EXPECT().UpdateProject(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// StartDesktop should NOT be called (no pending)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s.service.waitForGoldenBuildCompletion(ctx, project.ID, sandbox.ID, "ses_1")

	// Building entry should be cleared
	s.service.mu.Lock()
	_, stillBuilding := s.service.building[key]
	s.service.mu.Unlock()
	assert.False(s.T(), stillBuilding)
}

func (s *GoldenBuildServiceSuite) TestNewBuildClearsPending() {
	project := &types.Project{
		ID: "prj_test",
		Metadata: types.ProjectMetadata{
			AutoWarmDockerCache: true,
		},
	}
	sandbox := &types.SandboxInstance{ID: "sb_1", Status: "online"}
	key := buildKey(project.ID, sandbox.ID)

	// Queue a pending rebuild
	s.service.mu.Lock()
	s.service.pendingRebuild[key] = project
	s.service.mu.Unlock()

	// Verify the pending is cleared when fanOutBuilds starts a new build.
	// fanOutBuilds calls delete(g.pendingRebuild, key) synchronously before
	// spawning the goroutine.
	s.store.EXPECT().ListSandboxInstances(gomock.Any()).Return([]*types.SandboxInstance{sandbox}, nil)
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return([]*types.GitRepository{
		{ID: "repo_1"},
	}, nil).AnyTimes()
	s.store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(&types.Session{ID: "ses_1"}, nil).AnyTimes()
	s.store.EXPECT().GetProject(gomock.Any(), gomock.Any()).Return(project, nil).AnyTimes()
	s.store.EXPECT().UpdateProject(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	s.service.fanOutBuilds(context.Background(), project)

	// Give the goroutine a moment to run (it'll fail on nil specTaskService, that's fine)
	time.Sleep(100 * time.Millisecond)

	// Pending should be cleared (fanOutBuilds clears it synchronously)
	s.service.mu.Lock()
	_, stillPending := s.service.pendingRebuild[key]
	s.service.mu.Unlock()
	assert.False(s.T(), stillPending, "starting a new build should clear pending")
}

// waitWithResult runs waitForGoldenBuildCompletion against a finished build
// whose hydra result is res, and returns the sandbox cache state it persisted.
func (s *GoldenBuildServiceSuite) waitWithResult(res *hydra.GoldenBuildResult) *types.SandboxCacheState {
	return s.waitWithResults(res, "running", 1)
}

// waitWithResults is waitWithResult where hydra keeps returning res for polls
// polls, and the build session's external_agent_status is sessionStatus.
func (s *GoldenBuildServiceSuite) waitWithResults(res *hydra.GoldenBuildResult, sessionStatus string, polls int) *types.SandboxCacheState {
	orig := goldenBuildPollInterval
	goldenBuildPollInterval = 10 * time.Millisecond
	defer func() { goldenBuildPollInterval = orig }()

	project := &types.Project{ID: "prj_test"}
	s.executor.EXPECT().HasRunningContainer(gomock.Any(), "ses_build").Return(false).Times(polls)
	s.executor.EXPECT().GetGoldenBuildResult(gomock.Any(), "sb_1", "prj_test").Return(res, nil).Times(polls)
	s.store.EXPECT().GetSession(gomock.Any(), "ses_build").Return(&types.Session{
		ID:       "ses_build",
		Metadata: types.SessionMetadata{ExternalAgentStatus: sessionStatus, GoldenBuild: true},
	}, nil).AnyTimes()
	s.store.EXPECT().GetProject(gomock.Any(), "prj_test").Return(project, nil)
	s.store.EXPECT().UpdateProject(gomock.Any(), gomock.Any()).Return(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.service.waitForGoldenBuildCompletion(ctx, project.ID, "sb_1", "ses_build")

	return project.Metadata.DockerCacheStatus.Sandboxes["sb_1"]
}

// The startup script exited 0 but hydra failed to promote the cache: the
// cache was NOT refreshed, so the status must be failed with hydra's error.
func (s *GoldenBuildServiceSuite) TestPromotionFailureMarksCacheFailed() {
	state := s.waitWithResult(&hydra.GoldenBuildResult{
		Success:  false,
		ExitCode: "0",
		Error:    "Golden cache promotion failed: zvol device /dev/zvol/x did not appear within 2m0s",
	})

	assert.Equal(s.T(), "failed", state.Status)
	assert.Equal(s.T(), "Golden cache promotion failed: zvol device /dev/zvol/x did not appear within 2m0s", state.Error)
	assert.Nil(s.T(), state.LastReadyAt)
}

func (s *GoldenBuildServiceSuite) TestScriptFailureReportsExitCode() {
	state := s.waitWithResult(&hydra.GoldenBuildResult{Success: false, ExitCode: "2"})

	assert.Equal(s.T(), "failed", state.Status)
	assert.Equal(s.T(), "Startup script exited with code 2", state.Error)
}

func (s *GoldenBuildServiceSuite) TestSuccessfulBuildMarksCacheReady() {
	state := s.waitWithResult(&hydra.GoldenBuildResult{Success: true, ExitCode: "0", CacheSizeBytes: 42})

	assert.Equal(s.T(), "ready", state.Status)
	assert.Empty(s.T(), state.Error)
	assert.Equal(s.T(), int64(42), state.SizeBytes)
}

// The meta incident: the desktop idle checker stopped the golden build. The
// failure must name the cause, not the generic "Startup script failed".
func (s *GoldenBuildServiceSuite) TestIdleStoppedBuildReportsIdleChecker() {
	state := s.waitWithResults(&hydra.GoldenBuildResult{
		SessionID: "ses_build",
		ExitCode:  "exited",
		Error:     "build container was removed without writing a golden build result",
	}, "terminated_idle", 1)

	assert.Equal(s.T(), "failed", state.Status)
	assert.Equal(s.T(), "Stopped by the desktop idle checker: build container was removed without writing a golden build result", state.Error)
}

// Hydra keeps the latest result per project. A successful result from an
// earlier build must not mark this build ready; with no result of its own the
// build fails with an explicit cause.
func (s *GoldenBuildServiceSuite) TestStaleResultFromEarlierBuildIsIgnored() {
	state := s.waitWithResults(&hydra.GoldenBuildResult{
		SessionID: "ses_previous",
		Success:   true,
		ExitCode:  "0",
	}, "stopped", goldenBuildResultPolls+1)

	assert.Equal(s.T(), "failed", state.Status)
	assert.Equal(s.T(), "Build container stopped without reporting a result (stopped externally or crashed)", state.Error)
}

// Golden build sessions are created marked GoldenBuild so the desktop idle
// checker (ListIdleDesktops) never stops them.
func (s *GoldenBuildServiceSuite) TestGoldenBuildSessionIsMarkedGoldenBuild() {
	project := &types.Project{ID: "prj_test", Name: "test"}
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return([]*types.GitRepository{{ID: "repo_1"}}, nil)
	var created types.Session
	s.store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			created = session
			return &session, nil
		},
	)
	// Status updates; the build then stops at the nil specTaskService.
	s.store.EXPECT().GetProject(gomock.Any(), "prj_test").Return(project, nil).AnyTimes()
	s.store.EXPECT().UpdateProject(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	s.service.runGoldenBuildOnSandbox(context.Background(), project, "sb_1")

	assert.True(s.T(), created.Metadata.GoldenBuild, "golden build session must be marked GoldenBuild")
}
