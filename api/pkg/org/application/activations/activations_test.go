package activations

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

// fakeEnsurer / fakeDispatcher are the minimal Activate collaborators the
// tests wire so the audit-row pre-allocation can be exercised through the
// public Activate command rather than a standalone helper.
type fakeEnsurer struct{}

func (fakeEnsurer) Ensure(_ context.Context, _ string, _ orgchart.WorkerID) (string, string, string, error) {
	return "prj-1", "app-1", "repo-1", nil
}

type fakeDispatcher struct{ gotID activation.ID }

func (f *fakeDispatcher) DispatchManual(_ context.Context, _ string, _ orgchart.WorkerID, _ string, activationID activation.ID) {
	f.gotID = activationID
}

// TestActivate_PreAllocatesAuditRow: with a wired repo, Activate mints the
// `a-<id>` audit row, persists it, surfaces the id in the result, and
// hands that same id to the dispatcher.
func TestActivate_PreAllocatesAuditRow(t *testing.T) {
	t.Parallel()
	st := memory.New()
	disp := &fakeDispatcher{}
	svc := New(Deps{
		Repo:       st.Activations,
		Now:        func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) },
		NewID:      func() string { return "fixed" },
		Ensurer:    fakeEnsurer{},
		Dispatcher: disp,
	})

	res, err := svc.Activate(context.Background(), "org-test", "w-mark")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if res.ActivationID != "a-fixed" {
		t.Fatalf("activation id = %q, want a-fixed", res.ActivationID)
	}
	if res.ProjectID != "prj-1" || res.AgentAppID != "app-1" {
		t.Fatalf("project/agent ids = %q/%q, want prj-1/app-1", res.ProjectID, res.AgentAppID)
	}
	if disp.gotID != "a-fixed" {
		t.Fatalf("dispatcher got id %q, want a-fixed", disp.gotID)
	}
	got, err := st.Activations.Get(context.Background(), "org-test", res.ActivationID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("activation row not persisted")
	}
}

// TestActivate_NoRepoMintsNoRow: with no repo wired, Activate skips the
// pre-allocation — the result and the dispatcher both get an empty id, so
// the Spawner mints its own (the previous inline behaviour).
func TestActivate_NoRepoMintsNoRow(t *testing.T) {
	t.Parallel()
	disp := &fakeDispatcher{}
	svc := New(Deps{
		NewID:      func() string { return "x" }, // no Repo
		Ensurer:    fakeEnsurer{},
		Dispatcher: disp,
	})

	res, err := svc.Activate(context.Background(), "org-test", "w-mark")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if res.ActivationID != "" {
		t.Fatalf("activation id = %q, want empty (no pre-allocation)", res.ActivationID)
	}
	if disp.gotID != "" {
		t.Fatalf("dispatcher got id %q, want empty", disp.gotID)
	}
}
