package external_agent

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/license"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/wolf"
	"github.com/stretchr/testify/mock"
)

// MockStoreForWolf embeds testify mock and implements only the methods needed for these tests
type MockStoreForWolf struct {
	mock.Mock
}

func (m *MockStoreForWolf) GetIdleExternalAgents(ctx context.Context, cutoff time.Time, agentTypes []string) ([]*types.ExternalAgentActivity, error) {
	args := m.Called(ctx, cutoff, agentTypes)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.ExternalAgentActivity), args.Error(1)
}

func (m *MockStoreForWolf) GetSpecTaskExternalAgentByID(ctx context.Context, agentID string) (*types.SpecTaskExternalAgent, error) {
	args := m.Called(ctx, agentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.SpecTaskExternalAgent), args.Error(1)
}

func (m *MockStoreForWolf) UpdateSpecTaskExternalAgent(ctx context.Context, agent *types.SpecTaskExternalAgent) error {
	args := m.Called(ctx, agent)
	return args.Error(0)
}

func (m *MockStoreForWolf) GetSession(ctx context.Context, id string) (*types.Session, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Session), args.Error(1)
}

func (m *MockStoreForWolf) UpdateSession(ctx context.Context, session types.Session) (*types.Session, error) {
	args := m.Called(ctx, session)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Session), args.Error(1)
}

func (m *MockStoreForWolf) DeleteExternalAgentActivity(ctx context.Context, agentID string) error {
	args := m.Called(ctx, agentID)
	return args.Error(0)
}

func (m *MockStoreForWolf) AttachRepositoryToProject(ctx context.Context, projectID string, repoID string) error {
	args := m.Called(ctx, projectID, repoID)
	return args.Error(0)
}

func (m *MockStoreForWolf) CreateGitRepository(ctx context.Context, repo *types.GitRepository) error {
	args := m.Called(ctx, repo)
	return args.Error(0)
}

func (m *MockStoreForWolf) DeleteGitRepository(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStoreForWolf) CreateSampleProject(ctx context.Context, sample *types.SampleProject) (*types.SampleProject, error) {
	args := m.Called(ctx, sample)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.SampleProject), args.Error(1)
}

func (m *MockStoreForWolf) CreateSpecTaskDesignReview(ctx context.Context, review *types.SpecTaskDesignReview) error {
	args := m.Called(ctx, review)
	return args.Error(0)
}

func (m *MockStoreForWolf) CreateSpecTaskDesignReviewComment(ctx context.Context, comment *types.SpecTaskDesignReviewComment) error {
	args := m.Called(ctx, comment)
	return args.Error(0)
}

func (m *MockStoreForWolf) CreateSpecTaskDesignReviewCommentReply(ctx context.Context, reply *types.SpecTaskDesignReviewCommentReply) error {
	args := m.Called(ctx, reply)
	return args.Error(0)
}

func (m *MockStoreForWolf) CreateSpecTaskGitPushEvent(ctx context.Context, event *types.SpecTaskGitPushEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockStoreForWolf) CreateSpecTask(ctx context.Context, task *types.SpecTask) error {
	args := m.Called(ctx, task)
	return args.Error(0)
}

func (m *MockStoreForWolf) CreateSpecTaskWorkSession(ctx context.Context, workSession *types.SpecTaskWorkSession) error {
	args := m.Called(ctx, workSession)
	return args.Error(0)
}

func (m *MockStoreForWolf) CreateSpecTaskZedThread(ctx context.Context, zedThread *types.SpecTaskZedThread) error {
	args := m.Called(ctx, zedThread)
	return args.Error(0)
}

func (m *MockStoreForWolf) CreateSpecTaskImplementationTask(ctx context.Context, implTask *types.SpecTaskImplementationTask) error {
	args := m.Called(ctx, implTask)
	return args.Error(0)
}

func (m *MockStoreForWolf) CreateSpecTaskExternalAgent(ctx context.Context, agent *types.SpecTaskExternalAgent) error {
	args := m.Called(ctx, agent)
	return args.Error(0)
}

// Additional methods added to satisfy store.Store interface evolution
func (m *MockStoreForWolf) CreateKnowledgeEmbedding(ctx context.Context, embeddings ...*types.KnowledgeEmbeddingItem) error {
	return nil
}
func (m *MockStoreForWolf) DecrementWolfSandboxCount(ctx context.Context, id string) error {
	return nil
}
func (m *MockStoreForWolf) DeregisterWolfInstance(ctx context.Context, id string) error {
	return nil
}
func (m *MockStoreForWolf) DetachRepositoryFromProject(ctx context.Context, repoID string) error {
	return nil
}
func (m *MockStoreForWolf) GetCommentByInteractionID(ctx context.Context, interactionID string) (*types.SpecTaskDesignReviewComment, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetWolfInstance(ctx context.Context, id string) (*types.WolfInstance, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetWolfInstancesOlderThanHeartbeat(ctx context.Context, olderThan time.Time) ([]*types.WolfInstance, error) {
	return nil, nil
}
func (m *MockStoreForWolf) IncrementWolfSandboxCount(ctx context.Context, id string) error {
	return nil
}
func (m *MockStoreForWolf) ListWolfInstances(ctx context.Context) ([]*types.WolfInstance, error) {
	return nil, nil
}
func (m *MockStoreForWolf) RegisterWolfInstance(ctx context.Context, instance *types.WolfInstance) error {
	return nil
}
func (m *MockStoreForWolf) UpdateWolfHeartbeat(ctx context.Context, id string, swayVersion string) error {
	return nil
}
func (m *MockStoreForWolf) UpdateWolfStatus(ctx context.Context, id string, status string) error {
	return nil
}
func (m *MockStoreForWolf) ResetWolfInstanceOnReconnect(ctx context.Context, id string) error {
	return nil
}
func (m *MockStoreForWolf) GetExternalAgentActivityByLobbyID(ctx context.Context, lobbyID string) (*types.ExternalAgentActivity, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetGitRepository(ctx context.Context, id string) (*types.GitRepository, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetLatestDesignReview(ctx context.Context, specTaskID string) (*types.SpecTaskDesignReview, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetProject(ctx context.Context, id string) (*types.Project, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetProjectExploratorySession(ctx context.Context, projectID string) (*types.Session, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetProjectRepositories(ctx context.Context, projectID string) ([]*types.GitRepository, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetSampleProject(ctx context.Context, id string) (*types.SampleProject, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetSessionIncludingDeleted(ctx context.Context, id string) (*types.Session, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Session), args.Error(1)
}
func (m *MockStoreForWolf) GetSpecTaskDesignReview(ctx context.Context, id string) (*types.SpecTaskDesignReview, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetSpecTaskDesignReviewComment(ctx context.Context, id string) (*types.SpecTaskDesignReviewComment, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetSpecTaskDesignReviewCommentReply(ctx context.Context, id string) (*types.SpecTaskDesignReviewCommentReply, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetSpecTaskGitPushEvent(ctx context.Context, id string) (*types.SpecTaskGitPushEvent, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetSpecTaskGitPushEventByCommit(ctx context.Context, specTaskID, commitHash string) (*types.SpecTaskGitPushEvent, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetUnresolvedCommentsForTask(ctx context.Context, specTaskID string) ([]types.SpecTaskDesignReviewComment, error) {
	return nil, nil
}
func (m *MockStoreForWolf) Includes(m2 map[string]bool, s string) bool { return false }
func (m *MockStoreForWolf) ListGitRepositories(ctx context.Context, request *types.ListGitRepositoriesRequest) ([]*types.GitRepository, error) {
	return nil, nil
}
func (m *MockStoreForWolf) ListProjects(ctx context.Context, query *store.ListProjectsQuery) ([]*types.Project, error) {
	return nil, nil
}
func (m *MockStoreForWolf) ListSampleProjects(ctx context.Context) ([]*types.SampleProject, error) {
	return nil, nil
}
func (m *MockStoreForWolf) ListSessionsQuery(ctx context.Context, query *store.ListSessionsQuery) ([]*types.Session, error) {
	return nil, nil
}
func (m *MockStoreForWolf) ListSpecTaskDesignReviewCommentReplies(ctx context.Context, commentID string) ([]types.SpecTaskDesignReviewCommentReply, error) {
	return nil, nil
}
func (m *MockStoreForWolf) ListSpecTaskDesignReviewComments(ctx context.Context, reviewID string) ([]types.SpecTaskDesignReviewComment, error) {
	return nil, nil
}
func (m *MockStoreForWolf) ListSpecTaskDesignReviews(ctx context.Context, specTaskID string) ([]types.SpecTaskDesignReview, error) {
	return nil, nil
}
func (m *MockStoreForWolf) ListSpecTaskGitPushEvents(ctx context.Context, specTaskID string) ([]types.SpecTaskGitPushEvent, error) {
	return nil, nil
}
func (m *MockStoreForWolf) ListUnprocessedGitPushEvents(ctx context.Context) ([]types.SpecTaskGitPushEvent, error) {
	return nil, nil
}
func (m *MockStoreForWolf) ListUnresolvedComments(ctx context.Context, reviewID string) ([]types.SpecTaskDesignReviewComment, error) {
	return nil, nil
}
func (m *MockStoreForWolf) QueryKnowledgeEmbeddings(ctx context.Context, q *types.KnowledgeEmbeddingQuery) ([]*types.KnowledgeEmbeddingItem, error) {
	return nil, nil
}
func (m *MockStoreForWolf) SetProjectPrimaryRepository(ctx context.Context, projectID string, repoID string) error {
	return nil
}
func (m *MockStoreForWolf) UpdateGitRepository(ctx context.Context, repo *types.GitRepository) error {
	return nil
}
func (m *MockStoreForWolf) UpdateProject(ctx context.Context, project *types.Project) error {
	return nil
}
func (m *MockStoreForWolf) UpdateSpecTaskDesignReview(ctx context.Context, review *types.SpecTaskDesignReview) error {
	return nil
}
func (m *MockStoreForWolf) UpdateSpecTaskDesignReviewComment(ctx context.Context, comment *types.SpecTaskDesignReviewComment) error {
	return nil
}
func (m *MockStoreForWolf) UpdateSpecTaskGitPushEvent(ctx context.Context, event *types.SpecTaskGitPushEvent) error {
	return nil
}

// Stub methods to satisfy store.Store interface (auto-generated from store_mocks.go)
// These are not used in the cleanup tests, but needed to satisfy the interface

// MockWolfClient mocks the Wolf client and implements WolfClientInterface
type MockWolfClient struct {
	mock.Mock
}

func (m *MockWolfClient) AddApp(ctx context.Context, app *wolf.App) error {
	args := m.Called(ctx, app)
	return args.Error(0)
}

func (m *MockWolfClient) RemoveApp(ctx context.Context, appID string) error {
	args := m.Called(ctx, appID)
	return args.Error(0)
}

func (m *MockWolfClient) CreateSession(ctx context.Context, session *wolf.Session) (string, error) {
	args := m.Called(ctx, session)
	return args.String(0), args.Error(1)
}

func (m *MockWolfClient) StopSession(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *MockWolfClient) ListApps(ctx context.Context) ([]wolf.App, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]wolf.App), args.Error(1)
}

func (m *MockWolfClient) CreateLobby(ctx context.Context, req *wolf.CreateLobbyRequest) (*wolf.LobbyCreateResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*wolf.LobbyCreateResponse), args.Error(1)
}

func (m *MockWolfClient) StopLobby(ctx context.Context, req *wolf.StopLobbyRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockWolfClient) ListLobbies(ctx context.Context) ([]wolf.Lobby, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]wolf.Lobby), args.Error(1)
}

func (m *MockWolfClient) GetSystemMemory(ctx context.Context) (*wolf.SystemMemoryResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*wolf.SystemMemoryResponse), args.Error(1)
}

func (m *MockWolfClient) JoinLobby(ctx context.Context, req *wolf.JoinLobbyRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockWolfClient) ListSessions(ctx context.Context) ([]wolf.WolfStreamSession, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]wolf.WolfStreamSession), args.Error(1)
}

func (m *MockWolfClient) GetSystemHealth(ctx context.Context) (*wolf.SystemHealthResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*wolf.SystemHealthResponse), args.Error(1)
}

func TestCleanupIdleExternalAgents_NoIdleAgents(t *testing.T) {
	// TODO: This test needs to be rewritten to mock the multi-Wolf architecture
	// with connman and session-based Wolf instance lookup.
	// See wolf_executor.go changes for context - wolfClient field was replaced
	// with connman-based Wolf client lookup per session.
	t.Skip("Test needs rewrite for multi-Wolf architecture (connman + session-based Wolf lookup)")

	ctx := context.Background()
	mockStore := new(MockStoreForWolf)

	executor := &WolfExecutor{
		store:                       mockStore,
		workspaceBasePathForCloning: "/tmp/test-workspaces",
	}

	// Mock: No idle agents
	mockStore.On("GetIdleExternalAgents", ctx, mock.Anything, []string{"spectask", "exploratory", "agent"}).
		Return([]*types.ExternalAgentActivity{}, nil)

	// Execute
	executor.cleanupIdleExternalAgents(ctx)

	// Verify: No cleanup actions taken - just check store expectations
	mockStore.AssertExpectations(t)
}

func TestCleanupIdleExternalAgents_TerminatesIdleAgent(t *testing.T) {
	// TODO: This test needs to be rewritten to mock the multi-Wolf architecture
	// with connman and session-based Wolf instance lookup.
	t.Skip("Test needs rewrite for multi-Wolf architecture (connman + session-based Wolf lookup)")

	ctx := context.Background()
	mockStore := new(MockStoreForWolf)

	executor := &WolfExecutor{
		store:                       mockStore,
		workspaceBasePathForCloning: "/tmp/test-workspaces",
	}

	// Idle agent activity
	idleActivity := &types.ExternalAgentActivity{
		ExternalAgentID: "zed-spectask-idle123",
		SpecTaskID:      "spec_idle123",
		LastInteraction: time.Now().Add(-35 * time.Minute),
		AgentType:       "spectask",
		WolfAppID:       "wolf_idle_456",
		WorkspaceDir:    "/workspaces/spectasks/spec_idle123/work",
		UserID:          "user_idle",
	}

	idleAgent := &types.SpecTaskExternalAgent{
		ID:              "zed-spectask-idle123",
		SpecTaskID:      "spec_idle123",
		WolfAppID:       "wolf_idle_456",
		HelixSessionIDs: []string{"ses_001", "ses_002"},
		Status:          "running",
	}

	session1 := &types.Session{
		ID:    "ses_001",
		Owner: "user_idle",
		Metadata: types.SessionMetadata{
			SpecTaskID:   "spec_idle123",
			WolfLobbyID:  "lobby_001",
			WolfLobbyPIN: "1234",
		},
	}

	session2 := &types.Session{
		ID:    "ses_002",
		Owner: "user_idle",
		Metadata: types.SessionMetadata{
			SpecTaskID:   "spec_idle123",
			WolfLobbyID:  "lobby_002",
			WolfLobbyPIN: "5678",
		},
	}

	// Setup mocks
	mockStore.On("GetIdleExternalAgents", ctx, mock.Anything, []string{"spectask", "exploratory", "agent"}).
		Return([]*types.ExternalAgentActivity{idleActivity}, nil)

	mockStore.On("GetSpecTaskExternalAgentByID", ctx, "zed-spectask-idle123").
		Return(idleAgent, nil)

	mockStore.On("UpdateSpecTaskExternalAgent", ctx, mock.MatchedBy(func(agent *types.SpecTaskExternalAgent) bool {
		return agent.Status == "terminated"
	})).Return(nil)

	mockStore.On("GetSession", ctx, "ses_001").Return(session1, nil)
	mockStore.On("GetSession", ctx, "ses_002").Return(session2, nil)
	mockStore.On("GetSessionIncludingDeleted", mock.Anything, "ses_001").Return(session1, nil)
	mockStore.On("GetSessionIncludingDeleted", mock.Anything, "ses_002").Return(session2, nil)

	mockStore.On("UpdateSession", ctx, mock.MatchedBy(func(session types.Session) bool {
		return session.Metadata.ExternalAgentStatus == "terminated_idle"
	})).Return(&types.Session{}, nil).Times(2)

	mockStore.On("DeleteExternalAgentActivity", ctx, "zed-spectask-idle123").Return(nil)

	// Execute
	executor.cleanupIdleExternalAgents(ctx)

	// Verify: Agent status updated to terminated
	mockStore.AssertCalled(t, "UpdateSpecTaskExternalAgent", ctx, mock.Anything)

	// Verify: All sessions updated
	mockStore.AssertNumberOfCalls(t, "UpdateSession", 2)

	// Verify: Activity record deleted
	mockStore.AssertCalled(t, "DeleteExternalAgentActivity", ctx, "zed-spectask-idle123")
}

func TestCleanupIdleExternalAgents_WolfRemovalFails(t *testing.T) {
	// TODO: This test needs to be rewritten to mock the multi-Wolf architecture
	// with connman and session-based Wolf instance lookup.
	t.Skip("Test needs rewrite for multi-Wolf architecture (connman + session-based Wolf lookup)")

	ctx := context.Background()
	mockStore := new(MockStoreForWolf)

	executor := &WolfExecutor{
		store:                       mockStore,
		workspaceBasePathForCloning: "/tmp/test-workspaces",
	}

	idleActivity := &types.ExternalAgentActivity{
		ExternalAgentID: "zed-spectask-wolf-fail",
		SpecTaskID:      "spec_wolf_fail",
		LastInteraction: time.Now().Add(-40 * time.Minute),
		AgentType:       "spectask",
		WolfAppID:       "wolf_fail_789",
		WorkspaceDir:    "/workspaces/spectasks/spec_wolf_fail/work",
		UserID:          "user_fail",
	}

	idleAgent := &types.SpecTaskExternalAgent{
		ID:              "zed-spectask-wolf-fail",
		SpecTaskID:      "spec_wolf_fail",
		WolfAppID:       "wolf_fail_789",
		HelixSessionIDs: []string{"ses_fail_001"},
		Status:          "running",
	}

	session := &types.Session{
		ID:    "ses_fail_001",
		Owner: "user_fail",
		Metadata: types.SessionMetadata{
			SpecTaskID:   "spec_wolf_fail",
			WolfLobbyID:  "lobby_fail_001",
			WolfLobbyPIN: "9999",
		},
	}

	// Setup mocks
	mockStore.On("GetIdleExternalAgents", ctx, mock.Anything, []string{"spectask", "exploratory", "agent"}).
		Return([]*types.ExternalAgentActivity{idleActivity}, nil)

	// Cleanup should continue even without Wolf client
	mockStore.On("GetSpecTaskExternalAgentByID", ctx, "zed-spectask-wolf-fail").
		Return(idleAgent, nil)

	mockStore.On("UpdateSpecTaskExternalAgent", ctx, mock.Anything).Return(nil)
	mockStore.On("GetSession", ctx, "ses_fail_001").Return(session, nil)
	mockStore.On("GetSessionIncludingDeleted", mock.Anything, "ses_fail_001").Return(session, nil)
	mockStore.On("UpdateSession", ctx, mock.Anything).Return(&types.Session{}, nil)
	mockStore.On("DeleteExternalAgentActivity", ctx, "zed-spectask-wolf-fail").Return(nil)

	// Execute
	executor.cleanupIdleExternalAgents(ctx)

	// Verify: Continues with cleanup even though Wolf removal failed
	mockStore.AssertCalled(t, "UpdateSpecTaskExternalAgent", ctx, mock.Anything)
	mockStore.AssertCalled(t, "DeleteExternalAgentActivity", ctx, "zed-spectask-wolf-fail")
}

func TestCleanupIdleExternalAgents_MultipleAgents(t *testing.T) {
	// TODO: This test needs to be rewritten to mock the multi-Wolf architecture
	// with connman and session-based Wolf instance lookup.
	t.Skip("Test needs rewrite for multi-Wolf architecture (connman + session-based Wolf lookup)")

	ctx := context.Background()
	mockStore := new(MockStoreForWolf)

	executor := &WolfExecutor{
		store:                       mockStore,
		workspaceBasePathForCloning: "/tmp/test-workspaces",
	}

	// Multiple idle agents
	idleActivities := []*types.ExternalAgentActivity{
		{
			ExternalAgentID: "zed-spectask-idle1",
			SpecTaskID:      "spec_idle1",
			LastInteraction: time.Now().Add(-31 * time.Minute),
			AgentType:       "spectask",
			WolfAppID:       "wolf_1",
			WorkspaceDir:    "/workspaces/spectasks/spec_idle1/work",
			UserID:          "user1",
		},
		{
			ExternalAgentID: "zed-spectask-idle2",
			SpecTaskID:      "spec_idle2",
			LastInteraction: time.Now().Add(-45 * time.Minute),
			AgentType:       "spectask",
			WolfAppID:       "wolf_2",
			WorkspaceDir:    "/workspaces/spectasks/spec_idle2/work",
			UserID:          "user2",
		},
	}

	agent1 := &types.SpecTaskExternalAgent{
		ID:              "zed-spectask-idle1",
		WolfAppID:       "wolf_1",
		HelixSessionIDs: []string{},
		Status:          "running",
	}

	agent2 := &types.SpecTaskExternalAgent{
		ID:              "zed-spectask-idle2",
		WolfAppID:       "wolf_2",
		HelixSessionIDs: []string{},
		Status:          "running",
	}

	// Setup mocks
	mockStore.On("GetIdleExternalAgents", ctx, mock.Anything, []string{"spectask", "exploratory", "agent"}).
		Return(idleActivities, nil)

	mockStore.On("GetSpecTaskExternalAgentByID", ctx, "zed-spectask-idle1").Return(agent1, nil)
	mockStore.On("GetSpecTaskExternalAgentByID", ctx, "zed-spectask-idle2").Return(agent2, nil)

	mockStore.On("UpdateSpecTaskExternalAgent", ctx, mock.Anything).Return(nil).Times(2)
	mockStore.On("DeleteExternalAgentActivity", ctx, "zed-spectask-idle1").Return(nil)
	mockStore.On("DeleteExternalAgentActivity", ctx, "zed-spectask-idle2").Return(nil)

	// Execute
	executor.cleanupIdleExternalAgents(ctx)

	// Verify: Both agents terminated
	mockStore.AssertNumberOfCalls(t, "DeleteExternalAgentActivity", 2)
	mockStore.AssertNumberOfCalls(t, "UpdateSpecTaskExternalAgent", 2)
}

// Auto-generated stub methods below (not actively used in tests, just to satisfy interface)

func (m *MockStoreForWolf) CleanupExpiredSessions(ctx context.Context, timeout time.Duration) error {
	return nil
}

func (m *MockStoreForWolf) CleanupStaleAgentRunners(ctx context.Context, staleThreshold time.Duration) (int64, error) {
	return 0, nil
}

func (m *MockStoreForWolf) CountUsers(ctx context.Context) (int64, error) { return 0, nil }

func (m *MockStoreForWolf) CreateAPIKey(ctx context.Context, apiKey *types.ApiKey) (*types.ApiKey, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateAccessGrant(ctx context.Context, resourceAccess *types.AccessGrant, roles []*types.Role) (*types.AccessGrant, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateAccessGrantRoleBinding(ctx context.Context, binding *types.AccessGrantRoleBinding) (*types.AccessGrantRoleBinding, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateAgentRunner(ctx context.Context, runnerID string) (*types.AgentRunner, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateAgentSession(ctx context.Context, session *types.AgentSession) error {
	return nil
}

func (m *MockStoreForWolf) CreateAgentSessionStatus(ctx context.Context, status *types.AgentSessionStatus) error {
	return nil
}

func (m *MockStoreForWolf) CreateAgentWorkItem(ctx context.Context, workItem *types.AgentWorkItem) error {
	return nil
}

func (m *MockStoreForWolf) CreateApp(ctx context.Context, tool *types.App) (*types.App, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateCrispThread(ctx context.Context, thread *types.CrispThread) (*types.CrispThread, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateDataEntity(ctx context.Context, dataEntity *types.DataEntity) (*types.DataEntity, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateDynamicModelInfo(ctx context.Context, modelInfo *types.DynamicModelInfo) (*types.DynamicModelInfo, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateHelpRequest(ctx context.Context, request *types.HelpRequest) error {
	return nil
}

func (m *MockStoreForWolf) CreateImplementationSessions(ctx context.Context, specTaskID string, config *types.SpecTaskImplementationSessionsCreateRequest) ([]*types.SpecTaskWorkSession, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateInteraction(ctx context.Context, interaction *types.Interaction) (*types.Interaction, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateInteractions(ctx context.Context, interactions ...*types.Interaction) error {
	return nil
}

func (m *MockStoreForWolf) CreateJobCompletion(ctx context.Context, completion *types.JobCompletion) error {
	return nil
}

func (m *MockStoreForWolf) CreateKnowledge(ctx context.Context, knowledge *types.Knowledge) (*types.Knowledge, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateKnowledgeVersion(ctx context.Context, version *types.KnowledgeVersion) (*types.KnowledgeVersion, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateLLMCall(ctx context.Context, call *types.LLMCall) (*types.LLMCall, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateMemory(ctx context.Context, memory *types.Memory) (*types.Memory, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateModel(ctx context.Context, model *types.Model) (*types.Model, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateOAuthConnection(ctx context.Context, connection *types.OAuthConnection) (*types.OAuthConnection, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateOAuthProvider(ctx context.Context, provider *types.OAuthProvider) (*types.OAuthProvider, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateOAuthRequestToken(ctx context.Context, token *types.OAuthRequestToken) (*types.OAuthRequestToken, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateOrganization(ctx context.Context, org *types.Organization) (*types.Organization, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateOrganizationMembership(ctx context.Context, membership *types.OrganizationMembership) (*types.OrganizationMembership, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreatePersonalDevEnvironment(ctx context.Context, pde *types.PersonalDevEnvironment) (*types.PersonalDevEnvironment, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateProject(ctx context.Context, project *types.Project) (*types.Project, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateProviderEndpoint(ctx context.Context, providerEndpoint *types.ProviderEndpoint) (*types.ProviderEndpoint, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateRole(ctx context.Context, role *types.Role) (*types.Role, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateSSHKey(ctx context.Context, key *types.SSHKey) (*types.SSHKey, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateSecret(ctx context.Context, secret *types.Secret) (*types.Secret, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateSession(ctx context.Context, session types.Session) (*types.Session, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateSlackThread(ctx context.Context, thread *types.SlackThread) (*types.SlackThread, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateSlot(ctx context.Context, slot *types.RunnerSlot) (*types.RunnerSlot, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateStepInfo(ctx context.Context, stepInfo *types.StepInfo) (*types.StepInfo, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateTeam(ctx context.Context, team *types.Team) (*types.Team, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateTeamMembership(ctx context.Context, membership *types.TeamMembership) (*types.TeamMembership, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateTriggerConfiguration(ctx context.Context, triggerConfig *types.TriggerConfiguration) (*types.TriggerConfiguration, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateTriggerExecution(ctx context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateUsageMetric(ctx context.Context, metric *types.UsageMetric) (*types.UsageMetric, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateUser(ctx context.Context, user *types.User) (*types.User, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateWallet(ctx context.Context, wallet *types.Wallet) (*types.Wallet, error) {
	return nil, nil
}

func (m *MockStoreForWolf) DeleteAPIKey(ctx context.Context, apiKey string) error { return nil }

func (m *MockStoreForWolf) DeleteAccessGrant(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteAccessGrantRoleBinding(ctx context.Context, accessGrantID, roleID string) error {
	return nil
}

func (m *MockStoreForWolf) DeleteAgentRunner(ctx context.Context, runnerID string) error { return nil }

func (m *MockStoreForWolf) DeleteApp(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteCrispThread(ctx context.Context, olderThan time.Time) error {
	return nil
}

func (m *MockStoreForWolf) DeleteDataEntity(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteDynamicModelInfo(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteInteraction(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteKnowledge(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteKnowledgeVersion(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteKnowledgeEmbedding(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteMemory(ctx context.Context, memory *types.Memory) error { return nil }

func (m *MockStoreForWolf) DeleteModel(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteOAuthConnection(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteOAuthProvider(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteOAuthRequestToken(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteOrganization(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteOrganizationMembership(ctx context.Context, organizationID, userID string) error {
	return nil
}

func (m *MockStoreForWolf) DeletePersonalDevEnvironment(ctx context.Context, id string) error {
	return nil
}

func (m *MockStoreForWolf) DeleteProviderEndpoint(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteProject(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteSampleProject(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteRole(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteSSHKey(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteSecret(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteSession(ctx context.Context, id string) (*types.Session, error) {
	return nil, nil
}

func (m *MockStoreForWolf) DeleteSlackThread(ctx context.Context, olderThan time.Time) error {
	return nil
}

func (m *MockStoreForWolf) DeleteSlot(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteSpecTaskDesignReview(ctx context.Context, id string) error {
	return nil
}

func (m *MockStoreForWolf) DeleteSpecTaskDesignReviewComment(ctx context.Context, id string) error {
	return nil
}

func (m *MockStoreForWolf) DeleteSpecTaskExternalAgent(ctx context.Context, agentID string) error {
	return nil
}

func (m *MockStoreForWolf) DeleteSpecTaskImplementationTask(ctx context.Context, id string) error {
	return nil
}

func (m *MockStoreForWolf) DeleteSpecTaskWorkSession(ctx context.Context, id string) error {
	return nil
}

func (m *MockStoreForWolf) DeleteSpecTaskZedThread(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteStepInfo(ctx context.Context, sessionID string) error { return nil }

func (m *MockStoreForWolf) DeleteTeam(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteTeamMembership(ctx context.Context, teamID, userID string) error {
	return nil
}

func (m *MockStoreForWolf) DeleteTriggerConfiguration(ctx context.Context, id string) error {
	return nil
}

func (m *MockStoreForWolf) DeleteUsageMetrics(ctx context.Context, appID string) error { return nil }

func (m *MockStoreForWolf) DeleteUser(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteWallet(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) DeleteZedSettingsOverride(ctx context.Context, sessionID string) error {
	return nil
}

func (m *MockStoreForWolf) EnsureUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GenerateRandomState(ctx context.Context) (string, error) { return "", nil }

func (m *MockStoreForWolf) GetAPIKey(ctx context.Context, apiKey string) (*types.ApiKey, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetAccessGrantRoleBindings(ctx context.Context, q *store.GetAccessGrantRoleBindingsQuery) ([]*types.AccessGrantRoleBinding, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetAgentRunner(ctx context.Context, runnerID string) (*types.AgentRunner, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetAgentSession(ctx context.Context, sessionID string) (*types.AgentSession, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetAgentSessionStatus(ctx context.Context, sessionID string) (*types.AgentSessionStatus, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetAgentWorkItem(ctx context.Context, workItemID string) (*types.AgentWorkItem, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetAgentWorkQueueStats(ctx context.Context) (*types.AgentWorkQueueStats, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetAggregatedUsageMetrics(ctx context.Context, q *store.GetAggregatedUsageMetricsQuery) ([]*types.AggregatedUsageMetric, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetApp(ctx context.Context, id string) (*types.App, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetAppDailyUsageMetrics(ctx context.Context, appID string, from, to time.Time) ([]*types.AggregatedUsageMetric, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetAppUsageMetrics(ctx context.Context, appID string, from, to time.Time) ([]*types.UsageMetric, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetAppUsersAggregatedUsageMetrics(ctx context.Context, appID string, from, to time.Time) ([]*types.UsersAggregatedUsageMetric, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetAppWithTools(ctx context.Context, id string) (*types.App, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetCrispThread(ctx context.Context, appID, crispSessionID string) (*types.CrispThread, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetDataEntity(ctx context.Context, id string) (*types.DataEntity, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetDecodedLicense(ctx context.Context) (*license.License, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetDynamicModelInfo(ctx context.Context, id string) (*types.DynamicModelInfo, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetEffectiveSystemSettings(ctx context.Context) (*types.SystemSettings, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetExternalAgentActivity(ctx context.Context, agentID string) (*types.ExternalAgentActivity, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetHelpRequestByID(ctx context.Context, requestID string) (*types.HelpRequest, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetInteraction(ctx context.Context, id string) (*types.Interaction, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetKnowledge(ctx context.Context, id string) (*types.Knowledge, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetKnowledgeVersion(ctx context.Context, id string) (*types.KnowledgeVersion, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetLicenseKey(ctx context.Context) (*types.LicenseKey, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetModel(ctx context.Context, id string) (*types.Model, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetOAuthConnection(ctx context.Context, id string) (*types.OAuthConnection, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetOAuthConnectionByUserAndProvider(ctx context.Context, userID, providerID string) (*types.OAuthConnection, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetOAuthConnectionsNearExpiry(ctx context.Context, expiresBefore time.Time) ([]*types.OAuthConnection, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetOAuthProvider(ctx context.Context, id string) (*types.OAuthProvider, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetOAuthRequestToken(ctx context.Context, userID, providerID string) ([]*types.OAuthRequestToken, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetOAuthRequestTokenByState(ctx context.Context, state string) ([]*types.OAuthRequestToken, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetOrCreateAgentRunner(ctx context.Context, runnerID string) (*types.AgentRunner, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetOrganization(ctx context.Context, q *store.GetOrganizationQuery) (*types.Organization, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetOrganizationMembership(ctx context.Context, q *store.GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetPendingReviews(ctx context.Context) ([]*types.JobCompletion, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetPersonalDevEnvironment(ctx context.Context, id string) (*types.PersonalDevEnvironment, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetPersonalDevEnvironmentByWolfAppID(ctx context.Context, wolfAppID string) (*types.PersonalDevEnvironment, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetProviderDailyUsageMetrics(ctx context.Context, providerID string, from, to time.Time) ([]*types.AggregatedUsageMetric, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetProviderEndpoint(ctx context.Context, q *store.GetProviderEndpointsQuery) (*types.ProviderEndpoint, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetRecentCompletions(ctx context.Context, limit int) ([]*types.JobCompletion, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetRole(ctx context.Context, id string) (*types.Role, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSSHKey(ctx context.Context, id string) (*types.SSHKey, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSecret(ctx context.Context, id string) (*types.Secret, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSessionsNeedingHelp(ctx context.Context) ([]*types.AgentSession, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSlackThread(ctx context.Context, appID, channel, threadKey string) (*types.SlackThread, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSlot(ctx context.Context, id string) (*types.RunnerSlot, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSpecTask(ctx context.Context, id string) (*types.SpecTask, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSpecTaskExternalAgent(ctx context.Context, specTaskID string) (*types.SpecTaskExternalAgent, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSpecTaskImplementationTask(ctx context.Context, id string) (*types.SpecTaskImplementationTask, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSpecTaskMultiSessionOverview(ctx context.Context, specTaskID string) (*types.SpecTaskMultiSessionOverviewResponse, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSpecTaskProgress(ctx context.Context, specTaskID string) (*types.SpecTaskProgressResponse, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSpecTaskWorkSession(ctx context.Context, id string) (*types.SpecTaskWorkSession, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSpecTaskWorkSessionByHelixSession(ctx context.Context, helixSessionID string) (*types.SpecTaskWorkSession, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSpecTaskZedThread(ctx context.Context, id string) (*types.SpecTaskZedThread, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSpecTaskZedThreadByWorkSession(ctx context.Context, workSessionID string) (*types.SpecTaskZedThread, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetSystemSettings(ctx context.Context) (*types.SystemSettings, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetTeam(ctx context.Context, q *store.GetTeamQuery) (*types.Team, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetTeamMembership(ctx context.Context, q *store.GetTeamMembershipQuery) (*types.TeamMembership, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetTriggerConfiguration(ctx context.Context, q *store.GetTriggerConfigurationQuery) (*types.TriggerConfiguration, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetUser(ctx context.Context, q *store.GetUserQuery) (*types.User, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetUserMeta(ctx context.Context, id string) (*types.UserMeta, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetUserMonthlyTokenUsage(ctx context.Context, userID string, providers []string) (int, error) {
	return 0, nil
}

func (m *MockStoreForWolf) GetUsersAggregatedUsageMetrics(ctx context.Context, provider string, from, to time.Time) ([]*types.UsersAggregatedUsageMetric, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetWallet(ctx context.Context, id string) (*types.Wallet, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetWalletByOrg(ctx context.Context, orgID string) (*types.Wallet, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetWalletByStripeCustomerID(ctx context.Context, stripeCustomerID string) (*types.Wallet, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetWalletByUser(ctx context.Context, userID string) (*types.Wallet, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetZedSettingsOverride(ctx context.Context, sessionID string) (*types.ZedSettingsOverride, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListAPIKeys(ctx context.Context, query *store.ListAPIKeysQuery) ([]*types.ApiKey, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListAccessGrants(ctx context.Context, q *store.ListAccessGrantsQuery) ([]*types.AccessGrant, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListActiveHelpRequests(ctx context.Context) ([]*types.HelpRequest, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListAgentRunners(ctx context.Context, query types.ListAgentRunnersQuery) ([]*types.AgentRunner, int64, error) {
	return nil, 0, nil
}

func (m *MockStoreForWolf) ListAgentSessionStatus(ctx context.Context, query *store.ListAgentSessionsQuery) (*types.AgentSessionsResponse, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListAgentSessions(ctx context.Context, query *store.ListAgentSessionsQuery) (*types.AgentSessionsListResponse, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListAgentWorkItems(ctx context.Context, query *store.ListAgentWorkItemsQuery) (*types.AgentWorkItemsListResponse, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListAllSlots(ctx context.Context) ([]*types.RunnerSlot, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListApps(ctx context.Context, q *store.ListAppsQuery) ([]*types.App, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListDataEntities(ctx context.Context, q *store.ListDataEntitiesQuery) ([]*types.DataEntity, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListDynamicModelInfos(ctx context.Context, q *types.ListDynamicModelInfosQuery) ([]*types.DynamicModelInfo, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListHelpRequests(ctx context.Context, query *store.ListHelpRequestsQuery) (*types.HelpRequestsListResponse, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListInteractions(ctx context.Context, query *types.ListInteractionsQuery) ([]*types.Interaction, int64, error) {
	return nil, 0, nil
}

func (m *MockStoreForWolf) ListKnowledge(ctx context.Context, q *store.ListKnowledgeQuery) ([]*types.Knowledge, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListKnowledgeVersions(ctx context.Context, q *store.ListKnowledgeVersionQuery) ([]*types.KnowledgeVersion, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListLLMCalls(ctx context.Context, q *store.ListLLMCallsQuery) ([]*types.LLMCall, int64, error) {
	return nil, 0, nil
}

func (m *MockStoreForWolf) ListMemories(ctx context.Context, q *types.ListMemoryRequest) ([]*types.Memory, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListModels(ctx context.Context, q *store.ListModelsQuery) ([]*types.Model, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListOAuthConnections(ctx context.Context, query *store.ListOAuthConnectionsQuery) ([]*types.OAuthConnection, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListOAuthProviders(ctx context.Context, query *store.ListOAuthProvidersQuery) ([]*types.OAuthProvider, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListOrganizationMemberships(ctx context.Context, query *store.ListOrganizationMembershipsQuery) ([]*types.OrganizationMembership, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListOrganizations(ctx context.Context, query *store.ListOrganizationsQuery) ([]*types.Organization, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListPersonalDevEnvironments(ctx context.Context, userID string) ([]*types.PersonalDevEnvironment, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListProviderEndpoints(ctx context.Context, q *store.ListProviderEndpointsQuery) ([]*types.ProviderEndpoint, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListRoles(ctx context.Context, organizationID string) ([]*types.Role, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListSSHKeys(ctx context.Context, userID string) ([]*types.SSHKey, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListSecrets(ctx context.Context, q *store.ListSecretsQuery) ([]*types.Secret, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListSessions(ctx context.Context, query store.ListSessionsQuery) ([]*types.Session, int64, error) {
	return nil, 0, nil
}

func (m *MockStoreForWolf) ListSlots(ctx context.Context, runnerID string) ([]*types.RunnerSlot, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListSpecTaskExternalAgents(ctx context.Context, userID string) ([]*types.SpecTaskExternalAgent, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListSpecTaskImplementationTasks(ctx context.Context, specTaskID string) ([]*types.SpecTaskImplementationTask, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListSpecTaskWorkSessions(ctx context.Context, specTaskID string) ([]*types.SpecTaskWorkSession, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListSpecTaskZedThreads(ctx context.Context, specTaskID string) ([]*types.SpecTaskZedThread, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListSpecTasks(ctx context.Context, filters *types.SpecTaskFilters) ([]*types.SpecTask, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListStepInfos(ctx context.Context, query *store.ListStepInfosQuery) ([]*types.StepInfo, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListTeamMemberships(ctx context.Context, query *store.ListTeamMembershipsQuery) ([]*types.TeamMembership, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListTeams(ctx context.Context, query *store.ListTeamsQuery) ([]*types.Team, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListTopUps(ctx context.Context, q *store.ListTopUpsQuery) ([]*types.TopUp, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListTransactions(ctx context.Context, q *store.ListTransactionsQuery) ([]*types.Transaction, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListTriggerConfigurations(ctx context.Context, q *store.ListTriggerConfigurationsQuery) ([]*types.TriggerConfiguration, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListTriggerExecutions(ctx context.Context, q *store.ListTriggerExecutionsQuery) ([]*types.TriggerExecution, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ListUsers(ctx context.Context, query *store.ListUsersQuery) ([]*types.User, int64, error) {
	return nil, 0, nil
}

func (m *MockStoreForWolf) ListWorkSessionsBySpecTask(ctx context.Context, specTaskID string, phase *types.SpecTaskPhase) ([]*types.SpecTaskWorkSession, error) {
	return nil, nil
}

func (m *MockStoreForWolf) LookupKnowledge(ctx context.Context, q *store.LookupKnowledgeQuery) (*types.Knowledge, error) {
	return nil, nil
}

func (m *MockStoreForWolf) MarkSessionAsActive(ctx context.Context, sessionID, task string) error {
	return nil
}

func (m *MockStoreForWolf) MarkSessionAsCompleted(ctx context.Context, sessionID, completionType string) error {
	return nil
}

func (m *MockStoreForWolf) MarkSessionAsNeedingHelp(ctx context.Context, sessionID, task string) error {
	return nil
}

func (m *MockStoreForWolf) ParseAndCreateImplementationTasks(ctx context.Context, specTaskID, implementationPlan string) ([]*types.SpecTaskImplementationTask, error) {
	return nil, nil
}

func (m *MockStoreForWolf) ResetRunningExecutions(ctx context.Context) error { return nil }

func (m *MockStoreForWolf) SearchUsers(ctx context.Context, query *store.SearchUsersQuery) ([]*types.User, int64, error) {
	return nil, 0, nil
}

func (m *MockStoreForWolf) SeedModelsFromEnvironment(ctx context.Context) error { return nil }

func (m *MockStoreForWolf) SetLicenseKey(ctx context.Context, licenseKey string) error { return nil }

func (m *MockStoreForWolf) SpawnWorkSession(ctx context.Context, parentSessionID string, config *types.SpecTaskWorkSessionSpawnRequest) (*types.SpecTaskWorkSession, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateAgentRunner(ctx context.Context, runner *types.AgentRunner) error {
	return nil
}

func (m *MockStoreForWolf) UpdateAgentRunnerHeartbeat(ctx context.Context, runnerID string) error {
	return nil
}

func (m *MockStoreForWolf) UpdateAgentRunnerStatus(ctx context.Context, runnerID, status string) error {
	return nil
}

func (m *MockStoreForWolf) UpdateAgentSession(ctx context.Context, session *types.AgentSession) error {
	return nil
}

func (m *MockStoreForWolf) UpdateAgentSessionStatus(ctx context.Context, status *types.AgentSessionStatus) error {
	return nil
}

func (m *MockStoreForWolf) UpdateAgentWorkItem(ctx context.Context, workItem *types.AgentWorkItem) error {
	return nil
}

func (m *MockStoreForWolf) UpdateApp(ctx context.Context, tool *types.App) (*types.App, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateDataEntity(ctx context.Context, dataEntity *types.DataEntity) (*types.DataEntity, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateDynamicModelInfo(ctx context.Context, modelInfo *types.DynamicModelInfo) (*types.DynamicModelInfo, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateHelpRequest(ctx context.Context, request *types.HelpRequest) error {
	return nil
}

func (m *MockStoreForWolf) UpdateInteraction(ctx context.Context, interaction *types.Interaction) (*types.Interaction, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateKnowledge(ctx context.Context, knowledge *types.Knowledge) (*types.Knowledge, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateKnowledgeState(ctx context.Context, id string, state types.KnowledgeState, message string) error {
	return nil
}

func (m *MockStoreForWolf) UpdateMemory(ctx context.Context, memory *types.Memory) (*types.Memory, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateModel(ctx context.Context, model *types.Model) (*types.Model, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateOAuthConnection(ctx context.Context, connection *types.OAuthConnection) (*types.OAuthConnection, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateOAuthProvider(ctx context.Context, provider *types.OAuthProvider) (*types.OAuthProvider, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateOrganization(ctx context.Context, org *types.Organization) (*types.Organization, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateOrganizationMembership(ctx context.Context, membership *types.OrganizationMembership) (*types.OrganizationMembership, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdatePersonalDevEnvironment(ctx context.Context, pde *types.PersonalDevEnvironment) (*types.PersonalDevEnvironment, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateProviderEndpoint(ctx context.Context, providerEndpoint *types.ProviderEndpoint) (*types.ProviderEndpoint, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateRole(ctx context.Context, role *types.Role) (*types.Role, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateSSHKeyLastUsed(ctx context.Context, id string) error { return nil }

func (m *MockStoreForWolf) UpdateSecret(ctx context.Context, secret *types.Secret) (*types.Secret, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateSessionMeta(ctx context.Context, data types.SessionMetaUpdate) (*types.Session, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateSessionName(ctx context.Context, sessionID, name string) error {
	return nil
}

func (m *MockStoreForWolf) UpdateSlot(ctx context.Context, slot *types.RunnerSlot) (*types.RunnerSlot, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateSpecTask(ctx context.Context, task *types.SpecTask) error {
	return nil
}

func (m *MockStoreForWolf) UpdateSpecTaskImplementationTask(ctx context.Context, implTask *types.SpecTaskImplementationTask) error {
	return nil
}

func (m *MockStoreForWolf) UpdateSpecTaskWorkSession(ctx context.Context, workSession *types.SpecTaskWorkSession) error {
	return nil
}

func (m *MockStoreForWolf) UpdateSpecTaskZedInstance(ctx context.Context, specTaskID, zedInstanceID string) error {
	return nil
}

func (m *MockStoreForWolf) UpdateSpecTaskZedThread(ctx context.Context, zedThread *types.SpecTaskZedThread) error {
	return nil
}

func (m *MockStoreForWolf) UpdateSystemSettings(ctx context.Context, req *types.SystemSettingsRequest) (*types.SystemSettings, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateTeam(ctx context.Context, team *types.Team) (*types.Team, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateTriggerConfiguration(ctx context.Context, triggerConfig *types.TriggerConfiguration) (*types.TriggerConfiguration, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateTriggerExecution(ctx context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateUser(ctx context.Context, user *types.User) (*types.User, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateWallet(ctx context.Context, wallet *types.Wallet) (*types.Wallet, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpdateWalletBalance(ctx context.Context, walletID string, amount float64, meta types.TransactionMetadata) (*types.Wallet, error) {
	return nil, nil
}

func (m *MockStoreForWolf) UpsertExternalAgentActivity(ctx context.Context, activity *types.ExternalAgentActivity) error {
	return nil
}

func (m *MockStoreForWolf) UpsertZedSettingsOverride(ctx context.Context, override *types.ZedSettingsOverride) error {
	return nil
}

func (m *MockStoreForWolf) CreateQuestionSet(ctx context.Context, questionSet *types.QuestionSet) (*types.QuestionSet, error) {
	return nil, nil
}

func (m *MockStoreForWolf) GetQuestionSet(ctx context.Context, id string) (*types.QuestionSet, error) {
	return nil, nil
}
func (m *MockStoreForWolf) UpdateQuestionSet(ctx context.Context, questionSet *types.QuestionSet) (*types.QuestionSet, error) {
	return nil, nil
}
func (m *MockStoreForWolf) DeleteQuestionSet(ctx context.Context, id string) error {
	return nil
}

func (m *MockStoreForWolf) ListQuestionSets(ctx context.Context, req *types.ListQuestionSetsRequest) ([]*types.QuestionSet, error) {
	return nil, nil
}

func (m *MockStoreForWolf) CreateQuestionSetExecution(ctx context.Context, execution *types.QuestionSetExecution) (*types.QuestionSetExecution, error) {
	return nil, nil
}
func (m *MockStoreForWolf) GetQuestionSetExecution(ctx context.Context, id string) (*types.QuestionSetExecution, error) {
	return nil, nil
}
func (m *MockStoreForWolf) UpdateQuestionSetExecution(ctx context.Context, execution *types.QuestionSetExecution) (*types.QuestionSetExecution, error) {
	return nil, nil
}
func (m *MockStoreForWolf) DeleteQuestionSetExecution(ctx context.Context, id string) error {
	return nil
}
func (m *MockStoreForWolf) ListQuestionSetExecutions(ctx context.Context, q *store.ListQuestionSetExecutionsQuery) ([]*types.QuestionSetExecution, error) {
	return nil, nil
}
