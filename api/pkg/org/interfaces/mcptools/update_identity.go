package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// UpdateIdentity rewrites a Worker's IdentityContent — the per-Worker
// description (persona for AI, profile for a human). It is a single DB
// write; the new content takes effect on the Worker's next activation,
// when the Spawner projects identity into the Environment.
//
// Identity is owner-managed: subordinate Workers don't get this tool.
// The identity supplied at hire time stays in place until rewritten.
type UpdateIdentity struct {
	deps Deps
}

const UpdateIdentityName tool.Name = "update_identity"

var updateIdentitySchema = mustSchema[updateIdentityArgs]()

func (t *UpdateIdentity) Name() tool.Name                 { return UpdateIdentityName }
func (t *UpdateIdentity) InputSchema() *jsonschema.Schema { return updateIdentitySchema }
func (t *UpdateIdentity) Description() string {
	return "Replace a Worker's IdentityContent (persona / profile). The change takes effect " +
		"on the Worker's next activation, when the Spawner projects current identity into " +
		"their Environment. Owner-only."
}

type updateIdentityArgs struct {
	WorkerID string `json:"workerId"`
	Content  string `json:"content"`
}

func (t *UpdateIdentity) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args updateIdentityArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.WorkerID == "" {
		return nil, fmt.Errorf("workerId is required")
	}
	if args.Content == "" {
		return nil, fmt.Errorf("content is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("update_identity: caller has no OrgID")
	}

	if _, err := t.deps.Workers.UpdateIdentity(ctx, orgID, orgchart.BotID(args.WorkerID), args.Content); err != nil {
		return nil, fmt.Errorf("worker %q: %w", args.WorkerID, err)
	}
	// Mirror the new identity into the Worker's Environment so a running
	// session sees it without waiting for the next activation. Workspace
	// side-effect, not store state, so it stays in the MCP adapter (the
	// REST chart UI doesn't need it — the Spawner re-projects identity at
	// the start of every activation).
	_ = t.deps.Workspace.MirrorFile(ctx, orgID, orgchart.BotID(args.WorkerID), "identity.md", args.Content, fmt.Sprintf("update_identity: %s", args.WorkerID))
	return json.Marshal(map[string]string{"id": args.WorkerID})
}
