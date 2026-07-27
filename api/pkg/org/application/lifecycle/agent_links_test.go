package lifecycle_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
)

type lifecycleRuntime struct {
	store            *store.Store
	projectErr       error
	linkedErr        error
	deleteAppErr     error
	projectDeleted   bool
	deletedApps      []string
	cleanupCancelled bool
	linkedCancelled  bool
}

func (r *lifecycleRuntime) DeleteProject(context.Context, string) error {
	if r.projectDeleted {
		return runtimehelix.ErrProjectNotFound
	}
	if r.projectErr == nil {
		r.projectDeleted = true
	}
	return r.projectErr
}

func (r *lifecycleRuntime) DeleteApp(ctx context.Context, id string) error {
	r.cleanupCancelled = ctx.Err() != nil
	r.deletedApps = append(r.deletedApps, id)
	return r.deleteAppErr
}

func (r *lifecycleRuntime) DeleteLinkedAgent(ctx context.Context, orgID string, botID orgchart.BotID, appID string) error {
	r.linkedCancelled = ctx.Err() != nil
	if r.linkedErr != nil {
		return r.linkedErr
	}
	if r.store == nil {
		return nil
	}
	if r.store.BotRuntimeState != nil {
		if err := r.store.BotRuntimeState.Clear(ctx, orgID, botID, runtimehelix.Backend); err != nil {
			return err
		}
	}
	return r.store.Bots.Delete(ctx, orgID, botID)
}

type fixedAgentCreator struct {
	id string
}

func (c fixedAgentCreator) CreateAgent(context.Context, string, string, string) (string, error) {
	return c.id, nil
}

type cancellingReconciler struct {
	cancel context.CancelFunc
}

func (r cancellingReconciler) Reconcile(context.Context, string, ...orgchart.BotID) error {
	r.cancel()
	return errors.New("reconcile failed")
}

type losingClaimBots struct {
	store.Bots
	winner string
}

type failingClaimBots struct {
	store.Bots
	err error
}

func (b failingClaimBots) ClaimAgentApp(context.Context, string, orgchart.BotID, string) (bool, error) {
	return false, b.err
}

func (b losingClaimBots) ClaimAgentApp(ctx context.Context, orgID string, id orgchart.BotID, _ string) (bool, error) {
	current, err := b.Bots.Get(ctx, orgID, id)
	if err != nil {
		return false, err
	}
	if err := b.Bots.Update(ctx, current.WithAgentAppID(b.winner)); err != nil {
		return false, err
	}
	return false, nil
}

func TestReconcileAgentLinksPreservesReplicaWinner(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	bot, err := orgchart.NewBot("b-agent", "instructions", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bots.Create(ctx, bot); err != nil {
		t.Fatal(err)
	}
	copyStore := *st
	copyStore.Bots = losingClaimBots{Bots: st.Bots, winner: "app-winner"}
	runtime := &lifecycleRuntime{}
	svc := &lifecycle.Service{
		Store:  &copyStore,
		Agents: fixedAgentCreator{id: "app-loser"},
		Helix:  runtime,
		Bots: bots.New(bots.Deps{
			Bots:  copyStore.Bots,
			Now:   func() time.Time { return time.Now().UTC() },
			NewID: func() string { return "unused" },
		}),
	}

	if err := svc.ReconcileAgentLinks(ctx, "org-test"); err != nil {
		t.Fatalf("reconcile links: %v", err)
	}
	got, err := st.Bots.Get(ctx, "org-test", bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentAppID != "app-winner" {
		t.Fatalf("winner = %q", got.AgentAppID)
	}
	if len(runtime.deletedApps) != 1 || runtime.deletedApps[0] != "app-loser" {
		t.Fatalf("discarded apps = %v", runtime.deletedApps)
	}
}

func TestReconcileAgentLinksReportsClaimAndCleanupFailures(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	bot, err := orgchart.NewBot("b-agent", "instructions", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bots.Create(ctx, bot); err != nil {
		t.Fatal(err)
	}
	copyStore := *st
	copyStore.Bots = failingClaimBots{Bots: st.Bots, err: errors.New("claim failed")}
	runtime := &lifecycleRuntime{deleteAppErr: errors.New("cleanup failed")}
	svc := &lifecycle.Service{
		Store:  &copyStore,
		Agents: fixedAgentCreator{id: "app-unlinked"},
		Helix:  runtime,
		Bots: bots.New(bots.Deps{
			Bots:  copyStore.Bots,
			Now:   func() time.Time { return time.Now().UTC() },
			NewID: func() string { return "unused" },
		}),
	}

	err = svc.ReconcileAgentLinks(ctx, "org-test")
	if err == nil || !strings.Contains(err.Error(), "claim failed") || !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatalf("reconcile error = %v", err)
	}
}

func TestDeleteFailuresPreserveGraphAnchorAndRetry(t *testing.T) {
	for _, tc := range []struct {
		name       string
		projectErr error
		linkedErr  error
	}{
		{name: "project", projectErr: errors.New("project delete failed")},
		{name: "app", linkedErr: errors.New("app delete failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := memory.New()
			ctx := context.Background()
			bot, err := orgchart.NewBot("b-agent", "instructions", nil, time.Now().UTC(), "org-test")
			if err != nil {
				t.Fatal(err)
			}
			bot = bot.WithAgentAppID("app-agent")
			if err := st.Bots.Create(ctx, bot); err != nil {
				t.Fatal(err)
			}
			if err := runtimehelix.SaveProject(ctx, st, "org-test", bot.ID, "project-agent", "app-agent", "repo-agent"); err != nil {
				t.Fatal(err)
			}
			runtime := &lifecycleRuntime{store: st, projectErr: tc.projectErr, linkedErr: tc.linkedErr}
			svc := &lifecycle.Service{Store: st, Helix: runtime}

			if err := svc.Delete(ctx, "org-test", bot.ID); err == nil {
				t.Fatal("delete succeeded despite runtime failure")
			}
			got, err := st.Bots.Get(ctx, "org-test", bot.ID)
			if err != nil {
				t.Fatalf("graph anchor removed: %v", err)
			}
			if got.AgentAppID != "app-agent" {
				t.Fatalf("graph link = %q", got.AgentAppID)
			}
			state, err := runtimehelix.LoadState(ctx, st, "org-test", bot.ID)
			if err != nil {
				t.Fatal(err)
			}
			if state.ProjectID != "project-agent" || state.AgentAppID != "app-agent" {
				t.Fatalf("runtime state changed: %+v", state)
			}

			runtime.projectErr = nil
			runtime.linkedErr = nil
			if err := svc.Delete(ctx, "org-test", bot.ID); err != nil {
				t.Fatalf("retry delete: %v", err)
			}
			recreated := bot.WithAgentAppID("app-recreated")
			if err := st.Bots.Create(ctx, recreated); err != nil {
				t.Fatalf("recreate after delete: %v", err)
			}
		})
	}
}

func TestCreateCleanupIgnoresCancelledRequestContext(t *testing.T) {
	st := memory.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	existing, err := orgchart.NewBot("b-agent", "existing", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bots.Create(context.Background(), existing); err != nil {
		t.Fatal(err)
	}
	runtime := &lifecycleRuntime{}
	svc := &lifecycle.Service{
		Store:  st,
		Helix:  runtime,
		Agents: fixedAgentCreator{id: "app-cleanup"},
		Bots: bots.New(bots.Deps{
			Bots:  st.Bots,
			Now:   func() time.Time { return time.Now().UTC() },
			NewID: func() string { return "unused" },
		}),
		Now:   func() time.Time { return time.Now().UTC() },
		NewID: func() string { return "unused" },
	}

	if _, err := svc.Create(ctx, "org-test", lifecycle.CreateParams{ID: "b-agent", Content: "new"}); err == nil {
		t.Fatal("duplicate create succeeded")
	}
	if runtime.cleanupCancelled {
		t.Fatal("App cleanup inherited cancelled request context")
	}
	if len(runtime.deletedApps) != 1 || runtime.deletedApps[0] != "app-cleanup" {
		t.Fatalf("cleaned apps = %v", runtime.deletedApps)
	}
}

func TestCreateRollbackIgnoresCancelledRequestContext(t *testing.T) {
	st := memory.New()
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &lifecycleRuntime{store: st}
	svc := &lifecycle.Service{
		Store:          st,
		Helix:          runtime,
		Agents:         fixedAgentCreator{id: "app-cleanup"},
		BotReconcilers: []lifecycle.BotReconciler{cancellingReconciler{cancel: cancel}},
		Bots: bots.New(bots.Deps{
			Bots:  st.Bots,
			Now:   func() time.Time { return time.Now().UTC() },
			NewID: func() string { return "unused" },
		}),
		Now:   func() time.Time { return time.Now().UTC() },
		NewID: func() string { return "unused" },
	}

	if _, err := svc.Create(ctx, "org-test", lifecycle.CreateParams{ID: "b-agent", Content: "new"}); err == nil {
		t.Fatal("create succeeded despite reconcile failure")
	}
	if runtime.linkedCancelled {
		t.Fatal("lifecycle rollback inherited cancelled request context")
	}
	if _, err := st.Bots.Get(context.Background(), "org-test", "b-agent"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("rolled back bot still exists: %v", err)
	}
}
