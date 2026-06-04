// Package streaming owns the streaming aggregate: Stream,
// Subscription, Event, the canonical Message envelope and its
// Attachment value type, and the typed Principal value object that
// identifies who originated an Event or Message.
//
// Lifted from api/pkg/org/{stream,event,message,principal} leaf
// packages and api/pkg/org/domain/{stream,event,subscription,
// message,principal}.go in the DDD restructure. IDs lose their
// per-entity package prefix (stream.ID -> streaming.StreamID,
// event.ID -> streaming.EventID).
//
// Cycle break: this package intentionally does NOT import
// api/pkg/org/domain/orgchart. Worker-typed fields (Event.Source,
// Stream.CreatedBy, Subscription.WorkerID, Principal.ID for
// KindWorker) are carried as plain string — the IDs in orgchart are
// declared as type aliases for string, so callers can assign
// orgchart.WorkerID values directly with no cast. This keeps the DAG
// one-way: orgchart imports streaming (for StreamID typing on
// Role.Streams), streaming never imports orgchart back.
package streaming

// StreamID identifies a Stream. Convention: `s-<slug>` (e.g.
// `s-general`, `s-inbox`). Activation streams use the deterministic
// pattern `s-activations-<workerID>`.
type StreamID = string

// EventID identifies an Event. Convention: `e-<uuid>`.
type EventID = string
