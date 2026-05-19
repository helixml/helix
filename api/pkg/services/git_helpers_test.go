package services

import "testing"

func TestSpecTitleFromRequirements(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "standard H1 with Requirements prefix",
			content: "# Requirements: Add Dark Mode Toggle\n\nbody...",
			want:    "Add Dark Mode Toggle",
		},
		{
			name:    "H1 without prefix",
			content: "# Snappy Spec-Task Titles\n\nbody...",
			want:    "Snappy Spec-Task Titles",
		},
		{
			name:    "skips leading blank lines",
			content: "\n\n# Requirements: Refactor Auth\n",
			want:    "Refactor Auth",
		},
		{
			name:    "drops bare Requirements",
			content: "# Requirements\n\nfollow-up line",
			want:    "follow-up line",
		},
		{
			name:    "empty content yields empty",
			content: "",
			want:    "",
		},
		{
			name:    "no headings, no content",
			content: "   \n\n   ",
			want:    "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SpecTitleFromRequirements(tc.content)
			if got != tc.want {
				t.Fatalf("SpecTitleFromRequirements:\n  in:   %q\n  got:  %q\n  want: %q", tc.content, got, tc.want)
			}
		})
	}
}
