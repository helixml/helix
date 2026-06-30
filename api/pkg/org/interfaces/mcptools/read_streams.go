package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

type topicView struct {
	ID            streaming.TopicID `json:"id"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	CreatedBy     orgchart.BotID  `json:"createdBy"`
	CreatedAt     time.Time          `json:"createdAt"`
	TransportKind string             `json:"transportKind"`
}

func topicViewOf(s streaming.Topic) topicView {
	return topicView{
		ID:            s.ID,
		Name:          s.Name,
		Description:   s.Description,
		CreatedBy:     s.CreatedBy,
		CreatedAt:     s.CreatedAt,
		TransportKind: string(s.Transport.Kind),
	}
}

// ListTopics returns every Topic.
type ListTopics struct {
	deps Deps
}

const ListTopicsName tool.Name = "list_topics"

var listTopicsSchema = mustSchema[listTopicsArgs]()

type listTopicsArgs struct{}

func (t *ListTopics) Name() tool.Name                 { return ListTopicsName }
func (t *ListTopics) InputSchema() *jsonschema.Schema { return listTopicsSchema }
func (t *ListTopics) Description() string {
	return "List every Topic: id, name, description, creator, transport kind, and created-at."
}

func (t *ListTopics) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("list_topics: caller has no OrgID")
	}
	topics, err := t.deps.Queries.ListTopics(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	out := make([]topicView, 0, len(topics))
	for _, s := range topics {
		out = append(out, topicViewOf(s))
	}
	return json.Marshal(map[string]any{"topics": out})
}

// GetTopic returns one Topic by ID.
type GetTopic struct {
	deps Deps
}

const GetTopicName tool.Name = "get_topic"

var getTopicSchema = mustSchema[getTopicArgs]()

type getTopicArgs struct {
	ID string `json:"id"`
}

func (t *GetTopic) Name() tool.Name                 { return GetTopicName }
func (t *GetTopic) InputSchema() *jsonschema.Schema { return getTopicSchema }
func (t *GetTopic) Description() string {
	return "Fetch one Topic by id."
}

func (t *GetTopic) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args getTopicArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("get_topic: caller has no OrgID")
	}
	s, err := t.deps.Queries.GetTopic(ctx, orgID, streaming.TopicID(args.ID))
	if err != nil {
		return nil, fmt.Errorf("get topic %q: %w", args.ID, err)
	}
	return json.Marshal(topicViewOf(s))
}

// ListTopicEvents returns recent Events on one Topic, newest first.
// Non-blocking — callers who want to wait for new events use read_events.
type ListTopicEvents struct {
	deps Deps
}

const ListTopicEventsName tool.Name = "list_topic_events"

var listTopicEventsSchema = mustSchema[listTopicEventsArgs]()

const (
	listTopicEventsDefaultLimit = 50
	listTopicEventsMaxLimit     = 200
)

type listTopicEventsArgs struct {
	TopicID string `json:"topicId"`
	Limit    int    `json:"limit,omitempty"`
}

// UnmarshalJSON tolerates a string-encoded Limit — same LLM-quirk
// fix as read_events. See decodeFlexInt comment.
func (a *listTopicEventsArgs) UnmarshalJSON(data []byte) error {
	type plain listTopicEventsArgs
	type tolerant struct {
		*plain
		Limit json.RawMessage `json:"limit,omitempty"`
	}
	t := tolerant{plain: (*plain)(a)}
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	v, err := decodeFlexInt(t.Limit)
	if err != nil {
		return fmt.Errorf("limit: %w", err)
	}
	a.Limit = v
	return nil
}

func (t *ListTopicEvents) Name() tool.Name                 { return ListTopicEventsName }
func (t *ListTopicEvents) InputSchema() *jsonschema.Schema { return listTopicEventsSchema }
func (t *ListTopicEvents) Description() string {
	return "List recent Events on a Topic, newest first. Returns immediately. limit defaults " +
		"to 50, capped at 200."
}

func (t *ListTopicEvents) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args listTopicEventsArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.TopicID == "" {
		return nil, fmt.Errorf("topicId is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("list_topic_events: caller has no OrgID")
	}
	limit := args.Limit
	if limit <= 0 {
		limit = listTopicEventsDefaultLimit
	}
	if limit > listTopicEventsMaxLimit {
		limit = listTopicEventsMaxLimit
	}
	topicID := streaming.TopicID(args.TopicID)
	if _, err := t.deps.Queries.GetTopic(ctx, orgID, topicID); err != nil {
		return nil, fmt.Errorf("topic %q: %w", topicID, err)
	}
	events, err := t.deps.Queries.TopicEvents(ctx, orgID, topicID, limit)
	if err != nil {
		return nil, fmt.Errorf("list events for %q: %w", topicID, err)
	}
	out := make([]eventView, 0, len(events))
	for _, e := range events {
		out = append(out, eventViewOf(e))
	}
	return json.Marshal(map[string]any{"events": out})
}
