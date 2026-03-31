package tui

import (
	"testing"
)

func TestCollapseThinkingTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no thinking tags",
			input:    "Hello world",
			expected: "Hello world",
		},
		{
			name:     "thinking tag",
			input:    "Before <thinking>internal reasoning here</thinking> After",
			expected: "Before  After",
		},
		{
			name:     "think tag",
			input:    "Start <think>let me think about this</think> End",
			expected: "Start  End",
		},
		{
			name:     "multiline thinking",
			input:    "Hello\n<thinking>\nline 1\nline 2\n</thinking>\nGoodbye",
			expected: "Hello\n\nGoodbye",
		},
		{
			name:     "unclosed tag strips to end",
			input:    "Before <thinking>never closed",
			expected: "Before",
		},
		{
			name:     "multiple thinking blocks",
			input:    "<think>first</think>middle<think>second</think>end",
			expected: "middleend",
		},
		{
			name:     "empty content",
			input:    "",
			expected: "",
		},
		{
			name:     "only thinking",
			input:    "<thinking>all internal</thinking>",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collapseThinkingTags(tt.input)
			if got != tt.expected {
				t.Errorf("collapseThinkingTags(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestStripToolCallHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strip tool call header",
			input:    "**Tool Call: Edit file**\nStatus: Completed\n\nactual content",
			expected: "actual content",
		},
		{
			name:     "strip diff header",
			input:    "Diff: /path/to/file\n+added\n-removed",
			expected: "+added\n-removed",
		},
		{
			name:     "no headers",
			input:    "just content\nmore content",
			expected: "just content\nmore content",
		},
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripToolCallHeaders(tt.input)
			if got != tt.expected {
				t.Errorf("stripToolCallHeaders(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestHasDiffLines(t *testing.T) {
	if !hasDiffLines("+added line\n-removed line") {
		t.Error("expected true for diff content")
	}
	if hasDiffLines("no diff here\njust text") {
		t.Error("expected false for non-diff content")
	}
	if hasDiffLines("+++header\n---header") {
		t.Error("expected false for diff headers only")
	}
}
