package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// ListSecrets returns the calling Bot's own project secrets as a
// name→value map, read live. It is the read counterpart to the
// container's boot-time env-var injection: a project secret added AFTER
// the desktop booted is not in the running shell's environment (a
// process's env is frozen at start), so the agent reads it here and
// exports it — the same shape as mint_credential feeding gh/git tokens
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
	return "List your project's secrets as a name→value map (your own project only). " +
		"Project secrets are injected as environment variables when your desktop starts, " +
		"but a secret added AFTER your container booted is NOT yet in your shell — a running " +
		"process's environment cannot change. Call this to read the current secrets and " +
		"**export the ones you need before the command that uses them** " +
		"(e.g. `export DRONE_TOKEN=$(list_secrets | jq -r '.secrets.DRONE_TOKEN')`). " +
		"Re-read after you add or change a secret mid-session. " +
		"Returns: { secrets: { NAME: value, ... } }."
}

func (t *ListSecrets) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	if inv.Caller == nil {
		return nil, fmt.Errorf("caller missing on invocation")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("caller has no organization id")
	}
	botID := inv.Caller.ID()
	if botID == "" {
		return nil, fmt.Errorf("caller has no bot id")
	}
	cfg := t.deps.ProjectConfig
	if cfg == nil {
		return nil, runtime.ErrProjectConfigUnsupported
	}
	secrets, err := cfg.ListWorkerProjectSecrets(ctx, orgID, orgchart.BotID(botID))
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	if secrets == nil {
		secrets = map[string]string{}
	}
	return json.Marshal(map[string]any{"secrets": secrets})
}
