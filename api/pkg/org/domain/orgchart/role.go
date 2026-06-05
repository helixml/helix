package orgchart

import (
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Role is a job description. Owner-only: Workers cannot edit their own
// Role. The owner edits Content (and Tools) via UpdateRole, and the
// new value fans out to every Worker holding this Role.
//
// Content is the canonical markdown the Worker reads on activation
// (it lands in role.md inside the Worker's Environment). Identity
// (name, voice, personality) is per-Worker, not per-Role.
//
// Tools is the live source of truth for a Worker's MCP surface: the
// helix-org MCP server resolves Worker → Role on every request and
// registers exactly the tools in Role.Tools. To change a Worker's
// capabilities, call update_role; capability is not a per-Worker
// attribute.
//
// Streams is a typed manifest the Role's prompt is expected to
// subscribe its Workers to. The store does NOT auto-subscribe; the
// hiring caller drives create_stream/subscribe explicitly because
// stream lifecycle (creation, transport config, cross-Role sharing)
// can't be derived mechanically from the Role.
type Role struct {
	ID             RoleID
	OrganizationID string
	Content        string
	Tools          []tool.Name
	Streams        []streaming.StreamID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewRole validates and constructs a Role. Treat the returned value
// as immutable. Tools and Streams may be empty; ID, Content, orgID,
// and now must all be non-empty (now non-zero).
func NewRole(id RoleID, content string, tools []tool.Name, streams []streaming.StreamID, now time.Time, orgID string) (Role, error) {
	if id == "" {
		return Role{}, errors.New("role id is empty")
	}
	if content == "" {
		return Role{}, errors.New("role content is empty")
	}
	if now.IsZero() {
		return Role{}, errors.New("role timestamp is zero")
	}
	if orgID == "" {
		return Role{}, errors.New("role orgID is empty")
	}
	return Role{
		ID:             id,
		OrganizationID: orgID,
		Content:        content,
		Tools:          tools,
		Streams:        streams,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}
