package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// An agent created with an org-scoped key and no organization_id belongs to
// the key's organization. It used to be created as a personal agent of the
// key's owner: invisible to every org listing and unmanageable with the key.
func TestCreateAgent_OrgScopedKeyDefaultsOrganization(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := store.NewMockStore(ctrl)
	m.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_key"}).
		Return(&types.Organization{ID: "org_key"}, nil)
	m.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(&types.OrganizationMembership{OrganizationID: "org_key", UserID: "usr_bob", Role: types.OrganizationRoleMember}, nil)
	m.EXPECT().ListApps(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	var created *types.App
	m.EXPECT().CreateApp(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, app *types.App) (*types.App, error) {
		created = app
		return nil, errors.New("stop after capture")
	})

	srv := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: m}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents",
		strings.NewReader(`{"config":{"helix":{"name":"scoped","assistants":[{"name":"a"}]}}}`))
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "usr_bob", OrganizationID: "org_key"}))

	_, _ = srv.createAgent(httptest.NewRecorder(), req)

	require.NotNil(t, created, "the agent must reach the store")
	require.Equal(t, "org_key", created.OrganizationID)
}

// The key still cannot create an agent in an org it is not scoped to.
func TestCreateAgent_OrgScopedKeyCannotTargetAnotherOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := store.NewMockStore(ctrl)
	m.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(&types.Organization{ID: "org_other"}, nil)
	m.EXPECT().CreateApp(gomock.Any(), gomock.Any()).Times(0)

	srv := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: m}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents",
		strings.NewReader(`{"organization_id":"org_other","config":{"helix":{"name":"x","assistants":[{"name":"a"}]}}}`))
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "usr_bob", OrganizationID: "org_key"}))

	_, httpErr := srv.createAgent(httptest.NewRecorder(), req)
	require.NotNil(t, httpErr)
	require.Equal(t, http.StatusForbidden, httpErr.StatusCode)
}
