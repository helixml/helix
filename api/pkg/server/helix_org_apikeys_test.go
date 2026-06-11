package server

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

	k := newHelixAPIKeys(st, newTestConfigs(t))
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

	k := newHelixAPIKeys(st, newTestConfigs(t))
	got, err := k.User(context.Background(), "usr-2")
	if err != nil {
		t.Fatalf("User: %v", err)
	}
	if got == "" {
		t.Fatal("expected a minted key")
	}
}

// TestAPIKeys_Service_MintsAndGrantsFlag: first-run service provisioning
// picks the admin, grants the alpha flag, mints a key, and caches it.
func TestAPIKeys_Service_MintsAndGrantsFlag(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	st := store.NewMockStore(ctrl)

	admin := &types.User{ID: "usr-admin", Email: "admin@test", Admin: true}
	st.EXPECT().ListUsers(gomock.Any(), &store.ListUsersQuery{Admin: true}).
		Return([]*types.User{admin}, int64(1), nil)
	st.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, u *types.User) (*types.User, error) {
			var granted bool
			for _, f := range u.AlphaFeatures {
				if f == alphaFeatureHelixOrg {
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
	k := newHelixAPIKeys(st, reg)
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

// TestAPIKeys_Service_NoAdmin: provisioning fails clearly when no admin
// exists.
func TestAPIKeys_Service_NoAdmin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	st := store.NewMockStore(ctrl)
	st.EXPECT().ListUsers(gomock.Any(), gomock.Any()).Return([]*types.User{}, int64(0), nil)

	k := newHelixAPIKeys(st, newTestConfigs(t))
	if _, err := k.Service(context.Background(), "org-test"); err == nil {
		t.Fatal("expected error when no admin user exists")
	}
}
