package github

import "testing"

func TestRepoMatches(t *testing.T) {
	cases := []struct {
		cfg, delivered string
		want           bool
	}{
		// exact, case-insensitive
		{"helixml/helix", "helixml/helix", true},
		{"helixml/helix", "HelixML/Helix", true},
		{"helixml/helix", "helixml/other", false},
		{"helixml/helix", "other/helix", false},
		// org wildcard
		{"winderai/*", "winderai/vision-rag-demo", true},
		{"winderai/*", "WinderAI/anything", true},
		{"winderai/*", "other/repo", false},
		{"winderai/*", "winderai", false}, // malformed delivered (no slash)
	}
	for _, c := range cases {
		if got := repoMatches(c.cfg, c.delivered); got != c.want {
			t.Errorf("repoMatches(%q, %q) = %v, want %v", c.cfg, c.delivered, got, c.want)
		}
	}
}
