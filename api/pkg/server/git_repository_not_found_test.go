package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// fixedRepoService answers GetRepository with err.
type fixedRepoService struct {
	gitRepositoryServicer
	err error
}

func (f *fixedRepoService) GetRepository(context.Context, string) (*types.GitRepository, error) {
	return nil, f.err
}

// A repository 404 does not reflect the requested id, and a store failure is a
// 500 without the database text.
func TestGetGitRepository_ErrorDoesNotEchoInput(t *testing.T) {
	const probe = "code-<script>alert(1)</script>"
	cases := []struct {
		err  error
		code int
		body string
	}{
		{errors.New("repository " + probe + " not found: " + store.ErrNotFound.Error()), http.StatusInternalServerError, "failed to load repository\n"},
		{wrapNotFound(probe), http.StatusNotFound, "repository not found\n"},
	}
	for _, tc := range cases {
		srv := &HelixAPIServer{gitRepositoryService: &fixedRepoService{err: tc.err}}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/git/repositories/x", nil)
		req = mux.SetURLVars(req, map[string]string{"id": probe})
		req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_a"}))
		rr := httptest.NewRecorder()
		srv.getGitRepository(rr, req)
		require.Equal(t, tc.code, rr.Code)
		require.Equal(t, tc.body, rr.Body.String())
	}
}

func wrapNotFound(repoID string) error {
	// Mirrors GitRepositoryService.GetRepository's wrapping.
	return errors.Join(errors.New("repository "+repoID+" not found"), store.ErrNotFound)
}

// A repository in another tenant answers the same 404 as a missing one, and a
// refusal never carries the internal authorization error text. An org member
// without a grant still gets 403.
func TestGetGitRepository_ForeignRepositoryGets404(t *testing.T) {
	cases := []struct {
		name  string
		repo  *types.GitRepository
		user  types.User
		setup func(m *store.MockStore)
		code  int
		body  string
	}{
		{"another user's personal repository", &types.GitRepository{ID: "r1", OwnerID: "usr_b"}, types.User{ID: "usr_a"},
			func(*store.MockStore) {}, http.StatusNotFound, "repository not found\n"},
		{"org the caller is not in", &types.GitRepository{ID: "r1", OrganizationID: "org_b"}, types.User{ID: "usr_a"},
			func(m *store.MockStore) {
				m.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
			}, http.StatusNotFound, "repository not found\n"},
		{"key scoped to another org", &types.GitRepository{ID: "r1", OrganizationID: "org_b"}, types.User{ID: "usr_a", OrganizationID: "org_a"},
			func(*store.MockStore) {}, http.StatusNotFound, "repository not found\n"},
		{"member without a grant", &types.GitRepository{ID: "r1", OrganizationID: "org_b", OwnerID: "usr_owner"}, types.User{ID: "usr_a"},
			func(m *store.MockStore) {
				m.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
					Return(&types.OrganizationMembership{OrganizationID: "org_b", UserID: "usr_a", Role: types.OrganizationRoleMember}, nil).AnyTimes()
				m.EXPECT().ListTeams(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				m.EXPECT().ListAccessGrants(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				m.EXPECT().ListProjects(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			}, http.StatusForbidden, "not authorized to access this repository\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := store.NewMockStore(gomock.NewController(t))
			tc.setup(m)
			srv := &HelixAPIServer{Store: m, gitRepositoryService: &fixedRepo{repo: tc.repo}}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/git/repositories/r1", nil)
			req = mux.SetURLVars(req, map[string]string{"id": "r1"})
			req = req.WithContext(setRequestUser(req.Context(), tc.user))
			rr := httptest.NewRecorder()
			srv.getGitRepository(rr, req)
			require.Equal(t, tc.code, rr.Code)
			require.Equal(t, tc.body, rr.Body.String())
		})
	}
}

type fixedRepo struct {
	gitRepositoryServicer
	repo *types.GitRepository
}

func (f *fixedRepo) GetRepository(context.Context, string) (*types.GitRepository, error) {
	return f.repo, nil
}
