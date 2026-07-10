package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// HumanInbox delivers a bot's message to a person's in-app inbox (the
// notification bell). Implemented at the composition root over the main
// Helix attention-event service, so mcptools stays decoupled from it. When
// nil, ask_human reports the inbox as unavailable.
type HumanInbox interface {
	// Notify writes an inbox item for the person's linked Helix user.
	// fromBotID is the sending bot's id (routing / reply target); fromBotName is
	// its display name (shown in the notification title).
	Notify(ctx context.Context, orgID, userID, fromBotID, fromBotName, personName, message string) error
}

// AskHuman lets a bot reach a real person directly: it delivers the message to
// the person's in-app inbox (the notification bell). Unlike `dm`, ANY bot can
// reach ANY person — there is no reporting-line requirement, because reaching a
// human is delivery, not a graph traversal. The person is a human node
// (kind=human) linked to a Helix account. Reply routing + external channels
// (email/Slack) are later stages; this is the in-app forward path.
type AskHuman struct {
	deps Deps
}

const AskHumanName tool.Name = "ask_human"

var askHumanSchema = mustSchema[askHumanArgs]()

func (t *AskHuman) Name() tool.Name                 { return AskHumanName }
func (t *AskHuman) InputSchema() *jsonschema.Schema { return askHumanSchema }
func (t *AskHuman) Description() string {
	return "Reach a real person: deliver a message to their in-app inbox (the notification " +
		"bell). Use this when you need a human's input, a decision, or to flag something to " +
		"them. `personId` is a person in the org — a human node whose id looks like `h-…`; " +
		"find people with `read_bots` (they have kind=human). Unlike `dm`, any bot can reach " +
		"any person directly — no reporting line needed. `message` is what to tell them."
}

type askHumanArgs struct {
	PersonID string `json:"personId"`
	Message  string `json:"message"`
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
	if t.deps.HumanInbox == nil {
		return nil, fmt.Errorf("ask_human: inbox delivery is not configured")
	}
	person, err := t.deps.Queries.GetBot(ctx, orgID, orgchart.BotID(args.PersonID))
	if err != nil {
		return nil, fmt.Errorf("person %q: %w", args.PersonID, err)
	}
	if !person.IsHuman() {
		return nil, fmt.Errorf("%q is not a person (kind=human)", args.PersonID)
	}
	if person.HelixUserID == "" {
		return nil, fmt.Errorf("person %q has no linked Helix account — cannot deliver in-app", args.PersonID)
	}
	name := person.Name
	if name == "" {
		name = string(person.ID)
	}
	// Prefer the sending bot's display name for the notification title (e.g.
	// "Chief of Staff", not "chief-of-staff"); fall back to its id.
	fromBotName := inv.Caller.ID()
	if caller, err := t.deps.Queries.GetBot(ctx, orgID, orgchart.BotID(inv.Caller.ID())); err == nil && caller.Name != "" {
		fromBotName = caller.Name
	}
	if err := t.deps.HumanInbox.Notify(ctx, orgID, person.HelixUserID, inv.Caller.ID(), fromBotName, name, args.Message); err != nil {
		return nil, fmt.Errorf("deliver to %q: %w", args.PersonID, err)
	}
	return json.Marshal(map[string]string{"delivered": "inbox", "person": args.PersonID})
}
