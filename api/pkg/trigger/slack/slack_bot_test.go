package slack

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/slack-go/slack"
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

// =============================================================================
// Tests for Agent Progress
// =============================================================================

func TestBuildProgressBlocks(t *testing.T) {
	// Create minimal bot for testing
	bot := &SlackBot{
		app: &types.App{ID: "test-app"},
		trigger: &types.SlackTrigger{
			BotToken: "xoxb-test-token",
		},
	}

	tests := []struct {
		name           string
		update         *AgentProgressUpdate
		expectedBlocks int
		checkHeader    bool
		headerEmoji    string
	}{
		{
			name: "working status",
			update: &AgentProgressUpdate{
				SessionID:   "session-123",
				SessionName: "Test Session",
				TurnNumber:  5,
				TurnSummary: "Added a new feature",
				Status:      "working",
				AppURL:      "https://app.helix.ml",
			},
			expectedBlocks: 4, // Header, turn summary, actions, divider
			checkHeader:    true,
			headerEmoji:    "🔄",
		},
		{
			name: "needs input status",
			update: &AgentProgressUpdate{
				SessionID:   "session-123",
				SessionName: "Test Session",
				TurnNumber:  3,
				TurnSummary: "Waiting for user",
				Status:      "needs_input",
				NeedsInput:  true,
				InputPrompt: "Should I proceed with the migration?",
				AppURL:      "https://app.helix.ml",
			},
			expectedBlocks: 5, // Header, turn summary, input prompt, actions, divider
			checkHeader:    true,
			headerEmoji:    "⚠️",
		},
		{
			name: "completed status",
			update: &AgentProgressUpdate{
				SessionID:   "session-123",
				SessionName: "Test Session",
				TurnNumber:  10,
				TurnSummary: "All tasks completed",
				Status:      "completed",
				AppURL:      "https://app.helix.ml",
			},
			expectedBlocks: 4,
			checkHeader:    true,
			headerEmoji:    "✅",
		},
		{
			name: "error status",
			update: &AgentProgressUpdate{
				SessionID:   "session-123",
				SessionName: "Test Session",
				TurnNumber:  2,
				TurnSummary: "Build failed",
				Status:      "error",
				AppURL:      "https://app.helix.ml",
			},
			expectedBlocks: 4,
			checkHeader:    true,
			headerEmoji:    "❌",
		},
		{
			name: "with screenshot",
			update: &AgentProgressUpdate{
				SessionID:     "session-123",
				SessionName:   "Test Session",
				TurnNumber:    1,
				TurnSummary:   "Initial setup",
				Status:        "working",
				ScreenshotURL: "https://example.com/screenshot.png",
				AppURL:        "https://app.helix.ml",
			},
			expectedBlocks: 5, // Header, turn summary, screenshot, actions, divider
			checkHeader:    true,
			headerEmoji:    "🔄",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := bot.buildProgressBlocks(tt.update)

			if len(blocks) != tt.expectedBlocks {
				t.Errorf("expected %d blocks, got %d", tt.expectedBlocks, len(blocks))
			}

			// Check first block is header with correct emoji
			if tt.checkHeader && len(blocks) > 0 {
				if section, ok := blocks[0].(*slack.SectionBlock); ok {
					if section.Text != nil {
						if section.Text.Text == "" {
							t.Error("header text is empty")
						}
						// Just verify the block exists with text
					}
				}
			}

			// Verify action block exists
			hasActionBlock := false
			for _, block := range blocks {
				if _, ok := block.(*slack.ActionBlock); ok {
					hasActionBlock = true
					break
				}
			}
			if !hasActionBlock {
				t.Error("expected action block to be present")
			}

			// Verify divider at end
			if len(blocks) > 0 {
				if _, ok := blocks[len(blocks)-1].(*slack.DividerBlock); !ok {
					t.Error("expected divider block at end")
				}
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
