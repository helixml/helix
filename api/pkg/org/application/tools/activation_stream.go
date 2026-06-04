package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// EnsureActivationStream creates the per-Worker activation Stream
// (s-activations-<workerID>) and subscribes the observer to it. Single
// enforcement site for the invariant from 05 §3.6: every Worker has
// an activation Stream. Two callers today:
//
//   - tools/hire_worker (AI Workers): observer = hiring caller, so the
//     hiring Worker watches the new hire's transcript.
//   - bootstrap (the owner): observer = the owner themselves, so the
//     owner's chat turns show up on the streams page alongside every
//     AI Worker's.
//
// The description text is generic enough to fit both cases — humans
// produce chat turns, AI Workers produce assistant text + tool calls;
// the Stream carries both.
func EnsureActivationStream(ctx context.Context, s *store.Store, orgID string, workerID, observerID orgchart.WorkerID, now time.Time) error {
	streamID := activation.StreamID(workerID)
	stream, err := streaming.NewStream(
		streamID,
		"Activations: "+string(workerID),
		"Per-message activation transcript for "+string(workerID)+
			" — assistant text, tool calls, tool results, chat turns. "+
			"Read with read_events or worker_log to audit / tail.",
		observerID,
		now,
		transport.Transport{},
		orgID,
	)
	if err != nil {
		return fmt.Errorf("activation stream for %q: %w", workerID, err)
	}
	if err := s.Streams.Create(ctx, stream); err != nil {
		return fmt.Errorf("create activation stream for %q: %w", workerID, err)
	}
	// Subscriptions are position-anchored: resolve the observer's
	// position before creating the subscription. The observer is
	// either the hiring caller (AI hire) or the owner (bootstrap) —
	// both have positions. An observer with no position is a wiring
	// bug; surface it loudly.
	observer, err := s.Workers.Get(ctx, orgID, observerID)
	if err != nil {
		return fmt.Errorf("get observer worker %q: %w", observerID, err)
	}
	observerPosition := observer.Position()
	if observerPosition == "" {
		return fmt.Errorf("activation subscription: observer %q has no position", observerID)
	}
	sub, err := streaming.NewSubscription(string(observerPosition), streamID, now, orgID)
	if err != nil {
		return fmt.Errorf("activation subscription for position %q: %w", observerPosition, err)
	}
	if err := s.Subscriptions.Create(ctx, sub); err != nil {
		return fmt.Errorf("subscribe position %q to activation stream %q: %w", observerPosition, streamID, err)
	}
	return nil
}
