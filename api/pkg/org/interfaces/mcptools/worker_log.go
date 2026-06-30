package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// WorkerLog reads a single AI Worker's activation transcript — assistant
// text, tool calls, tool results — newest first. It's a shortcut over
// the underlying primitives: resolves the deterministic activation
// Topic (s-transcript-<workerID>), auto-subscribes the caller (so
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
		"the Worker's transcript on first call; subsequent calls reuse the " +
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

func (t *WorkerLog) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args workerLogArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.WorkerID == "" {
		return nil, fmt.Errorf("workerId is required")
	}

	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("worker_log: caller has no OrgID")
	}
	target := orgchart.BotID(args.WorkerID)
	wkr, err := t.deps.Queries.GetWorker(ctx, orgID, target)
	if err != nil {
		return nil, fmt.Errorf("worker %q: %w", target, err)
	}
	if wkr.Kind() != orgchart.WorkerKindAI {
		return nil, fmt.Errorf("worker %q is %s; only AI workers have activation logs",
			target, wkr.Kind())
	}

	topicID := activation.TranscriptID(target)
	if _, err := t.deps.Queries.GetTopic(ctx, orgID, topicID); err != nil {
		return nil, fmt.Errorf("transcript for %q: %w", target, err)
	}

	var actWindow *struct {
		started time.Time
		ended   time.Time
		open    bool
	}
	if args.ActivationID != "" {
		a, err := t.deps.Queries.GetActivation(ctx, orgID, activation.ID(args.ActivationID))
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

	// Auto-subscribe the caller via the subscriptions service (validates
	// the caller + topic exist, idempotent get-or-create). After this,
	// plain read_events also includes this Worker's transcript.
	caller := inv.Caller.ID()
	if _, _, err := t.deps.Subscriptions.Subscribe(ctx, orgID, caller, topicID); err != nil {
		return nil, fmt.Errorf("subscribe worker %q to %q: %w", caller, topicID, err)
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
	since := streaming.EventID(args.Since)

	fresh, err := t.fresh(ctx, orgID, topicID, limit, since)
	if err != nil {
		return nil, err
	}
	if actWindow != nil {
		fresh = filterToActivationWindow(fresh, actWindow.started, actWindow.ended, actWindow.open)
	}
	if len(fresh) > 0 || wait == 0 || t.deps.Hub == nil {
		return marshalEvents(fresh), nil
	}

	wake := t.deps.Hub.Subscribe(orgID, []streaming.TopicID{topicID})
	defer t.deps.Hub.Unsubscribe([]streaming.TopicID{topicID}, wake)

	timer := time.NewTimer(time.Duration(wait) * time.Second)
	defer timer.Stop()

	select {
	case <-wake:
	case <-timer.C:
	case <-ctx.Done():
		return marshalEvents(nil), nil
	}

	fresh, err = t.fresh(ctx, orgID, topicID, limit, since)
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
func filterToActivationWindow(events []streaming.Event, startedAt, endedAt time.Time, open bool) []streaming.Event {
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

// fresh returns events on the transcript newer than `since`
// (exclusive), newest-first, up to `limit`. Empty `since` means
// "return everything up to limit".
func (t *WorkerLog) fresh(ctx context.Context, orgID string, topicID streaming.TopicID, limit int, since streaming.EventID) ([]streaming.Event, error) {
	events, err := t.deps.Queries.TopicEvents(ctx, orgID, topicID, limit)
	if err != nil {
		return nil, fmt.Errorf("list events on %q: %w", topicID, err)
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
