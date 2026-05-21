package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/grant"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/domain"
)

type grantView struct {
	ID       grant.ID  `json:"id"`
	WorkerID worker.ID `json:"workerId"`
	ToolName tool.Name `json:"toolName"`
}

func grantViewOf(g domain.ToolGrant) grantView {
	return grantView{ID: g.ID, WorkerID: g.WorkerID, ToolName: g.ToolName}
}

// GetGrant returns one ToolGrant by ID.
type GetGrant struct {
	deps Deps
}

const GetGrantName tool.Name = "get_grant"

var getGrantSchema = mustSchema[getGrantArgs]()

type getGrantArgs struct {
	ID string `json:"id"`
}

func (t *GetGrant) Name() tool.Name                 { return GetGrantName }
func (t *GetGrant) InputSchema() *jsonschema.Schema { return getGrantSchema }
func (t *GetGrant) Description() string {
	return "Fetch one ToolGrant by id."
}

func (t *GetGrant) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args getGrantArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	g, err := t.deps.Store.Grants.Get(ctx, grant.ID(args.ID))
	if err != nil {
		return nil, fmt.Errorf("get grant %q: %w", args.ID, err)
	}
	return json.Marshal(grantViewOf(g))
}
