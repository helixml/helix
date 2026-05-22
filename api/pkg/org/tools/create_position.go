package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/domain"
)

// CreatePosition instantiates a Role as a concrete slot in the org chart.
type CreatePosition struct {
	deps Deps
}

const CreatePositionName tool.Name = "create_position"

var createPositionSchema = mustSchema[createPositionArgs]()

func (t *CreatePosition) Name() tool.Name                 { return CreatePositionName }
func (t *CreatePosition) InputSchema() *jsonschema.Schema { return createPositionSchema }
func (t *CreatePosition) Description() string {
	return "Instantiate a Role as a concrete slot in the org chart, optionally under a parent Position."
}

type createPositionArgs struct {
	ID       string `json:"id,omitempty"`
	RoleID   string `json:"roleId"`
	ParentID string `json:"parentId,omitempty"`
}

func (t *CreatePosition) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args createPositionArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	if _, err := t.deps.Store.Roles.Get(ctx, role.ID(args.RoleID)); err != nil {
		return nil, fmt.Errorf("role %q: %w", args.RoleID, err)
	}

	var parent *position.ID
	if args.ParentID != "" {
		p := position.ID(args.ParentID)
		if _, err := t.deps.Store.Positions.Get(ctx, p); err != nil {
			return nil, fmt.Errorf("parent %q: %w", args.ParentID, err)
		}
		parent = &p
	}

	id := position.ID(args.ID)
	if id == "" {
		id = position.ID("p-" + t.deps.NewID())
	}

	pos, err := domain.NewPosition(id, role.ID(args.RoleID), parent)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Positions.Create(ctx, pos); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(id)})
}
