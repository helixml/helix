package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestSpecDrivenTaskService_CreateTaskFromPrompt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	// Use nil controller since goroutine testing is complex and not critical for this unit test
	// mockController := (*controller.Controller)(nil)
	var mockPubsub pubsub.PubSub = nil

	service := NewSpecDrivenTaskService(
		mockStore,
		nil,
		"test-helix-agent",
		[]string{"test-zed-agent"},
		mockPubsub,
		nil, // externalAgentExecutor not needed for tests
		nil, // registerRequestMapping not needed for tests
		nil, // gitRepositoryService not needed for tests
		NewDisabledKoditService(),
	)
	service.SetTestMode(true)

	ctx := context.Background()
	req := &types.CreateTaskRequest{
		ProjectID: "test-project",
		Prompt:    "Create a user authentication system",
		Type:      "feature",
		Priority:  types.SpecTaskPriorityHigh,
		UserID:    "test-user",
	}

	// Mock expectations
	mockStore.EXPECT().GetProject(ctx, "test-project").Return(&types.Project{
		ID:                "test-project",
		DefaultHelixAppID: "test-app-id",
	}, nil)
	mockStore.EXPECT().GetApp(ctx, "test-app-id").Return(&types.App{
		ID: "test-app-id",
	}, nil)
	mockStore.EXPECT().IncrementGlobalTaskNumber(ctx).Return(1, nil)
	mockStore.EXPECT().CreateSpecTask(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, task *types.SpecTask) error {
			assert.Equal(t, "test-project", task.ProjectID)
			assert.Equal(t, "Create a user authentication system", task.OriginalPrompt)
			assert.Equal(t, types.TaskStatusBacklog, task.Status)
			assert.Equal(t, "test-user", task.CreatedBy)
			assert.Equal(t, "feature", task.Type)
			assert.Equal(t, types.SpecTaskPriorityHigh, task.Priority)
			// Task number and design doc path should be assigned at creation
			assert.Equal(t, 1, task.TaskNumber)
			assert.NotEmpty(t, task.DesignDocPath)
			return nil
		},
	)

	// Note: We don't test the goroutine behavior in unit tests due to complexity
	// The spec generation goroutine will fail gracefully with nil controller

	// Execute
	task, err := service.CreateTaskFromPrompt(ctx, req)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, "test-project", task.ProjectID)
	assert.Equal(t, "Create a user authentication system", task.OriginalPrompt)
	assert.Equal(t, types.TaskStatusBacklog, task.Status)
	assert.Equal(t, "test-user", task.CreatedBy)
	// Task number and design doc path should be assigned at creation
	assert.Equal(t, 1, task.TaskNumber)
	assert.NotEmpty(t, task.DesignDocPath)

	// Note: Goroutine will fail gracefully, we only test the synchronous part
}

func TestSpecDrivenTaskService_CreateTaskFromPromptRejectsBlankExistingBranch(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	service := NewSpecDrivenTaskService(
		mockStore,
		nil,
		"test-helix-agent",
		nil,
		nil,
		nil,
		nil,
		nil,
		NewDisabledKoditService(),
	)

	task, err := service.CreateTaskFromPrompt(context.Background(), &types.CreateTaskRequest{
		BranchMode:    types.BranchModeExisting,
		WorkingBranch: "   ",
	})

	require.EqualError(t, err, "working branch is required for existing branch mode")
	require.Nil(t, task)
}

func TestSpecDrivenTaskService_HandleSpecGenerationComplete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	// mockController := (*controller.Controller)(nil)
	var mockPubsub pubsub.PubSub = nil

	service := NewSpecDrivenTaskService(
		mockStore,
		nil,
		"test-helix-agent",
		[]string{"test-zed-agent"},
		mockPubsub,
		nil, // externalAgentExecutor not needed for tests
		nil, // registerRequestMapping not needed for tests
		nil, // gitRepositoryService not needed for tests
		NewDisabledKoditService(),
	)
	service.SetTestMode(true)

	ctx := context.Background()
	taskID := "test-task-id"

	existingTask := &types.SpecTask{
		ID:     taskID,
		Status: types.TaskStatusSpecGeneration,
	}

	specs := &types.SpecGeneration{
		TaskID:             taskID,
		RequirementsSpec:   "Generated requirements specification",
		TechnicalDesign:    "Generated technical design",
		ImplementationPlan: "Generated implementation plan",
		GeneratedAt:        time.Now(),
		ModelUsed:          "test-model",
		TokensUsed:         1500,
	}

	// Mock expectations
	mockStore.EXPECT().GetSpecTask(ctx, taskID).Return(existingTask, nil)
	mockStore.EXPECT().UpdateSpecTask(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, task *types.SpecTask) error {
			assert.Equal(t, types.TaskStatusSpecReview, task.Status)
			assert.Equal(t, "Generated requirements specification", task.RequirementsSpec)
			assert.Equal(t, "Generated technical design", task.TechnicalDesign)
			assert.Equal(t, "Generated implementation plan", task.ImplementationPlan)
			return nil
		},
	)

	// Execute
	err := service.HandleSpecGenerationComplete(ctx, taskID, specs)

	// Assert
	require.NoError(t, err)
}

func TestGenerateTaskNameFromPrompt(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		expected string
	}{
		{
			name:     "short prompt unchanged",
			prompt:   "Fix the login bug",
			expected: "Fix the login bug",
		},
		{
			name:     "exactly 60 chars unchanged",
			prompt:   "This prompt is exactly sixty characters long, no truncation!",
			expected: "This prompt is exactly sixty characters long, no truncation!",
		},
		{
			name:     "long ASCII prompt truncated to 57 + ellipsis",
			prompt:   "This is a very long prompt that exceeds the sixty character limit and should be truncated",
			expected: "This is a very long prompt that exceeds the sixty charact...",
		},
		{
			name:     "multi-byte UTF-8 chars not split by truncation",
			prompt:   "Create a health check — monitor dashboard — verify alerts — ensure everything works correctly end to end",
			expected: "Create a health check — monitor dashboard — verify alerts...",
		},
		{
			name:     "em-dash at truncation boundary stays valid UTF-8",
			prompt:   "Check that em-dashes like — are handled at the boundary—this should not corrupt",
			expected: "Check that em-dashes like — are handled at the boundary—t...",
		},
		{
			name:     "CJK characters truncated by rune count not byte count",
			prompt:   "创建一个健康检查系统来监控所有的服务状态并且确保所有的服务都正常运行创建一个健康检查系统来监控所有的服务状态并且确保所有的服务都正常运行",
			expected: "创建一个健康检查系统来监控所有的服务状态并且确保所有的服务都正常运行创建一个健康检查系统来监控所有的服务状态并且确...",
		},
		{
			name:     "newlines collapsed to spaces",
			prompt:   "Line one\nLine two\nLine three",
			expected: "Line one Line two Line three",
		},
		{
			name:     "tabs and multiple spaces collapsed",
			prompt:   "Tabbed\t\ttext   with   extra   spaces",
			expected: "Tabbed text with extra spaces",
		},
		{
			name:     "hash with markdown headings and multi-byte chars",
			prompt:   "## Health Check — Monitor cluster status, verify alerts — ensure uptime SLA compliance",
			expected: "## Health Check — Monitor cluster status, verify alerts —...",
		},
		{
			name:     "empty prompt",
			prompt:   "",
			expected: "",
		},
		{
			name:     "whitespace only",
			prompt:   "   \n\t  ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateTaskNameFromPrompt(tt.prompt)
			assert.Equal(t, tt.expected, result)

			// Every result must be valid UTF-8 (the original bug: invalid UTF-8 reaching Postgres)
			assert.True(t, isValidUTF8(result), "result should be valid UTF-8: %q", result)

			// Truncated results should not exceed 60 runes
			if len([]rune(result)) > 60 {
				t.Errorf("result exceeds 60 runes: got %d", len([]rune(result)))
			}
		})
	}
}

// TestGenerateTaskNameFromPrompt_ByteTruncationRegression verifies that the old byte-level
// truncation bug (name[:57]) is fixed. With the old code, an em-dash (3-byte UTF-8: 0xe2 0x80 0x94)
// at the right position would be split, producing invalid UTF-8 that Postgres rejects with
// SQLSTATE 22021: "invalid byte sequence for encoding UTF8: 0xe2 0x80 0x2e"
func TestGenerateTaskNameFromPrompt_ByteTruncationRegression(t *testing.T) {
	// Construct a prompt where an em-dash lands exactly at byte position 55-57.
	// With old byte slicing (name[:57]), this would split the em-dash's 3 bytes,
	// and the appended "..." (0x2e 0x2e 0x2e) would create the invalid sequence 0xe2 0x80 0x2e.
	//
	// "aaaa...a" (55 ASCII bytes) + "—" (3 bytes: 0xe2 0x80 0x94) + more text
	// Old code: name[:57] = "aaaa...a" + 0xe2 0x80  (incomplete!)  + "..." = invalid UTF-8
	// New code: runes[:57] = "aaaa...a" + "—" + ... correctly handled
	prompt := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa—this triggers the old bug"

	result := GenerateTaskNameFromPrompt(prompt)

	assert.True(t, utf8.ValidString(result), "result must be valid UTF-8, got: %q (bytes: %x)", result, []byte(result))
	assert.LessOrEqual(t, len([]rune(result)), 60)
}

func isValidUTF8(s string) bool {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return false
		}
		i += size
	}
	return true
}

func TestSpecDrivenTaskService_ApproveSpecs_SynthesizesNilSpecApproval(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	var mockPubsub pubsub.PubSub = nil

	service := NewSpecDrivenTaskService(
		mockStore,
		nil,
		"test-helix-agent",
		[]string{"test-zed-agent"},
		mockPubsub,
		nil, nil, nil,
		NewDisabledKoditService(),
	)
	service.SetTestMode(true)

	ctx := context.Background()
	approvedAt := time.Now()

	// Task has SpecApprovedBy/SpecApprovedAt set but SpecApproval is nil —
	// this is the broken state from the approveImplementation fallback bug.
	taskInDB := &types.SpecTask{
		ID:            "task-stuck",
		ProjectID:     "project-1",
		Status:        types.TaskStatusSpecApproved,
		SpecApprovedBy: "user-1",
		SpecApprovedAt: &approvedAt,
		SpecApproval:  nil, // <-- the bug: this was never set
		TaskNumber:    42,
		Name:          "stuck-task",
	}

	mockStore.EXPECT().GetSpecTask(ctx, "task-stuck").Return(taskInDB, nil)
	mockStore.EXPECT().GetProject(ctx, "project-1").Return(&types.Project{
		ID:            "project-1",
		DefaultRepoID: "repo-1",
	}, nil)
	mockStore.EXPECT().GetGitRepository(ctx, "repo-1").Return(&types.GitRepository{
		ID:            "repo-1",
		DefaultBranch: "main",
	}, nil)
	mockStore.EXPECT().TransitionSpecTaskStatus(
		ctx,
		"task-stuck",
		gomock.Any(),
		types.TaskStatusImplementation,
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, _ []types.SpecTaskStatus, _ types.SpecTaskStatus, extraFields map[string]any) (bool, error) {
		// Synthesized SpecApproval must be persisted in the same atomic UPDATE,
		// otherwise the in-memory synthesis is lost on the next re-fetch.
		raw, ok := extraFields["spec_approval"]
		require.True(t, ok, "spec_approval field must be in extraFields when SpecApproval was synthesized")
		jsonStr, ok := raw.(string)
		require.True(t, ok, "spec_approval must be marshalled to a JSON string")
		assert.Contains(t, jsonStr, "task-stuck")
		assert.Contains(t, jsonStr, "user-1")
		return true, nil
	})

	err := service.ApproveSpecs(ctx, &types.SpecTask{ID: "task-stuck"})
	require.NoError(t, err)
	_ = approvedAt
}

func TestSpecDrivenTaskService_ApproveSpecs_NilSpecApprovalAndNilApprovedAt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	var mockPubsub pubsub.PubSub = nil

	service := NewSpecDrivenTaskService(
		mockStore,
		nil,
		"test-helix-agent",
		[]string{"test-zed-agent"},
		mockPubsub,
		nil, nil, nil,
		NewDisabledKoditService(),
	)
	service.SetTestMode(true)

	ctx := context.Background()

	// Both SpecApproval and SpecApprovedAt are nil — worst case scenario.
	taskInDB := &types.SpecTask{
		ID:            "task-worst-case",
		ProjectID:     "project-1",
		Status:        types.TaskStatusSpecApproved,
		SpecApprovedBy: "user-1",
		SpecApprovedAt: nil,
		SpecApproval:  nil,
		TaskNumber:    43,
		Name:          "worst-case-task",
	}

	mockStore.EXPECT().GetSpecTask(ctx, "task-worst-case").Return(taskInDB, nil)
	mockStore.EXPECT().GetProject(ctx, "project-1").Return(&types.Project{
		ID:            "project-1",
		DefaultRepoID: "repo-1",
	}, nil)
	mockStore.EXPECT().GetGitRepository(ctx, "repo-1").Return(&types.GitRepository{
		ID:            "repo-1",
		DefaultBranch: "main",
	}, nil)
	mockStore.EXPECT().TransitionSpecTaskStatus(
		ctx,
		"task-worst-case",
		gomock.Any(),
		types.TaskStatusImplementation,
		gomock.Any(),
	).Return(true, nil)

	err := service.ApproveSpecs(ctx, &types.SpecTask{ID: "task-worst-case"})
	require.NoError(t, err)
}

// TestSpecDrivenTaskService_ApproveSpecs_LosesAtomicTransitionRace verifies the
// authoritative race guard: if TransitionSpecTaskStatus reports it didn't update
// the row (transitioned=false), ApproveSpecs must bail without sending the
// implementation instruction. This is what prevents the duplicate-prompt bug
// when the HTTP handler goroutine and orchestrator polling loop both invoke
// ApproveSpecs simultaneously.
func TestSpecDrivenTaskService_ApproveSpecs_LosesAtomicTransitionRace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	var mockPubsub pubsub.PubSub = nil

	// enqueuer is wired up so we can prove it is NOT called when the
	// transition loses the race.
	sendCalls := 0
	sender := func(_ context.Context, _ *types.SpecTask, _ string, _ bool, _ string) error {
		sendCalls++
		return nil
	}

	service := NewSpecDrivenTaskService(
		mockStore,
		nil,
		"test-helix-agent",
		[]string{"test-zed-agent"},
		mockPubsub,
		nil, nil, nil,
		NewDisabledKoditService(),
	)
	service.EnqueueMessageToAgent = sender
	// Note: testMode=false so the message-send path would actually fire if we
	// reached it; the test asserts it does not.

	ctx := context.Background()
	taskInDB := &types.SpecTask{
		ID:                "task-loser",
		ProjectID:         "project-1",
		Status:            types.TaskStatusSpecApproved,
		PlanningSessionID: "ses-loser",
		SpecApproval:      &types.SpecApprovalResponse{Approved: true, ApprovedBy: "user-1"},
		TaskNumber:        99,
		Name:              "loser-task",
	}

	mockStore.EXPECT().GetSpecTask(ctx, "task-loser").Return(taskInDB, nil)
	mockStore.EXPECT().GetProject(ctx, "project-1").Return(&types.Project{
		ID:            "project-1",
		DefaultRepoID: "repo-1",
	}, nil)
	mockStore.EXPECT().GetGitRepository(ctx, "repo-1").Return(&types.GitRepository{
		ID:            "repo-1",
		DefaultBranch: "main",
	}, nil)
	// Simulate losing the race: the other caller already updated the row.
	mockStore.EXPECT().TransitionSpecTaskStatus(
		ctx,
		"task-loser",
		gomock.Any(),
		types.TaskStatusImplementation,
		gomock.Any(),
	).Return(false, nil)

	err := service.ApproveSpecs(ctx, &types.SpecTask{ID: "task-loser"})
	require.NoError(t, err)
	assert.Equal(t, 0, sendCalls, "messageSender must not be invoked when atomic transition loses the race")
}

func TestSpecDrivenTaskService_SelectZedAgent(t *testing.T) {
	// Test with agents available
	service := NewSpecDrivenTaskService(nil, nil, "test-helix-agent", []string{"agent1", "agent2"}, nil, nil, nil, nil, NewDisabledKoditService())
	agent := service.selectZedAgent()
	assert.Equal(t, "agent1", agent)

	// Test with no agents
	serviceNoAgents := NewSpecDrivenTaskService(nil, nil, "test-helix-agent", []string{}, nil, nil, nil, nil, NewDisabledKoditService())
	serviceNoAgents.SetTestMode(true)
	agent = serviceNoAgents.selectZedAgent()
	assert.Equal(t, "", agent)
}

// newIdentitySyncService builds a service wired up just enough to exercise
// syncGitIdentityToApprover. testMode is left false so the sync path runs;
// the ExecInDesktop callback is what the caller wants to observe/inject.
func newIdentitySyncService(t *testing.T, exec DesktopExecFunc) (*SpecDrivenTaskService, *store.MockStore, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)

	svc := NewSpecDrivenTaskService(
		mockStore,
		nil,
		"test-helix-agent",
		[]string{"test-zed-agent"},
		nil, nil, nil, nil,
		NewDisabledKoditService(),
	)
	svc.ExecInDesktop = exec
	return svc, mockStore, ctrl
}

type execCall struct {
	sessionID string
	command   []string
}

// recordingExec captures every invocation and returns a configurable error
// per index (indexed by invocation order).
func recordingExec(errs ...error) (DesktopExecFunc, *[]execCall) {
	var calls []execCall
	fn := func(_ context.Context, sessionID string, command []string) error {
		idx := len(calls)
		calls = append(calls, execCall{sessionID: sessionID, command: append([]string(nil), command...)})
		if idx < len(errs) {
			return errs[idx]
		}
		return nil
	}
	return fn, &calls
}

func TestSyncGitIdentityToApprover_Success(t *testing.T) {
	exec, calls := recordingExec()
	svc, mockStore, ctrl := newIdentitySyncService(t, exec)
	defer ctrl.Finish()

	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, q *store.GetUserQuery) (*types.User, error) {
			assert.Equal(t, "approver-1", q.ID)
			return &types.User{ID: "approver-1", FullName: "Approver One", Email: "approver@example.com"}, nil
		})

	task := &types.SpecTask{ID: "task-1", PlanningSessionID: "ses-1", SpecApprovedBy: "approver-1"}
	require.NoError(t, svc.syncGitIdentityToUser(context.Background(), task, task.SpecApprovedBy, "approver"))

	require.Len(t, *calls, 2, "expected email then name")
	assert.Equal(t, []string{"git", "config", "--global", "user.email", "approver@example.com"}, (*calls)[0].command)
	assert.Equal(t, []string{"git", "config", "--global", "user.name", "Approver One"}, (*calls)[1].command)
	assert.Equal(t, "ses-1", (*calls)[0].sessionID)
}

func TestSyncGitIdentityToApprover_FallsBackToUsernameThenEmailLocalPart(t *testing.T) {
	cases := []struct {
		name     string
		user     *types.User
		wantName string
	}{
		{
			name:     "uses username when fullname empty",
			user:     &types.User{ID: "u", Username: "alice", Email: "alice@example.com"},
			wantName: "alice",
		},
		{
			name:     "uses email local-part when both empty",
			user:     &types.User{ID: "u", Email: "bob@example.com"},
			wantName: "bob",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec, calls := recordingExec()
			svc, mockStore, ctrl := newIdentitySyncService(t, exec)
			defer ctrl.Finish()

			mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(tc.user, nil)

			task := &types.SpecTask{ID: "task-1", PlanningSessionID: "ses-1", SpecApprovedBy: "u"}
			require.NoError(t, svc.syncGitIdentityToUser(context.Background(), task, task.SpecApprovedBy, "approver"))

			require.Len(t, *calls, 2)
			assert.Equal(t, tc.wantName, (*calls)[1].command[4], "user.name should fall back sensibly")
		})
	}
}

func TestSyncGitIdentityToApprover_NoOpCases(t *testing.T) {
	t.Run("testMode short-circuits", func(t *testing.T) {
		exec, calls := recordingExec()
		svc, _, ctrl := newIdentitySyncService(t, exec)
		defer ctrl.Finish()
		svc.SetTestMode(true)
		require.NoError(t, svc.syncGitIdentityToUser(context.Background(), &types.SpecTask{PlanningSessionID: "s", SpecApprovedBy: "u"}, "u", "approver"))
		assert.Empty(t, *calls, "testMode should not exec")
	})
	t.Run("no session", func(t *testing.T) {
		exec, calls := recordingExec()
		svc, _, ctrl := newIdentitySyncService(t, exec)
		defer ctrl.Finish()
		require.NoError(t, svc.syncGitIdentityToUser(context.Background(), &types.SpecTask{SpecApprovedBy: "u"}, "u", "approver"))
		assert.Empty(t, *calls)
	})
	t.Run("no approver", func(t *testing.T) {
		exec, calls := recordingExec()
		svc, _, ctrl := newIdentitySyncService(t, exec)
		defer ctrl.Finish()
		require.NoError(t, svc.syncGitIdentityToUser(context.Background(), &types.SpecTask{PlanningSessionID: "s"}, "", "approver"))
		assert.Empty(t, *calls)
	})
	t.Run("ExecInDesktop nil", func(t *testing.T) {
		svc, _, ctrl := newIdentitySyncService(t, nil)
		defer ctrl.Finish()
		require.NoError(t, svc.syncGitIdentityToUser(context.Background(), &types.SpecTask{PlanningSessionID: "s", SpecApprovedBy: "u"}, "u", "approver"))
	})
}

func TestSyncGitIdentityToApprover_ErrorsSurface(t *testing.T) {
	ctx := context.Background()
	baseTask := &types.SpecTask{ID: "task-1", PlanningSessionID: "ses-1", SpecApprovedBy: "u"}

	t.Run("GetUser error bubbles up", func(t *testing.T) {
		exec, calls := recordingExec()
		svc, mockStore, ctrl := newIdentitySyncService(t, exec)
		defer ctrl.Finish()
		mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(nil, errors.New("db down"))
		err := svc.syncGitIdentityToUser(ctx, baseTask, baseTask.SpecApprovedBy, "approver")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db down")
		assert.Empty(t, *calls, "no exec should happen if GetUser fails")
	})

	t.Run("missing email is an error", func(t *testing.T) {
		exec, calls := recordingExec()
		svc, mockStore, ctrl := newIdentitySyncService(t, exec)
		defer ctrl.Finish()
		mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).
			Return(&types.User{ID: "u", FullName: "Someone"}, nil)
		err := svc.syncGitIdentityToUser(ctx, baseTask, baseTask.SpecApprovedBy, "approver")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no email")
		assert.Empty(t, *calls, "no exec without email")
	})

	t.Run("email exec failure aborts before name", func(t *testing.T) {
		exec, calls := recordingExec(errors.New("boom"))
		svc, mockStore, ctrl := newIdentitySyncService(t, exec)
		defer ctrl.Finish()
		mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).
			Return(&types.User{ID: "u", FullName: "N", Email: "e@x"}, nil)
		err := svc.syncGitIdentityToUser(ctx, baseTask, baseTask.SpecApprovedBy, "approver")
		require.Error(t, err)
		assert.Len(t, *calls, 1, "should stop after email failure, not touch user.name")
	})

	t.Run("name-only failure is tolerated", func(t *testing.T) {
		// nil, then error — email succeeds, name fails
		exec, calls := recordingExec(nil, errors.New("boom"))
		svc, mockStore, ctrl := newIdentitySyncService(t, exec)
		defer ctrl.Finish()
		mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).
			Return(&types.User{ID: "u", FullName: "N", Email: "e@x"}, nil)
		err := svc.syncGitIdentityToUser(ctx, baseTask, baseTask.SpecApprovedBy, "approver")
		require.NoError(t, err, "name-only failure is logged but not propagated")
		assert.Len(t, *calls, 2)
	})
}

func TestSyncGitIdentityToUser_UsesExplicitUserID(t *testing.T) {
	// Same helper, different phase. Verify the passed-in userID (not any
	// field on the task) is what's looked up and applied, so the planner
	// and approver code paths share the same implementation.
	exec, calls := recordingExec()
	svc, mockStore, ctrl := newIdentitySyncService(t, exec)
	defer ctrl.Finish()

	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, q *store.GetUserQuery) (*types.User, error) {
			assert.Equal(t, "planner-42", q.ID, "must look up the user passed via argument, not task.SpecApprovedBy")
			return &types.User{ID: q.ID, FullName: "Planner Forty-Two", Email: "p42@example.com"}, nil
		})

	// Deliberately set SpecApprovedBy to a different value to prove the
	// helper honours the userID argument, not the task field.
	task := &types.SpecTask{
		ID:                "task-x",
		PlanningSessionID: "ses-x",
		SpecApprovedBy:    "someone-else",
		PlanningStartedBy: "planner-42",
	}
	require.NoError(t, svc.syncGitIdentityToUser(context.Background(), task, task.PlanningStartedBy, "planner"))

	require.Len(t, *calls, 2)
	assert.Equal(t, "p42@example.com", (*calls)[0].command[4])
	assert.Equal(t, "Planner Forty-Two", (*calls)[1].command[4])
}

func TestIsNonRetryableIdentityError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"user not found sentinel is terminal", fmt.Errorf("%w: approver u", errIdentityUserNotFound), true},
		{"missing email sentinel is terminal", fmt.Errorf("%w: approver u", errIdentityNoEmail), true},
		{"container not ready is retryable", errors.New("failed to connect to desktop ses-1 via RevDial: connection refused"), false},
		{"exec failure is retryable", errors.New("exec command failed: bash: git: command not found (exit code 127)"), false},
		{"arbitrary DB error is retryable (might self-heal)", errors.New("failed to look up approver u: temporary db error"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isNonRetryableIdentityError(tc.err)
			assert.Equal(t, tc.want, got)
		})
	}
}
