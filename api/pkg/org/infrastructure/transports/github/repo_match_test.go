package github

import "testing"

func TestBranchAllowed(t *testing.T) {
	cases := []struct {
		patterns []string
		branch   string
		want     bool
	}{
		{nil, "main", true},                                // no filter → all branches
		{[]string{"*"}, "main", true},                      // wildcard → all branches
		{[]string{"*"}, "feature/x", true},                 // wildcard → all branches
		{[]string{"main"}, "", true},                       // non-branch event → unfiltered
		{[]string{"main"}, "main", true},                   // exact
		{[]string{"main"}, "MAIN", true},                   // case-insensitive
		{[]string{"main"}, "dev", false},                   // exact miss
		{[]string{"release/*"}, "release/1.2", true},       // prefix glob
		{[]string{"release/*"}, "release", true},           // glob matches bare prefix
		{[]string{"release/*"}, "releases/x", false},       // not a child of release/
		{[]string{"main", "release/*"}, "release/9", true}, // any-of
		{[]string{"main"}, "feature/x", false},
	}
	for _, c := range cases {
		if got := branchAllowed(c.patterns, c.branch); got != c.want {
			t.Errorf("branchAllowed(%v, %q) = %v, want %v", c.patterns, c.branch, got, c.want)
		}
	}
}

func TestPayloadBranch(t *testing.T) {
	if got := payloadBranch("push", map[string]any{"ref": "refs/heads/main"}); got != "main" {
		t.Errorf("push refs/heads/main -> %q, want main", got)
	}
	if got := payloadBranch("push", map[string]any{"ref": "refs/tags/v1"}); got != "" {
		t.Errorf("tag push -> %q, want empty", got)
	}
	if got := payloadBranch("create", map[string]any{"ref": "feature/x", "ref_type": "branch"}); got != "feature/x" {
		t.Errorf("create branch -> %q, want feature/x", got)
	}
	if got := payloadBranch("issues", map[string]any{}); got != "" {
		t.Errorf("issues -> %q, want empty", got)
	}
}

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
