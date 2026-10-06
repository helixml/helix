package mcptools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/workersecrets"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

type workerSecretCaller struct{}

func (workerSecretCaller) ID() string             { return "w-1" }
func (workerSecretCaller) OrganizationID() string { return "org-1" }

type workerSecretResolver struct{}

func (workerSecretResolver) Validate(context.Context, workersecret.Binding) error { return nil }
func (workerSecretResolver) Resolve(context.Context, workersecret.Binding) (workersecret.Resolved, error) {
	return workersecret.Resolved{Value: "raw-secret", Descriptor: workersecret.Descriptor{Available: true}}, nil
}

func TestWorkerSecretToolsSeparateDiscoveryFromRetrieval(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	node, err := orgchart.NewNode("w-1", "worker", nil, time.Now(), "org-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(ctx, node); err != nil {
		t.Fatal(err)
	}
	svc, _ := workersecrets.New(st.WorkerSecretBindings, st.Nodes, workerSecretResolver{}, time.Now, nil)
	if _, err := svc.Put(ctx, workersecret.Binding{OrganizationID: "org-1", WorkerID: "w-1", Name: "API_TOKEN", Usage: "export API_TOKEN", SourceKind: workersecret.SourceHelixSecret, SecretID: "sec-1"}); err != nil {
		t.Fatal(err)
	}
	deps := Deps{WorkerSecrets: svc}
	inv := tool.Invocation{Caller: workerSecretCaller{}, Args: json.RawMessage(`{}`)}
	listed, err := (&ListSecrets{deps: deps}).Invoke(ctx, inv)
	if err != nil {
		t.Fatal(err)
	}
	if string(listed) == "" || containsRawSecret(listed) {
		t.Fatalf("list_secrets leaked value: %s", listed)
	}
	inv.Args = json.RawMessage(`{"name":"API_TOKEN"}`)
	got, err := (&GetSecret{deps: deps}).Invoke(ctx, inv)
	if err != nil {
		t.Fatal(err)
	}
	if !containsRawSecret(got) {
		t.Fatalf("get_secret did not return value: %s", got)
	}
}
func containsRawSecret(raw []byte) bool {
	return string(raw) != "" && json.Valid(raw) && strings.Contains(string(raw), "raw-secret")
}
