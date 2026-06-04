// Tests for newGitHubOAuthResolver — the function that resolves
// the operator's GitHub access token for the helix-org github
// streams transport. Production wiring rebuilds this function with
// real store + oauth manager; we feed it a stub store so the test
// runs offline and pins the priority order.
//
// Pins two related regressions:
//
//  1. Newest-first across providers. The in-memory oauth.Manager's
//     `GetProviderByType` iterates a map in random order and returns
//     an arbitrary provider; when an operator added a second github
//     provider (e.g. swapping to an org-owned OAuth app for
//     helixml-org access), the resolver was sometimes using the
//     OLDER provider's ID to look up connections — finding none, and
//     returning "" even though a connection existed on the new
//     provider. Fix: iterate every github provider via
//     ListOAuthProviders, sorted by created_at DESC, and return the
//     first non-empty token found.
//
//  2. Most-recently-updated connection wins (sibling fix in
//     store_oauth.go's GetOAuthConnectionByUserAndProvider). Tested
//     in pkg/store tests; here we just pin the resolver's NEWEST
//     provider preference.
package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/oauth"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// stubOAuthStore is a minimal helixstore.Store that returns
// pre-canned data for the three methods newGitHubOAuthResolver
// calls. Any other method panics — that's what the embedded nil
// Store interface gives us (clear stack trace if a test
// accidentally exercises an untested code path).
type stubOAuthStore struct {
	helixstore.Store
	mu          sync.Mutex
	providers   []*types.OAuthProvider
	memberships []*types.OrganizationMembership
	connections map[string]*types.OAuthConnection // key: userID + "::" + providerID
}

func (s *stubOAuthStore) ListOAuthProviders(_ context.Context, _ *helixstore.ListOAuthProvidersQuery) ([]*types.OAuthProvider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*types.OAuthProvider, len(s.providers))
	copy(out, s.providers)
	return out, nil
}

func (s *stubOAuthStore) ListOrganizationMemberships(_ context.Context, _ *helixstore.ListOrganizationMembershipsQuery) ([]*types.OrganizationMembership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*types.OrganizationMembership, len(s.memberships))
	copy(out, s.memberships)
	return out, nil
}

func (s *stubOAuthStore) GetOAuthConnectionByUserAndProvider(_ context.Context, userID, providerID string) (*types.OAuthConnection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := userID + "::" + providerID
	if conn, ok := s.connections[key]; ok {
		return conn, nil
	}
	return nil, helixstore.ErrNotFound
}

// TestGitHubOAuthResolverPicksNewestProviderToken pins the
// regression. With TWO github providers in the store — an OLD
// provider (no working connection) and a NEW provider (with a
// fresh connection from an org-owned OAuth app) — the resolver
// must return the NEW provider's token, not "" because it
// happened to look at the old provider first.
func TestGitHubOAuthResolverPicksNewestProviderToken(t *testing.T) {
	t.Parallel()

	oldProvider := &types.OAuthProvider{
		ID:        "prov-old",
		Type:      types.OAuthProviderTypeGitHub,
		Enabled:   true,
		CreatedAt: time.Now().Add(-1 * time.Hour),
	}
	newProvider := &types.OAuthProvider{
		ID:        "prov-new",
		Type:      types.OAuthProviderTypeGitHub,
		Enabled:   true,
		CreatedAt: time.Now(),
	}
	store := &stubOAuthStore{
		providers:   []*types.OAuthProvider{oldProvider, newProvider}, // intentionally OLD first
		memberships: []*types.OrganizationMembership{{UserID: "usr-1", OrganizationID: "org-test"}},
		connections: map[string]*types.OAuthConnection{
			// Only the NEW provider has a connection — the old one
			// is a stale row from a previous setup with no live
			// token.
			"usr-1::prov-new": {AccessToken: "gho_newprovider_token"},
		},
	}

	// oauth.Manager is required-non-nil by newGitHubOAuthResolver,
	// even though the resolver no longer consults it directly. A
	// zero-provider manager is fine.
	mgr := oauth.NewManager(store, false)
	resolver := newGitHubOAuthResolver(mgr, store)
	if resolver == nil {
		t.Fatal("expected resolver, got nil")
	}

	got, err := resolver(context.Background(), "org-test")
	if err != nil {
		t.Fatalf("resolver err: %v", err)
	}
	if got != "gho_newprovider_token" {
		t.Errorf("token = %q, want %q (newest provider should win)", got, "gho_newprovider_token")
	}
}

// TestGitHubOAuthResolverNoProvidersReturnsEmpty pins the
// "operator hasn't connected GitHub yet" path: no github providers
// in the store → resolver returns ("", nil). NOT an error — the
// transport's downstream consumers degrade gracefully.
func TestGitHubOAuthResolverNoProvidersReturnsEmpty(t *testing.T) {
	t.Parallel()
	store := &stubOAuthStore{}
	resolver := newGitHubOAuthResolver(oauth.NewManager(store, false), store)
	got, err := resolver(context.Background(), "org-test")
	if err != nil {
		t.Fatalf("resolver err: %v", err)
	}
	if got != "" {
		t.Errorf("token = %q, want empty", got)
	}
}

// TestGitHubOAuthResolverFallsThroughToOlderProviderIfNewerHasNoConnection
// pins the "try every provider" behaviour. If the NEWEST provider
// has no live connection for any org member, the resolver should
// fall through to the next-newest provider rather than giving up.
func TestGitHubOAuthResolverFallsThroughToOlderProviderIfNewerHasNoConnection(t *testing.T) {
	t.Parallel()
	store := &stubOAuthStore{
		providers: []*types.OAuthProvider{
			{ID: "prov-new", Type: types.OAuthProviderTypeGitHub, Enabled: true, CreatedAt: time.Now()},
			{ID: "prov-old", Type: types.OAuthProviderTypeGitHub, Enabled: true, CreatedAt: time.Now().Add(-1 * time.Hour)},
		},
		memberships: []*types.OrganizationMembership{{UserID: "usr-1", OrganizationID: "org-test"}},
		connections: map[string]*types.OAuthConnection{
			// The NEW provider has no connection; the OLD one
			// still has a usable token.
			"usr-1::prov-old": {AccessToken: "gho_oldprovider_token"},
		},
	}
	resolver := newGitHubOAuthResolver(oauth.NewManager(store, false), store)
	got, err := resolver(context.Background(), "org-test")
	if err != nil {
		t.Fatalf("resolver err: %v", err)
	}
	if got != "gho_oldprovider_token" {
		t.Errorf("token = %q, want fallback to old provider", got)
	}
}

// TestGitHubOAuthResolverNilDepsReturnsNil pins the wiring opt-out:
// production code passes nil resolver when manager or store is nil,
// and api.Deps treats nil GitHubTokenResolver as "no fallback".
func TestGitHubOAuthResolverNilDepsReturnsNil(t *testing.T) {
	t.Parallel()
	if newGitHubOAuthResolver(nil, &stubOAuthStore{}) != nil {
		t.Error("expected nil resolver when manager is nil")
	}
	if newGitHubOAuthResolver(oauth.NewManager(&stubOAuthStore{}, false), nil) != nil {
		t.Error("expected nil resolver when store is nil")
	}
}

// TestGitHubOAuthResolverPropagatesStoreErrors pins the error
// path: a real DB error (not ErrNotFound) on connection lookup
// surfaces as a resolver error so the calling handler returns
// 500 instead of silently falling back to "".
type errOnConnLookupStore struct {
	stubOAuthStore
	err error
}

func (s *errOnConnLookupStore) GetOAuthConnectionByUserAndProvider(_ context.Context, _, _ string) (*types.OAuthConnection, error) {
	return nil, s.err
}

func TestGitHubOAuthResolverPropagatesStoreErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("db is on fire")
	store := &errOnConnLookupStore{
		stubOAuthStore: stubOAuthStore{
			providers:   []*types.OAuthProvider{{ID: "prov-1", Type: types.OAuthProviderTypeGitHub, Enabled: true, CreatedAt: time.Now()}},
			memberships: []*types.OrganizationMembership{{UserID: "usr-1", OrganizationID: "org-test"}},
		},
		err: boom,
	}
	resolver := newGitHubOAuthResolver(oauth.NewManager(store, false), store)
	_, err := resolver(context.Background(), "org-test")
	if err == nil || !errors.Is(err, boom) {
		t.Errorf("err = %v, want wrapping %v", err, boom)
	}
}
