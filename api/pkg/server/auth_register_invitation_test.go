package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func newRegisterTestServer(t *testing.T) (*HelixAPIServer, *store.MockStore) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	authn := auth.NewMockAuthenticator(ctrl)

	cfg := &config.ServerConfig{}
	cfg.Auth.Provider = types.AuthProviderRegular
	cfg.Auth.RegistrationEnabled = true

	st.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
	authn.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, u *types.User) (*types.User, error) {
		return u, nil
	})
	authn.EXPECT().GenerateUserToken(gomock.Any(), gomock.Any()).Return("token", nil)
	return &HelixAPIServer{Cfg: cfg, Store: st, authenticator: authn}, st
}

func register(t *testing.T, s *HelixAPIServer, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.register(rr, req)
	return rr
}

// Password sign-up does not prove ownership of the email address, so pending
// invitations (which may grant an owner role) must not be accepted by email
// alone. The mock store fails the test on any unexpected call, including
// ConsumePendingInvitations.
func TestRegister_DoesNotAcceptInvitationsByEmail(t *testing.T) {
	s, _ := newRegisterTestServer(t)
	rr := register(t, s, `{"email":"invited@example.com","password":"pw","password_confirm":"pw"}`)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
}

// The invite link carries the invitation ID: only that invitation is
// accepted (the store checks it was sent to the registering address).
func TestRegister_AcceptsTheInvitationFromTheLink(t *testing.T) {
	s, st := newRegisterTestServer(t)
	st.EXPECT().ConsumeInvitation(gomock.Any(), gomock.Any(), "oin_from_link").
		DoAndReturn(func(_ any, u *types.User, _ string) ([]*types.OrganizationMembership, error) {
			require.Equal(t, "invited@example.com", u.Email)
			return []*types.OrganizationMembership{{OrganizationID: "org_1", UserID: u.ID}}, nil
		})
	st.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, u *types.User) (*types.User, error) {
		require.True(t, u.OnboardingCompleted)
		require.False(t, u.Waitlisted)
		return u, nil
	})

	rr := register(t, s, `{"email":"invited@example.com","password":"pw","password_confirm":"pw","invitation_id":"oin_from_link"}`)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
}
