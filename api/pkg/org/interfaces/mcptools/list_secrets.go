package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
)

// ListSecrets returns the calling Bot's own project secrets as a
// name→value map, read live. It is the read counterpart to the
// container's boot-time env-var injection: a project secret added AFTER
// the desktop booted is not in the running shell's environment (a
// process's env is frozen at start), so the agent reads it here and
// then retrieves one by name through get_secret immediately before use.
// into the shell.
//
// Scope is the caller only: orgID and botID both come from
// inv.Caller, never from args, so a Bot can read its own project's
// secrets and no other's. No new exposure — these are the exact secrets
// the Bot already receives as env vars at boot.
type ListSecrets struct {
	deps Deps
}

const ListSecretsName tool.Name = "list_secrets"

// NewListSecrets is the exported constructor so tests can build the tool
// with a fake ProjectConfig without going through RegisterBuiltins.
func NewListSecrets(deps Deps) *ListSecrets { return &ListSecrets{deps: deps} }

type listSecretsArgs struct{}

var listSecretsSchema = mustSchema[listSecretsArgs]()

func (t *ListSecrets) Name() tool.Name                 { return ListSecretsName }
func (t *ListSecrets) InputSchema() *jsonschema.Schema { return listSecretsSchema }

func (t *ListSecrets) Description() string {
	return "List the names and usage metadata for credentials explicitly granted to this Worker. " +
		"Values and backend source details are never returned. Call get_secret immediately before an authenticated operation and again after a 401/403."
}

func (t *ListSecrets) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	orgID, err := projectConfigOrgID(inv)
	if err != nil {
		return nil, err
	}
	botID := inv.Caller.ID()
	if botID == "" {
		return nil, fmt.Errorf("caller has no bot id")
	}
	if t.deps.WorkerSecrets == nil {
		return nil, fmt.Errorf("worker secrets are not configured")
	}
	secrets, err := t.deps.WorkerSecrets.Descriptors(ctx, orgID, orgchart.NodeID(botID))
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	return json.Marshal(struct {
		Secrets []workersecret.Descriptor `json:"secrets"`
	}{Secrets: secrets})
}
