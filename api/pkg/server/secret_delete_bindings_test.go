package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// fakeSecretStore is a minimal stateful store.Store: only the two methods
// the secret delete handler touches are real. Every other method is
// promoted from the (nil) embedded interface and must not be called.
type fakeSecretStore struct {
	store.Store
	secrets map[string]*types.Secret
}

func (f *fakeSecretStore) GetSecret(_ context.Context, id string) (*types.Secret, error) {
	s, ok := f.secrets[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return s, nil
}

func (f *fakeSecretStore) DeleteSecret(_ context.Context, id string) error {
	if _, ok := f.secrets[id]; !ok {
		return store.ErrNotFound
	}
	delete(f.secrets, id)
	return nil
}

// newBoundSecretServer wires a HelixAPIServer holding one project secret
// that is granted to one Agent through a real in-memory org store.
func newBoundSecretServer(t *testing.T) (*HelixAPIServer, *fakeSecretStore) {
	t.Helper()
	fake := &fakeSecretStore{secrets: map[string]*types.Secret{
		"sec-1": {ID: "sec-1", Name: "SMOKE_TOKEN", Owner: "user-1", ProjectID: "prj-1"},
	}}
	orgStore := orgmemory.New()
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	if err := orgStore.WorkerSecretBindings.Create(context.Background(), workersecret.Binding{
		OrganizationID: "org-1", WorkerID: orgchart.NodeID("w-1"), Name: "SMOKE_TOKEN",
		SourceKind: workersecret.SourceHelixSecret, SecretID: "sec-1",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed binding: %v", err)
	}
	s := &HelixAPIServer{Store: fake, helixOrg: &helixOrgHandlers{store: orgStore}}
	return s, fake
}

func deleteSecretRequest(t *testing.T, s *HelixAPIServer, query string) (*types.Secret, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/sec-1"+query, nil)
	req = mux.SetURLVars(req, map[string]string{"id": "sec-1"})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user-1"}))
	secret, herr := s.deleteSecret(httptest.NewRecorder(), req)
	if herr != nil {
		return nil, herr.StatusCode
	}
	return secret, http.StatusOK
}

// A secret granted to an Agent cannot be deleted by accident: the delete
// is refused with 409 and both the secret and the binding survive.
func TestDeleteSecret_GrantedToAgent_Refused(t *testing.T) {
	s, fake := newBoundSecretServer(t)

	_, status := deleteSecretRequest(t, s, "")

	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if _, ok := fake.secrets["sec-1"]; !ok {
		t.Fatal("secret was deleted despite the refusal")
	}
	if _, err := s.helixOrg.store.WorkerSecretBindings.Get(context.Background(), "org-1", "w-1", "SMOKE_TOKEN"); err != nil {
		t.Fatalf("binding was removed despite the refusal: %v", err)
	}
}

// force=true is the deliberate destroy route: the secret goes and its
// grants are revoked with it, so no binding is left pointing at a dead id.
func TestDeleteSecret_Force_RevokesGrants(t *testing.T) {
	s, fake := newBoundSecretServer(t)

	_, status := deleteSecretRequest(t, s, "?force=true")

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if _, ok := fake.secrets["sec-1"]; ok {
		t.Fatal("secret survived a forced delete")
	}
	if _, err := s.helixOrg.store.WorkerSecretBindings.Get(context.Background(), "org-1", "w-1", "SMOKE_TOKEN"); err == nil {
		t.Fatal("binding survived a forced delete, orphaned at a dead secret id")
	}
}
