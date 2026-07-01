package orgchart

import (
	"errors"
	"time"

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
// A Bot's subscriptions are NOT stored on the Bot — they live as their
// own (bot, topic) rows (see streaming.Subscription / store.Subscriptions),
// which are the single source of truth. create_bot subscribes the new Bot
// to its initial topics by creating those rows; subscribe/unsubscribe
// change them later.
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
	// PreserveContext, when true, tells the runtime spawner NOT to wipe
	// the Bot's chat session before each re-activation. The default
	// (false) keeps the existing behaviour: every trigger starts on a
	// fresh context window. Enabling it lets the Bot accumulate context
	// across triggers (faster, more context-aware follow-ups — e.g. for
	// Slack), at the cost of the session growing toward the model's
	// context limit. See infrastructure/runtime/helix/spawner.go.
	PreserveContext bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewBot validates and constructs a Bot. Treat the returned value as
// immutable. Tools may be empty; ID, Content, orgID, and now must all be
// non-empty (now non-zero). ID is additionally validated as a
// filesystem-safe handle (it lands in os.MkdirAll at activation time).
func NewBot(id BotID, content string, tools []tool.Name, now time.Time, orgID string) (Bot, error) {
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

// WithUpdatedAt returns a copy of the Bot with UpdatedAt replaced.
func (b Bot) WithUpdatedAt(t time.Time) Bot {
	b.UpdatedAt = t
	return b
}

// WithPreserveContext returns a copy of the Bot with PreserveContext
// replaced.
func (b Bot) WithPreserveContext(preserve bool) Bot {
	b.PreserveContext = preserve
	return b
}
