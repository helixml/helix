package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/domain"
)

// CreateRole defines a new Role: an ID and the canonical markdown
// content that every Worker filling a Position with this Role will read
// at activation. Owner-only: holding the grant is the authorisation.
type CreateRole struct {
	deps Deps
}

const CreateRoleName tool.Name = "create_role"

var createRoleSchema = mustSchema[createRoleArgs]()

func (t *CreateRole) Name() tool.Name                 { return CreateRoleName }
func (t *CreateRole) InputSchema() *jsonschema.Schema { return createRoleSchema }
func (t *CreateRole) Description() string {
	return "Define a new Role with markdown content. The content is what every Worker " +
		"filling this Role reads on activation. Optional `tools` and `streams` are typed " +
		"manifests of the MCP tool names the Role's prompt expects to be granted on each " +
		"hire and the Stream IDs the Role's prompt expects to operate on. They are " +
		"reference data the hiring caller reads; hire_worker does NOT enforce them. " +
		"Use update_role to amend any field later."
}

type createRoleArgs struct {
	ID      string      `json:"id,omitempty"`
	Content string      `json:"content"`
	Tools   []tool.Name `json:"tools,omitempty"`
	Streams []stream.ID `json:"streams,omitempty"`
}

func (t *CreateRole) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args createRoleArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	id := role.ID(args.ID)
	if id == "" {
		id = role.ID("r-" + t.deps.NewID())
	}
	r, err := role.New(id, args.Content, args.Tools, args.Streams, t.deps.Now())
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Roles.Create(ctx, r); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(id)})
}
