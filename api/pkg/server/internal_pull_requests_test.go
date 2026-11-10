package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/suite"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// prStore is the slice of store.Store the internal pull request flow touches.
type prStore struct {
	store.Store
	mu    sync.Mutex
	repos map[string]*types.GitRepository
	users map[string]*types.User
	prs   []*types.GitPullRequest
}

func (s *prStore) GetGitRepository(_ context.Context, id string) (*types.GitRepository, error) {
	if r, ok := s.repos[id]; ok {
		c := *r
		return &c, nil
	}
	return nil, store.ErrNotFound
}

func (s *prStore) ListGitRepositories(_ context.Context, q *types.ListGitRepositoriesRequest) ([]*types.GitRepository, error) {
	var out []*types.GitRepository
	for _, r := range s.repos {
		if (q.OwnerID == "" || r.OwnerID == q.OwnerID) && (q.OrganizationID == "" || r.OrganizationID == q.OrganizationID) {
			c := *r
			out = append(out, &c)
		}
	}
	return out, nil
}

func (s *prStore) GetUser(_ context.Context, q *store.GetUserQuery) (*types.User, error) {
	if u, ok := s.users[q.ID]; ok {
		return u, nil
	}
	return nil, store.ErrNotFound
}

func (s *prStore) CreateGitPullRequest(_ context.Context, pr *types.GitPullRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, p := range s.prs {
		if p.RepositoryID == pr.RepositoryID && p.Number > n {
			n = p.Number
		}
	}
	pr.Number = n + 1
	c := *pr
	s.prs = append(s.prs, &c)
	return nil
}

func (s *prStore) GetGitPullRequest(_ context.Context, repoID string, number int) (*types.GitPullRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.prs {
		if p.RepositoryID == repoID && p.Number == number {
			c := *p
			return &c, nil
		}
	}
	return nil, store.ErrNotFound
}

func (s *prStore) ListGitPullRequests(_ context.Context, repoID, state string) ([]*types.GitPullRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*types.GitPullRequest
	for _, p := range s.prs {
		if p.RepositoryID == repoID && (state == "" || string(p.State) == state) {
			c := *p
			out = append(out, &c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number > out[j].Number })
	return out, nil
}

func (s *prStore) UpdateGitPullRequest(_ context.Context, pr *types.GitPullRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.prs {
		if p.ID == pr.ID {
			c := *pr
			s.prs[i] = &c
			return nil
		}
	}
	return store.ErrNotFound
}

type InternalPullRequestSuite struct {
	suite.Suite
	ctx      context.Context
	tmp      string
	work     string
	store    *prStore
	git      *services.GitRepositoryService
	server   *HelixAPIServer
	repo     *types.GitRepository
	deployed []string
}

func TestInternalPullRequestSuite(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	suite.Run(t, new(InternalPullRequestSuite))
}

func (s *InternalPullRequestSuite) SetupTest() {
	s.ctx = context.Background()
	s.tmp = s.T().TempDir()
	s.store = &prStore{
		repos: map[string]*types.GitRepository{},
		users: map[string]*types.User{
			"usr_owner": {ID: "usr_owner", Email: "owner@example.com", FullName: "Owner"},
			"usr_other": {ID: "usr_other", Email: "other@example.com"},
		},
	}
	s.git = services.NewGitRepositoryService(s.store, s.tmp, "http://helix.test", "Helix", "helix@example.com")
	s.Require().NoError(s.git.Initialize(s.ctx))

	bare := filepath.Join(s.tmp, "module.git")
	s.run("", "git", "init", "-q", "--bare", "-b", "main", bare)
	s.work = filepath.Join(s.tmp, "work")
	s.run("", "git", "clone", "-q", bare, s.work)
	s.run(s.work, "git", "checkout", "-q", "-b", "main")
	s.commit("README.md", "hello\n", "init")
	s.run(s.work, "git", "push", "-q", "origin", "main")

	s.repo = &types.GitRepository{ID: "repo_1", Name: "module", OwnerID: "usr_owner", LocalPath: bare, DefaultBranch: "main"}
	s.store.repos[s.repo.ID] = s.repo

	httpServer := services.NewGitHTTPServer(s.store, s.git, services.GitHTTPServerConfig{}, nil, nil)
	httpServer.SetOnDefaultBranchPush(func(_ context.Context, repoID, branch, _ string) {
		s.store.mu.Lock()
		s.deployed = append(s.deployed, repoID+":"+branch)
		s.store.mu.Unlock()
	})
	s.server = &HelixAPIServer{
		Store:         s.store,
		gitHTTPServer: httpServer,
		Cfg:           &config.ServerConfig{WebServer: config.WebServer{URL: "http://helix.test"}},
	}
	s.server.registerGitHubCompatRoutes(mux.NewRouter())
	s.git.SetInternalGitHubCompat("http://helix.test", s.server.newInternalCompatDispatch())
}

func (s *InternalPullRequestSuite) run(dir string, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	s.Require().NoError(err, "%s %v: %s", name, args, out)
	return strings.TrimSpace(string(out))
}

func (s *InternalPullRequestSuite) commit(file, content, msg string) {
	s.Require().NoError(os.WriteFile(filepath.Join(s.work, file), []byte(content), 0o644))
	s.run(s.work, "git", "add", file)
	s.run(s.work, "git", "commit", "-q", "-m", msg)
}

// branch creates branch from main with one commit and pushes it.
func (s *InternalPullRequestSuite) branch(name, file, content string) {
	s.run(s.work, "git", "checkout", "-q", "-B", name, "origin/main")
	s.commit(file, content, "change on "+name)
	s.run(s.work, "git", "push", "-q", "-f", "origin", name)
}

func (s *InternalPullRequestSuite) advanceMain(file, content string) {
	s.run(s.work, "git", "fetch", "-q", "origin")
	s.run(s.work, "git", "checkout", "-q", "-B", "main", "origin/main")
	s.commit(file, content, "main moves on")
	s.run(s.work, "git", "push", "-q", "origin", "main")
}

func (s *InternalPullRequestSuite) mainSHA() string {
	return s.run("", "git", "--git-dir", s.repo.LocalPath, "rev-parse", "main")
}

func (s *InternalPullRequestSuite) compat(method, path string, user *types.User) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, githubCompatPrefix+path, nil)
	if user != nil {
		req = req.WithContext(setRequestUser(req.Context(), *user))
	}
	rec := httptest.NewRecorder()
	s.server.githubCompatRouter.ServeHTTP(rec, req)
	return rec
}

func (s *InternalPullRequestSuite) TestCreateListGetMergeWithMergeCommit() {
	s.branch("feature/a", "a.txt", "a\n")

	id, err := s.git.CreatePullRequest(s.ctx, s.repo.ID, "Add a", "body", "feature/a", "main", "usr_owner")
	s.Require().NoError(err)
	s.Equal("1", id)

	_, err = s.git.CreatePullRequest(s.ctx, s.repo.ID, "Add a again", "", "feature/a", "main", "usr_owner")
	s.Require().Error(err)
	s.Contains(err.Error(), "already exists")

	prs, err := s.git.ListPullRequests(s.ctx, s.repo.ID)
	s.Require().NoError(err)
	s.Require().Len(prs, 1)
	s.Equal(types.PullRequestStateOpen, prs[0].State)
	s.Equal("feature/a", prs[0].SourceBranch)
	s.Equal("http://helix.test/api/github/api/v3/repos/u-usr_owner/module/pulls/1", prs[0].URL)

	ci, err := s.git.GetCIStatus(s.ctx, s.repo.ID, "1", prs[0].HeadSHA)
	s.Require().NoError(err)
	s.Equal(services.CIStatusNone, ci.State)

	// The base moves on without conflicting, so merging needs a merge commit.
	s.advanceMain("b.txt", "b\n")
	baseBefore := s.mainSHA()

	pr, err := s.server.gitHTTPServer.MergeInternalPullRequest(s.ctx, s.repo, 1, "", "", "", "usr_owner")
	s.Require().NoError(err)
	s.True(pr.Merged)
	s.Equal(pr.MergeCommit, s.mainSHA())
	parents := strings.Fields(s.run("", "git", "--git-dir", s.repo.LocalPath, "rev-list", "--parents", "-n1", "main"))
	s.Equal([]string{pr.MergeCommit, baseBefore, pr.HeadSHA}, parents)
	s.Equal([]string{"repo_1:main"}, s.deployed)

	got, err := s.git.GetPullRequest(s.ctx, s.repo.ID, "1")
	s.Require().NoError(err)
	s.Equal(types.PullRequestStateMerged, got.State)

	// The merged PR still shows its own change.
	stored, err := s.store.GetGitPullRequest(s.ctx, s.repo.ID, 1)
	s.Require().NoError(err)
	files, err := s.server.gitHTTPServer.InternalPRFileChanges(s.ctx, s.repo, stored)
	s.Require().NoError(err)
	s.Require().Len(files, 1)
	s.Equal("a.txt", files[0].Filename)
	s.Equal("added", files[0].Status)
	s.Equal(1, files[0].Additions)
	s.Contains(files[0].Patch, "+a")

	_, err = s.server.gitHTTPServer.MergeInternalPullRequest(s.ctx, s.repo, 1, "", "", "", "usr_owner")
	s.ErrorIs(err, services.ErrInternalPRClosed)
}

func (s *InternalPullRequestSuite) TestFastForwardMerge() {
	s.branch("feature/ff", "ff.txt", "ff\n")
	_, err := s.git.CreatePullRequest(s.ctx, s.repo.ID, "FF", "", "feature/ff", "main", "usr_owner")
	s.Require().NoError(err)
	head := s.run("", "git", "--git-dir", s.repo.LocalPath, "rev-parse", "feature/ff")

	pr, err := s.server.gitHTTPServer.MergeInternalPullRequest(s.ctx, s.repo, 1, "merge", "", "", "usr_owner")
	s.Require().NoError(err)
	s.Equal(head, pr.MergeCommit)
	s.Equal(head, s.mainSHA())
}

func (s *InternalPullRequestSuite) TestConflictIsNotMergeable() {
	s.branch("feature/c", "README.md", "theirs\n")
	_, err := s.git.CreatePullRequest(s.ctx, s.repo.ID, "Conflict", "", "feature/c", "main", "usr_owner")
	s.Require().NoError(err)
	s.advanceMain("README.md", "ours\n")
	before := s.mainSHA()

	rec := s.compat(http.MethodGet, "/repos/u-usr_owner/module/pulls/1", s.store.users["usr_owner"])
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"mergeable":false`)

	rec = s.compat(http.MethodPut, "/repos/u-usr_owner/module/pulls/1/merge", s.store.users["usr_owner"])
	s.Equal(http.StatusMethodNotAllowed, rec.Code)
	s.Contains(rec.Body.String(), "not mergeable")
	s.Equal(before, s.mainSHA())
	s.Empty(s.deployed)

	_, err = s.server.gitHTTPServer.MergeInternalPullRequest(s.ctx, s.repo, 1, "squash", "", "", "usr_owner")
	s.ErrorIs(err, services.ErrInternalPRMergeMethod)
}

func (s *InternalPullRequestSuite) TestLandedByAnotherRouteIsMerged() {
	s.branch("feature/l", "l.txt", "l\n")
	_, err := s.git.CreatePullRequest(s.ctx, s.repo.ID, "Landed", "", "feature/l", "main", "usr_owner")
	s.Require().NoError(err)
	// Task Accept fast-forwards main directly.
	_, err = services.MergeBranchFastForward(s.ctx, s.repo.LocalPath, "feature/l", "main")
	s.Require().NoError(err)

	got, err := s.git.GetPullRequest(s.ctx, s.repo.ID, "1")
	s.Require().NoError(err)
	s.Equal(types.PullRequestStateMerged, got.State)
}

func (s *InternalPullRequestSuite) TestNoCommitsAndUnknownBranch() {
	s.run(s.work, "git", "push", "-q", "origin", "main:feature/empty")
	_, err := s.git.CreatePullRequest(s.ctx, s.repo.ID, "Empty", "", "feature/empty", "main", "usr_owner")
	s.Require().Error(err)
	s.Contains(err.Error(), "No commits between")

	_, err = s.git.CreatePullRequest(s.ctx, s.repo.ID, "Missing", "", "feature/missing", "main", "usr_owner")
	s.Require().Error(err)
	s.Contains(err.Error(), "does not exist")
}

func (s *InternalPullRequestSuite) TestAuthorization() {
	s.branch("feature/z", "z.txt", "z\n")
	_, err := s.git.CreatePullRequest(s.ctx, s.repo.ID, "Z", "", "feature/z", "main", "usr_owner")
	s.Require().NoError(err)

	other := s.store.users["usr_other"]
	s.Equal(http.StatusNotFound, s.compat(http.MethodGet, "/repos/u-usr_owner/module/pulls/1", other).Code)
	s.Equal(http.StatusForbidden, s.compat(http.MethodPut, "/repos/u-usr_owner/module/pulls/1/merge", other).Code)
	s.Equal(http.StatusUnauthorized, s.compat(http.MethodGet, "/repos/u-usr_owner/module/pulls/1", nil).Code)

	_, err = s.git.CreatePullRequest(s.ctx, s.repo.ID, "Z2", "", "feature/z", "main", "usr_other")
	s.Require().Error(err)
}

func (s *InternalPullRequestSuite) TestCloseKeepsDiffAndHooksAreAccepted() {
	s.branch("feature/x", "x.txt", "x\n")
	_, err := s.git.CreatePullRequest(s.ctx, s.repo.ID, "X", "", "feature/x", "main", "usr_owner")
	s.Require().NoError(err)

	closed := "closed"
	pr, err := s.server.updateInternalPR(s.ctx, s.repo, 1, &types.GitPullRequestUpdate{State: &closed})
	s.Require().NoError(err)
	s.Equal(types.GitPullRequestStateClosed, pr.State)
	s.run("", "git", "--git-dir", s.repo.LocalPath, "branch", "-D", "feature/x")
	files, err := s.server.gitHTTPServer.InternalPRFileChanges(s.ctx, s.repo, pr)
	s.Require().NoError(err)
	s.Len(files, 1)

	owner := s.store.users["usr_owner"]
	s.Equal(http.StatusOK, s.compat(http.MethodGet, "/repos/u-usr_owner/module/hooks", owner).Code)
	s.Equal(http.StatusOK, s.compat(http.MethodGet, "/repos/u-usr_owner/module/pulls/1/comments", owner).Code)
}
