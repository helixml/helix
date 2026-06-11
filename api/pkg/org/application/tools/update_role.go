package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/roles"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// UpdateRole rewrites the canonical content of a Role. It is a single
// DB write — the new content takes effect on the next activation of
// every Worker holding this Role, because the Spawner projects current
// Role state into the Environment at the start of every activation.
// There is no fan-out, no cross-Environment write, and no on-disk
// source of truth.
//
// Workers can never modify their own Role — only the owner does, via
// this tool.
type UpdateRole struct {
	deps Deps
}

const UpdateRoleName tool.Name = "update_role"

var updateRoleSchema = mustSchema[updateRoleArgs]()

func (t *UpdateRole) Name() tool.Name                 { return UpdateRoleName }
func (t *UpdateRole) InputSchema() *jsonschema.Schema { return updateRoleSchema }
func (t *UpdateRole) Description() string {
	return "Replace a Role's markdown content. The change takes effect on each Worker's " +
		"next activation, when the Spawner projects current Role state into their " +
		"Environment. Owner-only."
}

type updateRoleArgs struct {
	RoleID  string `json:"roleId"`
	Content string `json:"content"`
}

func (t *UpdateRole) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args updateRoleArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.RoleID == "" {
		return nil, fmt.Errorf("roleId is required")
	}
	if args.Content == "" {
		return nil, fmt.Errorf("content is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("update_role: caller has no OrgID")
	}
	roleID := orgchart.RoleID(args.RoleID)

	// Content-only patch: the service preserves Tools and Streams (the
	// old inline path here rebuilt the Role with only Content and
	// silently wiped both — see application/roles).
	if _, err := t.deps.rolesService().Update(ctx, orgID, roleID, roles.UpdateParams{Content: &args.Content}); err != nil {
		return nil, fmt.Errorf("update role: %w", err)
	}

	// Mirror the new content into each holding Worker's Environment so a
	// running session sees it without waiting for the next activation.
	// This is a workspace side-effect, not store state, so it stays in
	// the MCP adapter (the REST chart UI doesn't need it — the Spawner
	// re-projects current Role state at the start of every activation).
	workers, _ := t.deps.Store.Workers.List(ctx, orgID)
	for _, w := range workers {
		if w.RoleID() != roleID {
			continue
		}
		_ = t.deps.Workspace.MirrorFile(ctx, orgID, w.ID(), "role.md", args.Content, fmt.Sprintf("update_role: %s", roleID))
	}
	return json.Marshal(map[string]string{"id": string(roleID)})
}
