package mcptools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// Spec-task delegation representatives: on-behalf-of tools act as the
// bound Agent, not the task id; delegation-incompatible states fail loudly
// with zero side effects.

type delegTaskCaller struct{}

func (delegTaskCaller) ID() string             { return "spt-deleg" }
func (delegTaskCaller) OrganizationID() string { return "org-test" }

func delegCtx(bound orgchart.NodeID) context.Context {
	ctx := runtime.WithProjectPrincipal(context.Background(), runtime.ProjectPrincipal{ProjectID: "prj-deleg", ActingUserID: "u-deleg"})
	return runtime.WithBoundWorker(ctx, bound)
}

func seedDelegNode(t *testing.T, st *store.Store, id string) {
	t.Helper()
	node, err := orgchart.NewNode(orgchart.NodeID(id), "# "+id, nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(context.Background(), node); err != nil {
		t.Fatal(err)
	}
}

func TestChatDelegatedPostsAsBoundAgent(t *testing.T) {
	t.Parallel()
	st := orggorm.GetOrgTestDB(t)
	ctx := context.Background()
	row, err := trigger.New("s-team-deleg", "org-test", "s-team-deleg", "", transport.KindLocal, nil, "b-owner", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Triggers.Create(ctx, row); err != nil {
		t.Fatal(err)
	}
	seedDelegNode(t, st, "b-owner")

	args, _ := json.Marshal(map[string]any{"triggerId": "s-team-deleg", "body": "task update"})
	raw, err := chatTool(t, st).Invoke(delegCtx("b-owner"), tool.Invocation{Caller: delegTaskCaller{}, Args: args})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &got); err != nil || got.ID == "" {
		t.Fatalf("decode: %v %+v", err, got)
	}
	events, err := st.Events.ListForStream(ctx, "org-test", "s-team-deleg", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d", len(events))
	}
	msg, _ := events[0].Message()
	// The stream reads as the Agent; the task id appears only in audit.
	if msg.From != "b-owner" {
		t.Fatalf("message From = %q, want the bound agent", msg.From)
	}
}

func TestChatFailsClosedWhenUnbound(t *testing.T) {
	t.Parallel()
	st := orggorm.GetOrgTestDB(t)
	ctx := context.Background()
	row, err := trigger.New("s-team-ghost", "org-test", "s-team-ghost", "", transport.KindLocal, nil, "b-owner", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Triggers.Create(ctx, row); err != nil {
		t.Fatal(err)
	}
	principal := runtime.WithProjectPrincipal(ctx, runtime.ProjectPrincipal{ProjectID: "prj-ghost", ActingUserID: "u-deleg"})
	args, _ := json.Marshal(map[string]any{"triggerId": "s-team-ghost", "body": "should not appear"})
	if _, err := chatTool(t, st).Invoke(principal, tool.Invocation{Caller: delegTaskCaller{}, Args: args}); err == nil {
		t.Fatal("unbound delegation must fail loudly")
	}
	events, err := st.Events.ListForStream(ctx, "org-test", "s-team-ghost", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("loud failure must write zero events, got %d", len(events))
	}
}

func TestManagersDelegatedWalksAgentLine(t *testing.T) {
	t.Parallel()
	st := orggorm.GetOrgTestDB(t)
	ctx := context.Background()
	seedDelegNode(t, st, "b-owner")
	seedDelegNode(t, st, "b-manager")
	line, err := orgchart.NewReportingLine("org-test", "b-manager", "b-owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReportingLines.Add(ctx, line); err != nil {
		t.Fatal(err)
	}

	deps := DefaultDeps(st).Build()
	raw, err := (&Managers{deps: deps}).Invoke(delegCtx("b-owner"), tool.Invocation{Caller: delegTaskCaller{}, Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Managers []struct {
			ID string `json:"id"`
		} `json:"managers"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Managers) != 1 || got.Managers[0].ID != "b-manager" {
		t.Fatalf("managers = %+v, want the agent's line", got.Managers)
	}

	// Unbound: loud error, not the silent empty list an unknown caller used to get.
	unbound := runtime.WithProjectPrincipal(ctx, runtime.ProjectPrincipal{ProjectID: "prj-ghost", ActingUserID: "u-deleg"})
	if _, err := (&Managers{deps: deps}).Invoke(unbound, tool.Invocation{Caller: delegTaskCaller{}, Args: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("unbound managers call must fail loudly")
	}
}
