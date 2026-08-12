package server

import (
	"context"
	"errors"
	"log/slog"
	"testing"

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
// per-workspace Topic reconciler over a real in-memory org Topics store.
// slackSocket stays nil — kickSlackSocket is nil-safe.
func newSlackCascadeServer() (*HelixAPIServer, *fakeServiceConnStore, helixorgstore.Topics) {
	fake := &fakeServiceConnStore{conns: map[string]*types.ServiceConnection{}}
	orgStore := orgmemory.New()
	s := &HelixAPIServer{
		Store: fake,
		helixOrg: &helixOrgHandlers{
			slackTopics: &slackWorkspaceTopics{topics: orgStore.Topics, logger: slog.Default()},
		},
	}
	return s, fake, orgStore.Topics
}

// seedWSConn adds a slack_workspace connection installed from appConnID,
// plus its auto-managed Topic (via the same reconciler production uses).
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

func wsTopicExists(t *testing.T, topics helixorgstore.Topics, orgID, connID string) bool {
	t.Helper()
	_, err := topics.Get(context.Background(), orgID, slackWorkspaceTopicID(connID))
	switch {
	case err == nil:
		return true
	case errors.Is(err, helixorgstore.ErrNotFound):
		return false
	default:
		t.Fatalf("Topics.Get: %v", err)
		return false
	}
}

// Deleting a global slack_app removes every workspace install made from it
// — and each install's auto-managed Topic — across all orgs, while an
// install from a different app is left untouched. Driven through the
// registered observer hook (reactToServiceConnectionChange), the same seam
// the service-connection delete handler fires.
func TestSlackApp_DeleteCascadesWorkspacesAndTopics(t *testing.T) {
	s, fake, topics := newSlackCascadeServer()
	seedWSConn(t, s, fake, "ws-a", "orgA", "app1")
	seedWSConn(t, s, fake, "ws-b", "orgB", "app1") // same app, different org
	seedWSConn(t, s, fake, "ws-c", "orgC", "app2") // a different app — must survive

	for _, w := range []struct{ org, conn string }{{"orgA", "ws-a"}, {"orgB", "ws-b"}, {"orgC", "ws-c"}} {
		if !wsTopicExists(t, topics, w.org, w.conn) {
			t.Fatalf("precondition: topic for %s missing", w.conn)
		}
	}

	app := &types.ServiceConnection{ID: "app1", Type: types.ServiceConnectionTypeSlackApp}
	s.reactToServiceConnectionChange(context.Background(), app, true)

	for _, w := range []struct{ org, conn string }{{"orgA", "ws-a"}, {"orgB", "ws-b"}} {
		if _, ok := fake.conns[w.conn]; ok {
			t.Errorf("workspace %s should be deleted", w.conn)
		}
		if wsTopicExists(t, topics, w.org, w.conn) {
			t.Errorf("topic for %s should be deleted", w.conn)
		}
	}
	if _, ok := fake.conns["ws-c"]; !ok {
		t.Error("ws-c (different app) must not be deleted")
	}
	if !wsTopicExists(t, topics, "orgC", "ws-c") {
		t.Error("ws-c topic (different app) must survive")
	}
}

// The hook reacts only to slack_app: deleting a github_app leaves slack
// workspace installs (and their topics) untouched.
func TestServiceConnectionChange_IgnoresNonSlackApp(t *testing.T) {
	s, fake, topics := newSlackCascadeServer()
	seedWSConn(t, s, fake, "ws-a", "orgA", "app1")

	gh := &types.ServiceConnection{ID: "gh1", Type: types.ServiceConnectionTypeGitHubApp}
	s.reactToServiceConnectionChange(context.Background(), gh, true)

	if _, ok := fake.conns["ws-a"]; !ok {
		t.Error("a github_app delete must not cascade slack workspaces")
	}
	if !wsTopicExists(t, topics, "orgA", "ws-a") {
		t.Error("slack topic must survive a github_app delete")
	}
}

// A slack_app create/edit (deleted=false) reconciles Socket Mode but must
// NOT cascade-delete workspace installs.
func TestSlackApp_NonDeleteDoesNotCascade(t *testing.T) {
	s, fake, topics := newSlackCascadeServer()
	seedWSConn(t, s, fake, "ws-a", "orgA", "app1")

	app := &types.ServiceConnection{ID: "app1", Type: types.ServiceConnectionTypeSlackApp}
	s.reactToServiceConnectionChange(context.Background(), app, false)

	if _, ok := fake.conns["ws-a"]; !ok {
		t.Error("a slack_app edit must not delete workspace installs")
	}
	if !wsTopicExists(t, topics, "orgA", "ws-a") {
		t.Error("a slack_app edit must not delete topics")
	}
}
