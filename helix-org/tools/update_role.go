package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix-org/domain"
)

// UpdateRole rewrites the canonical content of a Role and propagates it
// to every Worker filling a Position with that Role: their role.md is
// rewritten in place. Workers can never modify their own Role — only
// the owner does, via this tool. The next normal activation picks up
// the new content; nothing is published or notified.
//
// Cross-Environment writes are intentional: tools are explicitly
// allowed to cross Worker boundaries when the action is owner-driven.
type UpdateRole struct {
	deps Deps
}

const UpdateRoleName domain.ToolName = "update_role"

var updateRoleSchema = mustSchema[updateRoleArgs]()

func (t *UpdateRole) Name() domain.ToolName           { return UpdateRoleName }
func (t *UpdateRole) InputSchema() *jsonschema.Schema { return updateRoleSchema }
func (t *UpdateRole) Description() string {
	return "Replace a Role's markdown content. Atomically rewrites role.md in every Worker's " +
		"Environment that runs this Role. Owner-only."
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

	written, err := t.fanOut(ctx, roleID, args.Content)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"id":             string(roleID),
		"workersUpdated": written,
	})
}

// fanOut rewrites role.md in every Environment whose Worker is in a
// Position with this RoleID. Returns the count of Environments written.
// A missing Environment for a Worker is logged but doesn't fail the
// whole operation — the Role update is the source of truth and any
// hire that follows will pick up the new content from storage.
func (t *UpdateRole) fanOut(ctx context.Context, roleID domain.RoleID, content string) (int, error) {
	positions, err := t.deps.Store.Positions.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list positions: %w", err)
	}
	matching := make(map[domain.PositionID]struct{})
	for _, p := range positions {
		if p.RoleID == roleID {
			matching[p.ID] = struct{}{}
		}
	}
	if len(matching) == 0 {
		return 0, nil
	}

	workers, err := t.deps.Store.Workers.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list workers: %w", err)
	}
	written := 0
	for _, w := range workers {
		if !workerInPositions(w, matching) {
			continue
		}
		env, err := t.deps.Store.Environments.Get(ctx, w.ID())
		if err != nil {
			// Worker has no Environment row — odd but don't fail the
			// update. The Role was already saved; future hires will
			// see it.
			continue
		}
		if err := writeEnvFile(env.Path, "role.md", content); err != nil {
			return written, fmt.Errorf("worker %q: %w", w.ID(), err)
		}
		written++
	}
	return written, nil
}

func workerInPositions(w domain.Worker, positions map[domain.PositionID]struct{}) bool {
	for _, p := range w.Positions() {
		if _, ok := positions[p]; ok {
			return true
		}
	}
	return false
}
