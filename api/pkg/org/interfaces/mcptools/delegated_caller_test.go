package mcptools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/workersecrets"
	orgaudit "github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

// The delegation contract lives in ONE place now: the ingress wrapper.
// Tools are untouched — they see exactly one identity with one meaning.

type rawTaskCaller struct{}

func (rawTaskCaller) ID() string             { return "spt-deleg" }
func (rawTaskCaller) OrganizationID() string { return "org-test" }

func secretDepsFor(t *testing.T, worker string) Deps {
	t.Helper()
	ctx := context.Background()
	st := memory.New()
	node, err := orgchart.NewNode(orgchart.NodeID(worker), "owner", nil, time.Now(), "org-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(ctx, node); err != nil {
		t.Fatal(err)
	}
	svc, _ := workersecrets.New(st.WorkerSecretBindings, st.Nodes, workerSecretResolver{}, time.Now, nil)
	if _, err := svc.Put(ctx, workersecret.Binding{OrganizationID: "org-1", WorkerID: orgchart.NodeID(worker), Name: "API_TOKEN", Usage: "export API_TOKEN", SourceKind: workersecret.SourceHelixSecret, SecretID: "sec-1"}); err != nil {
		t.Fatal(err)
	}
	return Deps{WorkerSecrets: svc}
}

func TestDelegatedCallerIdentityAndAudit(t *testing.T) {
	w := DelegatedCaller{Inner: rawTaskCaller{}, Agent: "b-owner", AuditTaskID: "spt-deleg"}
	if w.ID() != "b-owner" {
		t.Fatalf("acting identity = %q", w.ID())
	}
	if w.OrganizationID() != "org-test" {
		t.Fatalf("org passthrough = %q", w.OrganizationID())
	}
	if got := w.AuditActorID(); got != "spt-deleg" {
		t.Fatalf("audit identity = %q", got)
	}
	if got := w.AuditActorType(); got != orgaudit.ActorSpecTask {
		t.Fatalf("audit actor type = %q", got)
	}
}

func TestDelegatedCallerPresentsAgentToToolsChat(t *testing.T) {
	t.Parallel()
	st := orggorm.GetOrgTestDB(t)
	ctx := context.Background()
	row, err := trigger.New("s-team-wrap", "org-test", "s-team-wrap", "", transport.KindLocal, nil, "b-owner", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Triggers.Create(ctx, row); err != nil {
		t.Fatal(err)
	}
	node, err := orgchart.NewNode("b-owner", "# owner", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(ctx, node); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(map[string]any{"triggerId": "s-team-wrap", "body": "task update"})
	w := DelegatedCaller{Inner: rawTaskCaller{}, Agent: "b-owner", AuditTaskID: "spt-deleg"}
	if _, err := chatTool(t, st).Invoke(ctx, tool.Invocation{Caller: w, Args: args}); err != nil {
		t.Fatal(err)
	}
	events, err := st.Events.ListForStream(ctx, "org-test", "s-team-wrap", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d", len(events))
	}
	msg, _ := events[0].Message()
	if msg.From != "b-owner" {
		t.Fatalf("stream reads as %q, want the bound agent", msg.From)
	}
}

type org1TaskCaller struct{}

func (org1TaskCaller) ID() string             { return "spt-deleg" }
func (org1TaskCaller) OrganizationID() string { return "org-1" }

func TestDelegatedCallerPresentsAgentToSecretTools(t *testing.T) {
	deps := secretDepsFor(t, "w-owner")
	w := DelegatedCaller{Inner: org1TaskCaller{}, Agent: "w-owner", AuditTaskID: "spt-deleg"}
	inv := tool.Invocation{Caller: w, Args: json.RawMessage(`{"name":"API_TOKEN"}`)}
	got, err := (&GetSecret{deps: deps}).Invoke(context.Background(), inv)
	if err != nil {
		t.Fatal(err)
	}
	if !containsRawSecret(got) {
		t.Fatalf("delegated get_secret did not read the agent's binding: %s", got)
	}
	// An unwrapped task caller stays exactly as safe as before: the task
	// node has no bindings and no node row, so lookups fail with zero side
	// effects (it is never served these tools in practice — see the
	// surface tests in api/pkg/server).
	inv.Caller = org1TaskCaller{}
	if _, err := (&GetSecret{deps: deps}).Invoke(context.Background(), inv); err == nil {
		t.Fatal("unwrapped task caller must not resolve any binding")
	}
}

func TestSecretDepsOwnedByBoundAgent(t *testing.T) {
	deps := secretDepsFor(t, "w-owner")
	w := DelegatedCaller{Inner: org1TaskCaller{}, Agent: "w-owner", AuditTaskID: "spt-deleg"}
	if _, err := (&ListSecrets{deps: deps}).Invoke(context.Background(), tool.Invocation{Caller: w, Args: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("delegated list_secrets: %v", err)
	}
}
