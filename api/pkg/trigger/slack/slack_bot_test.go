package slack

import (
	"testing"
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
