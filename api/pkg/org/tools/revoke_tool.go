package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/grant"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/domain"
)

// RevokeTool deletes an existing ToolGrant. Owner-only.
type RevokeTool struct {
	deps Deps
}

const RevokeToolName tool.Name = "revoke_tool"

var revokeToolSchema = mustSchema[revokeToolArgs]()

func (t *RevokeTool) Name() tool.Name                 { return RevokeToolName }
func (t *RevokeTool) Description() string             { return "Revoke an existing tool grant by ID." }
func (t *RevokeTool) InputSchema() *jsonschema.Schema { return revokeToolSchema }

type revokeToolArgs struct {
	GrantID string `json:"grantId"`
}

func (t *RevokeTool) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args revokeToolArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if err := t.deps.Store.Grants.Delete(ctx, grant.ID(args.GrantID)); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": args.GrantID})
}
