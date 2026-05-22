package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/org/domain"
)

// WorkerLog reads a single AI Worker's activation transcript — assistant
// text, tool calls, tool results — newest first. It's a shortcut over
// the underlying primitives: resolves the deterministic activation
// Stream (s-activations-<workerID>), auto-subscribes the caller (so
// the agent doesn't have to chain subscribe + read_events), then
// returns the events scoped to that one Worker.
//
// Same pagination/long-poll semantics as read_events: pass since=<eventId>
// to skip what you've already seen, wait=<seconds> (0..60) to block for
// new events.
type WorkerLog struct {
	deps Deps
}

const WorkerLogName tool.Name = "worker_log"

var workerLogSchema = mustSchema[workerLogArgs]()

func (t *WorkerLog) Name() tool.Name                 { return WorkerLogName }
func (t *WorkerLog) InputSchema() *jsonschema.Schema { return workerLogSchema }
func (t *WorkerLog) Description() string {
	return "Read a Worker's activation log — assistant text, tool calls, tool results — " +
		"newest first. Reach for this whenever the user wants to watch/audit/tail/" +
		"observe what a named Worker is doing or did. Auto-subscribes the caller to " +
		"the Worker's activation Stream on first call; subsequent calls reuse the " +
		"subscription. Same since/wait/limit semantics as read_events but scoped to " +
		"one Worker. Pass activationId to narrow further to a single activation's " +
		"transcript (the time window between that activation's start and end). " +
		"AI Workers only — Human Workers don't have activation logs."
}

type workerLogArgs struct {
	WorkerID     string `json:"workerId"`
	Limit        int    `json:"limit,omitempty"`
	Since        string `json:"since,omitempty"`
	Wait         int    `json:"wait,omitempty"`
	ActivationID string `json:"activationId,omitempty"`
}

// UnmarshalJSON tolerates string-encoded ints for limit and wait —
// same LLM-quirk fix as read_events. See decodeFlexInt comment.
func (a *workerLogArgs) UnmarshalJSON(data []byte) error {
	type plain workerLogArgs
	type tolerant struct {
		*plain
		Limit json.RawMessage `json:"limit,omitempty"`
		Wait  json.RawMessage `json:"wait,omitempty"`
	}
	t := tolerant{plain: (*plain)(a)}
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	if v, err := decodeFlexInt(t.Limit); err != nil {
		return fmt.Errorf("limit: %w", err)
	} else {
		a.Limit = v
	}
	if v, err := decodeFlexInt(t.Wait); err != nil {
		return fmt.Errorf("wait: %w", err)
	} else {
		a.Wait = v
	}
	return nil
}

func (t *WorkerLog) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args workerLogArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.WorkerID == "" {
		return nil, fmt.Errorf("workerId is required")
	}

	target := worker.ID(args.WorkerID)
	wkr, err := t.deps.Store.Workers.Get(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("worker %q: %w", target, err)
	}
	if wkr.Kind() != worker.KindAI {
		return nil, fmt.Errorf("worker %q is %s; only AI workers have activation logs",
			target, wkr.Kind())
	}

	streamID := activation.StreamID(target)
	if _, err := t.deps.Store.Streams.Get(ctx, streamID); err != nil {
		return nil, fmt.Errorf("activation stream for %q: %w", target, err)
	}

	// activationId narrows results to events that fall inside one
	// Activation's [StartedAt, EndedAt] window. Resolved here so the
	// caller surfaces a clear error for unknown / cross-Worker IDs
	// rather than silently returning [].
	var actWindow *struct {
		started time.Time
		ended   time.Time
		open    bool // true while EndedAt is nil — clamp the upper bound to now
	}
	if args.ActivationID != "" {
		if t.deps.Store.Activations == nil {
			return nil, fmt.Errorf("activationId filter unsupported: store has no Activations repo")
		}
		a, err := t.deps.Store.Activations.Get(ctx, activation.ID(args.ActivationID))
		if err != nil {
			return nil, fmt.Errorf("activation %q: %w", args.ActivationID, err)
		}
		if a.WorkerID != target {
			return nil, fmt.Errorf("activation %q belongs to worker %q, not %q", args.ActivationID, a.WorkerID, target)
		}
		w := struct {
			started time.Time
			ended   time.Time
			open    bool
		}{started: a.StartedAt}
		if a.EndedAt != nil {
			w.ended = *a.EndedAt
		} else {
			w.open = true
		}
		actWindow = &w
	}

	// Auto-subscribe the caller. Idempotent; harmless to re-run. After
	// this, plain read_events will also include this Worker's
	// transcript, which is usually the desired follow-up behaviour.
	caller := inv.Caller.ID()
	if _, err := t.deps.Store.Subscriptions.Find(ctx, caller, streamID); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		sub, err := domain.NewSubscription(caller, streamID, t.deps.Now())
		if err != nil {
			return nil, err
		}
		if err := t.deps.Store.Subscriptions.Create(ctx, sub); err != nil {
			return nil, fmt.Errorf("subscribe %q to %q: %w", caller, streamID, err)
		}
	}

	limit := args.Limit
	if limit <= 0 {
		limit = readEventsDefaultLimit
	}
	if limit > readEventsMaxLimit {
		limit = readEventsMaxLimit
	}
	wait := args.Wait
	if wait < 0 {
		wait = 0
	}
	if wait > readEventsMaxWaitSecs {
		wait = readEventsMaxWaitSecs
	}
	since := event.ID(args.Since)

	fresh, err := t.fresh(ctx, streamID, limit, since)
	if err != nil {
		return nil, err
	}
	if actWindow != nil {
		fresh = filterToActivationWindow(fresh, actWindow.started, actWindow.ended, actWindow.open)
	}
	if len(fresh) > 0 || wait == 0 || t.deps.Hub == nil {
		return marshalEvents(fresh), nil
	}

	wake := t.deps.Hub.Subscribe([]stream.ID{streamID})
	defer t.deps.Hub.Unsubscribe([]stream.ID{streamID}, wake)

	timer := time.NewTimer(time.Duration(wait) * time.Second)
	defer timer.Stop()

	select {
	case <-wake:
	case <-timer.C:
	case <-ctx.Done():
		return marshalEvents(nil), nil
	}

	fresh, err = t.fresh(ctx, streamID, limit, since)
	if err != nil {
		return nil, err
	}
	if actWindow != nil {
		fresh = filterToActivationWindow(fresh, actWindow.started, actWindow.ended, actWindow.open)
	}
	return marshalEvents(fresh), nil
}

// filterToActivationWindow keeps only events whose CreatedAt falls
// inside the activation's [startedAt, endedAt] window. When open is
// true, the upper bound is "still running" — no clamp; events after
// startedAt are accepted. Bounds are inclusive on both sides because
// the activation's start/exit marker events are written at exactly
// those timestamps and are part of the activation's transcript.
func filterToActivationWindow(events []domain.Event, startedAt, endedAt time.Time, open bool) []domain.Event {
	out := events[:0]
	for _, e := range events {
		if e.CreatedAt.Before(startedAt) {
			continue
		}
		if !open && e.CreatedAt.After(endedAt) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// fresh returns events on the activation stream newer than `since`
// (exclusive), newest-first, up to `limit`. Empty `since` means
// "return everything up to limit".
func (t *WorkerLog) fresh(ctx context.Context, streamID stream.ID, limit int, since event.ID) ([]domain.Event, error) {
	events, err := t.deps.Store.Events.ListForStream(ctx, streamID, limit)
	if err != nil {
		return nil, fmt.Errorf("list events on %q: %w", streamID, err)
	}
	if since == "" {
		return events, nil
	}
	for i, e := range events {
		if e.ID == since {
			return events[:i], nil
		}
	}
	return events, nil
}
