package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/helix-org/domain"
)

// TriggerKind discriminates why a Spawner is being invoked.
type TriggerKind string

const (
	// TriggerHire fires once when a Worker is first created.
	TriggerHire TriggerKind = "hire"
	// TriggerEvent fires whenever a Worker receives an event on a Stream
	// they subscribe to.
	TriggerEvent TriggerKind = "event"
)

// Trigger is the per-activation context the Spawner gives to the agent.
// The mandate (entry-point file contents) is the static role; Trigger is
// what just happened that woke this Worker up.
type Trigger struct {
	Kind TriggerKind

	// Event fields, set when Kind == TriggerEvent.
	EventID  domain.EventID
	StreamID domain.StreamID
	Source   domain.WorkerID
	// SourceKind is the WorkerKind ("human" / "ai") of Source — looked
	// up by the dispatcher at fan-out time and rendered into the
	// activation prompt so the recipient can apply the org-wide policy
	// (agent.md) of de-prioritising AI-origin events. Empty when the
	// event has no internal Source (system-emitted, or inbound from an
	// external transport with no resolved Worker).
	SourceKind domain.WorkerKind
	// Message is the canonical envelope parsed from the event body.
	// Every populated field (From, Subject, ThreadID, MessageID,
	// Extra, …) is rendered into the activation prompt so the
	// Worker can branch on transport-shaped metadata directly,
	// without a separate read_events round-trip.
	Message   domain.Message
	CreatedAt time.Time
}

// Spawner runs an AI Worker's agent process for a single activation
// and BLOCKS until the process exits. The Triggers slice tells the
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
// The zero value — nil — means "no process will be spawned", which is
// correct for tests and for HumanWorker activations.
type Spawner func(ctx context.Context, workerID domain.WorkerID, envPath string, triggers []Trigger) error

// WorkspaceSync mirrors the canonical Role and Identity content of a
// Worker into wherever that Worker's runtime reads them at activation
// time. Tools (update_role, update_identity) call MirrorFile after
// persisting to the DB so the next activation sees fresh content
// without waiting for the spawner's projection step.
//
// Naming: see ADR-0001 §7 — `MirrorFile` (not `PublishFile`). "Publish"
// is reserved for the MCP-tool sense ("append an Event to a Stream").
//
// `name` is a logical filename for this Worker — typically "role.md"
// or "identity.md". Each backend maps the name to its own on-target
// layout; callers must NOT include backend-specific path prefixes
// (no "workers/<id>/.context/...", no "job/..."). The mapping today:
//
//   - claude:   <envsDir>/<workerID>/<name>
//     (matches the layout `projectEnv` writes at activation)
//   - helix:    workers/<workerID>/.context/<name> on the helix-specs
//     branch of the Worker's per-Worker repo
//     (matches what `ProjectApplier.republishWorkerFiles`
//     writes and what the activation mandate tells the
//     agent to `git pull` and `cat`)
//
// `name` must be a clean, single-segment-or-relative filename — no
// leading slash, no "..", no escape from the Worker's namespace.
//
// Workers that aren't yet provisioned in the runtime backend (e.g. a
// Helix Worker before its first activation creates the project) are
// safe no-ops — implementations skip the publish and return nil.
type WorkspaceSync interface {
	MirrorFile(ctx context.Context, workerID domain.WorkerID, name, content, message string) error
}

// NoopWorkspaceSync is a WorkspaceSync that does nothing. Useful for
// tests and for backends that have no out-of-band mirror surface.
type NoopWorkspaceSync struct{}

// MirrorFile is the no-op WorkspaceSync: ignore the call and return nil.
func (NoopWorkspaceSync) MirrorFile(_ context.Context, _ domain.WorkerID, _, _, _ string) error {
	return nil
}

// validateWorkspaceName enforces the WorkspaceSync `name` contract —
// shared by every WorkspaceSync implementation so callers see the same
// rejection rules regardless of backend.
func validateWorkspaceName(name string) error {
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

// ValidateWorkspaceName is the public entry-point for WorkspaceSync
// implementations to reject malformed names. Kept exported so future
// out-of-tree backends share the same enforcement.
func ValidateWorkspaceName(name string) error { return validateWorkspaceName(name) }
