package helix

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// WorkspaceGit is the small slice of the helix git-repository
// servicer the helix-runtime workspace needs. *services.GitRepositoryService
// satisfies it; the production wiring in api/pkg/server/helix_org.go
// passes that concrete impl directly.
//
// We define a thin interface here rather than depending on
// *services.GitRepositoryService so workspace tests don't have to
// build a full GitRepositoryService.
//
// All file writes for Worker provisioning (canonical role.md /
// identity.md / agent.md plus any future per-Worker files) flow
// through the Workspace, which is the only place in the helix
// runtime that knows the on-branch path layout. WorkerProject
// delegates to Workspace for its first-apply file pushes rather
// than calling the git servicer directly.
type WorkspaceGit interface {
	CreateBranch(ctx context.Context, repoID, branchName, baseBranch string) error
	CreateOrUpdateFileContents(ctx context.Context, repoID, path, branch string, content []byte, commitMessage, authorName, authorEmail string) (string, error)
}

// Workspace is the runtime.WorkspaceSync implementation that pushes
// canonical role / identity content to the helix-specs branch of a
// Worker's per-Worker repo. Each call resolves the target repo from
// the Worker's runtime state (set by WorkerProject at first
// activation) and PUTs one file onto the configured branch at
// `workers/<workerID>/.context/<name>` — the same path layout
// WorkerProject.republishWorkerFiles writes and the activation
// mandate tells the agent to `git pull` and `cat`.
//
// Workers that haven't been activated against a Helix project yet
// (RepoID == "") are no-ops; callers don't have to gate on activation
// status.
type Workspace struct {
	git    WorkspaceGit
	store  *store.Store
	branch string
	author string
	email  string

	// repoLocks serialises pushes to the same repo. Helix's git write
	// path is not concurrency-safe per repo (it pre-syncs, writes,
	// post-pushes against a single working copy on the Helix host).
	// Two simultaneous CreateOrUpdateFileContents calls against the
	// same repo race on the working copy.
	mu        sync.Mutex
	repoLocks map[string]*sync.Mutex
}

// NewWorkspace constructs a Workspace that resolves repo IDs per
// call from the runtime-state sidecar. branch is the target branch
// (typically "helix-specs"); author/email are the commit metadata.
func NewWorkspace(git WorkspaceGit, st *store.Store, branch, author, email string) *Workspace {
	return &Workspace{
		git:       git,
		store:     st,
		branch:    branch,
		author:    author,
		email:     email,
		repoLocks: map[string]*sync.Mutex{},
	}
}

// MirrorFile satisfies runtime.WorkspaceSync. `name` is the logical
// filename for this Worker (e.g. "role.md"); the Helix backend writes
// it at `workers/<workerID>/.context/<name>` on the helix-specs
// branch. Returns nil for Workers that aren't yet bound to a Helix
// project — callers don't have to gate on activation status.
//
// Renamed from PublishFile per ADR-0001 §7.
func (w *Workspace) MirrorFile(ctx context.Context, orgID string, workerID orgchart.WorkerID, name, content, message string) error {
	if workerID == "" {
		return errors.New("helix workspace: workerID is empty")
	}
	if err := runtime.ValidateWorkspaceName(name); err != nil {
		return fmt.Errorf("helix workspace: %w", err)
	}
	state, err := LoadState(ctx, w.store, orgID, workerID)
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
	if err := w.WriteWorkerFile(ctx, workerID, state.RepoID, name, content, message); err != nil {
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
		if err := SaveSession(ctx, w.store, orgID, workerID, ""); err != nil {
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

// EnsureBranch creates the branch (idempotent — re-creating an
// existing branch is a 200). Used by WorkerProject before the first
// file push so the helix-specs branch exists.
func (w *Workspace) EnsureBranch(ctx context.Context, repoID, baseBranch string) error {
	if repoID == "" {
		return nil
	}
	return w.git.CreateBranch(ctx, repoID, w.branch, baseBranch)
}

// WriteOrgFile writes an org-wide file (no worker prefix) onto the
// Workspace's branch — `.context/<name>`. Used for the org-wide
// agent.md policy that every Worker reads.
func (w *Workspace) WriteOrgFile(ctx context.Context, repoID, name, content, message string) error {
	if repoID == "" {
		return nil
	}
	if err := runtime.ValidateWorkspaceName(name); err != nil {
		return fmt.Errorf("helix workspace: %w", err)
	}
	return w.writeAt(ctx, repoID, ".context/"+name, content, message)
}

// WriteWorkerFile writes a per-Worker file at
// `workers/<workerID>/.context/<name>`. Used by WorkerProject's
// first-apply path; MirrorFile is the public WorkspaceSync surface
// that wraps this with session-invalidation semantics.
func (w *Workspace) WriteWorkerFile(ctx context.Context, workerID orgchart.WorkerID, repoID, name, content, message string) error {
	if workerID == "" {
		return errors.New("helix workspace: workerID is empty")
	}
	if repoID == "" {
		return nil
	}
	if err := runtime.ValidateWorkspaceName(name); err != nil {
		return fmt.Errorf("helix workspace: %w", err)
	}
	return w.writeAt(ctx, repoID, "workers/"+string(workerID)+"/.context/"+name, content, message)
}

func (w *Workspace) writeAt(ctx context.Context, repoID, path, content, message string) error {
	if message == "" {
		message = fmt.Sprintf("update %s", path)
	}
	lock := w.lockFor(repoID)
	lock.Lock()
	defer lock.Unlock()
	_, err := w.git.CreateOrUpdateFileContents(ctx, repoID, path, w.branch, []byte(content), message, w.author, w.email)
	return err
}

// Compile-time check.
var _ runtime.WorkspaceSync = (*Workspace)(nil)
