package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

const GetSecretName tool.Name = "get_secret"

type getSecretArgs struct {
	Name string `json:"name"`
}

var getSecretSchema = mustSchema[getSecretArgs]()

type GetSecret struct{ deps Deps }

func (t *GetSecret) Name() tool.Name                 { return GetSecretName }
func (t *GetSecret) InputSchema() *jsonschema.Schema { return getSecretSchema }
func (t *GetSecret) Description() string {
	return "Return the current value of one credential explicitly granted to this Worker. Call immediately before an authenticated operation and again after a 401/403. The value is sensitive: do not print it or place it in command-line arguments. Args: name. Backend source and resource IDs are intentionally not accepted."
}
func (t *GetSecret) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args getSecretArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.Name == "" {
		return nil, fmt.Errorf("get_secret: name is required")
	}
	if inv.Caller == nil || inv.Caller.OrganizationID() == "" || inv.Caller.ID() == "" {
		return nil, fmt.Errorf("get_secret: caller identity is incomplete")
	}
	if t.deps.WorkerSecrets == nil {
		return nil, fmt.Errorf("worker secrets are not configured")
	}
	res, err := t.deps.WorkerSecrets.Get(ctx, inv.Caller.OrganizationID(), orgchart.NodeID(inv.Caller.ID()), args.Name)
	if err != nil {
		return nil, err
	}
	return json.Marshal(res)
}
