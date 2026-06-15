// Package activation owns the runtime-activation concept: the
// per-turn context that wakes an AI Worker. Today this package
// carries:
//
//   - Trigger / TriggerKind: why a Spawner was invoked (lifted from
//     helix-org/agent in B3c).
//   - StreamID(workerID): the canonical derivation of the per-Worker
//     activation Stream ID (`s-activations-<workerID>`), lifted from
//     helix-org/agent in B5.1.
//
// The full Activation aggregate (planned for the remainder of B5)
// will land here next — at which point the per-Worker queue, the
// transcript-segment VO, the lifecycle (Start/End/Outcome), and the
// audit-row mapping all move into this package alongside Trigger.
package activation

import (
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// TriggerKind discriminates why a Spawner is being invoked.
type TriggerKind string

const (
	// TriggerHire fires once when a Worker is first created.
	TriggerHire TriggerKind = "hire"

	// TriggerEvent fires whenever a Worker receives an event on a
	// Stream they subscribe to.
	TriggerEvent TriggerKind = "event"

	// TriggerManual fires when an operator manually wakes a Worker
	// from the UI (the worker page's "Start Desktop" button).
	// Functionally identical to TriggerHire in terms of the activation
	// pipeline — ensureProject + AttachHelixOrgMCP + ensureSession —
	// just with a different label so the audit row + activation marker
	// distinguish "operator clicked the button" from "Worker was just
	// hired" or "Stream event arrived".
	TriggerManual TriggerKind = "manual"
)

// Trigger is the per-activation context the runtime gives to the
// AI Worker's process. The mandate (entry-point file contents) is
// the static Role; Trigger is what just happened that woke this
// Worker up.
//
// Fields are populated according to Kind:
//   - TriggerHire: Kind only (plus optional ActivationID below).
//   - TriggerEvent: Kind, EventID, StreamID, Source, SourceKind,
//     Message, CreatedAt all populated by the dispatcher at fan-out
//     time.
type Trigger struct {
	Kind TriggerKind

	// ActivationID, when non-empty, is the pre-allocated audit-row ID
	// the caller wants the Spawner to use for this activation. Hire
	// flows pre-create the row in the caller's request context so
	// hire_worker can return the ID synchronously; the Spawner picks
	// up the same row instead of creating a sibling. Empty means
	// "Spawner mints its own ID" — the event-driven path today.
	ActivationID ID

	// Event fields, set when Kind == TriggerEvent.
	EventID  streaming.EventID
	StreamID streaming.StreamID
	Source   orgchart.WorkerID

	// SourceKind is the orgchart.WorkerKind ("human" / "ai") of Source —
	// looked up by the dispatcher at fan-out time and rendered into
	// the activation prompt so the recipient can apply the org-wide
	// policy of de-prioritising AI-origin events.
	// Empty when the event has no internal Source (system-emitted,
	// or inbound from an external transport with no resolved Worker).
	SourceKind orgchart.WorkerKind

	// Message is the canonical envelope parsed from the event body.
	// Every populated field (From, Subject, ThreadID, MessageID,
	// Extra, …) is rendered into the activation prompt so the Worker
	// can branch on transport-shaped metadata directly, without a
	// separate read_events round-trip.
	Message streaming.Message

	CreatedAt time.Time
}
