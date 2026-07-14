package orgchart

import (
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// BotKind distinguishes an ordinary agent Bot from a human placeholder.
// A Go alias (not a named type) so it stays a plain string at the
// boundary, matching BotID.
type BotKind = string

// BotKindHuman marks a Bot as a human placeholder — never activated,
// reachable via its Identity handles. The empty kind is an agent Bot.
const BotKindHuman BotKind = "human"

// Bot is the single org-chart aggregate: the merge of the former Role
// and Worker. A Bot *is* its own job description (no role binding). Kind
// distinguishes an ordinary agent Bot (the default) from a human
// placeholder (BotKindHuman) — see Kind below; there is otherwise no
// per-Bot subtype.
//
// ID is the stable, filesystem-safe handle (it names the runtime env,
// repo and agent app, and is referenced by MCP tools). Name is the
// human-readable display label shown in the UI; it is free text and may
// be empty (callers fall back to the ID). Renaming a Bot changes Name,
// never ID.
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
// Kind is "" for an ordinary agent Bot (the default) or BotKindHuman for
// a human placeholder — a real person represented in the graph. A human
// Bot is never spawned/activated: its Content is the person's
// responsibility description, its Identity holds their cross-system
// handles (Slack, GitHub, email, …), and HelixUserID optionally links it
// to a real Helix org member so that signed-in user receives the in-app
// asks addressed to this node. See
// design/2026-07-07-humans-in-the-org.md.
//
// Reporting lines (who reports to whom), subscriptions, the per-Bot
// transcript/team/DM streams, and the runtime project/agent are all
// anchored on the Bot — see ReportingLine, streaming.Subscription, and
// the runtime packages.
type Bot struct {
	ID             BotID
	OrganizationID string
	// Name is the human-readable display label (e.g. "Chief of Staff").
	// Free text, may be empty — the UI falls back to ID. Distinct from
	// ID, which is the immutable handle.
	Name    string
	Content string
	Tools   []tool.Name
	// ProjectIDs is the explicit allowlist of Helix projects this Bot may
	// target through cross-project tools. The Bot's own runtime project is
	// always allowed and remains the default when a tool omits project_id.
	ProjectIDs []string
	// PreserveContext, when true, tells the runtime spawner NOT to wipe
	// the Bot's chat session before each re-activation. The default
	// (false) keeps the existing behaviour: every trigger starts on a
	// fresh context window. Enabling it lets the Bot accumulate context
	// across triggers (faster, more context-aware follow-ups — e.g. for
	// Slack), at the cost of the session growing toward the model's
	// context limit. See infrastructure/runtime/helix/spawner.go.
	PreserveContext bool
	// Kind is "" (agent, the default) or BotKindHuman. A human Bot is
	// never spawned — the dispatcher delivers to it instead of activating.
	Kind BotKind
	// HelixUserID optionally links a human Bot to a real Helix org member.
	// Set → that signed-in user receives the in-app asks addressed here.
	// Empty for agent Bots and for humans with no Helix account.
	HelixUserID string
	// Identity maps a channel name (slack, github, email, discord, …) to
	// the person's handle on that channel — how the org reaches them.
	// Only meaningful for a human Bot; nil for agents.
	Identity  map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
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

// WithName returns a copy of the Bot with Name (the display label)
// replaced.
func (b Bot) WithName(name string) Bot {
	b.Name = name
	return b
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

// WithProjectIDs returns a copy of the Bot with its project allowlist replaced.
func (b Bot) WithProjectIDs(projectIDs []string) Bot {
	b.ProjectIDs = projectIDs
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

// WithKind returns a copy of the Bot with Kind replaced.
func (b Bot) WithKind(kind BotKind) Bot {
	b.Kind = kind
	return b
}

// WithHelixUserID returns a copy of the Bot with HelixUserID replaced.
func (b Bot) WithHelixUserID(userID string) Bot {
	b.HelixUserID = userID
	return b
}

// WithIdentity returns a copy of the Bot with Identity replaced.
func (b Bot) WithIdentity(identity map[string]string) Bot {
	b.Identity = identity
	return b
}

// IsHuman reports whether this Bot is a human placeholder.
func (b Bot) IsHuman() bool { return b.Kind == BotKindHuman }
