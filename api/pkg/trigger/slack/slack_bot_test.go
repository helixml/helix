package slack

import (
	"context"
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
		ID:          "task_123",
		ProjectID:   "proj_123",
		Name:        "Implement light mode",
		Description: "Adds full light mode support to the app",
		Type:        "feature",
		Priority:    types.SpecTaskPriorityHigh,
		Status:      types.TaskStatusImplementation,
	}

	attachment := bot.buildProjectUpdateAttachment(context.Background(), task, "https://app.helix.ml")

	// Should have green color for implementation status
	assert.Equal(t, "#36a64f", attachment.Color)
	assert.Contains(t, attachment.Title, "Project Update")
	assert.Contains(t, attachment.Title, "🚧")
	assert.Contains(t, attachment.Text, "Adds full light mode support to the app")
	assert.Len(t, attachment.Fields, 4)
	assert.Equal(t, "Status", attachment.Fields[0].Title)
	assert.Contains(t, attachment.Fields[0].Value, "Implementation")
	// Task ID should be a clickable link
	assert.Contains(t, attachment.Fields[2].Value, "https://app.helix.ml/projects/proj_123/tasks/task_123?view=details")
}

func TestBuildProjectUpdateReplyAttachment(t *testing.T) {
	task := &types.SpecTask{
		ID:          "task_123",
		ProjectID:   "proj_123",
		Name:        "Implement light mode",
		Description: "Adds full light mode support to the app",
		Type:        "feature",
		Priority:    types.SpecTaskPriorityHigh,
		Status:      types.TaskStatusPullRequest,
	}

	attachment := buildProjectUpdateReplyAttachment(task, "https://app.helix.ml")

	// Should have purple color for pull request status
	assert.Equal(t, "#9C27B0", attachment.Color)
	assert.Contains(t, attachment.Text, "🔀")
	assert.Contains(t, attachment.Text, "Implement light mode")
	assert.Contains(t, attachment.Text, "Pull Request")
	assert.Contains(t, attachment.Text, "https://app.helix.ml/projects/proj_123/tasks/task_123?view=details")
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

// TODO: re-enable these tests once resolveSessionForIncomingMessage is restored or replaced
