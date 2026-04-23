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
	s.store.EXPECT().ListSandboxes(gomock.Any()).Return([]*types.SandboxInstance{sandbox}, nil)
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
	s.store.EXPECT().ListSandboxes(gomock.Any()).Return([]*types.SandboxInstance{sandbox}, nil).Times(3)
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

	s.store.EXPECT().ListSandboxes(gomock.Any()).Return([]*types.SandboxInstance{sandbox}, nil)
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
	s.store.EXPECT().ListSandboxes(gomock.Any()).DoAndReturn(
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
	s.store.EXPECT().ListSandboxes(gomock.Any()).Return([]*types.SandboxInstance{sandbox}, nil)
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
