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

func TestProcessCitationsForChatWithLinks(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		documentIDs map[string]string
		linkFormat  LinkFormat
		expected    string
	}{
		{
			name:        "no document IDs - falls back to no links",
			input:       "Content [DOC_ID:abc].\n\n<excerpts><excerpt><document_id>abc</document_id><snippet>Snippet text</snippet></excerpt></excerpts>",
			documentIDs: nil,
			linkFormat:  LinkFormatMarkdown,
			expected:    "Content [1].\n\n---\n**Sources:**\n[1] Snippet text\n",
		},
		{
			name:  "with SharePoint URL - markdown format",
			input: "Content [DOC_ID:abc123].\n\n<excerpts><excerpt><document_id>abc123</document_id><snippet>Emergency procedures</snippet></excerpt></excerpts>",
			documentIDs: map[string]string{
				"https://sharepoint.com/docs/emergency.md": "abc123",
			},
			linkFormat: LinkFormatMarkdown,
			expected:   "Content [1].\n\n---\n**Sources:**\n[[1]](https://sharepoint.com/docs/emergency.md) Emergency procedures\n",
		},
		{
			name:  "with SharePoint URL - slack format",
			input: "Content [DOC_ID:abc123].\n\n<excerpts><excerpt><document_id>abc123</document_id><snippet>Emergency procedures</snippet></excerpt></excerpts>",
			documentIDs: map[string]string{
				"https://sharepoint.com/docs/emergency.md": "abc123",
			},
			linkFormat: LinkFormatSlack,
			expected:   "Content [1].\n\n---\n**Sources:**\n<https://sharepoint.com/docs/emergency.md|[1]> Emergency procedures\n",
		},
		{
			name:  "multiple URLs with links",
			input: "First [DOC_ID:doc1]. Second [DOC_ID:doc2].\n\n<excerpts><excerpt><document_id>doc1</document_id><snippet>First doc</snippet></excerpt><excerpt><document_id>doc2</document_id><snippet>Second doc</snippet></excerpt></excerpts>",
			documentIDs: map[string]string{
				"https://sharepoint.com/first.md":  "doc1",
				"https://sharepoint.com/second.md": "doc2",
			},
			linkFormat: LinkFormatMarkdown,
			expected:   "First [1]. Second [2].\n\n---\n**Sources:**\n[[1]](https://sharepoint.com/first.md) First doc\n[[2]](https://sharepoint.com/second.md) Second doc\n",
		},
		{
			name:  "non-URL path - no link generated",
			input: "Content [DOC_ID:abc].\n\n<excerpts><excerpt><document_id>abc</document_id><snippet>Local file</snippet></excerpt></excerpts>",
			documentIDs: map[string]string{
				"dev/apps/app123/document.pdf": "abc",
			},
			linkFormat: LinkFormatMarkdown,
			expected:   "Content [1].\n\n---\n**Sources:**\n[1] Local file\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ProcessCitationsForChatWithLinks(tt.input, tt.documentIDs, tt.linkFormat)
			if result != tt.expected {
				t.Errorf("ProcessCitationsForChatWithLinks() =\n%q\nwant:\n%q", result, tt.expected)
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
			name:     "only DOC_ID markers without excerpts",
			input:    "Point one [DOC_ID:abc]. Point two [DOC_ID:def].",
			expected: "Point one [1]. Point two [2].\n\n---\n**Sources:**\n[1] Source document\n[2] Source document\n",
		},
		{
			name:     "DOC_ID and excerpts together",
			input:    "Content with citation [DOC_ID:abc123].\n\n<excerpts><excerpt><document_id>abc123</document_id><snippet>Source text from the document</snippet></excerpt></excerpts>",
			expected: "Content with citation [1].\n\n---\n**Sources:**\n[1] Source text from the document\n",
		},
		{
			name:     "multiple citations with excerpts",
			input:    "First point [DOC_ID:doc1]. Second point [DOC_ID:doc2].\n\n<excerpts><excerpt><document_id>doc1</document_id><snippet>First source snippet</snippet></excerpt><excerpt><document_id>doc2</document_id><snippet>Second source snippet</snippet></excerpt></excerpts>",
			expected: "First point [1]. Second point [2].\n\n---\n**Sources:**\n[1] First source snippet\n[2] Second source snippet\n",
		},
		{
			name:     "real world Teams example",
			input:    "The evacuation points are at Stairwell A [DOC_ID:357638e68b]. Immediate evacuation required [DOC_ID:25c1dfb246].\n\n<excerpts><excerpt><document_id>357638e68b</document_id><snippet>Emergency Exit: Stairwell A (North)</snippet></excerpt><excerpt><document_id>25c1dfb246</document_id><snippet>Immediate evacuation is required for all personnel</snippet></excerpt></excerpts>",
			expected: "The evacuation points are at Stairwell A [1]. Immediate evacuation required [2].\n\n---\n**Sources:**\n[1] Emergency Exit: Stairwell A (North)\n[2] Immediate evacuation is required for all personnel\n",
		},
		{
			name:     "long snippet gets truncated",
			input:    "Citation here [DOC_ID:long].\n\n<excerpts><excerpt><document_id>long</document_id><snippet>This is a very long snippet that exceeds one hundred characters and should be truncated to keep the references section readable</snippet></excerpt></excerpts>",
			expected: "Citation here [1].\n\n---\n**Sources:**\n[1] This is a very long snippet that exceeds one hundred characters and should be truncated to keep t...\n",
		},
		{
			name:     "unclosed excerpts tag (streaming) - no references added",
			input:    "Content here [DOC_ID:abc].\n\n<excerpts><excerpt><document_id>abc",
			expected: "Content here [1].\n\n---\n**Sources:**\n[1] Source document\n",
		},
		{
			name:     "repeated citation uses same number",
			input:    "First mention [DOC_ID:same]. Second mention [DOC_ID:same]. Different [DOC_ID:other].\n\n<excerpts><excerpt><document_id>same</document_id><snippet>Same document</snippet></excerpt><excerpt><document_id>other</document_id><snippet>Other document</snippet></excerpt></excerpts>",
			expected: "First mention [1]. Second mention [1]. Different [2].\n\n---\n**Sources:**\n[1] Same document\n[2] Other document\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ProcessCitationsForChat(tt.input)
			if result != tt.expected {
				t.Errorf("ProcessCitationsForChat() =\n%q\nwant:\n%q", result, tt.expected)
			}
		})
	}
}
