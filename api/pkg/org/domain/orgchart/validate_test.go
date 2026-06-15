package orgchart

import "testing"

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

// TestNewWorkerRejectsTraversalID pins that the domain constructors
// refuse a path-traversal id, so a malicious id can never be stored and
// later fed to os.MkdirAll / os.RemoveAll on the worker's env dir.
func TestNewWorkerRejectsTraversalID(t *testing.T) {
	t.Parallel()
	if _, err := NewAIWorker("../../etc/cron.d/x", "r-eng", "", "org-test"); err == nil {
		t.Fatal("NewAIWorker with traversal id: want error")
	}
	if _, err := NewHumanWorker("a/b", "r-eng", "", "org-test"); err == nil {
		t.Fatal("NewHumanWorker with separator id: want error")
	}
}
