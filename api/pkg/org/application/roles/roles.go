// Package roles is the application service that owns the structural
// Role use cases — Create and Update. It is the single home for the
// role-mutation logic the MCP create_role/update_role tools and the
// REST role handlers used to each implement independently (and had
// already drifted: the MCP update_role rebuilt the Role with only
// Content, silently wiping Tools and Streams — this service fixes that
// by doing a proper read-modify-write that preserves unpatched fields).
//
// The service depends only on the narrow store.Roles repository plus a
// clock, an id-generator, and the injected base-tool list — never the
// whole *store.Store (CLAUDE.md §5.0: small interfaces, ≤4
// collaborators). BaseTools is injected (rather than imported from the
// tools package) to keep the dependency edge one-way: the tools package
// imports this service, not the reverse.
package roles

import (
	"context"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Roles owns the role-mutation use cases.
type Roles struct {
	roles     store.Roles
	now       func() time.Time
	newID     func() string
	baseTools []tool.Name
}

// Deps are the constructor-injected collaborators for New.
type Deps struct {
	Roles store.Roles
	Now   func() time.Time
	NewID func() string
	// BaseTools is the universal read baseline unioned into every
	// created Role so no Role can miss the read primitives every Worker
	// needs. Injected by the wiring (tools.BaseReadTools) to avoid an
	// import cycle.
	BaseTools []tool.Name
}

// New constructs the Roles service.
func New(deps Deps) *Roles {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Roles{roles: deps.Roles, now: now, newID: deps.NewID, baseTools: deps.BaseTools}
}

// CreateParams describes a new Role. ID is optional — when empty a
// fresh `r-<id>` is minted. Tools is unioned with the injected base
// read tools; Streams is the typed manifest stored verbatim.
type CreateParams struct {
	ID      string
	Content string
	Tools   []tool.Name
	Streams []streaming.StreamID
}

// Create builds and persists a new Role, returning the created
// aggregate. The caller's tools are unioned with the base read tools
// (caller order preserved, baseline appended, deduped).
func (r *Roles) Create(ctx context.Context, orgID string, p CreateParams) (orgchart.Role, error) {
	id := orgchart.RoleID(strings.TrimSpace(p.ID))
	if id == "" {
		id = orgchart.RoleID("r-" + r.newID())
	}
	role, err := orgchart.NewRole(id, p.Content, MergeTools(p.Tools, r.baseTools), p.Streams, r.now(), orgID)
	if err != nil {
		return orgchart.Role{}, err
	}
	if err := r.roles.Create(ctx, role); err != nil {
		return orgchart.Role{}, err
	}
	return role, nil
}

// Get returns the Role by (orgID, id), or store.ErrNotFound (wrapped).
// A thin read used by collaborators that need to validate a Role exists
// (e.g. workers.Hire) without reaching for the store directly.
func (r *Roles) Get(ctx context.Context, orgID string, id orgchart.RoleID) (orgchart.Role, error) {
	return r.roles.Get(ctx, orgID, id)
}

// UpdateParams patches the mutable fields of a Role. A nil pointer
// leaves the corresponding field unchanged — this is what preserves
// Tools/Streams on a content-only update (the old MCP bug).
type UpdateParams struct {
	Content *string
	Tools   *[]tool.Name
	Streams *[]streaming.StreamID
}

// Update reads the existing Role, applies the patch via the domain's
// With* builders, bumps UpdatedAt, and persists. Returns
// store.ErrNotFound (wrapped) when the (orgID, id) row is absent.
func (r *Roles) Update(ctx context.Context, orgID string, id orgchart.RoleID, p UpdateParams) (orgchart.Role, error) {
	existing, err := r.roles.Get(ctx, orgID, id)
	if err != nil {
		return orgchart.Role{}, err
	}
	updated := existing
	if p.Content != nil {
		updated = updated.WithContent(*p.Content)
	}
	if p.Tools != nil {
		updated = updated.WithTools(*p.Tools)
	}
	if p.Streams != nil {
		updated = updated.WithStreams(*p.Streams)
	}
	updated = updated.WithUpdatedAt(r.now())
	if err := r.roles.Update(ctx, updated); err != nil {
		return orgchart.Role{}, err
	}
	return updated, nil
}

// MergeTools returns the union of `existing` and `base`: the order of
// `existing` is preserved, any `base` entries not already present are
// appended in base order, and duplicates within `existing` are dropped.
// It is the single dedup-union algorithm shared by role creation and
// the tools-package RoleReconciler (tools.MergeBaseReadTools delegates
// here).
func MergeTools(existing, base []tool.Name) []tool.Name {
	seen := make(map[tool.Name]struct{}, len(existing)+len(base))
	out := make([]tool.Name, 0, len(existing)+len(base))
	for _, name := range existing {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, name := range base {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
