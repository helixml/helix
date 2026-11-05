package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func TestIsRebasePending(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Minute)
	later := now.Add(1 * time.Minute)

	cases := []struct {
		name string
		task *types.SpecTask
		want bool
	}{
		{
			name: "nil task",
			task: nil,
			want: false,
		},
		{
			name: "no rebase requested - first approve click ever",
			task: &types.SpecTask{LastPushAt: &earlier},
			want: false,
		},
		{
			name: "rebase requested, no push at all - shouldn't happen but pending",
			task: &types.SpecTask{RebaseRequestedAt: &now},
			want: true,
		},
		{
			name: "rebase requested, agent pushed before request - still pending",
			task: &types.SpecTask{RebaseRequestedAt: &now, LastPushAt: &earlier},
			want: true,
		},
		{
			name: "rebase requested, agent pushed at exact same instant - pending (defensive)",
			task: &types.SpecTask{RebaseRequestedAt: &now, LastPushAt: &now},
			want: true,
		},
		{
			name: "rebase requested, agent has pushed since - NOT pending, retry path",
			task: &types.SpecTask{RebaseRequestedAt: &now, LastPushAt: &later},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isRebasePending(tc.task))
		})
	}
}

func TestShouldOpenPullRequest(t *testing.T) {
	s := &HelixAPIServer{}

	cases := []struct {
		name string
		repo *types.GitRepository
		want bool
	}{
		{
			name: "GitHub with OAuth connection",
			repo: &types.GitRepository{
				ExternalType:      types.ExternalRepositoryTypeGitHub,
				OAuthConnectionID: "conn_123",
			},
			want: true,
		},
		{
			name: "GitHub with PAT only",
			repo: &types.GitRepository{
				ExternalType: types.ExternalRepositoryTypeGitHub,
				GitHub:       &types.GitHub{PersonalAccessToken: "ghp_xxx"},
			},
			want: true,
		},
		{
			name: "Azure DevOps configured",
			repo: &types.GitRepository{
				ExternalType: types.ExternalRepositoryTypeADO,
				AzureDevOps:  &types.AzureDevOps{},
			},
			want: true,
		},
		{
			name: "GitLab with OAuth connection",
			repo: &types.GitRepository{
				ExternalType:      types.ExternalRepositoryTypeGitLab,
				OAuthConnectionID: "conn_456",
			},
			want: true,
		},
		{
			name: "GitLab with personal access token",
			repo: &types.GitRepository{
				ExternalType: types.ExternalRepositoryTypeGitLab,
				GitLab:       &types.GitLab{PersonalAccessToken: "glpat_xxx"},
			},
			want: true,
		},
		{
			name: "GitLab with bare type (auth resolved at client layer)",
			repo: &types.GitRepository{
				ExternalType: types.ExternalRepositoryTypeGitLab,
			},
			want: true,
		},
		{
			name: "Bitbucket — not yet wired into shouldOpenPullRequest",
			repo: &types.GitRepository{
				ExternalType: types.ExternalRepositoryTypeBitbucket,
			},
			want: false,
		},
		{
			name: "Internal repo (no external type)",
			repo: &types.GitRepository{},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, s.shouldOpenPullRequest(tc.repo))
		})
	}
}

// External repos land work only through PRs the agent proposes; approving
// implementation has nothing to do there, and must say so rather than queue a
// templated prompt for the agent. Done tasks are not approvable either.
func TestApproveImplementationRefusesExternalReposAndDoneTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	s := &HelixAPIServer{Store: st}
	const userID = "usr_1"
	project := &types.Project{ID: "prj_1", UserID: userID, DefaultRepoID: "repo_1"}
	repo := &types.GitRepository{
		ID:                "repo_1",
		ExternalType:      types.ExternalRepositoryTypeGitHub,
		OAuthConnectionID: "conn_1",
	}

	call := func(status types.SpecTaskStatus) *httptest.ResponseRecorder {
		st.EXPECT().GetSpecTask(gomock.Any(), "spt_1").Return(&types.SpecTask{ID: "spt_1", ProjectID: "prj_1", Status: status}, nil)
		st.EXPECT().GetProject(gomock.Any(), "prj_1").Return(project, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/spec-tasks/spt_1/approve-implementation", nil)
		req = mux.SetURLVars(req, map[string]string{"spec_task_id": "spt_1"})
		req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))
		rec := httptest.NewRecorder()
		s.approveImplementation(rec, req)
		return rec
	}

	st.EXPECT().GetGitRepository(gomock.Any(), "repo_1").Return(repo, nil)
	rec := call(types.TaskStatusImplementation)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), "agent's proposals")

	rec = call(types.TaskStatusDone)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
