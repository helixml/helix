package orgchart

import (
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// NodeKind distinguishes an ordinary agent Node from a human placeholder.
// A Go alias (not a named type) so it stays a plain string at the
// boundary, matching NodeID.
type NodeKind = string

// NodeKindHuman marks a Node as a human placeholder — never activated,
// reachable via its Identity handles. The empty kind is an agent Node.
const NodeKindHuman NodeKind = "human"

// Node is the single org-chart aggregate: the merge of the former Role
// and Worker. A Node *is* its own job description (no role binding). Kind
// distinguishes an ordinary agent Node (the default) from a human
// placeholder (NodeKindHuman) — see Kind below; there is otherwise no
// per-Node subtype.
//
// ID is the stable, filesystem-safe handle (it names the runtime env,
// repo and agent app, and is referenced by MCP tools). Name is the
// human-readable display label shown in the UI; it is free text and may
// be empty (callers fall back to the ID). Renaming a Node changes Name,
// never ID.
//
// Content is the canonical markdown the Node's agent reads on activation
// (it lands in the runtime instruction file). Tools is
// the live source of truth for the Node's MCP surface: the helix-org MCP
// server registers exactly the tools in Node.Tools on every request, so
// editing a Node's Tools changes its capability on the next MCP request.
//
// A Node's subscriptions are NOT stored on the Node — they live as their
// own (bot, topic) rows (see streaming.Subscription / store.Subscriptions),
// which are the single source of truth. create_bot subscribes the new Node
// to its initial topics by creating those rows; subscribe/unsubscribe
// change them later.
//
// Kind is "" for an ordinary agent Node (the default) or NodeKindHuman for
// a human placeholder — a real person represented in the graph. A human
// Node is never spawned/activated: its Content is the person's
// responsibility description, its Identity holds their cross-system
// handles (Slack, GitHub, email, …), and HelixUserID optionally links it
// to a real Helix org member so that signed-in user receives the in-app
// asks addressed to this node. See
// design/2026-07-07-humans-in-the-org.md.
//
// Reporting lines (who reports to whom), subscriptions, the per-Node
// transcript/team/DM streams, and the runtime project/agent are all
// anchored on the Node — see ReportingLine, streaming.Subscription, and
// the runtime packages.
type Node struct {
	ID             NodeID
	OrganizationID string
	// AgentID is the canonical Helix Agent backing this org node.
	// It is required for nodes and empty for human placeholders.
	AgentID string
	// Name is the human-readable display label (e.g. "Chief of Staff").
	// Free text, may be empty — the UI falls back to ID. Distinct from
	// ID, which is the immutable handle.
	Name    string
	Content string
	Tools   []tool.Name
	// ProjectIDs is the explicit allowlist of Helix projects this Node may
	// target through cross-project tools. The Node's own runtime project is
	// always allowed and remains the default when a tool omits project_id.
	ProjectIDs []string
	// PreserveContext, when true, tells the runtime spawner NOT to wipe
	// the Node's chat session before each re-activation. The default
	// (false) keeps the existing behaviour: every trigger starts on a
	// fresh context window. Enabling it lets the Node accumulate context
	// across triggers (faster, more context-aware follow-ups — e.g. for
	// Slack), at the cost of the session growing toward the model's
	// context limit. See infrastructure/runtime/helix/spawner.go.
	PreserveContext bool
	// Kind is "" (agent, the default) or NodeKindHuman. A human Node is
	// never spawned — the dispatcher delivers to it instead of activating.
	Kind NodeKind
	// HelixUserID optionally links a human Node to a real Helix org member.
	// Set → that signed-in user receives the in-app asks addressed here.
	// Empty for agent Nodes and for humans with no Helix account.
	HelixUserID string
	// Identity maps a channel name (slack, github, email, discord, …) to
	// the person's handle on that channel — how the org reaches them.
	// Only meaningful for a human Node; nil for agents.
	Identity  map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewNode validates and constructs a Node. Treat the returned value as
// immutable. Tools may be empty; ID, Content, orgID, and now must all be
// non-empty (now non-zero). ID is additionally validated as a
// filesystem-safe handle (it lands in os.MkdirAll at activation time).
func NewNode(id NodeID, content string, tools []tool.Name, now time.Time, orgID string) (Node, error) {
	if err := ValidID(id); err != nil {
		return Node{}, err
	}
	if content == "" {
		return Node{}, errors.New("node content is empty")
	}
	if now.IsZero() {
		return Node{}, errors.New("node timestamp is zero")
	}
	if orgID == "" {
		return Node{}, errors.New("node orgID is empty")
	}
	return Node{
		ID:             id,
		OrganizationID: orgID,
		Content:        content,
		Tools:          tools,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// WithName returns a copy of the Node with Name (the display label)
// replaced.
func (n Node) WithName(name string) Node {
	n.Name = name
	return n
}

// WithAgentID returns a copy of the Node linked to the canonical Agent.
func (n Node) WithAgentID(agentID string) Node {
	n.AgentID = agentID
	return n
}

// WithContent returns a copy of the Node with Content replaced. The
// With* builders are the supported way to mutate a Node outside the
// domain package (immutability + tell-don't-ask) — the application
// service composes them instead of poking exported fields in a handler.
func (n Node) WithContent(content string) Node {
	n.Content = content
	return n
}

// WithTools returns a copy of the Node with Tools replaced.
func (n Node) WithTools(tools []tool.Name) Node {
	n.Tools = tools
	return n
}

// WithProjectIDs returns a copy of the Node with its project allowlist replaced.
func (n Node) WithProjectIDs(projectIDs []string) Node {
	n.ProjectIDs = projectIDs
	return n
}

// WithUpdatedAt returns a copy of the Node with UpdatedAt replaced.
func (n Node) WithUpdatedAt(t time.Time) Node {
	n.UpdatedAt = t
	return n
}

// WithPreserveContext returns a copy of the Node with PreserveContext
// replaced.
func (n Node) WithPreserveContext(preserve bool) Node {
	n.PreserveContext = preserve
	return n
}

// WithKind returns a copy of the Node with Kind replaced.
func (n Node) WithKind(kind NodeKind) Node {
	n.Kind = kind
	return n
}

// WithHelixUserID returns a copy of the Node with HelixUserID replaced.
func (n Node) WithHelixUserID(userID string) Node {
	n.HelixUserID = userID
	return n
}

// WithIdentity returns a copy of the Node with Identity replaced.
func (n Node) WithIdentity(identity map[string]string) Node {
	n.Identity = identity
	return n
}

// IsHuman reports whether this Node is a human placeholder.
func (n Node) IsHuman() bool { return n.Kind == NodeKindHuman }
