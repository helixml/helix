package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type repoWriteFixture struct {
	server *HelixAPIServer
	user   *types.User
	bare   string
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Seed", "GIT_AUTHOR_EMAIL=seed@example.com",
		"GIT_COMMITTER_NAME=Seed", "GIT_COMMITTER_EMAIL=seed@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// newRepoWriteFixture serves a repository owned by the caller whose master
// branch holds cfg -> .git/config and d -> .git.
func newRepoWriteFixture(t *testing.T) *repoWriteFixture {
	t.Helper()
	dir := t.TempDir()
	bare := filepath.Join(dir, "repo.git")
	work := filepath.Join(dir, "seed")
	runGit(t, dir, "init", "-q", "--bare", "-b", "master", bare)
	runGit(t, dir, "init", "-q", "-b", "master", work)
	require.NoError(t, os.WriteFile(filepath.Join(work, "README.md"), []byte("# repo\n"), 0o644))
	require.NoError(t, os.Symlink(".git/config", filepath.Join(work, "cfg")))
	require.NoError(t, os.Symlink(".git", filepath.Join(work, "d")))
	runGit(t, work, "add", "-A")
	runGit(t, work, "commit", "-q", "-m", "seed")
	runGit(t, work, "push", "-q", bare, "master:master")

	user := &types.User{ID: "usr_alice", Email: "alice@example.com", FullName: "Alice"}
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo_1").Return(&types.GitRepository{
		ID: "repo_1", OwnerID: user.ID, LocalPath: bare, DefaultBranch: "master",
	}, nil).AnyTimes()
	return &repoWriteFixture{
		server: &HelixAPIServer{
			Store:                mockStore,
			gitRepositoryService: services.NewGitRepositoryService(mockStore, dir, "http://helix.test", "Helix", "helix@example.com"),
		},
		user: user,
		bare: bare,
	}
}

func (f *repoWriteFixture) call(t *testing.T, handler http.HandlerFunc, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, reader)
	req = req.WithContext(setTestRequestUser(req.Context(), f.user))
	req = mux.SetURLVars(req, map[string]string{"id": "repo_1"})
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

func (f *repoWriteFixture) refs(t *testing.T) string {
	return runGit(t, f.bare, "for-each-ref", "--format=%(refname) %(objectname)")
}

var badBranchNames = []string{"-x", "--upload-pack=touch /tmp/x", "a..b", "*", "a b", "x:y", "a,b\x01"}

func TestRepositoryBranchQueryRejectsInvalidNames(t *testing.T) {
	f := newRepoWriteFixture(t)
	handlers := map[string]http.HandlerFunc{
		"pull":      f.server.pullFromRemote,
		"push":      f.server.pushToRemote,
		"push-pull": f.server.pushPullGitRepository,
	}
	for name, handler := range handlers {
		for _, branch := range badBranchNames {
			rr := f.call(t, handler, http.MethodPost, "/api/v1/git/repositories/repo_1/"+name+"?branch="+url.QueryEscape(branch), nil)
			assert.Equal(t, http.StatusBadRequest, rr.Code, "%s %q: %s", name, branch, rr.Body.String())
		}
	}
}

func TestCreateBranchRejectsInvalidNames(t *testing.T) {
	f := newRepoWriteFixture(t)
	before := f.refs(t)
	for _, branch := range badBranchNames {
		rr := f.call(t, f.server.createGitRepositoryBranch, http.MethodPost, "/api/v1/git/repositories/repo_1/branches",
			types.CreateBranchRequest{BranchName: branch, BaseBranch: "master"})
		assert.Equal(t, http.StatusBadRequest, rr.Code, "%q: %s", branch, rr.Body.String())
	}
	assert.Equal(t, before, f.refs(t))
}

func TestFileContentsWriteRejectsInvalidBranchAndSymlinkPaths(t *testing.T) {
	f := newRepoWriteFixture(t)
	before := f.refs(t)
	put := func(path, branch string) *httptest.ResponseRecorder {
		return f.call(t, f.server.createOrUpdateGitRepositoryFileContents, http.MethodPut, "/api/v1/git/repositories/repo_1/contents",
			types.UpdateGitRepositoryFileContentsRequest{Path: path, Branch: branch, Content: "[core]\n\tfsmonitor = touch /tmp/pwned\n"})
	}

	for _, branch := range badBranchNames {
		rr := put("x.md", branch)
		assert.Equal(t, http.StatusBadRequest, rr.Code, "%q: %s", branch, rr.Body.String())
	}
	for _, path := range []string{"cfg", "d/config"} {
		rr := put(path, "master")
		assert.Equal(t, http.StatusBadRequest, rr.Code, "%q: %s", path, rr.Body.String())
	}
	assert.Equal(t, before, f.refs(t))

	rr := put("docs/ok.md", "master")
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp types.GitRepositoryFileResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, runGit(t, f.bare, "rev-parse", "refs/heads/master"), resp.Content)
}
