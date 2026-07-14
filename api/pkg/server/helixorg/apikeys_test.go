package helixorg

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// newTestConfigs returns a real config registry over an in-memory org
// configs store, with the helix.api_key spec registered so Service can
// cache the minted key.
func newTestConfigs(t *testing.T) *configregistry.Registry {
	t.Helper()
	st := orgmemory.New()
	reg := configregistry.New(st.Configs)
	reg.Register(configregistry.Spec{Key: "helix.api_key", Type: configregistry.TypeString, Description: "test"})
	return reg
}

// TestAPIKeys_User_ReturnsExisting: a user that already has an api key
// gets it back, with no new key minted.
func TestAPIKeys_User_ReturnsExisting(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	st := store.NewMockStore(ctrl)

	st.EXPECT().ListAPIKeys(gomock.Any(), &store.ListAPIKeysQuery{Owner: "usr-1", Type: types.APIkeytypeAPI}).
		Return([]*types.ApiKey{{Key: "hl-existing"}}, nil)
	// No CreateAPIKey expected.

	k := NewHelixAPIKeys(st, newTestConfigs(t))
	got, err := k.User(context.Background(), "usr-1")
	if err != nil {
		t.Fatalf("User: %v", err)
	}
	if got != "hl-existing" {
		t.Fatalf("got %q, want hl-existing", got)
	}
}

// TestAPIKeys_User_MintsWhenNone: a user with no key gets one minted and
// persisted.
func TestAPIKeys_User_MintsWhenNone(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	st := store.NewMockStore(ctrl)

	st.EXPECT().ListAPIKeys(gomock.Any(), gomock.Any()).Return([]*types.ApiKey{}, nil)
	st.EXPECT().CreateAPIKey(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.ApiKey) (*types.ApiKey, error) {
			if k.Owner != "usr-2" || k.Type != types.APIkeytypeAPI {
				t.Errorf("unexpected key: %+v", k)
			}
			return k, nil
		})

	k := NewHelixAPIKeys(st, newTestConfigs(t))
	got, err := k.User(context.Background(), "usr-2")
	if err != nil {
		t.Fatalf("User: %v", err)
	}
	if got == "" {
		t.Fatal("expected a minted key")
	}
}

// TestAPIKeys_Service_MintsForOrganizationOwner verifies service
// provisioning uses the organization owner without requiring admin access.
func TestAPIKeys_Service_MintsForOrganizationOwner(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	st := store.NewMockStore(ctrl)

	owner := &types.User{ID: "usr-owner", Email: "owner@test"}
	st.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org-test"}).
		Return(&types.Organization{ID: "org-test", Owner: owner.ID}, nil)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: owner.ID}).Return(owner, nil)
	st.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, u *types.User) (*types.User, error) {
			var granted bool
			for _, f := range u.AlphaFeatures {
				if f == AlphaFeature {
					granted = true
				}
			}
			if !granted {
				t.Errorf("alpha flag not granted: %+v", u.AlphaFeatures)
			}
			return u, nil
		})
	st.EXPECT().CreateAPIKey(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.ApiKey) (*types.ApiKey, error) { return k, nil })

	reg := newTestConfigs(t)
	k := NewHelixAPIKeys(st, reg)
	got, err := k.Service(context.Background(), "org-test")
	if err != nil {
		t.Fatalf("Service: %v", err)
	}
	if got == "" {
		t.Fatal("expected a minted service key")
	}
	// Cached in config for next time.
	if v, _ := reg.GetString(context.Background(), "org-test", "helix.api_key"); v != got {
		t.Fatalf("service key not cached: config=%q minted=%q", v, got)
	}
}

func TestAPIKeys_Service_NoOrganizationOwner(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org-test"}).
		Return(&types.Organization{ID: "org-test"}, nil)

	k := NewHelixAPIKeys(st, newTestConfigs(t))
	if _, err := k.Service(context.Background(), "org-test"); err == nil {
		t.Fatal("expected error when the organization has no owner")
	}
}
