package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// HumanDelivery delivers a bot's message through the person's configured
// contact route. The composition root owns the concrete Helix and Slack APIs.
type HumanDelivery interface {
	Deliver(ctx context.Context, orgID string, person orgchart.Bot, fromBotID, fromBotName, message string, expectsReply bool) (string, error)
}

// AskHuman lets a bot reach a real person directly: it delivers the message to
// the person's in-app inbox (the notification bell). Unlike `dm`, ANY bot can
// reach ANY person — there is no reporting-line requirement, because reaching a
// human is delivery, not a graph traversal. The person is a human node
// (kind=human) with a configured contact identity.
type AskHuman struct {
	deps Deps
}

const AskHumanName tool.Name = "ask_human"

var askHumanSchema = mustSchema[askHumanArgs]()

func (t *AskHuman) Name() tool.Name                 { return AskHumanName }
func (t *AskHuman) InputSchema() *jsonschema.Schema { return askHumanSchema }
func (t *AskHuman) Description() string {
	return "Reach a real person through their preferred contact route. Use this when you need a human's input, a decision, or to flag something to " +
		"them. `personId` is a person in the org — a human node whose id looks like `h-…`; " +
		"find people with `read_bots` (they have kind=human). Unlike `dm`, any bot can reach " +
		"any person directly — no reporting line needed. `message` is what to tell them. " +
		"Set `expectsReply` to false for status updates, confirmations, and FYIs that do NOT " +
		"need an answer — those are delivered as read-only notices with no reply prompt. " +
		"Leave it unset (default true) only when you genuinely want the person to respond."
}

type askHumanArgs struct {
	PersonID string `json:"personId"`
	Message  string `json:"message"`
	// ExpectsReply defaults to true when omitted. Set false for FYIs.
	ExpectsReply *bool `json:"expectsReply,omitempty"`
}

func (t *AskHuman) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args askHumanArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.PersonID == "" || args.Message == "" {
		return nil, fmt.Errorf("personId and message are required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("ask_human: caller has no OrgID")
	}
	if t.deps.HumanDelivery == nil {
		return nil, fmt.Errorf("ask_human: human delivery is not configured")
	}
	person, err := t.deps.Queries.GetBot(ctx, orgID, orgchart.BotID(args.PersonID))
	if err != nil {
		return nil, fmt.Errorf("person %q: %w", args.PersonID, err)
	}
	if !person.IsHuman() {
		return nil, fmt.Errorf("%q is not a person (kind=human)", args.PersonID)
	}
	// Prefer the sending bot's display name for the notification title (e.g.
	// "Chief of Staff", not "chief-of-staff"); fall back to its id.
	fromBotName := inv.Caller.ID()
	if caller, err := t.deps.Queries.GetBot(ctx, orgID, orgchart.BotID(inv.Caller.ID())); err == nil && caller.Name != "" {
		fromBotName = caller.Name
	}
	expectsReply := args.ExpectsReply == nil || *args.ExpectsReply
	delivered, err := t.deps.HumanDelivery.Deliver(ctx, orgID, person, inv.Caller.ID(), fromBotName, args.Message, expectsReply)
	if err != nil {
		return nil, fmt.Errorf("deliver to %q: %w", args.PersonID, err)
	}
	return json.Marshal(map[string]string{"delivered": delivered, "person": args.PersonID})
}
