package lifecycle_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/seedprompts"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
)

type lifecycleRuntime struct {
	store            *store.Store
	linkedErr        error
	deleteAppErr     error
	deleteProjectErr error
	projectDeleted   bool
	deletedProjects  []string
	deletedApps      []string
	cleanupCancelled bool
	linkedCancelled  bool
	stoppedSessions  []string
}

type interleavedDeleteRuntime struct {
	store   *store.Store
	started chan struct{}
	resume  chan struct{}
	mu      sync.Mutex
	calls   int
}

type olderSuccessfulDeleteRuntime struct {
	store   *store.Store
	started chan struct{}
	resume  chan struct{}
	mu      sync.Mutex
	calls   int
}

func (r *olderSuccessfulDeleteRuntime) DeleteProject(context.Context, string) error {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	if call == 1 {
		close(r.started)
		<-r.resume
		return nil
	}
	return errors.New("newer project delete failed")
}

func (*olderSuccessfulDeleteRuntime) DeleteApp(context.Context, string) error { return nil }

func (r *olderSuccessfulDeleteRuntime) DeleteLinkedAgent(ctx context.Context, orgID string, botID orgchart.NodeID, _, _ string) error {
	if err := r.store.NodeRuntimeState.Clear(ctx, orgID, botID, runtimehelix.Backend); err != nil {
		return err
	}
	return r.store.Nodes.Delete(ctx, orgID, botID)
}

func (r *interleavedDeleteRuntime) DeleteProject(context.Context, string) error {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	if call == 1 {
		close(r.started)
		<-r.resume
		return errors.New("project delete failed")
	}
	return nil
}

func (*interleavedDeleteRuntime) DeleteApp(context.Context, string) error { return nil }

func (r *interleavedDeleteRuntime) DeleteLinkedAgent(ctx context.Context, orgID string, botID orgchart.NodeID, _, _ string) error {
	if err := r.store.NodeRuntimeState.Clear(ctx, orgID, botID, runtimehelix.Backend); err != nil {
		return err
	}
	return r.store.Nodes.Delete(ctx, orgID, botID)
}

func (r *lifecycleRuntime) DeleteProject(_ context.Context, id string) error {
	r.projectDeleted = true
	r.deletedProjects = append(r.deletedProjects, id)
	return r.deleteProjectErr
}

func (r *lifecycleRuntime) DeleteApp(ctx context.Context, id string) error {
	r.cleanupCancelled = ctx.Err() != nil
	r.deletedApps = append(r.deletedApps, id)
	return r.deleteAppErr
}

func (r *lifecycleRuntime) DeleteLinkedAgent(ctx context.Context, orgID string, botID orgchart.NodeID, appID, sessionID string) error {
	r.linkedCancelled = ctx.Err() != nil
	if r.linkedErr != nil {
		return r.linkedErr
	}
	if sessionID != "" {
		r.stoppedSessions = append(r.stoppedSessions, sessionID)
	}
	if r.store == nil {
		return nil
	}
	if r.store.NodeRuntimeState != nil {
		if err := r.store.NodeRuntimeState.Clear(ctx, orgID, botID, runtimehelix.Backend); err != nil {
			return err
		}
	}
	return r.store.Nodes.Delete(ctx, orgID, botID)
}

type fixedAgentCreator struct {
	id string
}

func (c fixedAgentCreator) CreateAgent(context.Context, string, string, string, lifecycle.AgentConfig) (string, error) {
	return c.id, nil
}

type cancellingReconciler struct {
	cancel context.CancelFunc
}

func (r cancellingReconciler) Reconcile(context.Context, string, ...orgchart.NodeID) error {
	r.cancel()
	return errors.New("reconcile failed")
}

type losingClaimBots struct {
	store.Nodes
	winner string
}

type failingClaimBots struct {
	store.Nodes
	err error
}

func (b failingClaimBots) ClaimAgentApp(context.Context, string, orgchart.NodeID, string) (bool, error) {
	return false, b.err
}

func (b losingClaimBots) ClaimAgentApp(ctx context.Context, orgID string, id orgchart.NodeID, _ string) (bool, error) {
	current, err := b.Nodes.Get(ctx, orgID, id)
	if err != nil {
		return false, err
	}
	if err := b.Nodes.Update(ctx, current.WithAgentID(b.winner)); err != nil {
		return false, err
	}
	return false, nil
}

func TestReconcileAgentLinksPreservesReplicaWinner(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	bot, err := orgchart.NewNode("b-agent", "instructions", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(ctx, bot); err != nil {
		t.Fatal(err)
	}
	copyStore := *st
	copyStore.Nodes = losingClaimBots{Nodes: st.Nodes, winner: "app-winner"}
	runtime := &lifecycleRuntime{}
	svc := &lifecycle.Service{
		Store:  &copyStore,
		Agents: fixedAgentCreator{id: "app-loser"},
		Helix:  runtime,
		Nodes: nodes.New(nodes.Deps{
			Nodes: copyStore.Nodes,
			Now:   func() time.Time { return time.Now().UTC() },
			NewID: func() string { return "unused" },
		}),
	}

	if err := svc.ReconcileAgentLinks(ctx, "org-test"); err != nil {
		t.Fatalf("reconcile links: %v", err)
	}
	got, err := st.Nodes.Get(ctx, "org-test", bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentID != "app-winner" {
		t.Fatalf("winner = %q", got.AgentID)
	}
	if len(runtime.deletedApps) != 1 || runtime.deletedApps[0] != "app-loser" {
		t.Fatalf("discarded apps = %v", runtime.deletedApps)
	}
}

func TestReconcileAgentLinksReportsClaimAndCleanupFailures(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	bot, err := orgchart.NewNode("b-agent", "instructions", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(ctx, bot); err != nil {
		t.Fatal(err)
	}
	copyStore := *st
	copyStore.Nodes = failingClaimBots{Nodes: st.Nodes, err: errors.New("claim failed")}
	runtime := &lifecycleRuntime{deleteAppErr: errors.New("cleanup failed")}
	svc := &lifecycle.Service{
		Store:  &copyStore,
		Agents: fixedAgentCreator{id: "app-unlinked"},
		Helix:  runtime,
		Nodes: nodes.New(nodes.Deps{
			Nodes: copyStore.Nodes,
			Now:   func() time.Time { return time.Now().UTC() },
			NewID: func() string { return "unused" },
		}),
	}

	err = svc.ReconcileAgentLinks(ctx, "org-test")
	if err == nil || !strings.Contains(err.Error(), "claim failed") || !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatalf("reconcile error = %v", err)
	}
}

func TestDeleteArchivesOwnedProjectAndPreservesAllowedProjects(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	bot, err := orgchart.NewNode("b-agent", "instructions", nil, time.Now().UTC(), "org-test")
	require.NoError(t, err)
	bot = bot.WithAgentID("app-agent")
	bot = bot.WithProjectIDs([]string{"project-configured"})
	require.NoError(t, st.Nodes.Create(ctx, bot))
	require.NoError(t, runtimehelix.SaveProject(ctx, st, "org-test", bot.ID, "project-owned", "app-agent", "repo-owned"))
	require.NoError(t, runtimehelix.SaveSession(ctx, st, "org-test", bot.ID, "session-agent"))
	runtime := &lifecycleRuntime{store: st}
	svc := &lifecycle.Service{Store: st, Helix: runtime}

	require.NoError(t, svc.Delete(ctx, "org-test", bot.ID))
	require.Equal(t, []string{"project-owned"}, runtime.deletedProjects)
	require.Equal(t, []string{"session-agent"}, runtime.stoppedSessions)
	if _, err := st.Nodes.Get(ctx, "org-test", bot.ID); err == nil {
		t.Fatal("node still exists")
	}
	require.NoError(t, st.Nodes.Create(ctx, bot.WithAgentID("app-replacement")))
}

func TestDeleteFailurePreservesGraphAnchorAndRetry(t *testing.T) {
	for _, tc := range []struct {
		name             string
		linkedErr        error
		deleteProjectErr error
	}{
		{name: "app", linkedErr: errors.New("app delete failed")},
		{name: "project", deleteProjectErr: errors.New("project delete failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := memory.New()
			ctx := context.Background()
			bot, err := orgchart.NewNode("b-agent", "instructions", nil, time.Now().UTC(), "org-test")
			if err != nil {
				t.Fatal(err)
			}
			bot = bot.WithAgentID("app-agent")
			if err := st.Nodes.Create(ctx, bot); err != nil {
				t.Fatal(err)
			}
			if err := runtimehelix.SaveProject(ctx, st, "org-test", bot.ID, "project-agent", "app-agent", "repo-agent"); err != nil {
				t.Fatal(err)
			}
			runtime := &lifecycleRuntime{store: st, linkedErr: tc.linkedErr, deleteProjectErr: tc.deleteProjectErr}
			svc := &lifecycle.Service{Store: st, Helix: runtime}

			if err := svc.Delete(ctx, "org-test", bot.ID); err == nil {
				t.Fatal("delete succeeded despite runtime failure")
			}
			got, err := st.Nodes.Get(ctx, "org-test", bot.ID)
			if err != nil {
				t.Fatalf("graph anchor removed: %v", err)
			}
			if got.AgentID != "app-agent" {
				t.Fatalf("graph link = %q", got.AgentID)
			}
			state, err := runtimehelix.LoadState(ctx, st, "org-test", bot.ID)
			if err != nil {
				t.Fatal(err)
			}
			if state.ProjectID != "project-agent" || state.AgentID != "app-agent" {
				t.Fatalf("runtime state changed: %+v", state)
			}

			runtime.linkedErr = nil
			runtime.deleteProjectErr = nil
			if err := svc.Delete(ctx, "org-test", bot.ID); err != nil {
				t.Fatalf("retry delete: %v", err)
			}
			recreated := bot.WithAgentID("app-recreated")
			if err := st.Nodes.Create(ctx, recreated); err != nil {
				t.Fatalf("recreate after delete: %v", err)
			}
		})
	}
}

func TestDeleteChiefOfStaffFailureRollsBackDeletionMarker(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	bot, err := orgchart.NewNode(seedprompts.ChiefOfStaffBotID, "instructions", nil, time.Now().UTC(), "org-test")
	require.NoError(t, err)
	bot = bot.WithAgentID("app-agent")
	require.NoError(t, st.Nodes.Create(ctx, bot))
	require.NoError(t, runtimehelix.SaveProject(ctx, st, "org-test", bot.ID, "project-agent", "app-agent", "repo-agent"))
	svc := &lifecycle.Service{Store: st, Helix: &lifecycleRuntime{deleteProjectErr: errors.New("project delete failed")}}

	require.Error(t, svc.Delete(ctx, "org-test", bot.ID))
	marked, err := svc.ChiefOfStaffDeletionMarked(ctx, "org-test")
	require.NoError(t, err)
	require.False(t, marked)
	_, err = st.Nodes.Get(ctx, "org-test", bot.ID)
	require.NoError(t, err)
}

func TestConcurrentChiefOfStaffDeleteFailurePreservesSuccessfulMarker(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	bot, err := orgchart.NewNode(seedprompts.ChiefOfStaffBotID, "instructions", nil, time.Now().UTC(), "org-test")
	require.NoError(t, err)
	bot = bot.WithAgentID("app-agent")
	require.NoError(t, st.Nodes.Create(ctx, bot))
	require.NoError(t, runtimehelix.SaveProject(ctx, st, "org-test", bot.ID, "project-agent", "app-agent", "repo-agent"))
	runtime := &interleavedDeleteRuntime{store: st, started: make(chan struct{}), resume: make(chan struct{})}
	svc := &lifecycle.Service{Store: st, Helix: runtime}
	firstDone := make(chan error, 1)
	go func() { firstDone <- svc.Delete(ctx, "org-test", bot.ID) }()
	<-runtime.started

	require.NoError(t, svc.Delete(ctx, "org-test", bot.ID))
	close(runtime.resume)
	require.Error(t, <-firstDone)
	marked, err := svc.ChiefOfStaffDeletionMarked(ctx, "org-test")
	require.NoError(t, err)
	require.True(t, marked)
}

func TestOlderSuccessfulChiefOfStaffDeleteRestoresMarkerAfterNewerFailure(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	bot, err := orgchart.NewNode(seedprompts.ChiefOfStaffBotID, "instructions", nil, time.Now().UTC(), "org-test")
	require.NoError(t, err)
	bot = bot.WithAgentID("app-agent")
	require.NoError(t, st.Nodes.Create(ctx, bot))
	require.NoError(t, runtimehelix.SaveProject(ctx, st, "org-test", bot.ID, "project-agent", "app-agent", "repo-agent"))
	runtime := &olderSuccessfulDeleteRuntime{store: st, started: make(chan struct{}), resume: make(chan struct{})}
	svc := &lifecycle.Service{Store: st, Helix: runtime}
	olderDone := make(chan error, 1)
	go func() { olderDone <- svc.Delete(ctx, "org-test", bot.ID) }()
	<-runtime.started

	require.Error(t, svc.Delete(ctx, "org-test", bot.ID))
	close(runtime.resume)
	require.NoError(t, <-olderDone)
	marked, err := svc.ChiefOfStaffDeletionMarked(ctx, "org-test")
	require.NoError(t, err)
	require.True(t, marked)
}

func TestCreateCleanupIgnoresCancelledRequestContext(t *testing.T) {
	st := memory.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	existing, err := orgchart.NewNode("b-agent", "existing", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(context.Background(), existing); err != nil {
		t.Fatal(err)
	}
	runtime := &lifecycleRuntime{}
	svc := &lifecycle.Service{
		Store:  st,
		Helix:  runtime,
		Agents: fixedAgentCreator{id: "app-cleanup"},
		Nodes: nodes.New(nodes.Deps{
			Nodes: st.Nodes,
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
		Store:           st,
		Helix:           runtime,
		Agents:          fixedAgentCreator{id: "app-cleanup"},
		NodeReconcilers: []lifecycle.NodeReconciler{cancellingReconciler{cancel: cancel}},
		Nodes: nodes.New(nodes.Deps{
			Nodes: st.Nodes,
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
	if _, err := st.Nodes.Get(context.Background(), "org-test", "b-agent"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("rolled back bot still exists: %v", err)
	}
}
