package services

import (
	"strings"
	"testing"
)

// commentStillPresent mirrors the matching decision in checkCommentResolution:
// a comment is auto-resolved only when its normalized quoted text is NOT found
// in the normalized document.
func commentStillPresent(doc, quoted string) bool {
	nq := normalizeForCommentMatch(quoted)
	if nq == "" {
		// Fail safe: can't confidently decide, treat as present (keep the comment).
		return true
	}
	return strings.Contains(normalizeForCommentMatch(doc), nq)
}

func TestNormalizeForCommentMatch(t *testing.T) {
	cases := []struct {
		name   string
		doc    string
		quoted string
		// present=true means the comment should be KEPT (quoted text found);
		// present=false means it should be auto-resolved (text genuinely gone).
		present bool
	}{
		{
			name:    "plain text present",
			doc:     "The system stores comments in the database.",
			quoted:  "stores comments in the database",
			present: true,
		},
		{
			name:    "bold formatting in source only",
			doc:     "The **quoted text** is important.",
			quoted:  "quoted text",
			present: true,
		},
		{
			name:    "inline code in source only",
			doc:     "Call `findQuotedTextPosition` to locate it.",
			quoted:  "findQuotedTextPosition",
			present: true,
		},
		{
			name:    "link renders to visible text",
			doc:     "See the [design document](https://example.com/x) for details.",
			quoted:  "design document",
			present: true,
		},
		{
			name:    "cross-paragraph selection with newline",
			doc:     "First paragraph line.\n\nSecond paragraph line.",
			quoted:  "First paragraph line. Second paragraph line.",
			present: true,
		},
		{
			name:    "list bullet marker in source only",
			doc:     "- First item\n- Second item",
			quoted:  "Second item",
			present: true,
		},
		{
			name:    "genuinely removed text is auto-resolved",
			doc:     "The document now says something completely different.",
			quoted:  "text that no longer exists anywhere",
			present: false,
		},
		{
			name:    "empty quote is kept (fail safe)",
			doc:     "Some content.",
			quoted:  "   ",
			present: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := commentStillPresent(tc.doc, tc.quoted)
			if got != tc.present {
				t.Errorf("commentStillPresent(%q, %q) = %v, want %v",
					tc.doc, tc.quoted, got, tc.present)
			}
		})
	}
}
