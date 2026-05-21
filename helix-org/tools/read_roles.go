package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/helix-org/domain"
)

type roleView struct {
	ID        domain.RoleID `json:"id"`
	Content   string        `json:"content"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

func roleViewOf(r domain.Role) roleView {
	return roleView{ID: r.ID, Content: r.Content, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

// ListRoles returns every Role in the org.
type ListRoles struct {
	deps Deps
}

const ListRolesName domain.ToolName = "list_roles"

var listRolesSchema = mustSchema[listRolesArgs]()

type listRolesArgs struct{}

func (t *ListRoles) Name() domain.ToolName           { return ListRolesName }
func (t *ListRoles) InputSchema() *jsonschema.Schema { return listRolesSchema }
func (t *ListRoles) Description() string {
	return "List every Role: id, markdown content, and timestamps. Use this to discover what " +
		"roles exist before creating a Position."
}

func (t *ListRoles) Invoke(ctx context.Context, _ domain.Invocation) (json.RawMessage, error) {
	roles, err := t.deps.Store.Roles.List(ctx)
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

const GetRoleName domain.ToolName = "get_role"

var getRoleSchema = mustSchema[getRoleArgs]()

type getRoleArgs struct {
	ID string `json:"id"`
}

func (t *GetRole) Name() domain.ToolName           { return GetRoleName }
func (t *GetRole) InputSchema() *jsonschema.Schema { return getRoleSchema }
func (t *GetRole) Description() string {
	return "Fetch one Role by id and return its current markdown content."
}

func (t *GetRole) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args getRoleArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	role, err := t.deps.Store.Roles.Get(ctx, domain.RoleID(args.ID))
	if err != nil {
		return nil, fmt.Errorf("get role %q: %w", args.ID, err)
	}
	return json.Marshal(roleViewOf(role))
}
