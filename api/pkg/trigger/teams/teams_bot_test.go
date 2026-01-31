package teams

import (
	"testing"

	"github.com/infracloudio/msbotbuilder-go/schema"
)

func TestConvertMarkdownToTeamsFormat(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		expected string
	}{
		{
			name:     "plain text",
			markdown: "This is plain text",
			expected: "This is plain text",
		},
		{
			name:     "bold text preserved",
			markdown: "This is **bold** text",
			expected: "This is **bold** text",
		},
		{
			name:     "italic text preserved",
			markdown: "This is *italic* text",
			expected: "This is *italic* text",
		},
		{
			name:     "code block preserved",
			markdown: "```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```",
			expected: "```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```",
		},
		{
			name:     "inline code preserved",
			markdown: "Use the `code` function",
			expected: "Use the `code` function",
		},
		{
			name:     "triple newlines reduced to double",
			markdown: "First paragraph\n\n\nSecond paragraph",
			expected: "First paragraph\n\nSecond paragraph",
		},
		{
			name:     "quadruple newlines reduced to double",
			markdown: "First paragraph\n\n\n\nSecond paragraph",
			expected: "First paragraph\n\nSecond paragraph",
		},
		{
			name:     "multiple occurrences of triple newlines",
			markdown: "First\n\n\nSecond\n\n\nThird",
			expected: "First\n\nSecond\n\nThird",
		},
		{
			name:     "double newlines preserved",
			markdown: "First paragraph\n\nSecond paragraph",
			expected: "First paragraph\n\nSecond paragraph",
		},
		{
			name:     "single newlines preserved",
			markdown: "Line 1\nLine 2",
			expected: "Line 1\nLine 2",
		},
		{
			name:     "list items preserved",
			markdown: "- Item 1\n- Item 2\n- Item 3",
			expected: "- Item 1\n- Item 2\n- Item 3",
		},
		{
			name:     "link preserved",
			markdown: "Check out [Teams](https://teams.microsoft.com)",
			expected: "Check out [Teams](https://teams.microsoft.com)",
		},
		{
			name:     "empty string",
			markdown: "",
			expected: "",
		},
		{
			name:     "only newlines",
			markdown: "\n\n\n\n",
			expected: "\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertMarkdownToTeamsFormat(tt.markdown)
			if result != tt.expected {
				t.Errorf("convertMarkdownToTeamsFormat() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestRemoveBotMention(t *testing.T) {
	// Create a minimal TeamsBot for testing
	bot := &TeamsBot{}

	tests := []struct {
		name     string
		activity schema.Activity
		expected string
	}{
		{
			name: "simple mention at start",
			activity: schema.Activity{
				Text: "<at>BotName</at> hello there",
			},
			expected: "hello there",
		},
		{
			name: "mention in middle",
			activity: schema.Activity{
				Text: "Hey <at>BotName</at> how are you?",
			},
			expected: "Hey  how are you?",
		},
		{
			name: "mention at end",
			activity: schema.Activity{
				Text: "Hello <at>BotName</at>",
			},
			expected: "Hello",
		},
		{
			name: "multiple mentions",
			activity: schema.Activity{
				Text: "<at>Bot1</at> and <at>Bot2</at> hello",
			},
			expected: "and  hello",
		},
		{
			name: "no mention",
			activity: schema.Activity{
				Text: "Just a regular message",
			},
			expected: "Just a regular message",
		},
		{
			name: "empty text",
			activity: schema.Activity{
				Text: "",
			},
			expected: "",
		},
		{
			name: "only mention",
			activity: schema.Activity{
				Text: "<at>BotName</at>",
			},
			expected: "",
		},
		{
			name: "mention with special characters in name",
			activity: schema.Activity{
				Text: "<at>Bot Name (Test)</at> hello",
			},
			expected: "hello",
		},
		{
			name: "mention with numbers",
			activity: schema.Activity{
				Text: "<at>Bot123</at> test message",
			},
			expected: "test message",
		},
		{
			name: "incomplete mention tag not removed",
			activity: schema.Activity{
				Text: "<at>BotName hello",
			},
			expected: "<at>BotName hello",
		},
		{
			name: "mention with whitespace around",
			activity: schema.Activity{
				Text: "   <at>BotName</at>   hello   ",
			},
			expected: "hello",
		},
		{
			name: "nested at tags - inner one removed",
			activity: schema.Activity{
				Text: "Hello <at>Bot <at>Nested</at> Name</at> world",
			},
			// The regex will match <at>Nested</at> first
			expected: "Hello <at>Bot  Name</at> world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bot.removeBotMention(tt.activity)
			if result != tt.expected {
				t.Errorf("removeBotMention() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTokenCacheInitialization(t *testing.T) {
	bot := NewTeamsBot(nil, nil, nil, nil, nil)

	if bot.tokenCache == nil {
		t.Error("tokenCache should be initialized")
	}

	if bot.tokenCache.token != "" {
		t.Errorf("tokenCache.token should be empty, got %q", bot.tokenCache.token)
	}

	if !bot.tokenCache.expiresAt.IsZero() {
		t.Errorf("tokenCache.expiresAt should be zero, got %v", bot.tokenCache.expiresAt)
	}
}
