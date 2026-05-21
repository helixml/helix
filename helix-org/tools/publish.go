package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/helix-org/domain"
)

// Publish appends an Event to a named Stream, attributed to the caller.
// It does exactly one thing: append an event to an existing Stream. It
// does not create Streams or manage subscriptions; for the common
// "direct message a Worker" case, see the dm tool, which bundles
// create-stream + subscribe-both + publish into a single call.
//
// All events are stored as canonical Message JSON (see domain.Message).
// The minimal call form — streamId + body — yields a Message with
// From=caller and Body=body. Optional fields (to, subject, threadId,
// inReplyTo, messageId, bodyContentType, attachments) let the caller
// publish a richer envelope when threading or recipients matter.
type Publish struct {
	deps Deps
}

const PublishName domain.ToolName = "publish"

var publishSchema = mustSchema[publishArgs]()

func (t *Publish) Name() domain.ToolName { return PublishName }
func (t *Publish) Description() string {
	return "Append an Event with the given body to a Stream. Wakes long-poll observers and " +
		"activates every subscribed AI Worker. Optional fields (to, subject, threadId, " +
		"inReplyTo, messageId, attachments) carry threading and recipient metadata for " +
		"messaging streams; omit them for plain text publishes."
}
func (t *Publish) InputSchema() *jsonschema.Schema { return publishSchema }

type publishArgs struct {
	StreamID        string              `json:"streamId"`
	Body            string              `json:"body"`
	To              []string            `json:"to,omitempty"`
	Subject         string              `json:"subject,omitempty"`
	BodyContentType string              `json:"bodyContentType,omitempty"`
	ThreadID        string              `json:"threadId,omitempty"`
	InReplyTo       string              `json:"inReplyTo,omitempty"`
	MessageID       string              `json:"messageId,omitempty"`
	Attachments     []domain.Attachment `json:"attachments,omitempty"`
}

func (t *Publish) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args publishArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.StreamID == "" || args.Body == "" {
		return nil, fmt.Errorf("streamId and body are required")
	}
	streamID := domain.StreamID(args.StreamID)
	stream, err := t.deps.Store.Streams.Get(ctx, streamID)
	if err != nil {
		return nil, fmt.Errorf("stream %q: %w", streamID, err)
	}
	// GitHub streams are inbound-only. Acting on a repo (label,
	// comment, review, open PR) is the Worker's job via `gh` in its
	// Environment — wrapping each github action behind publish would
	// reinvent the gh CLI's flag set with worse ergonomics. Surface
	// the mistake loudly rather than silently no-op'ing.
	if stream.Transport.Kind == domain.TransportGitHub {
		return nil, fmt.Errorf("stream %q: publish is not supported on github transport streams; use `gh` from your Environment to act on the repo", streamID)
	}
	msg := domain.Message{
		From:            string(inv.Caller.ID()),
		To:              args.To,
		Subject:         args.Subject,
		Body:            args.Body,
		BodyContentType: args.BodyContentType,
		ThreadID:        args.ThreadID,
		InReplyTo:       args.InReplyTo,
		MessageID:       args.MessageID,
		Attachments:     args.Attachments,
	}
	event, err := domain.NewMessageEvent(
		domain.EventID("e-"+t.deps.NewID()),
		streamID,
		inv.Caller.ID(),
		msg,
		t.deps.Now(),
	)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Events.Append(ctx, event); err != nil {
		return nil, err
	}
	// Wake long-poll observers (read_events with wait>0).
	if t.deps.Broadcaster != nil {
		t.deps.Broadcaster.Notify(streamID)
	}
	// Activate every subscribed AI Worker. Background; returns immediately.
	if t.deps.Dispatcher != nil {
		t.deps.Dispatcher.Dispatch(ctx, event)
	}
	return json.Marshal(map[string]string{"id": string(event.ID), "streamId": string(streamID)})
}
