package helixorg

import (
	"context"
	"testing"

	"github.com/google/go-github/v61/github"

	githubclient "github.com/helixml/helix/api/pkg/github"
	"github.com/helixml/helix/api/pkg/types"
)

// fakeServiceConnections is a hand-written in-memory ServiceConnections
// fake (no gomock — the sync logic is what we're exercising, not call
// expectations).
type fakeServiceConnections struct {
	conns   []*types.ServiceConnection
	updated []*types.ServiceConnection
	deleted []string
}

func (f *fakeServiceConnections) ListServiceConnectionsByType(_ context.Context, _ string, _ types.ServiceConnectionType) ([]*types.ServiceConnection, error) {
	return f.conns, nil
}
func (f *fakeServiceConnections) UpdateServiceConnection(_ context.Context, c *types.ServiceConnection) error {
	f.updated = append(f.updated, c)
	return nil
}
func (f *fakeServiceConnections) DeleteServiceConnection(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func testIntegration(fake *fakeServiceConnections, listInstalls func(context.Context, int64, string, string) ([]*github.Installation, error)) *GitHubIntegration {
	return &GitHubIntegration{
		conns:        fake,
		decrypt:      func(*types.ServiceConnection) (string, error) { return "pem", nil },
		appSlug:      "fallback-slug",
		webURL:       "https://github.com",
		listInstalls: listInstalls,
	}
}

// TestInstallationStatus_BackfillsInstallationAndOwner: a connection
// stored with no installation id / owner gets both synced from GitHub on
// the next status check, and the gate reports installed=true.
func TestInstallationStatus_BackfillsInstallationAndOwner(t *testing.T) {
	fake := &fakeServiceConnections{conns: []*types.ServiceConnection{
		{ID: "sc-1", GitHubAppID: 99, GitHubInstallationID: 0, GitHubAppSlug: "my-app"},
	}}
	g := testIntegration(fake, func(context.Context, int64, string, string) ([]*github.Installation, error) {
		return []*github.Installation{{
			ID:      github.Int64(42),
			Account: &github.User{Login: github.String("acme")},
		}}, nil
	})

	st, err := g.InstallationStatus(context.Background(), "org-test")
	if err != nil {
		t.Fatalf("InstallationStatus: %v", err)
	}
	if !st.AppExists || !st.Installed {
		t.Fatalf("want appExists+installed, got %+v", st)
	}
	if len(fake.updated) != 1 || fake.updated[0].GitHubInstallationID != 42 || fake.updated[0].GitHubAppOwner != "acme" {
		t.Fatalf("expected install-id/owner backfill, updated=%+v", fake.updated)
	}
	// Install/manage URLs use the connection's own slug + synced owner.
	if st.InstallURL != "https://github.com/apps/my-app/installations/new" {
		t.Fatalf("InstallURL = %q", st.InstallURL)
	}
	if st.ManageURL != "https://github.com/organizations/acme/settings/apps/my-app" {
		t.Fatalf("ManageURL = %q", st.ManageURL)
	}
}

// TestInstallationStatus_DeletesStaleConnection: when GitHub reports the
// app no longer exists, the stored connection is deleted and the gate
// reverts to "not installed" (so the UI offers "Create the Helix app").
func TestInstallationStatus_DeletesStaleConnection(t *testing.T) {
	fake := &fakeServiceConnections{conns: []*types.ServiceConnection{
		{ID: "sc-stale", GitHubAppID: 7, GitHubInstallationID: 5},
	}}
	g := testIntegration(fake, func(context.Context, int64, string, string) ([]*github.Installation, error) {
		return nil, githubclient.ErrAppNotFound
	})

	st, err := g.InstallationStatus(context.Background(), "org-test")
	if err != nil {
		t.Fatalf("InstallationStatus: %v", err)
	}
	if len(fake.deleted) != 1 || fake.deleted[0] != "sc-stale" {
		t.Fatalf("expected stale connection delete, deleted=%v", fake.deleted)
	}
	if st.AppExists || st.Installed {
		t.Fatalf("stale app should not count as existing/installed, got %+v", st)
	}
}

// TestInstallationStatus_NoMutationWhenInSync: a connection already
// carrying the right installation id + owner is not re-persisted.
func TestInstallationStatus_NoMutationWhenInSync(t *testing.T) {
	fake := &fakeServiceConnections{conns: []*types.ServiceConnection{
		{ID: "sc-1", GitHubAppID: 99, GitHubInstallationID: 42, GitHubAppOwner: "acme", GitHubAppSlug: "my-app"},
	}}
	g := testIntegration(fake, func(context.Context, int64, string, string) ([]*github.Installation, error) {
		return []*github.Installation{{ID: github.Int64(42), Account: &github.User{Login: github.String("acme")}}}, nil
	})

	st, err := g.InstallationStatus(context.Background(), "org-test")
	if err != nil {
		t.Fatalf("InstallationStatus: %v", err)
	}
	if len(fake.updated) != 0 {
		t.Fatalf("no update expected when already in sync, updated=%+v", fake.updated)
	}
	if !st.Installed {
		t.Fatalf("want installed, got %+v", st)
	}
}
