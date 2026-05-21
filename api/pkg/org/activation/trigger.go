// Package activation owns the runtime-activation concept: the
// per-turn context that wakes an AI Worker. Today this package only
// carries Trigger and TriggerKind (lifted from helix-org/agent in
// B3c); the full Activation aggregate (planned for B5) will land
// here later — at which point the per-Worker queue, the transcript,
// the lifecycle (Start/End/Outcome), and the audit-row mapping all
// move into this package alongside Trigger.
package activation

import (
	"time"

	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/message"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/worker"
)

// TriggerKind discriminates why a Spawner is being invoked.
type TriggerKind string

const (
	// TriggerHire fires once when a Worker is first created.
	TriggerHire TriggerKind = "hire"

	// TriggerEvent fires whenever a Worker receives an event on a
	// Stream they subscribe to.
	TriggerEvent TriggerKind = "event"
)

// Trigger is the per-activation context the runtime gives to the
// AI Worker's process. The mandate (entry-point file contents) is
// the static Role; Trigger is what just happened that woke this
// Worker up.
//
// Fields are populated according to Kind:
//   - TriggerHire: Kind only.
//   - TriggerEvent: Kind, EventID, StreamID, Source, SourceKind,
//     Message, CreatedAt all populated by the dispatcher at fan-out
//     time.
type Trigger struct {
	Kind TriggerKind

	// Event fields, set when Kind == TriggerEvent.
	EventID  event.ID
	StreamID stream.ID
	Source   worker.ID

	// SourceKind is the worker.Kind ("human" / "ai") of Source —
	// looked up by the dispatcher at fan-out time and rendered into
	// the activation prompt so the recipient can apply the org-wide
	// policy (worker-policy.md) of de-prioritising AI-origin events.
	// Empty when the event has no internal Source (system-emitted,
	// or inbound from an external transport with no resolved Worker).
	SourceKind worker.Kind

	// Message is the canonical envelope parsed from the event body.
	// Every populated field (From, Subject, ThreadID, MessageID,
	// Extra, …) is rendered into the activation prompt so the Worker
	// can branch on transport-shaped metadata directly, without a
	// separate read_events round-trip.
	Message message.Message

	CreatedAt time.Time
}
