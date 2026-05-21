package claude

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceWritesUnderEnvDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := NewWorkspace(dir)
	if err := w.PublishFile(context.Background(), "w-eng", "role.md", "# Role", ""); err != nil {
		t.Fatalf("publish: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "w-eng", "role.md")) //nolint:gosec // dir is t.TempDir()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "# Role" {
		t.Errorf("content = %q", got)
	}
}

func TestWorkspaceRejectsBadName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := NewWorkspace(dir)
	for _, bad := range []string{"", "/role.md", "../role.md", "a/../../b"} {
		if err := w.PublishFile(context.Background(), "w-eng", bad, "x", ""); err == nil {
			t.Errorf("name %q: expected error", bad)
		}
	}
}
