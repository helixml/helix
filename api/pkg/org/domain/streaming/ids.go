// Package streaming owns the streaming aggregate: Topic,
// Subscription, Event, the canonical Message envelope and its
// Attachment value type, and the typed Principal value object that
// identifies who originated an Event or Message.
//
// Lifted from api/pkg/org/{topic,event,message,principal} leaf
// packages and api/pkg/org/domain/{topic,event,subscription,
// message,principal}.go in the DDD restructure. IDs lose their
// per-entity package prefix (topic.ID -> streaming.TopicID,
// event.ID -> streaming.EventID).
//
// Cycle break: this package intentionally does NOT import
// api/pkg/org/domain/orgchart. Worker-typed fields (Event.Source,
// Topic.CreatedBy, Subscription.WorkerID, Principal.ID for
// KindWorker) are carried as plain string — the IDs in orgchart are
// declared as type aliases for string, so callers can assign
// orgchart.WorkerID values directly with no cast. This keeps the DAG
// one-way: orgchart imports streaming (for TopicID typing on
// Role.Topics), streaming never imports orgchart back.
package streaming

// TopicID identifies a Topic. Convention: `s-<slug>` (e.g.
// `s-general`, `s-inbox`). Transcripts use the deterministic
// pattern `s-transcript-<workerID>`.
type TopicID = string

// EventID identifies an Event. Convention: `e-<uuid>`.
type EventID = string
