package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
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
	ID      string               `json:"id,omitempty"`
	Content string               `json:"content"`
	Tools   []tool.Name          `json:"tools,omitempty"`
	Streams []streaming.StreamID `json:"streams,omitempty"`
}

func (t *CreateRole) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args createRoleArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("create_role: caller has no OrgID")
	}
	id := orgchart.RoleID(args.ID)
	if id == "" {
		id = orgchart.RoleID("r-" + t.deps.NewID())
	}
	r, err := orgchart.NewRole(id, args.Content, args.Tools, args.Streams, t.deps.Now(), orgID)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Roles.Create(ctx, r); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(id)})
}
