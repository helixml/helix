package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Publish appends an Event to a named Stream, attributed to the caller.
// It does exactly one thing: append an event to an existing Stream. It
// does not create Streams or manage subscriptions; for the common
// "direct message a Worker" case, see the dm tool, which bundles
// create-stream + subscribe-both + publish into a single call.
//
// All events are stored as canonical Message JSON (see streaming.Message).
// The minimal call form — streamId + body — yields a Message with
// From=caller and Body=body. Optional fields (to, subject, threadId,
// inReplyTo, messageId, bodyContentType, attachments) let the caller
// publish a richer envelope when threading or recipients matter.
type Publish struct {
	deps Deps
}

const PublishName tool.Name = "publish"

var publishSchema = mustSchema[publishArgs]()

func (t *Publish) Name() tool.Name { return PublishName }
func (t *Publish) Description() string {
	return "Append an Event with the given body to a Stream. Wakes long-poll observers and " +
		"activates every subscribed AI Worker. Optional fields (to, subject, threadId, " +
		"inReplyTo, messageId, attachments) carry threading and recipient metadata for " +
		"messaging streams; omit them for plain text publishes."
}
func (t *Publish) InputSchema() *jsonschema.Schema { return publishSchema }

type publishArgs struct {
	StreamID        string                 `json:"streamId"`
	Body            string                 `json:"body"`
	To              []string               `json:"to,omitempty"`
	Subject         string                 `json:"subject,omitempty"`
	BodyContentType string                 `json:"bodyContentType,omitempty"`
	ThreadID        string                 `json:"threadId,omitempty"`
	InReplyTo       string                 `json:"inReplyTo,omitempty"`
	MessageID       string                 `json:"messageId,omitempty"`
	Attachments     []streaming.Attachment `json:"attachments,omitempty"`
}

func (t *Publish) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args publishArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.StreamID == "" || args.Body == "" {
		return nil, fmt.Errorf("streamId and body are required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("publish: caller has no OrgID")
	}
	streamID := streaming.StreamID(args.StreamID)
	msg := streaming.Message{
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
	// The service owns the append→notify→dispatch trio and rejects
	// github-transport streams (inbound only — act on the repo with `gh`
	// from the Environment) with ErrPublishToGitHub.
	event, err := t.deps.publishingService().Publish(ctx, orgID, streamID, string(inv.Caller.ID()), msg)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(event.ID), "streamId": string(streamID)})
}
