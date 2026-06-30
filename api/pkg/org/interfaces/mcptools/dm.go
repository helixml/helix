package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/channels"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// DM sends a direct message to a single other Worker over the 1:1
// channel the two share.
//
// IMPORTANT — DM access is scoped to the reporting graph. A DM channel
// is NOT created on demand here; it is provisioned by the topology
// reconciler for every reporting edge (manager ↔ report). So a Worker
// can only DM the people it shares a reporting line with — its managers
// (escalate up) and its direct reports (message down 1:1). DMing an
// arbitrary peer or a skip-level Worker has no channel and is refused.
// This is deliberate: peer-to-peer / cross-tree reach is a decision the
// org makes by wiring a reporting line (or explicitly creating a
// topic), not something any Worker can do implicitly to anyone.
//
// The Topic ID is deterministic from the sorted pair
// (channels.DMTopicID), so A→B and B→A land on the same Topic and the
// back-and-forth stays ordered in one place. The managers / reports
// read tools hand back that same id so callers know which channel to use.
type DM struct {
	deps Deps
}

const DMName tool.Name = "dm"

var dmSchema = mustSchema[dmArgs]()

func (t *DM) Name() tool.Name { return DMName }
func (t *DM) Description() string {
	return "Send a direct message (DM/PM/private message) to a single other Bot. " +
		"You can only DM bots you share a reporting line with — your managers " +
		"(call `managers` to find them and escalate up) or your direct reports (call " +
		"`reports` to message one 1:1). There is NO implicit DM channel to an " +
		"arbitrary peer or a skip-level bot: those have no channel and the call is " +
		"refused. The channel is provisioned automatically when the reporting line " +
		"exists. Publishes the body and returns the topicId — read_events on it " +
		"(with wait) to catch the reply."
}
func (t *DM) InputSchema() *jsonschema.Schema { return dmSchema }

type dmArgs struct {
	ToBotID string `json:"toBotId"`
	Body    string `json:"body"`
}

func (t *DM) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args dmArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ToBotID == "" || args.Body == "" {
		return nil, fmt.Errorf("toBotId and body are required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("dm: caller has no OrgID")
	}
	sender := inv.Caller.ID()
	recipient := orgchart.BotID(args.ToBotID)
	if sender == recipient {
		return nil, fmt.Errorf("cannot DM yourself")
	}
	if _, err := t.deps.Queries.GetBot(ctx, orgID, recipient); err != nil {
		return nil, fmt.Errorf("recipient %q: %w", recipient, err)
	}

	// The DM channel must already exist. The reconciler provisions one per
	// reporting edge; we do NOT create it here. A missing channel means
	// the caller has no reporting relationship with the recipient — say
	// so plainly rather than silently minting a channel that the org
	// never sanctioned.
	topicID := channels.DMTopicID(sender, recipient)
	if _, err := t.deps.Queries.GetTopic(ctx, orgID, topicID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf(
				"no DM channel with %q: you can only DM bots you share a reporting line with — "+
					"your managers (call `managers`) or your direct reports (call `reports`). "+
					"To reach %q directly, a reporting line must be wired between you; "+
					"otherwise escalate to a manager or brief via your team topic",
				recipient, recipient)
		}
		return nil, fmt.Errorf("lookup dm topic %q: %w", topicID, err)
	}

	msg := streaming.Message{
		From: string(sender),
		To:   []string{string(recipient)},
		Body: args.Body,
	}
	// Delegate to the publishing service — the single owner of the
	// append → notify → dispatch trio. dm must NOT reimplement it (that
	// is how the DM fan-out drifts from every other publish path).
	event, err := t.deps.Publishing.Publish(ctx, orgID, topicID, string(sender), msg)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{
		"id":       string(event.ID),
		"topicId": string(topicID),
		"to":       string(recipient),
	})
}
