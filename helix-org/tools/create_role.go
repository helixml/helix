package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/helix-org/domain"
)

// CreateRole defines a new Role: an ID and the canonical markdown
// content that every Worker filling a Position with this Role will read
// at activation. Owner-only: holding the grant is the authorisation.
type CreateRole struct {
	deps Deps
}

const CreateRoleName domain.ToolName = "create_role"

var createRoleSchema = mustSchema[createRoleArgs]()

func (t *CreateRole) Name() domain.ToolName           { return CreateRoleName }
func (t *CreateRole) InputSchema() *jsonschema.Schema { return createRoleSchema }
func (t *CreateRole) Description() string {
	return "Define a new Role with markdown content. The content is what every Worker " +
		"filling this Role reads on activation. Use update_role to amend it later."
}

type createRoleArgs struct {
	ID      string `json:"id,omitempty"`
	Content string `json:"content"`
}

func (t *CreateRole) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args createRoleArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	id := domain.RoleID(args.ID)
	if id == "" {
		id = domain.RoleID("r-" + t.deps.NewID())
	}
	role, err := domain.NewRole(id, args.Content, t.deps.Now())
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Roles.Create(ctx, role); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(id)})
}
