package mcptools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/workersecrets"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

type taskSecretCaller struct{}

func (taskSecretCaller) ID() string             { return "spt-1" }
func (taskSecretCaller) OrganizationID() string { return "org-1" }

func secretDeps(t *testing.T) Deps {
	t.Helper()
	ctx := context.Background()
	st := memory.New()
	node, err := orgchart.NewNode("w-owner", "owner", nil, time.Now(), "org-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(ctx, node); err != nil {
		t.Fatal(err)
	}
	svc, _ := workersecrets.New(st.WorkerSecretBindings, st.Nodes, workerSecretResolver{}, time.Now, nil)
	if _, err := svc.Put(ctx, workersecret.Binding{OrganizationID: "org-1", WorkerID: "w-owner", Name: "API_TOKEN", Usage: "export API_TOKEN", SourceKind: workersecret.SourceHelixSecret, SecretID: "sec-1"}); err != nil {
		t.Fatal(err)
	}
	return Deps{WorkerSecrets: svc}
}

func TestSecretToolsResolveSubjectForBoundTask(t *testing.T) {
	deps := secretDeps(t)
	ctx := runtime.WithProjectPrincipal(context.Background(), runtime.ProjectPrincipal{ProjectID: "prj-1", ActingUserID: "u-1"})
	ctx = runtime.WithBoundWorker(ctx, "w-owner")
	inv := tool.Invocation{Caller: taskSecretCaller{}, Args: json.RawMessage(`{}`)}
	listed, err := (&ListSecrets{deps: deps}).Invoke(ctx, inv)
	if err != nil {
		t.Fatal(err)
	}
	if !containsRawSecret(listed) && string(listed) == "" {
		t.Fatalf("list_secrets empty for bound task: %s", listed)
	}
	if json.Valid(listed) && len(listed) == 0 {
		t.Fatal("empty list")
	}
	inv.Args = json.RawMessage(`{"name":"API_TOKEN"}`)
	got, err := (&GetSecret{deps: deps}).Invoke(ctx, inv)
	if err != nil {
		t.Fatal(err)
	}
	if !containsRawSecret(got) {
		t.Fatalf("bound task should read the agent's secret, got %s", got)
	}
}

func TestSecretToolsFailLoudlyForUnboundTask(t *testing.T) {
	deps := secretDeps(t)
	ctx := runtime.WithProjectPrincipal(context.Background(), runtime.ProjectPrincipal{ProjectID: "prj-ghost", ActingUserID: "u-1"})
	inv := tool.Invocation{Caller: taskSecretCaller{}, Args: json.RawMessage(`{"name":"API_TOKEN"}`)}
	if _, err := (&GetSecret{deps: deps}).Invoke(ctx, inv); err == nil {
		t.Fatal("get_secret must fail for an unbound task")
	}
	inv.Args = json.RawMessage(`{}`)
	if _, err := (&ListSecrets{deps: deps}).Invoke(ctx, inv); err == nil {
		t.Fatal("list_secrets must fail for an unbound task")
	}
}

func TestSecretToolsReadTaskOwnBindingNever(t *testing.T) {
	// The task id ("spt-1") has no bindings at all; even a caller carrying a
	// real identity must never fall back to reading by caller id when a
	// principal is present but the bond is stale.
	deps := secretDeps(t)
	ctx := runtime.WithProjectPrincipal(context.Background(), runtime.ProjectPrincipal{ProjectID: "prj-1", ActingUserID: "u-1"})
	ctx = runtime.WithBoundWorker(ctx, "w-ghost") // node that no longer exists
	inv := tool.Invocation{Caller: taskSecretCaller{}, Args: json.RawMessage(`{"name":"API_TOKEN"}`)}
	if _, err := (&GetSecret{deps: deps}).Invoke(ctx, inv); err == nil {
		t.Fatal("stale bond must not resolve any binding")
	}
}
