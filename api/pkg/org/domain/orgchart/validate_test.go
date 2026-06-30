package orgchart

import (
	"testing"
	"time"
)

func TestValidID(t *testing.T) {
	t.Parallel()
	ok := []string{"w-owner", "aaa", "w-mark-2", "r-software-engineer", "w-3f2a1b9c-uuid", "s-transcript-w-owner"}
	for _, id := range ok {
		if err := ValidID(id); err != nil {
			t.Errorf("ValidID(%q) = %v, want nil", id, err)
		}
	}
	bad := []string{"", "../etc", "..", "a/b", `a\b`, "w-../x", "..\x00", "foo/../bar", "/abs"}
	for _, id := range bad {
		if err := ValidID(id); err == nil {
			t.Errorf("ValidID(%q) = nil, want error (path-injection vector)", id)
		}
	}
}

// TestNewBotRejectsTraversalID pins that the domain constructor refuses
// a path-traversal id, so a malicious id can never be stored and later
// fed to os.MkdirAll / os.RemoveAll on the bot's env dir.
func TestNewBotRejectsTraversalID(t *testing.T) {
	t.Parallel()
	now := time.Now()
	if _, err := NewBot("../../etc/cron.d/x", "content", nil, nil, now, "org-test"); err == nil {
		t.Fatal("NewBot with traversal id: want error")
	}
	if _, err := NewBot("a/b", "content", nil, nil, now, "org-test"); err == nil {
		t.Fatal("NewBot with separator id: want error")
	}
}
