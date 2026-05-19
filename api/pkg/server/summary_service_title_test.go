package server

import "testing"

func TestCleanGeneratedTitle(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"trims whitespace", "  Snappy spec-task titles  ", "Snappy spec-task titles"},
		{"strips wrapping quotes", `"Snappy spec-task titles"`, "Snappy spec-task titles"},
		{"strips single quotes", `'Generate titles'`, "Generate titles"},
		{"strips Title: prefix", "Title: Add dark mode toggle", "Add dark mode toggle"},
		{"strips Task: prefix", "Task: Fix login crash", "Fix login crash"},
		{"strips markdown H1 prefix", "# Generate snappy spec task titles", "Generate snappy spec task titles"},
		{"strips markdown H2 prefix", "## Fix the build", "Fix the build"},
		{"strips markdown and quotes together", `"# Add dark mode"`, "Add dark mode"},
		{"strips trailing period", "Refactor auth middleware.", "Refactor auth middleware"},
		{"strips trailing exclamation", "Ship it!", "Ship it"},
		{"truncates at word boundary when long", "Add a really long descriptive title that is definitely well over the sixty character cap", "Add a really long descriptive title that is definitely well"},
		{"hard-truncates with ellipsis when no word boundary", "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnop", "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcde..."},
		{"empty input returns empty", "", ""},
		{"only quotes returns empty", `""`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanGeneratedTitle(tc.in)
			if got != tc.want {
				t.Fatalf("cleanGeneratedTitle(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if len(got) > 60 {
				t.Fatalf("cleanGeneratedTitle(%q) returned %d chars, want ≤ 60", tc.in, len(got))
			}
		})
	}
}
