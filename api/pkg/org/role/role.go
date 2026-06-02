// Package role owns the Role concept: a job description in markdown,
// plus two typed manifests declaring the tools and streams the Role's
// prompt expects on each hire.
//
// Canonical home, lifted from helix-org/domain/role.go in B7. See
// helix-org/design/2026-05-21-redesign/08-migration-plan.md and the
// canonical-location rule in helix-org/CLAUDE.md.
package role

import (
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/tool"
)

// ID identifies a Role. Convention: `r-<kebab-case>` (e.g.
// `r-secretary`, `r-software-engineer`). Stored as a string so
// external systems (logs, URLs, JSON) can carry it unchanged.
type ID string

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
// Streams it expects to operate on. They are **reference data only**
// — hire_worker does NOT enforce them, does NOT auto-grant, and does
// NOT auto-subscribe. The hiring caller is responsible for issuing
// matching grants and subscriptions. The fields exist so the hiring
// caller's prompt can read them programmatically (via get_role)
// rather than parsing the `## Tools (MCP)` and `## Streams` sections
// out of Content. See ADR-0001 §5 and the design-philosophy bullet
// "No workflow in code" in helix-org/CLAUDE.md.
//
// Either field may be empty: a Role with no Tools/Streams declared is
// still valid — the hiring caller's prompt is then responsible for
// figuring out what to grant/subscribe from Content alone.
type Role struct {
	ID             ID
	OrganizationID string
	Content        string
	Tools          []tool.Name
	Streams        []stream.ID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// New validates and constructs a Role. Treat the returned value as
// immutable. Tools and Streams may be empty; ID, Content, orgID, and
// now must all be non-empty (now non-zero). orgID is required because
// every Role is scoped to a helix.Organization — the composite (id,
// org_id) PK is what lets short readable IDs (`r-owner`, `r-cfo`)
// repeat across tenants.
func New(id ID, content string, tools []tool.Name, streams []stream.ID, now time.Time, orgID string) (Role, error) {
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
