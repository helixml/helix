package chat

import "testing"

// TestPickTitleHonoursPriority pins the rule:
// custom-title (user-set) > ai-title (claude-generated) > first user prompt.
// This is the contract recents render relies on.
func TestPickTitleHonoursPriority(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                       string
		custom, ai, fallback, want string
	}{
		{"custom wins over ai", "Manual", "Generated", "first prompt", "Manual"},
		{"custom wins alone", "Manual", "", "", "Manual"},
		{"ai wins over fallback", "", "Generated", "first prompt", "Generated"},
		{"fallback only", "", "", "first prompt", "first prompt"},
		{"all empty", "", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := pickTitle(tc.custom, tc.ai, tc.fallback)
			if got != tc.want {
				t.Fatalf("pickTitle(%q,%q,%q) = %q, want %q", tc.custom, tc.ai, tc.fallback, got, tc.want)
			}
		})
	}
}
