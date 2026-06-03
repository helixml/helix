package orgchart

import (
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Role is a job description. Owner-only: Workers cannot edit their own
// Role. The owner edits Content via UpdateRole, and the new content
// fans out to every Worker filling a Position with this Role.
//
// Content is the canonical markdown the Worker reads on activation
// (it lands in role.md inside the Worker's Environment). Identity
// (name, voice, personality) is per-Worker, not per-Role.
//
// Tools and Streams are typed manifests: the list of MCP tools the
// Role's prompt expects to be granted on hire, and the list of
// Streams it expects to operate on. They are reference data only —
// hire_worker does NOT enforce them, does NOT auto-grant, and does
// NOT auto-subscribe. The hiring caller is responsible for issuing
// matching grants and subscriptions.
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
