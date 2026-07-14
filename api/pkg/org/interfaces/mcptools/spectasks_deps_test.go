package mcptools_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	orgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
)

// seedMemberBot creates a Bot row so the caller passes the service-level
// org-membership check (DefaultDeps wires the Queries verifier).
func seedMemberBot(t *testing.T, s *orgstore.Store, orgID, botID string) {
	t.Helper()
	bot, err := orgchart.NewBot(botID, "# bot", nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new bot: %v", err)
	}
	if err := s.Bots.Create(context.Background(), bot); err != nil {
		t.Fatalf("create bot: %v", err)
	}
}

// TestDefaultDepsWiresSpecTasksNoop pins that DefaultDeps + Build produce
// a non-nil SpecTasks application service backed by the noop port, so a
// call returns ErrSpecTasksUnsupported rather than panicking on a nil
// interface.
func TestDefaultDepsWiresSpecTasksNoop(t *testing.T) {
	t.Parallel()
	s := orggorm.GetOrgTestDB(t)
	deps := mcptools.DefaultDeps(s).Build()
	if deps.SpecTasks == nil {
		t.Fatal("Deps.SpecTasks is nil; expected a non-nil service over the noop port")
	}
	seedMemberBot(t, s, "org-1", "w-a")
	_, err := deps.SpecTasks.Get(context.Background(), fakeWorker{id: "w-a", org: "org-1"}, "", "task_1")
	if !errors.Is(err, runtime.ErrSpecTasksUnsupported) {
		t.Errorf("err = %v, want ErrSpecTasksUnsupported", err)
	}
}

// TestConfigBuildUsesInjectedSpecTasksPort pins that a real port set on
// the Config is the one the built service talks to.
func TestConfigBuildUsesInjectedSpecTasksPort(t *testing.T) {
	t.Parallel()
	s := orggorm.GetOrgTestDB(t)
	cfg := mcptools.DefaultDeps(s)
	cfg.SpecTasks = stubPort{}
	deps := cfg.Build()
	seedMemberBot(t, s, "org-1", "w-a")
	view, err := deps.SpecTasks.Get(context.Background(), fakeWorker{id: "w-a", org: "org-1"}, "", "task_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if view.ID != "stub" {
		t.Errorf("view.ID = %q, want stub (injected port not used)", view.ID)
	}
}

type fakeWorker struct {
	id  string
	org string
}

func (w fakeWorker) ID() string             { return w.id }
func (w fakeWorker) OrganizationID() string { return w.org }

// stubPort is a runtime.SpecTasks that returns a recognisable view so the
// test can prove the injected port is wired through Build.
type stubPort struct{ runtime.NoopSpecTasks }

func (stubPort) Get(_ context.Context, _ string, _ string, _, _ string) (runtime.SpecTaskView, error) {
	return runtime.SpecTaskView{ID: "stub"}, nil
}
