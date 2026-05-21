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
// both will land when H1 refactors the helix backend off the
// helixclient loopback and onto direct controller calls — at that
// point the two contracts can satisfy one struct.
//
// The two existing concrete runtimes (helix-org/agent/claude and
// helix-org/agent/helix) keep their location for now. They will move
// into api/pkg/org/runtime/{claude,helix}/ when H1 / H3 land.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/worker"
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
type Spawner func(ctx context.Context, workerID worker.ID, envPath string, triggers []activation.Trigger) error

// WorkspaceSync mirrors the canonical Role and Identity content of a
// Worker into wherever that Worker's runtime reads them at activation
// time. Tools (update_role, update_identity) call MirrorFile after
// persisting to the DB so the next activation sees fresh content
// without waiting for the spawner's projection step.
//
// `name` is a logical filename for this Worker — typically "role.md"
// or "identity.md". Each backend maps the name to its own on-target
// layout; callers must NOT include backend-specific path prefixes
// (no "workers/<id>/.context/...", no "job/..."). The mapping today:
//
//   - claude: <envsDir>/<workerID>/<name>
//     (matches the layout `projectEnv` writes at activation)
//   - helix:  workers/<workerID>/.context/<name> on the helix-specs
//     branch of the Worker's per-Worker repo
//     (matches what `ProjectApplier.republishWorkerFiles` writes
//     and what the activation mandate tells the agent to `git
//     pull` and `cat`)
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
	MirrorFile(ctx context.Context, workerID worker.ID, name, content, message string) error
}

// NoopWorkspaceSync is a WorkspaceSync that does nothing. Useful for
// tests and for backends that have no out-of-band mirror surface.
type NoopWorkspaceSync struct{}

// MirrorFile is the no-op WorkspaceSync: ignore the call and return nil.
func (NoopWorkspaceSync) MirrorFile(_ context.Context, _ worker.ID, _, _, _ string) error {
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
