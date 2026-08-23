package server

import (
	"context"
	"log/slog"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/triggers"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// fakeServiceConnStore is a minimal stateful store.Store: only the two
// methods the slack_app delete cascade touches are real; every other
// method is promoted from the (nil) embedded interface and must not be
// called by the code under test.
type fakeServiceConnStore struct {
	store.Store
	conns map[string]*types.ServiceConnection
}

func (f *fakeServiceConnStore) ListServiceConnectionsByType(_ context.Context, orgID string, t types.ServiceConnectionType) ([]*types.ServiceConnection, error) {
	var out []*types.ServiceConnection
	for _, c := range f.conns {
		if c.Type == t && (orgID == "" || c.OrganizationID == orgID) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeServiceConnStore) DeleteServiceConnection(_ context.Context, id string) error {
	delete(f.conns, id)
	return nil
}

// newSlackCascadeServer wires a HelixAPIServer with just enough to run the
// slack_app delete cascade: a stateful service-connection store and the
// per-workspace Trigger reconciler over a real in-memory org store.
// slackSocket stays nil — kickSlackSocket is nil-safe.
func newSlackCascadeServer() (*HelixAPIServer, *fakeServiceConnStore, helixorgstore.Triggers) {
	fake := &fakeServiceConnStore{conns: map[string]*types.ServiceConnection{}}
	orgStore := orgmemory.New()
	triggerSvc := triggers.New(triggers.Deps{
		Triggers: orgStore.Triggers, Attachments: orgStore.WorkerAttachments, Events: orgStore.Events,
	})
	s := &HelixAPIServer{
		Store: fake,
		helixOrg: &helixOrgHandlers{
			slackTopics: &slackWorkspaceTopics{triggers: triggerSvc, logger: slog.Default()},
		},
	}
	return s, fake, orgStore.Triggers
}

// seedWSConn adds a slack_workspace connection installed from appConnID,
// plus its auto-managed Trigger (via the same reconciler production uses).
func seedWSConn(t *testing.T, s *HelixAPIServer, fake *fakeServiceConnStore, connID, orgID, appConnID string) {
	t.Helper()
	fake.conns[connID] = &types.ServiceConnection{
		ID:                   connID,
		OrganizationID:       orgID,
		Type:                 types.ServiceConnectionTypeSlackWorkspace,
		SlackAppConnectionID: appConnID,
	}
	s.helixOrg.slackTopics.ensure(context.Background(), orgID, connID, "ws-"+connID, "app")
}

func wsTriggerExists(t *testing.T, repo helixorgstore.Triggers, orgID, connID string) bool {
	t.Helper()
	rows, err := repo.Find(context.Background(), helixorgstore.WithOrg(orgID),
		helixorgstore.WithID(slackWorkspaceTriggerID(connID)), helixorgstore.WithLimit(1))
	if err != nil {
		t.Fatalf("Triggers.Find: %v", err)
	}
	return len(rows) > 0
}

// Deleting a global slack_app removes every workspace install made from it
// — and each install's auto-managed Trigger — across all orgs, while an
// install from a different app is left untouched. Driven through the
// registered observer hook (reactToServiceConnectionChange), the same seam
// the service-connection delete handler fires.
func TestSlackApp_DeleteCascadesWorkspacesAndTriggers(t *testing.T) {
	s, fake, triggerRepo := newSlackCascadeServer()
	seedWSConn(t, s, fake, "ws-a", "orgA", "app1")
	seedWSConn(t, s, fake, "ws-b", "orgB", "app1") // same app, different org
	seedWSConn(t, s, fake, "ws-c", "orgC", "app2") // a different app — must survive

	for _, w := range []struct{ org, conn string }{{"orgA", "ws-a"}, {"orgB", "ws-b"}, {"orgC", "ws-c"}} {
		if !wsTriggerExists(t, triggerRepo, w.org, w.conn) {
			t.Fatalf("precondition: trigger for %s missing", w.conn)
		}
	}

	app := &types.ServiceConnection{ID: "app1", Type: types.ServiceConnectionTypeSlackApp}
	s.reactToServiceConnectionChange(context.Background(), app, true)

	for _, w := range []struct{ org, conn string }{{"orgA", "ws-a"}, {"orgB", "ws-b"}} {
		if _, ok := fake.conns[w.conn]; ok {
			t.Errorf("workspace %s should be deleted", w.conn)
		}
		if wsTriggerExists(t, triggerRepo, w.org, w.conn) {
			t.Errorf("trigger for %s should be deleted", w.conn)
		}
	}
	if _, ok := fake.conns["ws-c"]; !ok {
		t.Error("ws-c (different app) must not be deleted")
	}
	if !wsTriggerExists(t, triggerRepo, "orgC", "ws-c") {
		t.Error("ws-c trigger (different app) must survive")
	}
}

// The hook reacts only to slack_app: deleting a github_app leaves slack
// workspace installs (and their Triggers) untouched.
func TestServiceConnectionChange_IgnoresNonSlackApp(t *testing.T) {
	s, fake, triggerRepo := newSlackCascadeServer()
	seedWSConn(t, s, fake, "ws-a", "orgA", "app1")

	gh := &types.ServiceConnection{ID: "gh1", Type: types.ServiceConnectionTypeGitHubApp}
	s.reactToServiceConnectionChange(context.Background(), gh, true)

	if _, ok := fake.conns["ws-a"]; !ok {
		t.Error("a github_app delete must not cascade slack workspaces")
	}
	if !wsTriggerExists(t, triggerRepo, "orgA", "ws-a") {
		t.Error("slack trigger must survive a github_app delete")
	}
}

// A slack_app create/edit (deleted=false) reconciles Socket Mode but must
// NOT cascade-delete workspace installs.
func TestSlackApp_NonDeleteDoesNotCascade(t *testing.T) {
	s, fake, triggerRepo := newSlackCascadeServer()
	seedWSConn(t, s, fake, "ws-a", "orgA", "app1")

	app := &types.ServiceConnection{ID: "app1", Type: types.ServiceConnectionTypeSlackApp}
	s.reactToServiceConnectionChange(context.Background(), app, false)

	if _, ok := fake.conns["ws-a"]; !ok {
		t.Error("a slack_app edit must not delete workspace installs")
	}
	if !wsTriggerExists(t, triggerRepo, "orgA", "ws-a") {
		t.Error("a slack_app edit must not delete triggers")
	}
}
