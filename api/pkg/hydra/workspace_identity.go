package hydra

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// workspaceMountDestination is where a desktop's workspace is mounted. Every
// harness keeps its session store under it (.opencode-state, .claude-state,
// QWEN_HOME, …), so when this directory is new, every thread the API has on
// record for the session is gone from the agent's side.
const workspaceMountDestination = "/home/retro/work"

// workspaceIDFile holds the workspace's identity. A single file at the root —
// rather than a directory — keeps hydra (root) from leaving root-owned
// directories in the agent's home.
const workspaceIDFile = ".helix-workspace-id"

// workspaceIdentity is what the API needs to tell a surviving workspace from a
// recreated one. ID is stable for the lifetime of the directory; Created
// reports that this call made the directory.
type workspaceIdentity struct {
	ID      string
	Created bool
}

// prepareWorkspaceIdentity must run before prepareMountSources, which creates
// missing mount sources and would hide that the workspace was new. Returns the
// zero identity for containers without a workspace bind mount.
func prepareWorkspaceIdentity(mounts []MountConfig) (workspaceIdentity, error) {
	var root string
	for _, m := range mounts {
		if m.Type != "volume" && m.Destination == workspaceMountDestination && m.Source != "" {
			root = m.Source
			break
		}
	}
	if root == "" {
		return workspaceIdentity{}, nil
	}

	var identity workspaceIdentity
	if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
		identity.Created = true
	} else if err != nil {
		return workspaceIdentity{}, fmt.Errorf("stat workspace %s: %w", root, err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return workspaceIdentity{}, fmt.Errorf("create workspace %s: %w", root, err)
	}

	path := filepath.Join(root, workspaceIDFile)
	data, err := os.ReadFile(path)
	switch {
	case err == nil && strings.TrimSpace(string(data)) != "":
		identity.ID = strings.TrimSpace(string(data))
		return identity, nil
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return workspaceIdentity{}, fmt.Errorf("read workspace id %s: %w", path, err)
	}

	identity.ID = uuid.NewString()
	if err := os.WriteFile(path, []byte(identity.ID+"\n"), 0o644); err != nil {
		return workspaceIdentity{}, fmt.Errorf("write workspace id %s: %w", path, err)
	}
	return identity, nil
}
