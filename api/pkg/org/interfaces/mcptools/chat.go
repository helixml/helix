package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Chat sends a message into a named internal conversation — a team chat,
// or any other local Trigger the org has created as a channel. It does
// exactly one thing: append the message to that conversation, which
// activates every other Worker attached to it.
//
// Chat is the multi-party counterpart of dm: dm addresses one Worker over
// the 1:1 channel the reporting line provisions, chat addresses everyone
// attached to a named conversation.
//
// It cannot reach outside the org. A Trigger backed by an external
// transport (GitHub, Slack, email, …) is inbound-only; to act on a
// provider, fetch a granted credential with get_secret and call that
// provider's own CLI or HTTP API.
//
// All events are stored as canonical Message JSON (see streaming.Message).
// The minimal call form — triggerId + body — yields a Message with
// From=caller and Body=body. Optional fields (to, subject, threadId,
// inReplyTo, messageId, bodyContentType, attachments) let the caller send
// a richer envelope when threading or recipients matter.
type Chat struct {
	deps Deps
}

const ChatName tool.Name = "chat"

var chatSchema = mustSchema[chatArgs]()

func (t *Chat) Name() tool.Name { return ChatName }
func (t *Chat) Description() string {
	return "Send a message into a named internal conversation (a team chat, or another " +
		"local channel). Every other Worker attached to that conversation is activated " +
		"with your message, and long-poll readers wake. Pass `triggerId` (from " +
		"list_triggers) and `body`. Use `dm` instead for a 1:1 with a single Worker. " +
		"chat is internal-only: to act on GitHub, Slack, email or any other provider, " +
		"call get_secret and use that provider's own CLI or HTTP API. Optional fields " +
		"(to, subject, threadId, inReplyTo, messageId, attachments) carry threading and " +
		"recipient metadata; omit them for plain text."
}
func (t *Chat) InputSchema() *jsonschema.Schema { return chatSchema }

type chatArgs struct {
	TriggerID       string                 `json:"triggerId"`
	Body            string                 `json:"body"`
	To              []string               `json:"to,omitempty"`
	Subject         string                 `json:"subject,omitempty"`
	BodyContentType string                 `json:"bodyContentType,omitempty"`
	ThreadID        string                 `json:"threadId,omitempty"`
	InReplyTo       string                 `json:"inReplyTo,omitempty"`
	MessageID       string                 `json:"messageId,omitempty"`
	Attachments     []streaming.Attachment `json:"attachments,omitempty"`
}

func (t *Chat) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args chatArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.TriggerID == "" || args.Body == "" {
		return nil, fmt.Errorf("triggerId and body are required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("chat: caller has no OrgID")
	}
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
	// The service owns append, notify, and route, and rejects a send to
	// anything that is not an internal channel before appending.
	event, err := t.deps.Publishing.SendToChannel(ctx, orgID, args.TriggerID, string(inv.Caller.ID()), msg)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{
		"id":        string(event.ID),
		"triggerId": args.TriggerID,
	})
}
