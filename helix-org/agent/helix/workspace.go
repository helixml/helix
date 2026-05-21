package helix

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/agent"
	"github.com/helixml/helix/helix-org/helix/helixclient"
	"github.com/helixml/helix/helix-org/store"
)

// Workspace is the agent.WorkspaceSync implementation that pushes
// canonical role / identity content to the helix-specs branch of a
// Worker's per-Worker repo. Each call resolves the target repo from
// the Worker's runtime state (set by ProjectApplier at first
// activation) and PUTs one file onto the configured branch at
// `workers/<workerID>/.context/<name>` — the same path layout
// ProjectApplier.republishWorkerFiles writes and the activation
// mandate tells the agent to `git pull` and `cat`.
//
// Workers that haven't been activated against a Helix project yet
// (RepoID == "") are no-ops; callers don't have to gate on activation
// status.
type Workspace struct {
	client helixclient.Client
	store  *store.Store
	branch string
	author string
	email  string

	// repoLocks serialises pushes to the same repo. Helix's git write
	// path is not concurrency-safe per repo (it pre-syncs, writes,
	// post-pushes against a single working copy on the Helix host).
	// Two simultaneous PutFile calls against the same repo race on
	// the working copy.
	mu        sync.Mutex
	repoLocks map[string]*sync.Mutex
}

// NewWorkspace constructs a Workspace that resolves repo IDs per
// call from the runtime-state sidecar. branch is the target branch
// (typically "helix-specs"); author/email are the commit metadata.
func NewWorkspace(client helixclient.Client, st *store.Store, branch, author, email string) *Workspace {
	return &Workspace{
		client:    client,
		store:     st,
		branch:    branch,
		author:    author,
		email:     email,
		repoLocks: map[string]*sync.Mutex{},
	}
}

// MirrorFile satisfies agent.WorkspaceSync. `name` is the logical
// filename for this Worker (e.g. "role.md"); the Helix backend writes
// it at `workers/<workerID>/.context/<name>` on the helix-specs
// branch. Returns nil for Workers that aren't yet bound to a Helix
// project — callers don't have to gate on activation status.
//
// Renamed from PublishFile per ADR-0001 §7.
func (w *Workspace) MirrorFile(ctx context.Context, workerID worker.ID, name, content, message string) error {
	if workerID == "" {
		return errors.New("helix workspace: workerID is empty")
	}
	if err := agent.ValidateWorkspaceName(name); err != nil {
		return fmt.Errorf("helix workspace: %w", err)
	}
	state, err := LoadState(ctx, w.store, workerID)
	if err != nil {
		return fmt.Errorf("helix workspace: load state %q: %w", workerID, err)
	}
	if state.RepoID == "" {
		// Worker not yet bound to a Helix project — silently skip.
		// First activation will populate the project and write the
		// canonical files; this branch is for updates that happen
		// before the first activation completes.
		return nil
	}
	repoPath := "workers/" + string(workerID) + "/.context/" + name
	if message == "" {
		message = fmt.Sprintf("update %s", repoPath)
	}
	lock := w.lockFor(state.RepoID)
	lock.Lock()
	defer lock.Unlock()
	if err := w.client.PutFile(ctx, state.RepoID, helixclient.PutFileRequest{
		Path:    repoPath,
		Branch:  w.branch,
		Message: message,
		Author:  w.author,
		Email:   w.email,
		Content: content,
	}); err != nil {
		return err
	}
	// Invalidate the Worker's warm Helix chat session so the next
	// activation opens a fresh one — which means a fresh Claude Code
	// context that re-reads role.md / identity.md from scratch rather
	// than reusing the prior turn's cached content. Warm-session reuse
	// is what makes routine activations fast, but it's also why a live
	// role edit otherwise has no effect: Claude keeps acting on the
	// content it cat'd on the first activation. Only invalidate on
	// canonical role/identity edits (the two filenames the activation
	// mandate tells the agent to re-read) — checkpoint pushes or other
	// in-worker writes keep the session warm.
	if name == "role.md" || name == "identity.md" {
		if err := SaveSession(ctx, w.store, workerID, ""); err != nil {
			// Non-fatal: the next activation will still pull the new
			// content; it just won't re-read it from a fresh context.
			return nil
		}
	}
	return nil
}

func (w *Workspace) lockFor(repoID string) *sync.Mutex {
	w.mu.Lock()
	defer w.mu.Unlock()
	if l, ok := w.repoLocks[repoID]; ok {
		return l
	}
	l := &sync.Mutex{}
	w.repoLocks[repoID] = l
	return l
}

// Compile-time check.
var _ agent.WorkspaceSync = (*Workspace)(nil)
