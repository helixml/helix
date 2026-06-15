package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

type roleView struct {
	ID        orgchart.RoleID `json:"id"`
	Content   string          `json:"content"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

func roleViewOf(r orgchart.Role) roleView {
	return roleView{ID: r.ID, Content: r.Content, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

// ListRoles returns every Role in the org.
type ListRoles struct {
	deps Deps
}

const ListRolesName tool.Name = "list_roles"

var listRolesSchema = mustSchema[listRolesArgs]()

type listRolesArgs struct{}

func (t *ListRoles) Name() tool.Name                 { return ListRolesName }
func (t *ListRoles) InputSchema() *jsonschema.Schema { return listRolesSchema }
func (t *ListRoles) Description() string {
	return "List every Role: id, markdown content, and timestamps. Use this to discover what " +
		"roles exist before hiring."
}

func (t *ListRoles) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("list_roles: caller has no OrgID")
	}
	roles, err := t.deps.Store.Roles.List(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	out := make([]roleView, 0, len(roles))
	for _, r := range roles {
		out = append(out, roleViewOf(r))
	}
	return json.Marshal(map[string]any{"roles": out})
}

// GetRole returns one Role by ID.
type GetRole struct {
	deps Deps
}

const GetRoleName tool.Name = "get_role"

var getRoleSchema = mustSchema[getRoleArgs]()

type getRoleArgs struct {
	ID string `json:"id"`
}

func (t *GetRole) Name() tool.Name                 { return GetRoleName }
func (t *GetRole) InputSchema() *jsonschema.Schema { return getRoleSchema }
func (t *GetRole) Description() string {
	return "Fetch one Role by id and return its current markdown content."
}

func (t *GetRole) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args getRoleArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("get_role: caller has no OrgID")
	}
	got, err := t.deps.Store.Roles.Get(ctx, orgID, orgchart.RoleID(args.ID))
	if err != nil {
		return nil, fmt.Errorf("get role %q: %w", args.ID, err)
	}
	return json.Marshal(roleViewOf(got))
}
