// Package transcript records a Worker's activation turns onto its
// transcript — the per-Worker, append-only, observable log of everything
// the Worker did across its activations (assistant text, tool calls, tool
// results, chat turns). It is the single writer every activation path —
// the event-dispatcher-driven AI Workers and the human chat surface (the
// owner) — uses to record a turn.
//
// A transcript is a RECORD, not a communication channel. Recording a turn
// appends the event and notifies live observers (so the transcript UI
// tails it), but it deliberately does NOT dispatch/fan-out: a Worker
// narrating what it just did must not re-trigger the managers observing
// it (that would loop forever). Contrast application/publishing, whose
// Publish is the signal path (append + notify + dispatch). The two share
// the streaming.Topic substrate but mean different things.
package transcript

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// Notifier wakes long-poll observers tailing a transcript. *wakebus.Bus
// satisfies it. Mirror of publishing.Notifier — kept behind a narrow
// interface so this package doesn't depend on the concrete bus.
type Notifier interface {
	Notify(orgID string, topicID streaming.TopicID)
}

// Recorder appends turns to Workers' transcripts. It owns only the narrow
// Events repository plus a notifier/clock/id-source — never the whole
// *store.Store.
type Recorder struct {
	events   store.Events
	notifier Notifier
	now      func() time.Time
	newID    func() string
	logger   *slog.Logger
}

// Deps are the constructor-injected collaborators for New. Events/Now/
// NewID are required for a functioning recorder (any nil → Record is a
// no-op, useful in tests that don't wire the org-graph). Notifier is
// optional (nil → no live wake).
type Deps struct {
	Events   store.Events
	Notifier Notifier
	Now      func() time.Time
	NewID    func() string
	Logger   *slog.Logger
}

// New constructs a Recorder.
func New(deps Deps) *Recorder {
	return &Recorder{
		events:   deps.Events,
		notifier: deps.Notifier,
		now:      deps.Now,
		newID:    deps.NewID,
		logger:   deps.Logger,
	}
}

// Record appends one turn (body) to the Worker's transcript — the
// `s-transcript-<workerID>` topic — with the canonical Message envelope
// ({from: workerID, body}) and notifies observers so live UI subscribers
// see it. Recording is observe-only: it never dispatches, so a Worker
// recording its own turn never re-triggers its observers.
//
// Returns (ID, nil) on success so callers can correlate transcript
// segments with the originating activation. A nil events repo / clock /
// id-source or a blank body short-circuits to ("", nil) — useful in
// tests that don't wire the full org-graph dependencies.
func (r *Recorder) Record(ctx context.Context, orgID string, workerID orgchart.BotID, body string) (streaming.EventID, error) {
	if r == nil || r.events == nil || r.newID == nil || r.now == nil || strings.TrimSpace(body) == "" {
		return "", nil
	}
	topicID := activation.TranscriptID(workerID)
	event, err := streaming.NewMessageEvent(
		streaming.EventID("e-"+r.newID()),
		topicID,
		workerID,
		streaming.Message{From: string(workerID), Body: body},
		r.now(),
		orgID,
	)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("transcript record: build", "worker", workerID, "err", err)
		}
		return "", err
	}
	if err := r.events.Append(ctx, event); err != nil {
		if r.logger != nil {
			r.logger.Warn("transcript record: append", "worker", workerID, "err", err)
		}
		return "", err
	}
	if r.notifier != nil {
		r.notifier.Notify(orgID, topicID)
	}
	return event.ID, nil
}
