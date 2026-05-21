package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/helix-org/domain"
)

type positionView struct {
	ID       domain.PositionID  `json:"id"`
	RoleID   domain.RoleID      `json:"roleId"`
	ParentID *domain.PositionID `json:"parentId"`
}

func positionViewOf(p domain.Position) positionView {
	return positionView{ID: p.ID, RoleID: p.RoleID, ParentID: p.ParentID}
}

// ListPositions returns every Position in the org chart.
type ListPositions struct {
	deps Deps
}

const ListPositionsName domain.ToolName = "list_positions"

var listPositionsSchema = mustSchema[listPositionsArgs]()

type listPositionsArgs struct{}

func (t *ListPositions) Name() domain.ToolName           { return ListPositionsName }
func (t *ListPositions) InputSchema() *jsonschema.Schema { return listPositionsSchema }
func (t *ListPositions) Description() string {
	return "List every Position: id, the Role it instantiates, and its parent. Use this to " +
		"navigate the org chart."
}

func (t *ListPositions) Invoke(ctx context.Context, _ domain.Invocation) (json.RawMessage, error) {
	positions, err := t.deps.Store.Positions.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	out := make([]positionView, 0, len(positions))
	for _, p := range positions {
		out = append(out, positionViewOf(p))
	}
	return json.Marshal(map[string]any{"positions": out})
}

// GetPosition returns one Position by ID.
type GetPosition struct {
	deps Deps
}

const GetPositionName domain.ToolName = "get_position"

var getPositionSchema = mustSchema[getPositionArgs]()

type getPositionArgs struct {
	ID string `json:"id"`
}

func (t *GetPosition) Name() domain.ToolName           { return GetPositionName }
func (t *GetPosition) InputSchema() *jsonschema.Schema { return getPositionSchema }
func (t *GetPosition) Description() string {
	return "Fetch one Position by id."
}

func (t *GetPosition) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args getPositionArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	pos, err := t.deps.Store.Positions.Get(ctx, domain.PositionID(args.ID))
	if err != nil {
		return nil, fmt.Errorf("get position %q: %w", args.ID, err)
	}
	return json.Marshal(positionViewOf(pos))
}

// ListPositionChildren returns every direct subordinate of a Position.
type ListPositionChildren struct {
	deps Deps
}

const ListPositionChildrenName domain.ToolName = "list_position_children"

var listPositionChildrenSchema = mustSchema[listPositionChildrenArgs]()

type listPositionChildrenArgs struct {
	ParentID string `json:"parentId"`
}

func (t *ListPositionChildren) Name() domain.ToolName           { return ListPositionChildrenName }
func (t *ListPositionChildren) InputSchema() *jsonschema.Schema { return listPositionChildrenSchema }
func (t *ListPositionChildren) Description() string {
	return "List the direct children of a Position — the slots that report into it."
}

func (t *ListPositionChildren) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args listPositionChildrenArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ParentID == "" {
		return nil, fmt.Errorf("parentId is required")
	}
	positions, err := t.deps.Store.Positions.ListChildren(ctx, domain.PositionID(args.ParentID))
	if err != nil {
		return nil, fmt.Errorf("list children of %q: %w", args.ParentID, err)
	}
	out := make([]positionView, 0, len(positions))
	for _, p := range positions {
		out = append(out, positionViewOf(p))
	}
	return json.Marshal(map[string]any{"positions": out})
}
