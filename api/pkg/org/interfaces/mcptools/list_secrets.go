package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
)

// ListSecrets returns metadata for credentials bound to the calling
// subject: the Bot itself, or for a spec task its project's bound Agent
// (SubjectForCaller). Values and backing source details are never returned.
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
	subject, err := SubjectForCaller(ctx, inv.Caller)
	if err != nil {
		return nil, fmt.Errorf("list_secrets: %w", err)
	}
	if t.deps.WorkerSecrets == nil {
		return nil, fmt.Errorf("worker secrets are not configured")
	}
	secrets, err := t.deps.WorkerSecrets.Descriptors(ctx, orgID, subject)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	return json.Marshal(struct {
		Secrets []workersecret.Descriptor `json:"secrets"`
	}{Secrets: secrets})
}
