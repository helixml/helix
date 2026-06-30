package orgchart

import (
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Bot is the single org-chart aggregate: the merge of the former Role
// and Worker. A Bot has no identity beyond its name (ID) — there is no
// kind (human/ai), no separate identity description, and no role
// binding. A Bot *is* its own job description.
//
// Content is the canonical markdown the Bot's agent reads on activation
// (it lands in role.md inside the Bot's runtime environment). Tools is
// the live source of truth for the Bot's MCP surface: the helix-org MCP
// server registers exactly the tools in Bot.Tools on every request, so
// editing a Bot's Tools changes its capability on the next MCP request.
//
// Topics is a typed manifest the Bot's prompt is expected to subscribe
// to. The store does NOT auto-subscribe; the caller drives
// create_topic/subscribe explicitly because topic lifecycle can't be
// derived mechanically from the Bot.
//
// Reporting lines (who reports to whom), subscriptions, the per-Bot
// transcript/team/DM streams, and the runtime project/agent are all
// anchored on the Bot — see ReportingLine, streaming.Subscription, and
// the runtime packages.
type Bot struct {
	ID             BotID
	OrganizationID string
	Content        string
	Tools          []tool.Name
	Topics         []streaming.TopicID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewBot validates and constructs a Bot. Treat the returned value as
// immutable. Tools and Topics may be empty; ID, Content, orgID, and now
// must all be non-empty (now non-zero). ID is additionally validated as
// a filesystem-safe handle (it lands in os.MkdirAll at activation time).
func NewBot(id BotID, content string, tools []tool.Name, topics []streaming.TopicID, now time.Time, orgID string) (Bot, error) {
	if err := ValidID(id); err != nil {
		return Bot{}, err
	}
	if content == "" {
		return Bot{}, errors.New("bot content is empty")
	}
	if now.IsZero() {
		return Bot{}, errors.New("bot timestamp is zero")
	}
	if orgID == "" {
		return Bot{}, errors.New("bot orgID is empty")
	}
	return Bot{
		ID:             id,
		OrganizationID: orgID,
		Content:        content,
		Tools:          tools,
		Topics:         topics,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// WithContent returns a copy of the Bot with Content replaced. The
// With* builders are the supported way to mutate a Bot outside the
// domain package (immutability + tell-don't-ask) — the application
// service composes them instead of poking exported fields in a handler.
func (b Bot) WithContent(content string) Bot {
	b.Content = content
	return b
}

// WithTools returns a copy of the Bot with Tools replaced.
func (b Bot) WithTools(tools []tool.Name) Bot {
	b.Tools = tools
	return b
}

// WithTopics returns a copy of the Bot with Topics replaced.
func (b Bot) WithTopics(topics []streaming.TopicID) Bot {
	b.Topics = topics
	return b
}

// WithUpdatedAt returns a copy of the Bot with UpdatedAt replaced.
func (b Bot) WithUpdatedAt(t time.Time) Bot {
	b.UpdatedAt = t
	return b
}
