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

func TestBuildProjectUpdateBlocks(t *testing.T) {
	task := &types.SpecTask{
		ID:          "task_123",
		ProjectID:   "proj_123",
		Name:        "Implement light mode",
		Description: "Adds full light mode support to the app",
		Type:        "feature",
		Priority:    types.SpecTaskPriorityHigh,
		Status:      types.TaskStatusImplementation,
	}

	blocks := buildProjectUpdateBlocks(task)
	require.Len(t, blocks, 3)

	header, ok := blocks[0].(*slack.HeaderBlock)
	require.True(t, ok)
	assert.Contains(t, header.Text.Text, "Project Update")
	assert.Contains(t, header.Text.Text, "🚧")

	section, ok := blocks[1].(*slack.SectionBlock)
	require.True(t, ok)
	require.NotNil(t, section.Text)
	assert.Contains(t, section.Text.Text, "Implement light mode")
	assert.Len(t, section.Fields, 4)
	assert.Contains(t, section.Fields[0].Text, "Implementation")
}

func TestPostProjectUpdateCreatesSessionAndSlackThread(t *testing.T) {
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

	mockStore.EXPECT().CreateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			assert.Equal(t, "task_123", session.Metadata.SpecTaskID)
			assert.Equal(t, "proj_123", session.Metadata.ProjectID)
			return &session, nil
		},
	)

	mockStore.EXPECT().GetSlackThread(gomock.Any(), "app_123", "C123", "173.42").Return(nil, store.ErrNotFound)
	mockStore.EXPECT().CreateSlackThread(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, thread *types.SlackThread) (*types.SlackThread, error) {
			assert.Equal(t, "app_123", thread.AppID)
			assert.Equal(t, "C123", thread.Channel)
			assert.Equal(t, "173.42", thread.ThreadKey)
			return thread, nil
		},
	)

	err := bot.postProjectUpdate(context.Background(), task)
	require.NoError(t, err)
}
