// Package topology is the application-layer reconciler that converges
// the persisted Streams/Subscriptions onto the desired topology the
// reporting graph implies. The pure derivation — "what Streams and
// Subscriptions does this graph imply?" — lives in domain/topology;
// this package loads the graph from the store, calls
// domaintopology.DesiredTopology, diffs the desired set against what's
// persisted, and applies create/subscribe/unsubscribe/delete
// idempotently.
//
// This is the single owner of activation/team/DM Stream lifecycle. Every
// structural mutation (hire, add/remove reporting line, fire) announces
// *what changed* by calling Reconcile; the reconciler decides the
// stream consequences. Event-specific deltas drift; a declarative diff
// can't.
package topology

import (
	"context"
	"errors"
	"fmt"
	"time"

	domaintopology "github.com/helixml/helix/api/pkg/org/domain/topology"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// EnsureStreamWithMembers is the reconciler's create-stream-and-
// subscribe primitive. It get-or-creates the Stream (immutable once it
// exists, so a present row is left untouched) and idempotently
// subscribes each member.
//
// Subscriptions are worker-anchored; members must be existing Workers.
//
// Concurrency-safe by design. The Stream id is deterministic
// (s-dm-<pair>, s-team-<id>, s-activations-<id>), so two callers can
// race on the same id — two simultaneous DMs between the same pair, two
// reconciles touching one manager's team stream. A plain check-then-act
// would let the loser of the race hit the row's unique constraint and
// return a spurious error. Instead, on a Create failure we re-read the
// store: if the row is now present, another caller won the race and the
// outcome we wanted holds — proceed. Only a still-absent row is a
// genuine failure worth surfacing. This keeps Streams.Create /
// Subscriptions.Create strict for every other caller (createStream,
// hire_worker) while making *this* get-or-create boundary idempotent.
func EnsureStreamWithMembers(ctx context.Context, s *store.Store, stream streaming.Stream, now time.Time, members ...orgchart.WorkerID) error {
	if _, err := s.Streams.Get(ctx, stream.OrganizationID, stream.ID); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("lookup stream %q: %w", stream.ID, err)
		}
		if createErr := s.Streams.Create(ctx, stream); createErr != nil {
			// Lost the create race? A concurrent caller inserted the same
			// deterministic id between our Get and Create. Benign for a
			// get-or-create — re-check, and only surface the error if the
			// row still isn't there.
			if _, getErr := s.Streams.Get(ctx, stream.OrganizationID, stream.ID); getErr != nil {
				return fmt.Errorf("create stream %q: %w", stream.ID, createErr)
			}
		}
	}
	for _, m := range members {
		if _, err := s.Subscriptions.Find(ctx, stream.OrganizationID, m, stream.ID); err == nil {
			continue
		} else if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("find subscription %q→%q: %w", m, stream.ID, err)
		}
		sub, err := streaming.NewSubscription(string(m), stream.ID, now, stream.OrganizationID)
		if err != nil {
			return fmt.Errorf("build subscription %q→%q: %w", m, stream.ID, err)
		}
		if createErr := s.Subscriptions.Create(ctx, sub); createErr != nil {
			// Same race on the (worker, stream) subscription key: a
			// concurrent caller subscribed this member first. A
			// now-present row means success.
			if _, findErr := s.Subscriptions.Find(ctx, stream.OrganizationID, m, stream.ID); findErr != nil {
				return fmt.Errorf("subscribe %q→%q: %w", m, stream.ID, createErr)
			}
		}
	}
	return nil
}

// newDesiredStream builds the streaming.Stream the reconciler persists
// for a DesiredStream. Activation/team Streams are always local
// transport (the default).
func newDesiredStream(ds domaintopology.DesiredStream, now time.Time, orgID string) (streaming.Stream, error) {
	return streaming.NewStream(ds.ID, ds.Name, ds.Description, string(ds.CreatedBy), now, transport.Transport{}, orgID)
}
