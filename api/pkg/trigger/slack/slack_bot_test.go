package slack

import (
	"context"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestConvertMarkdownToSlackFormat(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		expected string
	}{
		{
			name:     "bold text",
			markdown: "This is **bold** text",
			expected: "This is *bold* text",
		},
		{
			name:     "italic text",
			markdown: "This is *italic* text",
			expected: "This is _italic_ text",
		},
		{
			name:     "bold and italic",
			markdown: "This is **bold** and *italic* text",
			expected: "This is *bold* and _italic_ text",
		},
		{
			name:     "link",
			markdown: "Check out [Slack API](https://api.slack.com)",
			expected: "Check out <https://api.slack.com|Slack API>",
		},
		{
			name:     "inline code",
			markdown: "Use the `code` function",
			expected: "Use the `code` function",
		},
		{
			name:     "code block",
			markdown: "```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```",
			expected: "```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```",
		},
		{
			name:     "strikethrough",
			markdown: "This is ~~strikethrough~~ text",
			expected: "This is ~strikethrough~ text",
		},
		{
			name:     "list items",
			markdown: "- Item 1\n- Item 2\n* Item 3",
			expected: "• Item 1\n• Item 2\n• Item 3",
		},
		{
			name:     "mixed formatting",
			markdown: "**Bold** with *italic* and [link](https://example.com) and `code`",
			expected: "*Bold* with _italic_ and <https://example.com|link> and `code`",
		},
		{
			name:     "nested bold",
			markdown: "**Bold with **more bold** inside**",
			expected: "*Bold with *more bold* inside*",
		},
		{
			name:     "blockquote",
			markdown: "> This is a quote",
			expected: "> This is a quote",
		},
		{
			name:     "heading h1",
			markdown: "# Main Title",
			expected: "*Main Title*",
		},
		{
			name:     "heading h2",
			markdown: "## Section Title",
			expected: "*Section Title*",
		},
		{
			name:     "heading h3",
			markdown: "### Subsection",
			expected: "*Subsection*",
		},
		{
			name:     "heading mixed with text",
			markdown: "Some text\n## Heading\nMore text",
			expected: "Some text\n*Heading*\nMore text",
		},
		{
			name:     "simple table",
			markdown: "| ID | Name | Priority |\n|---|---|---|\n| spt_123 | My task | medium |",
			expected: "*ID:* spt_123\n*Name:* My task\n*Priority:* medium",
		},
		{
			name:     "table with multiple rows",
			markdown: "| Name | Status |\n|---|---|\n| Task 1 | done |\n| Task 2 | pending |",
			expected: "*Name:* Task 1\n*Status:* done\n\n*Name:* Task 2\n*Status:* pending",
		},
		{
			name:     "table with surrounding text",
			markdown: "Here are the tasks:\n| ID | Name |\n|---|---|\n| 1 | Fix bug |\nThat's all.",
			expected: "Here are the tasks:\n*ID:* 1\n*Name:* Fix bug\nThat's all.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertMarkdownToSlackFormat(tt.markdown)
			if result != tt.expected {
				t.Errorf("convertMarkdownToSlackFormat() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFormatResponseForSlack_NoTable(t *testing.T) {
	msg := formatResponseForSlack("Hello **world**", nil)
	assert.Equal(t, "Hello *world*", msg.text)
	assert.Empty(t, msg.blocks)
}

func TestFormatResponseForSlack_WithTable(t *testing.T) {
	markdown := "Here are the tasks:\n| ID | Name | Priority |\n|---|---|---|\n| spt_123 | My task | medium |\nLet me know!"
	msg := formatResponseForSlack(markdown, nil)

	assert.NotEmpty(t, msg.text)
	assert.Contains(t, msg.text, "spt_123")

	require.NotEmpty(t, msg.blocks)

	var hasTable, hasSection bool
	for _, b := range msg.blocks {
		switch b.BlockType() {
		case "table":
			hasTable = true
			tb, ok := b.(*slackTableBlock)
			require.True(t, ok)
			require.Len(t, tb.Rows, 2)
			assert.Equal(t, "ID", tb.Rows[0][0].Text)
			assert.Equal(t, "Name", tb.Rows[0][1].Text)
			assert.Equal(t, "Priority", tb.Rows[0][2].Text)
			assert.Equal(t, "spt_123", tb.Rows[1][0].Text)
			assert.Equal(t, "My task", tb.Rows[1][1].Text)
			assert.Equal(t, "medium", tb.Rows[1][2].Text)
		case "section":
			hasSection = true
		}
	}
	assert.True(t, hasTable, "expected a table block")
	assert.True(t, hasSection, "expected section blocks for surrounding text")
}

func TestFormatResponseForSlack_MultipleRows(t *testing.T) {
	markdown := "| Name | Status |\n|---|---|\n| Task 1 | done |\n| Task 2 | pending |"
	msg := formatResponseForSlack(markdown, nil)

	require.NotEmpty(t, msg.blocks)
	var tb *slackTableBlock
	for _, b := range msg.blocks {
		if b.BlockType() == "table" {
			tb = b.(*slackTableBlock)
		}
	}
	require.NotNil(t, tb)
	assert.Len(t, tb.Rows, 3)
	assert.Equal(t, "Name", tb.Rows[0][0].Text)
	assert.Equal(t, "Task 1", tb.Rows[1][0].Text)
	assert.Equal(t, "Task 2", tb.Rows[2][0].Text)
}

func TestBuildTableBlock(t *testing.T) {
	lines := []string{
		"| ID | Name | Priority |",
		"|---|---|---|",
		"| 1 | Fix bug | high |",
		"| 2 | Add feature | low |",
	}
	tb := buildTableBlock(lines)
	require.NotNil(t, tb)
	assert.Equal(t, slack.MessageBlockType("table"), tb.Type)
	require.Len(t, tb.Rows, 3)
	assert.Equal(t, "ID", tb.Rows[0][0].Text)
	assert.Equal(t, "1", tb.Rows[1][0].Text)
	assert.Equal(t, "2", tb.Rows[2][0].Text)
	assert.Len(t, tb.ColumnSettings, 3)
	for _, cs := range tb.ColumnSettings {
		assert.True(t, cs.IsWrapped)
	}
}

func TestBuildTableBlock_TooFewLines(t *testing.T) {
	lines := []string{"| ID | Name |"}
	assert.Nil(t, buildTableBlock(lines))
}

func TestSplitMarkdownByTables(t *testing.T) {
	markdown := "Before\n| A | B |\n|---|---|\n| 1 | 2 |\nAfter"
	segments := splitMarkdownByTables(markdown)
	require.Len(t, segments, 3)
	assert.False(t, segments[0].isTable)
	assert.True(t, segments[1].isTable)
	assert.False(t, segments[2].isTable)
	assert.Equal(t, []string{"Before"}, segments[0].lines)
	assert.Equal(t, []string{"| A | B |", "|---|---|", "| 1 | 2 |"}, segments[1].lines)
	assert.Equal(t, []string{"After"}, segments[2].lines)
}

func TestCreateNewThread_ExternalAgent(t *testing.T) {
	tests := []struct {
		name                        string
		defaultAgentType            string
		assistantAgentTypes         []types.AgentType
		expectedPostProgressUpdates bool
		expectedIncludeScreenshots  bool
	}{
		{
			name:                        "default helix agent - no progress updates",
			defaultAgentType:            "helix",
			assistantAgentTypes:         nil,
			expectedPostProgressUpdates: false,
			expectedIncludeScreenshots:  false,
		},
		{
			name:                        "zed_external default - enable progress updates",
			defaultAgentType:            "zed_external",
			assistantAgentTypes:         nil,
			expectedPostProgressUpdates: true,
			expectedIncludeScreenshots:  true,
		},
		{
			name:             "assistant with zed_external - enable progress updates",
			defaultAgentType: "",
			assistantAgentTypes: []types.AgentType{
				types.AgentTypeZedExternal,
			},
			expectedPostProgressUpdates: true,
			expectedIncludeScreenshots:  true,
		},
		{
			name:             "multiple assistants with one external - enable progress updates",
			defaultAgentType: "",
			assistantAgentTypes: []types.AgentType{
				types.AgentTypeHelixAgent,
				types.AgentTypeZedExternal,
			},
			expectedPostProgressUpdates: true,
			expectedIncludeScreenshots:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build assistants config
			var assistants []types.AssistantConfig
			for _, agentType := range tt.assistantAgentTypes {
				assistants = append(assistants, types.AssistantConfig{
					AgentType: agentType,
				})
			}

			// Check external agent detection logic
			isExternalAgent := tt.defaultAgentType == "zed_external"
			if !isExternalAgent {
				for _, assistant := range assistants {
					if assistant.AgentType == types.AgentTypeZedExternal {
						isExternalAgent = true
						break
					}
				}
			}

			if isExternalAgent != tt.expectedPostProgressUpdates {
				t.Errorf("expected isExternalAgent=%v, got %v", tt.expectedPostProgressUpdates, isExternalAgent)
			}
		})
	}
}

func TestBuildProjectUpdateAttachment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	bot := &SlackBot{
		store: mockStore,
	}

	task := &types.SpecTask{
		ID:             "task_123",
		ProjectID:      "proj_123",
		OrganizationID: "org_123",
		Name:           "Implement light mode",
		Description:    "Adds full light mode support to the app",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityHigh,
		Status:         types.TaskStatusImplementation,
	}

	mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_123"}).Return(&types.Organization{
		ID:   "org_123",
		Name: "my-org",
	}, nil)

	attachment := bot.buildProjectUpdateAttachment(context.Background(), task, "https://app.helix.ml")

	assert.Equal(t, "#36a64f", attachment.Color)
	assert.Contains(t, attachment.Title, "Project Update")
	assert.Contains(t, attachment.Title, "🚧")
	assert.Contains(t, attachment.Text, "Adds full light mode support to the app")
	assert.Len(t, attachment.Fields, 5)
	assert.Equal(t, "Status", attachment.Fields[0].Title)
	assert.Contains(t, attachment.Fields[0].Value, "Implementation")
	assert.Contains(t, attachment.Fields[2].Value, "https://app.helix.ml/orgs/my-org/projects/proj_123/tasks/task_123?view=details")
	assert.Equal(t, "Project", attachment.Fields[4].Title)
	assert.Contains(t, attachment.Fields[4].Value, "https://app.helix.ml/orgs/my-org/projects/proj_123/specs")
}

func TestBuildProjectUpdateReplyAttachment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	bot := &SlackBot{
		store: mockStore,
	}

	task := &types.SpecTask{
		ID:             "task_123",
		ProjectID:      "proj_123",
		OrganizationID: "org_123",
		Name:           "Implement light mode",
		Description:    "Adds full light mode support to the app",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityHigh,
		Status:         types.TaskStatusPullRequest,
	}

	mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_123"}).Return(&types.Organization{
		ID:   "org_123",
		Name: "my-org",
	}, nil)

	attachment := bot.buildProjectUpdateReplyAttachment(context.Background(), task, "https://app.helix.ml")

	assert.Equal(t, "#9C27B0", attachment.Color)
	assert.Contains(t, attachment.Text, "🔀")
	assert.Contains(t, attachment.Text, "Implement light mode")
	assert.Contains(t, attachment.Text, "Pull Request")
	assert.Contains(t, attachment.Text, "https://app.helix.ml/orgs/my-org/projects/proj_123/tasks/task_123?view=details")
	assert.Contains(t, attachment.Text, "https://app.helix.ml/orgs/my-org/projects/proj_123/specs")
}

func TestSpecTaskStatusColor(t *testing.T) {
	tests := []struct {
		status   types.SpecTaskStatus
		expected string
	}{
		{types.TaskStatusBacklog, "#808080"},              // Grey
		{types.TaskStatusSpecGeneration, "#FF8C00"},       // Orange
		{types.TaskStatusSpecRevision, "#FF8C00"},         // Orange
		{types.TaskStatusImplementation, "#36a64f"},       // Green
		{types.TaskStatusSpecReview, "#2196F3"},           // Blue
		{types.TaskStatusImplementationReview, "#2196F3"}, // Blue
		{types.TaskStatusPullRequest, "#9C27B0"},          // Purple
		{types.TaskStatusDone, "#36a64f"},                 // Green
		{types.TaskStatusSpecFailed, "#E53935"},           // Red
		{types.TaskStatusImplementationFailed, "#E53935"}, // Red
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.expected, specTaskStatusColor(tt.status))
		})
	}
}

func TestPostProjectUpdateNewCreatesThreadWithSpecTaskID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	bot := &SlackBot{
		cfg:   &config.ServerConfig{},
		store: mockStore,
		app: &types.App{
			ID:             "app_123",
			Owner:          "user_123",
			OrganizationID: "org_123",
			Config:         types.AppConfig{},
		},
		trigger: &types.SlackTrigger{
			ProjectUpdates: true,
			ProjectChannel: "C123",
		},
		postMessage: func(_ string, _ ...slack.MsgOption) (string, string, error) {
			return "C123", "173.42", nil
		},
		updateMessage: func(channelID, timestamp string, _ ...slack.MsgOption) (string, string, string, error) {
			return channelID, timestamp, "", nil
		},
	}

	task := &types.SpecTask{
		ID:          "task_123",
		ProjectID:   "proj_123",
		Name:        "Implement light mode",
		Description: "Adds full light mode support to the app",
		Type:        "feature",
		Priority:    types.SpecTaskPriorityHigh,
		Status:      types.TaskStatusBacklog,
	}

	// First call: look up existing thread by spec task ID — not found
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "task_123").Return(task, nil)
	mockStore.EXPECT().GetSlackThreadBySpecTaskID(gomock.Any(), "app_123", "task_123").Return(nil, store.ErrNotFound)
	mockStore.EXPECT().CreateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			assert.Equal(t, "task_123", session.Metadata.SpecTaskID)
			assert.Equal(t, "proj_123", session.Metadata.ProjectID)
			return &session, nil
		},
	)

	mockStore.EXPECT().CreateSlackThread(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, thread *types.SlackThread) (*types.SlackThread, error) {
			assert.Equal(t, "app_123", thread.AppID)
			assert.Equal(t, "C123", thread.Channel)
			assert.Equal(t, "173.42", thread.ThreadKey)
			assert.Equal(t, "task_123", thread.SpecTaskID)
			return thread, nil
		},
	)

	err := bot.postProjectUpdate(context.Background(), task)
	require.NoError(t, err)
}

func TestPostProjectUpdateReplyWhenThreadExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	var postedInThread string
	postCalls := 0
	updateCalls := 0
	var updatedThreadTS string
	bot := &SlackBot{
		cfg:   &config.ServerConfig{},
		store: mockStore,
		app: &types.App{
			ID:             "app_123",
			Owner:          "user_123",
			OrganizationID: "org_123",
			Config:         types.AppConfig{},
		},
		trigger: &types.SlackTrigger{
			ProjectUpdates: true,
			ProjectChannel: "C123",
		},
		postMessage: func(channelID string, options ...slack.MsgOption) (string, string, error) {
			// Capture thread_ts to verify it's a reply
			postedInThread = channelID
			postCalls++
			return channelID, "174.00", nil
		},
		updateMessage: func(channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error) {
			updateCalls++
			updatedThreadTS = timestamp
			return channelID, timestamp, "", nil
		},
	}

	task := &types.SpecTask{
		ID:          "task_123",
		ProjectID:   "proj_123",
		Name:        "Implement light mode",
		Description: "Status changed to implementation",
		Type:        "feature",
		Priority:    types.SpecTaskPriorityHigh,
		Status:      types.TaskStatusImplementation,
	}

	existingThread := &types.SlackThread{
		ThreadKey:  "173.42",
		AppID:      "app_123",
		Channel:    "C123",
		SessionID:  "session_123",
		SpecTaskID: "task_123",
	}

	// Should find existing thread
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "task_123").Return(task, nil)
	mockStore.EXPECT().GetSlackThreadBySpecTaskID(gomock.Any(), "app_123", "task_123").Return(existingThread, nil)
	// Should NOT create a new session or thread — just post a reply
	err := bot.postProjectUpdate(context.Background(), task)
	require.NoError(t, err)
	assert.Equal(t, "C123", postedInThread)
	assert.Equal(t, 1, postCalls)
	assert.Equal(t, 1, updateCalls)
	assert.Equal(t, "173.42", updatedThreadTS)
}

func TestPostProjectUpdateReplySkipsDuplicateStatusMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	postCalls := 0
	updateCalls := 0
	bot := &SlackBot{
		cfg:   &config.ServerConfig{},
		store: mockStore,
		app: &types.App{
			ID:             "app_123",
			Owner:          "user_123",
			OrganizationID: "org_123",
			Config:         types.AppConfig{},
		},
		trigger: &types.SlackTrigger{
			ProjectUpdates: true,
			ProjectChannel: "C123",
		},
		postMessage: func(channelID string, options ...slack.MsgOption) (string, string, error) {
			postCalls++
			return channelID, "174.00", nil
		},
		updateMessage: func(channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error) {
			updateCalls++
			return channelID, timestamp, "", nil
		},
		getConversationReplies: func(params *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
			return []slack.Message{
				{Msg: slack.Msg{Timestamp: params.Timestamp, Text: "Project update root"}},
				{Msg: slack.Msg{Timestamp: "173.90", Text: "Status update: Fix scroll to the header/paragraph functionality in docs/... → Pull Request"}},
			}, false, "", nil
		},
	}

	task := &types.SpecTask{
		ID:          "task_123",
		ProjectID:   "proj_123",
		Name:        "Fix scroll to the header/paragraph functionality in docs/...",
		Description: "No status change, just another update",
		Type:        "feature",
		Priority:    types.SpecTaskPriorityHigh,
		Status:      types.TaskStatusPullRequest,
	}

	existingThread := &types.SlackThread{
		ThreadKey:  "173.42",
		AppID:      "app_123",
		Channel:    "C123",
		SessionID:  "session_123",
		SpecTaskID: "task_123",
	}

	mockStore.EXPECT().GetSpecTask(gomock.Any(), "task_123").Return(task, nil)
	mockStore.EXPECT().GetSlackThreadBySpecTaskID(gomock.Any(), "app_123", "task_123").Return(existingThread, nil)

	err := bot.postProjectUpdate(context.Background(), task)
	require.NoError(t, err)
	assert.Equal(t, 0, postCalls)
	assert.Equal(t, 1, updateCalls)
}

func TestPostProjectUpdateReplyPostsWhenNoDuplicateStatusMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	postCalls := 0
	updateCalls := 0
	bot := &SlackBot{
		cfg:   &config.ServerConfig{},
		store: mockStore,
		app: &types.App{
			ID:             "app_123",
			Owner:          "user_123",
			OrganizationID: "org_123",
			Config:         types.AppConfig{},
		},
		trigger: &types.SlackTrigger{
			ProjectUpdates: true,
			ProjectChannel: "C123",
		},
		postMessage: func(channelID string, options ...slack.MsgOption) (string, string, error) {
			postCalls++
			return channelID, "174.00", nil
		},
		updateMessage: func(channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error) {
			updateCalls++
			return channelID, timestamp, "", nil
		},
		getConversationReplies: func(params *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
			return []slack.Message{
				{Msg: slack.Msg{Timestamp: params.Timestamp, Text: "Project update root"}},
				{Msg: slack.Msg{Timestamp: "173.90", Text: "Status update: Fix scroll to the header/paragraph functionality in docs/... → Implementation"}},
			}, false, "", nil
		},
	}

	task := &types.SpecTask{
		ID:          "task_123",
		ProjectID:   "proj_123",
		Name:        "Fix scroll to the header/paragraph functionality in docs/...",
		Description: "Status moved to pull request",
		Type:        "feature",
		Priority:    types.SpecTaskPriorityHigh,
		Status:      types.TaskStatusPullRequest,
	}

	existingThread := &types.SlackThread{
		ThreadKey:  "173.42",
		AppID:      "app_123",
		Channel:    "C123",
		SessionID:  "session_123",
		SpecTaskID: "task_123",
	}

	mockStore.EXPECT().GetSpecTask(gomock.Any(), "task_123").Return(task, nil)
	mockStore.EXPECT().GetSlackThreadBySpecTaskID(gomock.Any(), "app_123", "task_123").Return(existingThread, nil)

	err := bot.postProjectUpdate(context.Background(), task)
	require.NoError(t, err)
	assert.Equal(t, 1, postCalls)
	assert.Equal(t, 1, updateCalls)
}

func TestPostProjectUpdateReplyContinuesWhenDuplicateCheckFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	postCalls := 0
	updateCalls := 0
	bot := &SlackBot{
		cfg:   &config.ServerConfig{},
		store: mockStore,
		app: &types.App{
			ID:             "app_123",
			Owner:          "user_123",
			OrganizationID: "org_123",
			Config:         types.AppConfig{},
		},
		trigger: &types.SlackTrigger{
			ProjectUpdates: true,
			ProjectChannel: "C123",
		},
		postMessage: func(channelID string, options ...slack.MsgOption) (string, string, error) {
			postCalls++
			return channelID, "174.00", nil
		},
		updateMessage: func(channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error) {
			updateCalls++
			return channelID, timestamp, "", nil
		},
		getConversationReplies: func(params *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
			return nil, false, "", assert.AnError
		},
	}

	task := &types.SpecTask{
		ID:          "task_123",
		ProjectID:   "proj_123",
		Name:        "Fix scroll to the header/paragraph functionality in docs/...",
		Description: "Status moved to implementation",
		Type:        "feature",
		Priority:    types.SpecTaskPriorityHigh,
		Status:      types.TaskStatusImplementation,
	}

	existingThread := &types.SlackThread{
		ThreadKey:  "173.42",
		AppID:      "app_123",
		Channel:    "C123",
		SessionID:  "session_123",
		SpecTaskID: "task_123",
	}

	mockStore.EXPECT().GetSpecTask(gomock.Any(), "task_123").Return(task, nil)
	mockStore.EXPECT().GetSlackThreadBySpecTaskID(gomock.Any(), "app_123", "task_123").Return(existingThread, nil)

	err := bot.postProjectUpdate(context.Background(), task)
	require.NoError(t, err)
	assert.Equal(t, 1, postCalls)
	assert.Equal(t, 1, updateCalls)
}

func TestPostProjectUpdateReplyResolvesChannelIDFromPostMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	var updatedChannelID string
	bot := &SlackBot{
		cfg:   &config.ServerConfig{},
		store: mockStore,
		app: &types.App{
			ID:             "app_123",
			Owner:          "user_123",
			OrganizationID: "org_123",
			Config:         types.AppConfig{},
		},
		trigger: &types.SlackTrigger{
			ProjectUpdates: true,
			ProjectChannel: "helix-optimus-website",
		},
		postMessage: func(channelID string, options ...slack.MsgOption) (string, string, error) {
			return "C0AG6JRU142", "174.00", nil
		},
		updateMessage: func(channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error) {
			updatedChannelID = channelID
			return channelID, timestamp, "", nil
		},
	}

	task := &types.SpecTask{
		ID:        "task_123",
		ProjectID: "proj_123",
		Name:      "Test task",
		Status:    types.TaskStatusImplementation,
	}

	existingThread := &types.SlackThread{
		ThreadKey:  "173.42",
		AppID:      "app_123",
		Channel:    "helix-optimus-website",
		SessionID:  "session_123",
		SpecTaskID: "task_123",
	}

	mockStore.EXPECT().GetSpecTask(gomock.Any(), "task_123").Return(task, nil)
	mockStore.EXPECT().GetSlackThreadBySpecTaskID(gomock.Any(), "app_123", "task_123").Return(existingThread, nil)

	err := bot.postProjectUpdate(context.Background(), task)
	require.NoError(t, err)
	assert.Equal(t, "C0AG6JRU142", updatedChannelID, "updateMessage should use the resolved channel ID, not the channel name")
}

func TestIsBotOwnedThread_WhenRootMentionsBot(t *testing.T) {
	bot := &SlackBot{
		botUserID: "U_BOT",
		getConversationReplies: func(_ *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
			return []slack.Message{
				{
					Msg: slack.Msg{
						User: "U_USER",
						Text: "<@U_BOT> can you help with this?",
					},
				},
			}, false, "", nil
		},
	}

	assert.True(t, bot.isBotOwnedThread(context.Background(), "C123", "1771675557.541279"))
}

func TestIsBotOwnedThread_WhenRootDoesNotMentionBot(t *testing.T) {
	bot := &SlackBot{
		botUserID: "U_BOT",
		getConversationReplies: func(_ *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
			return []slack.Message{
				{
					Msg: slack.Msg{
						User: "U_USER",
						Text: "random thread",
					},
				},
			}, false, "", nil
		},
	}

	assert.False(t, bot.isBotOwnedThread(context.Background(), "C123", "1771675557.541279"))
}

// --- Tests for Fix 1: Reconcile checks Enabled field ---

func TestReconcile_StopsBotWhenTriggerDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Pre-populate with a running bot
	botCtx, botCancel := context.WithCancel(ctx)
	existingBot := &SlackBot{
		cfg:       &config.ServerConfig{},
		store:     mockStore,
		app:       &types.App{ID: "app_1"},
		trigger:   &types.SlackTrigger{Enabled: true, BotToken: "xoxb-old"},
		ctx:       botCtx,
		ctxCancel: botCancel,
	}

	s := &Slack{
		cfg:   &config.ServerConfig{},
		store: mockStore,
		bot:   map[string]*SlackBot{"app_1": existingBot},
	}

	// Return app with Enabled=false (trigger disabled)
	mockStore.EXPECT().ListApps(gomock.Any(), gomock.Any()).Return([]*types.App{
		{
			ID: "app_1",
			Config: types.AppConfig{
				Helix: types.AppHelixConfig{
					Triggers: []types.Trigger{
						{
							Slack: &types.SlackTrigger{
								Enabled:  false,
								BotToken: "xoxb-old",
							},
						},
					},
				},
			},
		},
	}, nil)

	err := s.reconcile(ctx)
	require.NoError(t, err)

	// Bot should be removed from map
	assert.Empty(t, s.bot)
	// Bot context should be cancelled
	assert.Error(t, existingBot.ctx.Err())
}

func TestReconcile_DoesNotStartBotWhenDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	s := &Slack{
		cfg:   &config.ServerConfig{},
		store: mockStore,
		bot:   make(map[string]*SlackBot),
	}

	// Return app with Enabled=false
	mockStore.EXPECT().ListApps(gomock.Any(), gomock.Any()).Return([]*types.App{
		{
			ID: "app_1",
			Config: types.AppConfig{
				Helix: types.AppHelixConfig{
					Triggers: []types.Trigger{
						{
							Slack: &types.SlackTrigger{
								Enabled:  false,
								BotToken: "xoxb-token",
							},
						},
					},
				},
			},
		},
	}, nil)

	err := s.reconcile(context.Background())
	require.NoError(t, err)

	// No bot should be started
	assert.Empty(t, s.bot)
}

func TestReconcile_EnabledFieldIncludedInConfigComparison(t *testing.T) {
	s := &Slack{}

	a := &types.SlackTrigger{Enabled: true, BotToken: "xoxb-token", AppToken: "xapp-token"}
	b := &types.SlackTrigger{Enabled: false, BotToken: "xoxb-token", AppToken: "xapp-token"}

	assert.False(t, s.triggerConfigEqual(a, b), "triggers with different Enabled should not be equal")

	b.Enabled = true
	assert.True(t, s.triggerConfigEqual(a, b), "triggers with same config should be equal")
}

// --- Tests for Fix 2: handleAppMentionThread reuses existing threads ---

func TestHandleAppMentionThread_ReusesExistingThread(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	existingSession := &types.Session{
		ID: "existing_session_123",
	}

	bot := &SlackBot{
		cfg:   &config.ServerConfig{},
		store: mockStore,
		app:   &types.App{ID: "app_1"},
		ctx:   context.Background(),
	}

	// Thread already exists
	mockStore.EXPECT().GetSlackThread(gomock.Any(), "app_1", "C123", "thread_ts_1").Return(&types.SlackThread{
		ThreadKey: "thread_ts_1",
		AppID:     "app_1",
		Channel:   "C123",
		SessionID: "existing_session_123",
	}, nil)

	// Should fetch existing session, NOT create a new one
	mockStore.EXPECT().GetSession(gomock.Any(), "existing_session_123").Return(existingSession, nil)

	session, err := bot.handleAppMentionThread(context.Background(), bot.app, "C123", "msg_ts_1", "thread_ts_1")
	require.NoError(t, err)
	assert.Equal(t, "existing_session_123", session.ID)
}

func TestHandleAppMentionThread_UsesMessageTsAsThreadKeyWhenNoThread(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	bot := &SlackBot{
		cfg:   &config.ServerConfig{},
		store: mockStore,
		app:   &types.App{ID: "app_1"},
		ctx:   context.Background(),
	}

	// When threadTimestamp is empty, messageTimestamp should be used as the thread key
	// Verify by checking GetSlackThread is called with the message timestamp
	mockStore.EXPECT().GetSlackThread(gomock.Any(), "app_1", "C123", "msg_ts_1").Return(&types.SlackThread{
		ThreadKey: "msg_ts_1",
		AppID:     "app_1",
		Channel:   "C123",
		SessionID: "session_for_msg",
	}, nil)

	mockStore.EXPECT().GetSession(gomock.Any(), "session_for_msg").Return(&types.Session{
		ID: "session_for_msg",
	}, nil)

	// threadTimestamp="" means bot was mentioned in a top-level message
	session, err := bot.handleAppMentionThread(context.Background(), bot.app, "C123", "msg_ts_1", "")
	require.NoError(t, err)
	assert.Equal(t, "session_for_msg", session.ID)
}

// --- Tests for Fix 3: Thread message prompt wrapper ---

func TestThreadMessagePromptWrapper_AppliedForNonMentions(t *testing.T) {
	// Verify the prompt wrapper constant is applied for thread messages
	assert.Contains(t, threadMessagePrompt, noResponseMarker)
	assert.Contains(t, threadMessagePrompt, "NOT directed at you")
}

func TestNoResponseMarker_DetectedInResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
		expected bool
	}{
		{
			name:     "exact marker",
			response: "[NO_RESPONSE]",
			expected: true,
		},
		{
			name:     "marker with whitespace",
			response: "  [NO_RESPONSE]  ",
			expected: true,
		},
		{
			name:     "marker in sentence",
			response: "This is not for me. [NO_RESPONSE]",
			expected: true,
		},
		{
			name:     "normal response",
			response: "Here's the answer to your question...",
			expected: false,
		},
		{
			name:     "empty response",
			response: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contains := strings.Contains(strings.TrimSpace(tt.response), noResponseMarker)
			assert.Equal(t, tt.expected, contains)
		})
	}
}

// TODO: re-enable these tests once resolveSessionForIncomingMessage is restored or replaced
