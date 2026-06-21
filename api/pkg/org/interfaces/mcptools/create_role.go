package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/roles"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// CreateRole defines a new Role: an ID, the canonical markdown
// content that every Worker holding this Role will read at
// activation, and the Role's MCP tool list. Owner-only: holding
// create_role in your own Role is the authorisation.
type CreateRole struct {
	deps Deps
}

const CreateRoleName tool.Name = "create_role"

var createRoleSchema = mustSchema[createRoleArgs]()

func (t *CreateRole) Name() tool.Name                 { return CreateRoleName }
func (t *CreateRole) InputSchema() *jsonschema.Schema { return createRoleSchema }
func (t *CreateRole) Description() string {
	return "Define a new Role with markdown content. The content is what every Worker " +
		"holding this Role reads on activation. `tools` is the live MCP surface for " +
		"every Worker in this Role — populate it with every " +
		"MCP tool the Role needs. `streams` is a typed manifest of Stream IDs the " +
		"Role's prompt expects to operate on (the hiring caller still drives " +
		"create_stream/subscribe explicitly). Use update_role to amend any field later " +
		"— a tools change propagates to every Worker in this Role on their next MCP " +
		"request."
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
	r, err := t.deps.Roles.Create(ctx, orgID, roles.CreateParams{
		ID:      args.ID,
		Content: args.Content,
		Tools:   args.Tools,
		Streams: args.Streams,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(r.ID)})
}
