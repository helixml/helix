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
