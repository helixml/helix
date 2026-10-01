package services

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// fakeGoldenBuilds backs the mocked golden_builds store methods with a map,
// so the persisted state survives across GoldenBuildService instances (a
// simulated API restart).
type fakeGoldenBuilds struct {
	mu   sync.Mutex
	rows map[string]*types.SandboxCacheState
}

func (f *fakeGoldenBuilds) get(projectID, sandboxID string) *types.SandboxCacheState {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[projectID+"/"+sandboxID]
	if !ok {
		return nil
	}
	cp := *row
	return &cp
}

func (f *fakeGoldenBuilds) set(row *types.SandboxCacheState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *row
	f.rows[row.ProjectID+"/"+row.SandboxID] = &cp
}

func (f *fakeGoldenBuilds) install(m *store.MockStore) {
	m.EXPECT().UpdateGoldenBuild(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, projectID, sandboxID string, update func(*types.SandboxCacheState) bool) (*types.SandboxCacheState, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			key := projectID + "/" + sandboxID
			row, ok := f.rows[key]
			if !ok {
				row = &types.SandboxCacheState{ProjectID: projectID, SandboxID: sandboxID, Status: types.GoldenBuildStatusNone}
			}
			cp := *row
			if update(&cp) {
				f.rows[key] = &cp
			} else {
				f.rows[key] = row
				cp = *row
			}
			return &cp, nil
		}).AnyTimes()
	m.EXPECT().GetGoldenBuild(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, projectID, sandboxID string) (*types.SandboxCacheState, error) {
			if row := f.get(projectID, sandboxID); row != nil {
				return row, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()
	m.EXPECT().ListGoldenBuilds(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, q *store.ListGoldenBuildsQuery) ([]*types.SandboxCacheState, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			var out []*types.SandboxCacheState
			for _, row := range f.rows {
				if q.ProjectID != "" && row.ProjectID != q.ProjectID {
					continue
				}
				if q.SandboxID != "" && row.SandboxID != q.SandboxID {
					continue
				}
				if q.ActiveOnly && !row.Active() {
					continue
				}
				cp := *row
				out = append(out, &cp)
			}
			return out, nil
		}).AnyTimes()
}

type GoldenBuildServiceSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *MockContainerExecutor
	service  *GoldenBuildService
	builds   *fakeGoldenBuilds
	project  *types.Project

	// services are cancelled and drained at teardown.
	services []*GoldenBuildService
	cancels  []context.CancelFunc

	sessionSeq int
	sessionsMu sync.Mutex
	// started receives the session ID of every build container started.
	started chan string
}

func TestGoldenBuildServiceSuite(t *testing.T) {
	suite.Run(t, new(GoldenBuildServiceSuite))
}

func (s *GoldenBuildServiceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = NewMockContainerExecutor(s.ctrl)
	s.builds = &fakeGoldenBuilds{rows: make(map[string]*types.SandboxCacheState)}
	s.builds.install(s.store)
	s.services, s.cancels = nil, nil
	s.service = s.newService()
	s.project = &types.Project{
		ID:       "prj_test",
		Name:     "test",
		UserID:   "user_1",
		Metadata: types.ProjectMetadata{AutoWarmDockerCache: true},
	}
	s.started = make(chan string, 16)
	s.sessionSeq = 0

	origPoll, origBackoff, origGrace := goldenBuildPollInterval, goldenBuildRetryBackoff, goldenBuildClaimGrace
	goldenBuildPollInterval = 5 * time.Millisecond
	goldenBuildRetryBackoff = func(int) time.Duration { return 0 }
	s.T().Cleanup(func() {
		goldenBuildPollInterval, goldenBuildRetryBackoff, goldenBuildClaimGrace = origPoll, origBackoff, origGrace
	})

	s.store.EXPECT().GetProject(gomock.Any(), "prj_test").DoAndReturn(
		func(context.Context, string) (*types.Project, error) { return s.project, nil }).AnyTimes()
	s.store.EXPECT().ListSandboxInstances(gomock.Any()).Return(
		[]*types.SandboxInstance{{ID: "sb_1", Status: "online"}}, nil).AnyTimes()
	s.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).Return(
		&types.Session{Metadata: types.SessionMetadata{ExternalAgentStatus: "running", GoldenBuild: true}}, nil).AnyTimes()
}

// newService returns a GoldenBuildService whose background goroutines are
// stopped and drained at teardown.
func (s *GoldenBuildServiceSuite) newService() *GoldenBuildService {
	svc := NewGoldenBuildService(s.store, s.executor, nil)
	ctx, cancel := context.WithCancel(context.Background())
	svc.ctx = ctx
	s.services = append(s.services, svc)
	s.cancels = append(s.cancels, cancel)
	return svc
}

func (s *GoldenBuildServiceSuite) TearDownTest() {
	for _, cancel := range s.cancels {
		cancel()
	}
	for _, svc := range s.services {
		svc.wg.Wait()
	}
	s.ctrl.Finish()
}

// allowBuildStarts lets runGoldenBuildOnSandbox get as far as StartDesktop.
// The service under test has no SpecDrivenTaskService, so it is given one
// whose API key lookup is served by the mock store.
func (s *GoldenBuildServiceSuite) allowBuildStarts(svc *GoldenBuildService) {
	svc.specTaskService = &SpecDrivenTaskService{store: s.store}
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return(
		[]*types.GitRepository{{ID: "repo_1"}}, nil).AnyTimes()
	s.store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			s.sessionsMu.Lock()
			s.sessionSeq++
			session.ID = fmt.Sprintf("ses_%d", s.sessionSeq)
			s.sessionsMu.Unlock()
			return &session, nil
		}).AnyTimes()
	s.store.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(
		&types.ApiKey{Key: "hl-test"}, nil).AnyTimes()
	s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error) {
			s.started <- agent.SessionID
			return &types.DesktopAgentResponse{}, nil
		}).AnyTimes()
}

func (s *GoldenBuildServiceSuite) waitStarted() string {
	select {
	case id := <-s.started:
		return id
	case <-time.After(5 * time.Second):
		s.T().Fatal("no build container was started")
		return ""
	}
}

func (s *GoldenBuildServiceSuite) assertNoStart() {
	select {
	case id := <-s.started:
		s.T().Fatalf("unexpected build container start: %s", id)
	case <-time.After(100 * time.Millisecond):
	}
}

// waitFor polls the persisted state until cond holds.
func (s *GoldenBuildServiceSuite) waitFor(desc string, cond func(*types.SandboxCacheState) bool) *types.SandboxCacheState {
	var row *types.SandboxCacheState
	require.Eventually(s.T(), func() bool {
		row = s.builds.get("prj_test", "sb_1")
		return row != nil && cond(row)
	}, 5*time.Second, 5*time.Millisecond, "state never became %s (last: %+v)", desc, row)
	return row
}

func (s *GoldenBuildServiceSuite) buildingRow(sessionID string, attempt int) *types.SandboxCacheState {
	now := time.Now()
	return &types.SandboxCacheState{
		ProjectID: "prj_test", SandboxID: "sb_1",
		Status: types.GoldenBuildStatusBuilding, BuildSessionID: sessionID,
		Attempt: attempt, LastBuildAt: &now, TriggeredAt: &now,
	}
}

// containerDone makes sessionID's container gone and hydra report res for it.
func (s *GoldenBuildServiceSuite) containerDone(sessionID string, res *hydra.GoldenBuildResult) {
	s.executor.EXPECT().GoldenBuildContainerRunning(gomock.Any(), "sb_1", sessionID).Return(false, nil).AnyTimes()
	s.executor.EXPECT().GetGoldenBuildResult(gomock.Any(), "sb_1", "prj_test").Return(res, nil).AnyTimes()
}

func (s *GoldenBuildServiceSuite) TestTriggerSkipsWhenAutoWarmDisabled() {
	project := &types.Project{ID: "prj_test"}
	s.service.TriggerGoldenBuild(context.Background(), project)
	assert.Nil(s.T(), s.builds.get("prj_test", "sb_1"))
}

func (s *GoldenBuildServiceSuite) TestTriggerWhileBuildingQueuesOnePendingRebuild() {
	s.builds.set(s.buildingRow("ses_running", 1))

	s.service.TriggerGoldenBuild(context.Background(), s.project)
	s.service.TriggerGoldenBuild(context.Background(), s.project)

	row := s.builds.get("prj_test", "sb_1")
	assert.True(s.T(), row.PendingRebuild)
	assert.Equal(s.T(), types.GoldenBuildStatusBuilding, row.Status)
	assert.Equal(s.T(), "ses_running", row.BuildSessionID, "the running build is untouched")
}

func (s *GoldenBuildServiceSuite) TestManualTriggerWhileBuildingQueuesPending() {
	s.builds.set(s.buildingRow("ses_running", 1))

	err := s.service.TriggerManualGoldenBuild(context.Background(), s.project)
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "already running")
	assert.True(s.T(), s.builds.get("prj_test", "sb_1").PendingRebuild)
}

func (s *GoldenBuildServiceSuite) TestSuccessfulBuildIsReadyAndPersistsAttempt() {
	s.allowBuildStarts(s.service)
	s.executor.EXPECT().GoldenBuildContainerRunning(gomock.Any(), "sb_1", "ses_1").Return(false, nil).AnyTimes()
	s.executor.EXPECT().GetGoldenBuildResult(gomock.Any(), "sb_1", "prj_test").Return(
		&hydra.GoldenBuildResult{SessionID: "ses_1", Success: true, ExitCode: "0", CacheSizeBytes: 42}, nil).AnyTimes()

	s.service.TriggerGoldenBuild(context.Background(), s.project)
	assert.Equal(s.T(), "ses_1", s.waitStarted())

	row := s.waitFor("ready", func(r *types.SandboxCacheState) bool { return r.Status == types.GoldenBuildStatusReady })
	assert.Equal(s.T(), int64(42), row.SizeBytes)
	assert.Equal(s.T(), 1, row.Attempt)
	assert.Empty(s.T(), row.BuildSessionID)
	assert.NotNil(s.T(), row.LastReadyAt)
}

// The sandbox was recreated mid-build: Hydra comes back without the container
// and without a result. That is an interruption: the build is retried on the
// same sandbox as attempt 2 of the same trigger.
func (s *GoldenBuildServiceSuite) TestInterruptedBuildIsRetried() {
	s.allowBuildStarts(s.service)
	// Attempt 1: hydra unreachable for a while (sandbox recreating), then
	// back with no container and no result.
	unreachable := 3
	s.executor.EXPECT().GoldenBuildContainerRunning(gomock.Any(), "sb_1", "ses_1").DoAndReturn(
		func(context.Context, string, string) (bool, error) {
			if unreachable > 0 {
				unreachable--
				return false, fmt.Errorf("failed to dial Hydra via RevDial")
			}
			return false, nil
		}).AnyTimes()
	// Attempt 2 succeeds.
	s.executor.EXPECT().GoldenBuildContainerRunning(gomock.Any(), "sb_1", "ses_2").Return(false, nil).AnyTimes()
	var mu sync.Mutex
	var result *hydra.GoldenBuildResult
	s.executor.EXPECT().GetGoldenBuildResult(gomock.Any(), "sb_1", "prj_test").DoAndReturn(
		func(context.Context, string, string) (*hydra.GoldenBuildResult, error) {
			mu.Lock()
			defer mu.Unlock()
			return result, nil
		}).AnyTimes()

	s.service.TriggerGoldenBuild(context.Background(), s.project)
	assert.Equal(s.T(), "ses_1", s.waitStarted())

	retrying := s.waitFor("building attempt 2", func(r *types.SandboxCacheState) bool {
		return r.Status == types.GoldenBuildStatusBuilding && r.Attempt == 2
	})
	assert.Contains(s.T(), retrying.InterruptReason, "disappeared without a result")
	mu.Lock()
	result = &hydra.GoldenBuildResult{SessionID: "ses_2", Success: true, ExitCode: "0"}
	mu.Unlock()
	assert.Equal(s.T(), "ses_2", s.waitStarted())

	row := s.waitFor("ready", func(r *types.SandboxCacheState) bool { return r.Status == types.GoldenBuildStatusReady })
	assert.Equal(s.T(), 2, row.Attempt)
}

// The startup script ran and exited non-zero: a real failure, not retried.
func (s *GoldenBuildServiceSuite) TestNonZeroExitFailsWithoutRetry() {
	s.allowBuildStarts(s.service)
	s.containerDone("ses_1", &hydra.GoldenBuildResult{SessionID: "ses_1", ExitCode: "2"})

	s.service.TriggerGoldenBuild(context.Background(), s.project)
	s.waitStarted()

	row := s.waitFor("failed", func(r *types.SandboxCacheState) bool { return r.Status == types.GoldenBuildStatusFailed })
	assert.Equal(s.T(), "Startup script exited with code 2", row.Error)
	assert.Equal(s.T(), 1, row.Attempt)
	s.service.ReconcileSandbox(context.Background(), "sb_1")
	s.assertNoStart()
}

// Promotion failed after the script succeeded: the cache wasn't refreshed,
// so the build failed with hydra's error and is not retried.
func (s *GoldenBuildServiceSuite) TestPromotionFailureFailsWithoutRetry() {
	s.builds.set(s.buildingRow("ses_build", 1))
	s.containerDone("ses_build", &hydra.GoldenBuildResult{
		SessionID: "ses_build", ExitCode: "0",
		Error: "Golden cache promotion failed: zvol device /dev/zvol/x did not appear within 2m0s",
	})

	s.service.ReconcileSandbox(context.Background(), "sb_1")

	row := s.waitFor("failed", func(r *types.SandboxCacheState) bool { return r.Status == types.GoldenBuildStatusFailed })
	assert.Equal(s.T(), "Golden cache promotion failed: zvol device /dev/zvol/x did not appear within 2m0s", row.Error)
	assert.Nil(s.T(), row.LastReadyAt)
}

// The last allowed attempt was interrupted too: give up with the reason.
func (s *GoldenBuildServiceSuite) TestRetryBudgetExhaustedFails() {
	s.builds.set(s.buildingRow("ses_build", types.GoldenBuildMaxAttempts))
	s.containerDone("ses_build", &hydra.GoldenBuildResult{
		SessionID: "ses_build", ExitCode: "exited",
		Error: "build container was removed without writing a golden build result",
	})

	s.service.ReconcileSandbox(context.Background(), "sb_1")

	row := s.waitFor("failed", func(r *types.SandboxCacheState) bool { return r.Status == types.GoldenBuildStatusFailed })
	assert.Equal(s.T(), fmt.Sprintf("Interrupted %d times, giving up: build container was removed without writing a golden build result", types.GoldenBuildMaxAttempts), row.Error)
	s.assertNoStart()
}

// Interruptions are only retried for auto-warm projects.
func (s *GoldenBuildServiceSuite) TestInterruptedManualBuildWithoutAutoWarmFails() {
	s.project.Metadata.AutoWarmDockerCache = false
	s.builds.set(s.buildingRow("ses_build", 1))
	s.containerDone("ses_build", nil)

	s.service.ReconcileSandbox(context.Background(), "sb_1")

	row := s.waitFor("failed", func(r *types.SandboxCacheState) bool { return r.Status == types.GoldenBuildStatusFailed })
	assert.Contains(s.T(), row.Error, "not retried because auto-warm is off")
}

// The idle checker stopping a golden build names the cause.
func (s *GoldenBuildServiceSuite) TestIdleStoppedBuildReportsIdleChecker() {
	s.store = store.NewMockStore(s.ctrl)
	s.builds.install(s.store)
	s.service = s.newService()
	s.service.store = s.store
	s.project.Metadata.AutoWarmDockerCache = false
	s.store.EXPECT().GetProject(gomock.Any(), "prj_test").Return(s.project, nil).AnyTimes()
	s.store.EXPECT().GetSession(gomock.Any(), "ses_build").Return(&types.Session{
		Metadata: types.SessionMetadata{ExternalAgentStatus: "terminated_idle", GoldenBuild: true},
	}, nil).AnyTimes()
	s.builds.set(s.buildingRow("ses_build", 1))
	s.containerDone("ses_build", &hydra.GoldenBuildResult{
		SessionID: "ses_build", ExitCode: "exited",
		Error: "build container was removed without writing a golden build result",
	})

	s.service.ReconcileSandbox(context.Background(), "sb_1")

	row := s.waitFor("failed", func(r *types.SandboxCacheState) bool { return r.Status == types.GoldenBuildStatusFailed })
	assert.Contains(s.T(), row.Error, "stopped by the desktop idle checker: build container was removed")
}

// Hydra keeps the latest result per project; one from an earlier build is
// not this build's result.
func (s *GoldenBuildServiceSuite) TestStaleResultFromEarlierBuildIsIgnored() {
	s.project.Metadata.AutoWarmDockerCache = false
	s.builds.set(s.buildingRow("ses_build", 1))
	s.containerDone("ses_build", &hydra.GoldenBuildResult{SessionID: "ses_previous", Success: true, ExitCode: "0"})

	s.service.ReconcileSandbox(context.Background(), "sb_1")

	row := s.waitFor("failed", func(r *types.SandboxCacheState) bool { return r.Status == types.GoldenBuildStatusFailed })
	assert.Nil(s.T(), row.LastReadyAt)
	assert.Contains(s.T(), row.Error, "disappeared without a result")
}

// A merge arrives mid-build, then the API restarts. The new service instance
// reads the persisted state, resumes polling the still-running build, and
// once it completes starts the pending rebuild.
func (s *GoldenBuildServiceSuite) TestPendingRebuildSurvivesRestartAndFiresAfterCompletion() {
	s.builds.set(s.buildingRow("ses_running", 1))
	s.service.TriggerGoldenBuild(context.Background(), s.project)
	require.True(s.T(), s.builds.get("prj_test", "sb_1").PendingRebuild)

	// API restart: a fresh service with no in-memory state.
	restarted := s.newService()
	s.allowBuildStarts(restarted)
	var mu sync.Mutex
	running := true
	s.executor.EXPECT().GoldenBuildContainerRunning(gomock.Any(), "sb_1", "ses_running").DoAndReturn(
		func(context.Context, string, string) (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			return running, nil
		}).AnyTimes()
	s.executor.EXPECT().GetGoldenBuildResult(gomock.Any(), "sb_1", "prj_test").Return(
		&hydra.GoldenBuildResult{SessionID: "ses_running", Success: true, ExitCode: "0"}, nil).AnyTimes()
	s.executor.EXPECT().GoldenBuildContainerRunning(gomock.Any(), "sb_1", "ses_1").Return(true, nil).AnyTimes()

	restarted.ReconcileSandbox(context.Background(), "sb_1")
	restarted.ReconcileSandbox(context.Background(), "sb_1") // idempotent: one monitor
	s.assertNoStart()
	assert.True(s.T(), restarted.isMonitoring("ses_running"), "startup reconcile resumes polling the running build")

	mu.Lock()
	running = false
	mu.Unlock()

	assert.Equal(s.T(), "ses_1", s.waitStarted(), "pending rebuild starts after the running build completes")
	row := s.waitFor("rebuilding", func(r *types.SandboxCacheState) bool { return r.BuildSessionID == "ses_1" })
	assert.False(s.T(), row.PendingRebuild)
	assert.Equal(s.T(), 1, row.Attempt, "a pending rebuild is a new trigger with a fresh budget")
	assert.NotNil(s.T(), row.LastReadyAt, "the completed build was recorded before the rebuild")
}

// The API died between claiming a build and creating its session: the claim
// is abandoned and treated as an interruption.
func (s *GoldenBuildServiceSuite) TestAbandonedClaimIsRetried() {
	goldenBuildClaimGrace = 0
	row := s.buildingRow("", 1)
	s.builds.set(row)
	s.allowBuildStarts(s.service)
	s.executor.EXPECT().GoldenBuildContainerRunning(gomock.Any(), "sb_1", "ses_1").Return(true, nil).AnyTimes()

	s.service.ReconcileSandbox(context.Background(), "sb_1")

	assert.Equal(s.T(), "ses_1", s.waitStarted())
	got := s.waitFor("attempt 2", func(r *types.SandboxCacheState) bool { return r.Attempt == 2 })
	assert.Contains(s.T(), got.InterruptReason, "before the build container was created")
}

// A retry waits for its backoff.
func (s *GoldenBuildServiceSuite) TestRetryWaitsForBackoff() {
	next := time.Now().Add(time.Hour)
	s.builds.set(&types.SandboxCacheState{
		ProjectID: "prj_test", SandboxID: "sb_1", Status: types.GoldenBuildStatusRetrying,
		Attempt: 1, NextRetryAt: &next, InterruptReason: "sandbox restarted",
	})
	s.service.ReconcileSandbox(context.Background(), "sb_1")
	s.assertNoStart()
	assert.Equal(s.T(), types.GoldenBuildStatusRetrying, s.builds.get("prj_test", "sb_1").Status)
}

// Cancelling stops the container and the monitor exits without overwriting
// the cancelled state.
func (s *GoldenBuildServiceSuite) TestCancelStopsBuildAndMonitorDoesNotOverwrite() {
	s.builds.set(s.buildingRow("ses_build", 1))
	s.executor.EXPECT().GoldenBuildContainerRunning(gomock.Any(), "sb_1", "ses_build").Return(true, nil).AnyTimes()
	s.service.ReconcileSandbox(context.Background(), "sb_1")
	require.True(s.T(), s.service.isMonitoring("ses_build"))

	s.executor.EXPECT().StopDesktop(gomock.Any(), "ses_build").Return(nil)
	require.NoError(s.T(), s.service.CancelGoldenBuilds(context.Background(), s.project))

	require.Eventually(s.T(), func() bool { return !s.service.isMonitoring("ses_build") }, 5*time.Second, 5*time.Millisecond)
	row := s.builds.get("prj_test", "sb_1")
	assert.Equal(s.T(), types.GoldenBuildStatusNone, row.Status)
	assert.Empty(s.T(), row.Error)
}

// Golden build sessions are created marked GoldenBuild so the desktop idle
// checker (ListIdleDesktops) never stops them.
func (s *GoldenBuildServiceSuite) TestGoldenBuildSessionIsMarkedGoldenBuild() {
	s.allowBuildStarts(s.service)
	s.executor.EXPECT().GoldenBuildContainerRunning(gomock.Any(), "sb_1", "ses_1").Return(true, nil).AnyTimes()
	var created types.Session
	s.store = store.NewMockStore(s.ctrl)
	s.builds.install(s.store)
	s.service.store = s.store
	s.service.specTaskService = &SpecDrivenTaskService{store: s.store}
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return([]*types.GitRepository{{ID: "repo_1"}}, nil)
	s.store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			created = session
			session.ID = "ses_1"
			return &session, nil
		})
	s.store.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(&types.ApiKey{Key: "hl-test"}, nil).AnyTimes()
	s.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).Return(&types.Session{}, nil).AnyTimes()

	claimed, _, err := s.service.claimBuild(context.Background(), "prj_test", "sb_1", triggerNew)
	require.NoError(s.T(), err)
	require.True(s.T(), claimed)
	s.service.goBackground(func() { s.service.runGoldenBuildOnSandbox(s.project, "sb_1") })
	s.waitStarted()

	assert.True(s.T(), created.Metadata.GoldenBuild, "golden build session must be marked GoldenBuild")
}
