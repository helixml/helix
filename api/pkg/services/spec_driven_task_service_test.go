package services

import (
	"context"
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
			result := generateTaskNameFromPrompt(tt.prompt)
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

	result := generateTaskNameFromPrompt(prompt)

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
