package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func orgRequest(method, ref string, user types.User, body string) *http.Request {
	req := httptest.NewRequest(method, "/api/v1/organizations/"+ref, strings.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": ref})
	return req.WithContext(setRequestUser(req.Context(), user))
}

// GET /organizations/{id}: an org that exists but that the caller cannot see
// (no membership, or an org API key scoped to a different org) answers exactly
// like an org that does not exist, and no response carries store or scope text.
func TestGetOrganization_UniformNotFound(t *testing.T) {
	const bodyNotFound = "Organization not found\n"

	cases := []struct {
		name   string
		ref    string
		user   types.User
		expect func(m *store.MockStore)
		code   int
		body   string
	}{
		{
			name: "unknown org",
			ref:  "org_does_not_exist",
			user: types.User{ID: "usr_a"},
			expect: func(m *store.MockStore) {
				m.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
			},
			code: http.StatusNotFound,
			body: bodyNotFound,
		},
		{
			name: "existing org, caller is not a member",
			ref:  "org_victim",
			user: types.User{ID: "usr_a"},
			expect: func(m *store.MockStore) {
				m.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(&types.Organization{ID: "org_victim"}, nil)
				m.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
			},
			code: http.StatusNotFound,
			body: bodyNotFound,
		},
		{
			name: "existing org, org key scoped to another org",
			ref:  "org_victim",
			user: types.User{ID: "usr_a", OrganizationID: "org_attacker"},
			expect: func(m *store.MockStore) {
				m.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(&types.Organization{ID: "org_victim"}, nil)
			},
			code: http.StatusNotFound,
			body: bodyNotFound,
		},
		{
			name: "membership query fails",
			ref:  "org_mine",
			user: types.User{ID: "usr_a"},
			expect: func(m *store.MockStore) {
				m.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(&types.Organization{ID: "org_mine"}, nil)
				m.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(nil, errors.New("pq: connection refused"))
			},
			code: http.StatusInternalServerError,
			body: "Could not check org membership\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			m := store.NewMockStore(ctrl)
			tc.expect(m)
			srv := &HelixAPIServer{Store: m, Cfg: &config.ServerConfig{}}

			rr := httptest.NewRecorder()
			srv.getOrganization(rr, orgRequest(http.MethodGet, tc.ref, tc.user, ""))

			require.Equal(t, tc.code, rr.Code)
			require.Equal(t, tc.body, rr.Body.String())
		})
	}
}

// Update and delete answer a key scoped to another org with the same 404 as
// an unknown org, and change nothing (the mock store would fail the test on
// any write).
func TestUpdateDeleteOrganization_ForeignScopedKeyGets404(t *testing.T) {
	attacker := types.User{ID: "usr_a", OrganizationID: "org_attacker"}

	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			m := store.NewMockStore(ctrl)
			m.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(&types.Organization{ID: "org_victim"}, nil)
			srv := &HelixAPIServer{Store: m, Cfg: &config.ServerConfig{}}

			rr := httptest.NewRecorder()
			req := orgRequest(method, "org_victim", attacker, `{"display_name":"pwned"}`)
			if method == http.MethodPut {
				srv.updateOrganization(rr, req)
			} else {
				srv.deleteOrganization(rr, req)
			}

			require.Equal(t, http.StatusNotFound, rr.Code)
			require.Equal(t, "Organization not found\n", rr.Body.String())
		})
	}
}
