package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
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

func (t *CreatePosition) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args createPositionArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("create_position: caller has no OrgID")
	}

	if _, err := t.deps.Store.Roles.Get(ctx, orgID, orgchart.RoleID(args.RoleID)); err != nil {
		return nil, fmt.Errorf("role %q: %w", args.RoleID, err)
	}

	var parent *orgchart.PositionID
	if args.ParentID != "" {
		p := orgchart.PositionID(args.ParentID)
		if _, err := t.deps.Store.Positions.Get(ctx, orgID, p); err != nil {
			return nil, fmt.Errorf("parent %q: %w", args.ParentID, err)
		}
		parent = &p
	}

	id := orgchart.PositionID(args.ID)
	if id == "" {
		id = orgchart.PositionID("p-" + t.deps.NewID())
	}

	pos, err := orgchart.NewPosition(id, orgchart.RoleID(args.RoleID), parent, orgID)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Positions.Create(ctx, pos); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(id)})
}
