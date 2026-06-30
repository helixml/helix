// Package domainevent owns a generic, append-only audit log of
// *decisions* an org-graph component made — distinct from the runtime
// streaming.Event data plane (which carries Messages between Topics).
// The name is deliberately not "event" to avoid that collision: a
// DomainEvent is a recorded fact ("processor P decided Worker W is a
// participant of thread T"), not a payload in flight.
//
// v1 is intentionally minimal (see design/2026-06-26-slack-auto-router.md
// §5 / non-goals): one aggregate, append + query, no subscribers,
// projections engine, or replay. Derived state (e.g. Slack thread
// membership) is computed by *querying* the log over a time window, so
// nothing is reaped for correctness — the window is a read bound, not a
// retention policy.
package domainevent

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Type names a category of recorded decision. Free-form by design so new
// kinds of fact can be logged without touching the core; consumers filter
// on it. Defined as constants beside the component that emits them.
type Type = string

// TypeSlackThreadParticipant records that a Worker was routed into a Slack
// thread by an auto-router and is therefore a participant of it. Subject is
// the thread root, Worker the participant, Source the router's processor id.
const TypeSlackThreadParticipant Type = "slack.thread_participant"

// DomainEvent is one recorded decision.
//
//   - Subject is the entity the event is keyed and queried on (for a Slack
//     thread-participant fact: the thread root id).
//   - Worker is the orgchart.WorkerID the decision concerns (the
//     participant). Stored as a plain string — the aggregate does not import
//     orgchart, mirroring streaming.Topic.CreatedBy.
//   - Source is what made the decision (the router's processor id).
//   - Metadata is optional structured context, opaque to the store.
type DomainEvent struct {
	ID             string
	OrganizationID string
	Type           Type
	Subject        string
	Worker         string
	Source         string
	Metadata       json.RawMessage
	CreatedAt      time.Time
}

// New validates and constructs a DomainEvent. id, orgID, typ and subject
// are required; worker/source/metadata are optional.
func New(id, orgID string, typ Type, subject, worker, source string, metadata json.RawMessage, createdAt time.Time) (DomainEvent, error) {
	if id == "" {
		return DomainEvent{}, errors.New("domain event id is empty")
	}
	if orgID == "" {
		return DomainEvent{}, errors.New("domain event orgID is empty")
	}
	if typ == "" {
		return DomainEvent{}, errors.New("domain event type is empty")
	}
	if subject == "" {
		return DomainEvent{}, errors.New("domain event subject is empty")
	}
	if createdAt.IsZero() {
		return DomainEvent{}, errors.New("domain event createdAt is zero")
	}
	return DomainEvent{
		ID:             id,
		OrganizationID: orgID,
		Type:           typ,
		Subject:        subject,
		Worker:         worker,
		Source:         source,
		Metadata:       metadata,
		CreatedAt:      createdAt.UTC(),
	}, nil
}

// Repository persists DomainEvents. The interface lives beside the
// aggregate (like activation.Repository) so the storage boundary is part of
// the domain package, not a parallel declaration in store.go.
type Repository interface {
	// Append records one DomainEvent.
	Append(ctx context.Context, e DomainEvent) error
	// ListBySubject returns the org's DomainEvents of the given type for the
	// given subject, newest first, created at or after `since`. A zero
	// `since` applies no lower bound. Returns an empty slice (never
	// ErrNotFound) when none match.
	ListBySubject(ctx context.Context, orgID string, typ Type, subject string, since time.Time) ([]DomainEvent, error)
}

// Participants returns the distinct, order-stable set of Worker ids from a
// slice of participant DomainEvents (newest-first input preserved). A small
// read-side helper so callers don't re-implement the dedupe.
func Participants(events []DomainEvent) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(events))
	for _, e := range events {
		if e.Worker == "" {
			continue
		}
		if _, ok := seen[e.Worker]; ok {
			continue
		}
		seen[e.Worker] = struct{}{}
		out = append(out, e.Worker)
	}
	return out
}
