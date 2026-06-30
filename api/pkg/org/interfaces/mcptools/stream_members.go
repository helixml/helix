package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// TopicMembers returns the Worker IDs subscribed to a Topic right
// now. Read-only and non-blocking — the manager-style use case is "is
// the worker I'm about to message actually listening?". Composes with
// any outstanding-task tracking the caller does: see who's listening,
// and if the right party isn't, defer the work and reconcile later.
type TopicMembers struct {
	deps Deps
}

const TopicMembersName tool.Name = "topic_members"

var topicMembersSchema = mustSchema[topicMembersArgs]()

func (t *TopicMembers) Name() tool.Name                 { return TopicMembersName }
func (t *TopicMembers) InputSchema() *jsonschema.Schema { return topicMembersSchema }
func (t *TopicMembers) Description() string {
	return "List the Worker IDs currently subscribed to a Topic. Returns immediately. " +
		"Use this before publishing if you need to know whether a particular Worker is listening — " +
		"e.g. before sending the first recruiting brief, check that the recruiter is subscribed."
}

type topicMembersArgs struct {
	TopicID string `json:"topicId"`
}

func (t *TopicMembers) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args topicMembersArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.TopicID == "" {
		return nil, fmt.Errorf("topicId is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("topic_members: caller has no OrgID")
	}
	topicID := streaming.TopicID(args.TopicID)
	if _, err := t.deps.Queries.GetTopic(ctx, orgID, topicID); err != nil {
		return nil, fmt.Errorf("topic %q: %w", topicID, err)
	}
	subs, err := t.deps.Queries.TopicSubscribers(ctx, orgID, topicID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	members := make([]orgchart.BotID, 0, len(subs))
	for _, sub := range subs {
		members = append(members, orgchart.BotID(sub.BotID))
	}
	return json.Marshal(map[string]any{
		"topicId": string(topicID),
		"members": members,
	})
}
