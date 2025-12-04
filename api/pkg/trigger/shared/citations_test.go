package shared

import (
	"testing"
)

func TestConvertDocIDsToNumberedCitations(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no citations",
			input:    "This is a simple message with no citations.",
			expected: "This is a simple message with no citations.",
		},
		{
			name:     "single citation",
			input:    "The emergency exits are located at Stairwell A [DOC_ID:357638e68b].",
			expected: "The emergency exits are located at Stairwell A [1].",
		},
		{
			name:     "multiple different citations",
			input:    "First point [DOC_ID:abc123]. Second point [DOC_ID:def456]. Third point [DOC_ID:ghi789].",
			expected: "First point [1]. Second point [2]. Third point [3].",
		},
		{
			name:     "repeated citation gets same number",
			input:    "First mention [DOC_ID:abc123]. Second mention of same doc [DOC_ID:abc123]. Different doc [DOC_ID:def456].",
			expected: "First mention [1]. Second mention of same doc [1]. Different doc [2].",
		},
		{
			name:     "citation in multiline text",
			input:    "Line 1 [DOC_ID:doc1].\nLine 2 [DOC_ID:doc2].\nLine 3 [DOC_ID:doc1].",
			expected: "Line 1 [1].\nLine 2 [2].\nLine 3 [1].",
		},
		{
			name:     "real world example",
			input:    "The evacuation points during a fire are determined by the emergency exits available on each floor. For Floor 1, the emergency exits are located at Stairwell A (North) and Stairwell B (South) [DOC_ID:357638e68b]. Immediate evacuation is required when a fire alarm is activated [DOC_ID:25c1dfb246].",
			expected: "The evacuation points during a fire are determined by the emergency exits available on each floor. For Floor 1, the emergency exits are located at Stairwell A (North) and Stairwell B (South) [1]. Immediate evacuation is required when a fire alarm is activated [2].",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertDocIDsToNumberedCitations(tt.input)
			if result != tt.expected {
				t.Errorf("ConvertDocIDsToNumberedCitations() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestProcessCitationsForChat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no citations or excerpts",
			input:    "This is a simple message.",
			expected: "This is a simple message.",
		},
		{
			name:     "only DOC_ID markers",
			input:    "Point one [DOC_ID:abc]. Point two [DOC_ID:def].",
			expected: "Point one [1]. Point two [2].",
		},
		{
			name:     "excerpts block removed",
			input:    "Main content here.\n\n<excerpts><excerpt><document_id>abc123</document_id><snippet>Some snippet</snippet></excerpt></excerpts>",
			expected: "Main content here.",
		},
		{
			name:     "DOC_ID and excerpts together",
			input:    "Content with citation [DOC_ID:abc123].\n\n<excerpts><excerpt><document_id>abc123</document_id><snippet>Source text</snippet></excerpt></excerpts>",
			expected: "Content with citation [1].",
		},
		{
			name:     "multiple excerpts removed",
			input:    "First part [DOC_ID:doc1].\n\n<excerpts><excerpt><document_id>doc1</document_id><snippet>Snippet 1</snippet></excerpt><excerpt><document_id>doc2</document_id><snippet>Snippet 2</snippet></excerpt></excerpts>\n\nSecond part [DOC_ID:doc2].",
			expected: "First part [1].\n\nSecond part [2].",
		},
		{
			name:     "real world Teams example",
			input:    "The evacuation points are at Stairwell A [DOC_ID:357638e68b]. Immediate evacuation required [DOC_ID:25c1dfb246].\n\n<excerpts><excerpt><document_id>357638e68b</document_id><snippet>Emergency Exit: Stairwell A</snippet></excerpt><excerpt><document_id>25c1dfb246</document_id><snippet>Immediate evacuation is required</snippet></excerpt></excerpts>",
			expected: "The evacuation points are at Stairwell A [1]. Immediate evacuation required [2].",
		},
		{
			name:     "unclosed excerpts tag (streaming)",
			input:    "Content here [DOC_ID:abc].\n\n<excerpts><excerpt><document_id>abc",
			expected: "Content here [1].",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ProcessCitationsForChat(tt.input)
			if result != tt.expected {
				t.Errorf("ProcessCitationsForChat() = %q, want %q", result, tt.expected)
			}
		})
	}
}
