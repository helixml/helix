package claude

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/agent"
)

// Workspace is the agent.WorkspaceSync implementation for the local
// `claude` runtime. It writes files into <envsDir>/<workerID>/<name>
// — the same directory the spawner exec's claude in — so role and
// identity edits land on disk between activations without waiting for
// the spawner's projection step on the next run.
//
// The spawner re-projects role.md / identity.md / worker-policy.md
// from the DB at the start of every activation as a backstop, so a
// missed MirrorFile is recoverable. The WorkspaceSync push is just to
// keep the on-disk view fresh between activations.
type Workspace struct {
	EnvsDir string
}

// NewWorkspace returns a Workspace anchored at envsDir. Each Worker
// has a sibling subdirectory created by HireWorker.
func NewWorkspace(envsDir string) *Workspace {
	return &Workspace{EnvsDir: envsDir}
}

// MirrorFile writes content to <envsDir>/<workerID>/<name>. `name`
// must satisfy agent.ValidateWorkspaceName (no absolute paths, no
// upward traversal). `message` is unused by this backend (no commit
// log on the local filesystem).
//
// Renamed from PublishFile per ADR-0001 §7.
func (w *Workspace) MirrorFile(_ context.Context, workerID worker.ID, name, content, _ string) error {
	if w.EnvsDir == "" {
		return errors.New("claude workspace: EnvsDir is empty")
	}
	if workerID == "" {
		return errors.New("claude workspace: workerID is empty")
	}
	if err := agent.ValidateWorkspaceName(name); err != nil {
		return fmt.Errorf("claude workspace: %w", err)
	}
	envDir := filepath.Join(w.EnvsDir, string(workerID))
	full := filepath.Clean(filepath.Join(envDir, name))
	// Belt-and-braces: even after ValidateWorkspaceName, double-check
	// the resolved path stays inside the Worker's env dir.
	rel, err := filepath.Rel(envDir, full)
	if err != nil || rel == ".." || (len(rel) >= 3 && rel[:3] == ".."+string(os.PathSeparator)) {
		return fmt.Errorf("claude workspace: name %q escapes env dir", name)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return fmt.Errorf("claude workspace: mkdir: %w", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		return fmt.Errorf("claude workspace: write %q: %w", full, err)
	}
	return nil
}

// Compile-time check.
var _ agent.WorkspaceSync = (*Workspace)(nil)
