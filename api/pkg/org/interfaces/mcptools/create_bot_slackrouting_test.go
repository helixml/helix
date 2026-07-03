package mcptools

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// spyOrgReconciler records Reconcile(orgID) calls. It satisfies
// lifecycle.OrgReconciler — the whole-org reconciler contract the Slack
// auto-router implements.
type spyOrgReconciler struct {
	mu     sync.Mutex
	orgIDs []string
}

func (s *spyOrgReconciler) Reconcile(_ context.Context, orgID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orgIDs = append(s.orgIDs, orgID)
	return nil
}

func (s *spyOrgReconciler) calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.orgIDs))
	copy(out, s.orgIDs)
	return out
}

// TestCreateBotRunsInjectedOrgReconcilers is the regression guard for the
// bug where a bot added via the MCP create_bot tool did not reconcile the
// Slack auto-router: the MCP surface built its own lifecycle.Service with an
// empty OrgReconcilers slice, so runOrgReconcilers was a silent no-op.
//
// The fix gives mcptools.Config a Lifecycle injection seam so the composition
// root can share the one reconciler-complete lifecycle.Service with both the
// REST handlers and the MCP tools. This test injects a lifecycle.Service whose
// OrgReconcilers contains a spy, drives create_bot, and asserts the spy ran
// once for the caller's org — proving MCP-created bots trigger the whole-org
// (Slack auto-router) reconcile exactly as REST-created bots do.
func TestCreateBotRunsInjectedOrgReconcilers(t *testing.T) {
	t.Parallel()
	st := orggorm.GetOrgTestDB(t)
	now := time.Now().UTC()

	deps := DefaultDeps(st)
	deps.Now = func() time.Time { return now }
	var counter int
	deps.NewID = func() string {
		counter++
		return "id" + string(rune('a'+counter-1))
	}

	rec := reconcile.New(reconcile.Deps{
		Bots: st.Bots, ReportingLines: st.ReportingLines,
		Topics: st.Topics, Subscriptions: st.Subscriptions, Now: deps.Now,
	})
	botSvc := bots.New(bots.Deps{
		Bots: st.Bots, Lines: st.ReportingLines, Reconciler: rec,
		Now: deps.Now, NewID: deps.NewID, BaseTools: BaseReadTools,
	})
	spy := &spyOrgReconciler{}
	// The shared, reconciler-complete lifecycle.Service the composition root
	// builds — here with just enough wired for Create + the spy org reconciler.
	deps.Lifecycle = &lifecycle.Service{
		Store:          st,
		Bots:           botSvc,
		BotReconcilers: []lifecycle.BotReconciler{rec},
		OrgReconcilers: []lifecycle.OrgReconciler{spy},
		Now:            deps.Now,
		NewID:          deps.NewID,
	}

	tl := &CreateBot{deps: deps.Build()}
	args, _ := json.Marshal(createBotArgs{ID: "b-alice", Content: "# Alice", Tools: []string{}, Topics: []string{}})
	if _, err := tl.Invoke(context.Background(), tool.Invocation{
		Caller: botCaller{id: "b-owner", orgID: "org-test"},
		Args:   args,
	}); err != nil {
		t.Fatalf("Invoke create_bot: %v", err)
	}

	got := spy.calls()
	if len(got) != 1 {
		t.Fatalf("OrgReconciler.Reconcile calls = %d, want 1 (MCP create_bot must run the whole-org reconcilers)", len(got))
	}
	if got[0] != "org-test" {
		t.Errorf("OrgReconciler.Reconcile orgID = %q, want org-test", got[0])
	}
}
