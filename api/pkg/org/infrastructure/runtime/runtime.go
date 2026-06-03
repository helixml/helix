// Package runtime owns the ports that describe where an AI Worker
// physically executes. Two contracts live here today, lifted from
// helix-org/agent in B3d:
//
//   - Spawner: run a single activation and block until the agent
//     process exits.
//   - WorkspaceSync: mirror canonical Role / Identity content into
//     the runtime's per-Worker workspace.
//
// They are wired separately by the current callers (the dispatcher
// takes a Spawner; the tools.Deps takes a WorkspaceSync) because the
// helix-runtime constructors build them at different points with
// different dependencies. A unified `Runtime` interface combining
// both is a candidate follow-up — both contracts already satisfy one
// struct (runtimehelix's per-Worker types), so the unification is
// purely API-shape work.
//
// The sole concrete runtime lives at api/pkg/org/runtime/helix/
// (lifted in H1.0–H1.3d). The dev-only claude-subprocess runtime
// (helix-org/agent/claude) was deleted in B9.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

// Spawner runs an AI Worker's agent process for a single activation
// and BLOCKS until the process exits. The triggers slice tells the
// Spawner (and through it, the agent) why this activation is happening
// — first hire, or one or more events on subscribed Streams that
// arrived while a previous activation was running. The Dispatcher
// coalesces bursts so the slice is usually length 1, but the agent
// must handle longer slices when traffic queues up.
//
// Spawners are typically called from inside a Dispatcher that
// serialises calls per-Worker; callers must not invoke a Spawner for
// the same Worker concurrently.
//
// The zero value — nil — means "no process will be spawned", which
// is correct for tests and for HumanWorker activations.
type Spawner func(ctx context.Context, orgID string, workerID orgchart.WorkerID, envPath string, triggers []activation.Trigger) error

// WorkspaceSync mirrors the canonical Role and Identity content of a
// Worker into wherever that Worker's runtime reads them at activation
// time. Tools (update_role, update_identity) call MirrorFile after
// persisting to the DB so the next activation sees fresh content
// without waiting for the spawner's projection step.
//
// `name` is a logical filename for this Worker — typically "role.md"
// or "identity.md". The backend maps the name to its own on-target
// layout; callers must NOT include backend-specific path prefixes
// (no "workers/<id>/.context/...", no "job/..."). The mapping today
// (helix runtime, the sole concrete impl):
//
//   - workers/<workerID>/.context/<name> on the helix-specs branch
//     of the Worker's per-Worker repo (matches what
//     `WorkerProject.republishWorkerFiles` writes and what the
//     activation mandate tells the agent to `git pull` and `cat`)
//
// `name` must be a clean, single-segment-or-relative filename — no
// leading slash, no "..", no escape from the Worker's namespace.
//
// Workers that aren't yet provisioned in the runtime backend (e.g.
// a Helix Worker before its first activation creates the project)
// are safe no-ops — implementations skip the mirror and return nil.
//
// Naming: see ADR-0001 §7 — MirrorFile, not PublishFile. "Publish"
// is reserved for the MCP-tool sense ("append an Event to a Stream").
type WorkspaceSync interface {
	MirrorFile(ctx context.Context, orgID string, workerID orgchart.WorkerID, name, content, message string) error
}

// NoopWorkspaceSync is a WorkspaceSync that does nothing. Useful for
// tests and for backends that have no out-of-band mirror surface.
type NoopWorkspaceSync struct{}

// MirrorFile is the no-op WorkspaceSync: ignore the call and return nil.
func (NoopWorkspaceSync) MirrorFile(_ context.Context, _ string, _ orgchart.WorkerID, _, _, _ string) error {
	return nil
}

// ValidateWorkspaceName enforces the WorkspaceSync `name` contract —
// shared by every WorkspaceSync implementation so callers see the
// same rejection rules regardless of backend. Kept exported so
// future out-of-tree backends share the same enforcement.
func ValidateWorkspaceName(name string) error {
	if name == "" {
		return errors.New("workspace name is empty")
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("workspace name %q is absolute", name)
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return fmt.Errorf("workspace name %q traverses upward", name)
		}
	}
	return nil
}

// HireHook runs runtime-side bookkeeping immediately after a Worker
// is created. It's a single-method port — one publisher (the hire
// tool), one subscriber per runtime backend (helix-runtime records
// the hiring user; claude-runtime no-ops). Not an event bus: there is
// no fan-out, no second subscriber on the horizon, and the wiring
// point picks the right implementation at construction time.
//
// hiringUserID is the upstream caller's identifier captured from
// request context — typically a Helix user_id. Empty means
// "unauthenticated context" (standalone helix-org, MCP without a
// stashed user); implementations should treat that as a no-op rather
// than an error.
//
// An OnHire error is fatal to the hire today (matches existing
// behaviour at helix-org/tools/hire_worker.go:217-222 where the same
// SaveHiringUser call returns a wrapped error). Document the trade-off
// at the call site.
type HireHook interface {
	OnHire(ctx context.Context, orgID string, workerID orgchart.WorkerID, hiringUserID string) error
}

// NoopHireHook is a HireHook that does nothing. Useful for
// tests and for dev runtimes (claude) that don't need per-hire
// runtime-side state.
type NoopHireHook struct{}

// OnHire is the no-op HireHook: ignore the call and return nil.
func (NoopHireHook) OnHire(_ context.Context, _ string, _ orgchart.WorkerID, _ string) error {
	return nil
}
