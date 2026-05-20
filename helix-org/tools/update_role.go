package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix-org/domain"
)

// UpdateRole rewrites the canonical content of a Role. It is a single
// DB write — the new content takes effect on the next activation of
// every Worker filling a Position with this Role, because the Spawner
// projects current Role state into the Environment at the start of
// every activation. There is no fan-out, no cross-Environment write,
// and no on-disk source of truth.
//
// Workers can never modify their own Role — only the owner does, via
// this tool.
type UpdateRole struct {
	deps Deps
}

const UpdateRoleName domain.ToolName = "update_role"

var updateRoleSchema = mustSchema[updateRoleArgs]()

func (t *UpdateRole) Name() domain.ToolName           { return UpdateRoleName }
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

func (t *UpdateRole) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
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
	roleID := domain.RoleID(args.RoleID)

	existing, err := t.deps.Store.Roles.Get(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("role %q: %w", roleID, err)
	}

	updated := domain.Role{
		ID:        existing.ID,
		Content:   args.Content,
		CreatedAt: existing.CreatedAt,
		UpdatedAt: t.deps.Now(),
	}
	if err := t.deps.Store.Roles.Update(ctx, updated); err != nil {
		return nil, fmt.Errorf("update role: %w", err)
	}
	// Mirror role content into every Worker holding a Position with
	// this Role. Each runtime backend resolves the per-Worker target
	// from its own state — the claude runtime writes a file in
	// envsDir; the Helix runtime pushes to the per-Worker repo.
	positions, _ := t.deps.Store.Positions.List(ctx)
	workers, _ := t.deps.Store.Workers.List(ctx)
	positionWorkers := map[domain.PositionID][]domain.WorkerID{}
	for _, w := range workers {
		for _, p := range w.Positions() {
			positionWorkers[p] = append(positionWorkers[p], w.ID())
		}
	}
	for _, p := range positions {
		if p.RoleID != roleID {
			continue
		}
		for _, wid := range positionWorkers[p.ID] {
			_ = t.deps.Workspace.PublishFile(ctx, wid, "role.md", args.Content, fmt.Sprintf("update_role: %s", roleID))
		}
	}
	return json.Marshal(map[string]string{"id": string(roleID)})
}
