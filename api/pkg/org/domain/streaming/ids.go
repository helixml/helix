// Package streaming owns the streaming aggregate: Topic,
// Subscription, Event, the canonical Message envelope and its
// Attachment value type, and the typed Principal value object that
// identifies who originated an Event or Message.
//
// Lifted from api/pkg/org/{topic,event,message,principal} leaf
// packages and api/pkg/org/domain/{topic,event,subscription,
// message,principal}.go in the DDD restructure. IDs lose their
// per-entity package prefix (topic.ID -> streaming.StreamID,
// event.ID -> streaming.EventID).
//
// Cycle break: this package intentionally does NOT import
// api/pkg/org/domain/orgchart. Worker-typed fields (Event.Source,
// Topic.CreatedBy, Subscription.WorkerID, Principal.ID for
// KindWorker) are carried as plain string — the IDs in orgchart are
// declared as type aliases for string, so callers can assign
// orgchart.NodeID values directly with no cast. This keeps the DAG
// one-way: orgchart imports streaming (for StreamID typing on
// Role.Topics), streaming never imports orgchart back.
package streaming

// StreamID identifies one append-only event stream. Every Trigger owns
// the stream named by its own id; every Processor output branch owns the
// stream recorded on the branch. Convention: `s-<slug>` (e.g.
// `s-general`, `s-inbox`). Transcripts use the deterministic pattern
// `s-transcript-<workerID>`.
//
// It is a persistence key, never a routing address — routing is always
// expressed as an eventsource.SourceRef.
type StreamID = string

// EventID identifies an Event. Convention: `e-<uuid>`.
type EventID = string
