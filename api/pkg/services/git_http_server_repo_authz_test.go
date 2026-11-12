package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func gitServerWithRepo(t *testing.T, st *store.MockStore, repo *types.GitRepository) *GitHTTPServer {
	t.Helper()
	st.EXPECT().GetGitRepository(gomock.Any(), repo.ID).Return(repo, nil).AnyTimes()
	return &GitHTTPServer{
		store:          st,
		gitRepoService: &GitRepositoryService{store: st},
		enablePush:     true,
		authorizeFn: func(_ context.Context, user *types.User, repo *types.GitRepository, _ types.Action) error {
			if user.ID == repo.OwnerID {
				return nil
			}
			return errors.New("denied")
		},
	}
}

func gitReqAs(method, target, repoID string, user *types.User) *http.Request {
	r := gitReq(method, target, repoID)
	return r.WithContext(context.WithValue(r.Context(), "user", user))
}

// /clone-info and /status answer only for callers who may read the repository.
func TestGitStatusAndCloneInfoRequireReadAccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	repo := &types.GitRepository{ID: "repo_1", Name: "private", OwnerID: "owner", LocalPath: t.TempDir()}
	s := gitServerWithRepo(t, st, repo)
	stranger := &types.User{ID: "stranger"}
	owner := &types.User{ID: "owner"}

	for _, tc := range []struct {
		path    string
		handler http.HandlerFunc
	}{
		{"/git/repo_1/status", s.handleRepositoryStatus},
		{"/git/repo_1/clone-info", s.handleCloneInfo},
	} {
		rec := httptest.NewRecorder()
		tc.handler(rec, gitReqAs(http.MethodGet, tc.path, "repo_1", stranger))
		assert.Equal(t, http.StatusNotFound, rec.Code, "%s for a stranger", tc.path)

		rec = httptest.NewRecorder()
		tc.handler(rec, gitReqAs(http.MethodGet, tc.path, "repo_1", owner))
		assert.Equal(t, http.StatusOK, rec.Code, "%s for the owner", tc.path)
	}
}

// If the key record cannot be read the push is refused: treating the error as
// "no restriction" would let a restricted agent key push any ref.
func TestReceivePackFailsClosedWhenKeyLookupErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	repo := &types.GitRepository{ID: "repo_1", OwnerID: "owner", LocalPath: t.TempDir()}
	s := gitServerWithRepo(t, st, repo)
	s.authTokenHeader = "X-API-Key"
	st.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(nil, errors.New("connection reset"))

	r := gitReqAs(http.MethodPost, "/git/repo_1/git-receive-pack", "repo_1", &types.User{ID: "owner"})
	r.Header.Set("Content-Type", "application/x-git-receive-pack-request")
	r.Header.Set("X-API-Key", "hl-agent")
	rec := httptest.NewRecorder()

	s.handleReceivePack(rec, r)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
