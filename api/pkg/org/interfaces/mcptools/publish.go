package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Publish appends an Event to a named Topic, attributed to the caller.
// It does exactly one thing: append an event to an existing Topic. It
// does not create Topics or manage subscriptions; for the common
// "direct message a Worker" case, see the dm tool, which bundles
// create-topic + subscribe-both + publish into a single call.
//
// All events are stored as canonical Message JSON (see streaming.Message).
// The minimal call form — topicId + body — yields a Message with
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
	return "Append and route an Event through Helix. Publishing basic text to a configured Slack Topic posts it to Slack and returns a delivery receipt. " +
		"Use ask_human to contact a person. For rich Slack actions such as reactions, uploads, or edits, call mint_credential and use the Slack API directly. Wakes long-poll observers and " +
		"activates every subscribed AI Worker. Optional fields (to, subject, threadId, " +
		"inReplyTo, messageId, attachments) carry threading and recipient metadata for " +
		"messaging topics; omit them for plain text publishes."
}
func (t *Publish) InputSchema() *jsonschema.Schema { return publishSchema }

type publishArgs struct {
	TopicID         string                 `json:"topicId"`
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
	if args.TopicID == "" || args.Body == "" {
		return nil, fmt.Errorf("topicId and body are required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("publish: caller has no OrgID")
	}
	topicID := streaming.TopicID(args.TopicID)
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
	// github-transport topics (inbound only — act on the repo with `gh`
	// from the Environment) with ErrPublishToGitHub.
	result, err := t.deps.Publishing.PublishWithReceipt(ctx, orgID, topicID, string(inv.Caller.ID()), msg)
	if err != nil {
		return nil, err
	}
	response := map[string]any{
		"id":      string(result.Event.ID),
		"topicId": string(topicID),
		"scope":   "helix",
		"status":  "appended inside Helix; external delivery not confirmed",
		"delivery": map[string]string{
			"status":   "not_applicable",
			"provider": "helix",
		},
	}
	if result.Delivery != nil {
		response["delivery"] = result.Delivery
		response["status"] = "appended inside Helix and delivered to " + result.Delivery.Provider
	}
	return json.Marshal(response)
}
