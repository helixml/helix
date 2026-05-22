package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/org/domain"
)

// EnsureActivationStream creates the per-Worker activation Stream
// (s-activations-<workerID>) and subscribes the observer to it. Single
// enforcement site for the invariant from 05 §3.6: every Worker has
// an activation Stream. Two callers today:
//
//   - tools/hire_worker (AI Workers): observer = hiring caller, so the
//     hiring Worker watches the new hire's transcript.
//   - bootstrap (the owner): observer = the owner themselves, so the
//     owner's chat turns show up in /ui/streams alongside every AI
//     Worker's.
//
// The description text is generic enough to fit both cases — humans
// produce chat turns, AI Workers produce assistant text + tool calls;
// the Stream carries both.
func EnsureActivationStream(ctx context.Context, s *store.Store, workerID, observerID worker.ID, now time.Time) error {
	streamID := activation.StreamID(workerID)
	stream, err := domain.NewStream(
		streamID,
		"Activations: "+string(workerID),
		"Per-message activation transcript for "+string(workerID)+
			" — assistant text, tool calls, tool results, chat turns. "+
			"Read with read_events or worker_log to audit / tail.",
		observerID,
		now,
		transport.Transport{},
	)
	if err != nil {
		return fmt.Errorf("activation stream for %q: %w", workerID, err)
	}
	if err := s.Streams.Create(ctx, stream); err != nil {
		return fmt.Errorf("create activation stream for %q: %w", workerID, err)
	}
	sub, err := domain.NewSubscription(observerID, streamID, now)
	if err != nil {
		return fmt.Errorf("activation subscription for %q: %w", observerID, err)
	}
	if err := s.Subscriptions.Create(ctx, sub); err != nil {
		return fmt.Errorf("subscribe %q to activation stream %q: %w", observerID, streamID, err)
	}
	return nil
}
