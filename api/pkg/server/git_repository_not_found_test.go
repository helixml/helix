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
