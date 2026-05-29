package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/helix-org/domain"
)

// UpdateIdentity rewrites a Worker's IdentityContent — the per-Worker
// description (persona for AI, profile for a human). It is a single DB
// write; the new content takes effect on the Worker's next activation,
// when the Spawner projects identity into the Environment.
//
// Identity is owner-managed: subordinate Workers don't get this grant.
// The identity supplied at hire time stays in place until rewritten.
type UpdateIdentity struct {
	deps Deps
}

const UpdateIdentityName domain.ToolName = "update_identity"

var updateIdentitySchema = mustSchema[updateIdentityArgs]()

func (t *UpdateIdentity) Name() domain.ToolName           { return UpdateIdentityName }
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

func (t *UpdateIdentity) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
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

	existing, err := t.deps.Store.Workers.Get(ctx, domain.WorkerID(args.WorkerID))
	if err != nil {
		return nil, fmt.Errorf("worker %q: %w", args.WorkerID, err)
	}
	if err := t.deps.Store.Workers.Update(ctx, existing.WithIdentityContent(args.Content)); err != nil {
		return nil, fmt.Errorf("update worker: %w", err)
	}
	_ = t.deps.Workspace.PublishFile(ctx, domain.WorkerID(args.WorkerID), "identity.md", args.Content, fmt.Sprintf("update_identity: %s", args.WorkerID))
	return json.Marshal(map[string]string{"id": args.WorkerID})
}
