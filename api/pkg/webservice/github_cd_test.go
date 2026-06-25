package webservice

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

type fakeGitSyncer struct {
	head    string
	syncErr error
	synced  int
}

func (f *fakeGitSyncer) SyncBaseBranch(_ context.Context, _, _ string) error {
	f.synced++
	return f.syncErr
}
func (f *fakeGitSyncer) GetLocalBranchSHA(_ context.Context, _, _ string) (string, error) {
	return f.head, nil
}

type fakeRedeployer struct{ calls []DeployRequest }

func (f *fakeRedeployer) Redeploy(_ context.Context, req DeployRequest) (*types.WebServiceDeploy, error) {
	f.calls = append(f.calls, req)
	return &types.WebServiceDeploy{}, nil
}

func newWatcher(st store.Store, rd Redeployer, g GitSyncer) *GitHubDeployWatcher {
	return &GitHubDeployWatcher{store: st, controller: rd, git: g, lastSeen: map[string]string{}}
}

func mockStoreFor(ctrl *gomock.Controller, repo *types.GitRepository, deploys []*types.WebServiceDeploy) *store.MockStore {
	st := store.NewMockStore(ctrl)
	st.EXPECT().ListActiveWebServices(gomock.Any()).
		Return([]*types.ProjectWebServiceState{{ProjectID: "prj_1"}}, nil).AnyTimes()
	st.EXPECT().GetProject(gomock.Any(), "prj_1").
		Return(&types.Project{ID: "prj_1", DefaultRepoID: repo.ID, UserID: "u1"}, nil).AnyTimes()
	st.EXPECT().GetGitRepository(gomock.Any(), repo.ID).Return(repo, nil).AnyTimes()
	st.EXPECT().ListWebServiceDeploys(gomock.Any(), "prj_1", gomock.Any()).
		Return(deploys, nil).AnyTimes()
	return st
}

// Default branch advanced past the last deployed commit → redeploy once, with
// the new SHA and the project owner. A second tick with no further movement
// must NOT redeploy again.
func TestGitHubCD_DeploysOnAdvance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := &types.GitRepository{ID: "repo1", IsExternal: true, DefaultBranch: "main"}
	st := mockStoreFor(ctrl, repo, []*types.WebServiceDeploy{
		{Status: types.WebServiceDeployStatusLive, CommitSHA: "oldsha"},
	})
	git := &fakeGitSyncer{head: "newsha"}
	rd := &fakeRedeployer{}
	w := newWatcher(st, rd, git)

	w.runOnce(context.Background())
	w.runOnce(context.Background())

	if len(rd.calls) != 1 {
		t.Fatalf("expected exactly 1 redeploy, got %d", len(rd.calls))
	}
	if rd.calls[0].CommitSHA != "newsha" || rd.calls[0].Owner != "u1" || rd.calls[0].ProjectID != "prj_1" {
		t.Errorf("unexpected deploy request: %+v", rd.calls[0])
	}
}

// Helix-hosted (non-external) repos auto-deploy via the git post-receive hook —
// the watcher must skip them entirely (no sync, no deploy) to avoid double-deploys.
func TestGitHubCD_SkipsHelixHosted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := &types.GitRepository{ID: "repo1", IsExternal: false, DefaultBranch: "main"}
	st := mockStoreFor(ctrl, repo, nil)
	git := &fakeGitSyncer{head: "newsha"}
	rd := &fakeRedeployer{}
	w := newWatcher(st, rd, git)

	w.runOnce(context.Background())

	if git.synced != 0 {
		t.Errorf("non-external repo should not be synced, synced=%d", git.synced)
	}
	if len(rd.calls) != 0 {
		t.Errorf("non-external repo should not deploy, calls=%d", len(rd.calls))
	}
}

// HEAD already equals the last deployed commit → nothing to do (don't redeploy
// an app that's already on HEAD, e.g. right after a manual deploy).
func TestGitHubCD_NoDeployWhenUnchanged(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := &types.GitRepository{ID: "repo1", IsExternal: true, DefaultBranch: "main"}
	st := mockStoreFor(ctrl, repo, []*types.WebServiceDeploy{
		{Status: types.WebServiceDeployStatusLive, CommitSHA: "samesha"},
	})
	git := &fakeGitSyncer{head: "samesha"}
	rd := &fakeRedeployer{}
	w := newWatcher(st, rd, git)

	w.runOnce(context.Background())

	if len(rd.calls) != 0 {
		t.Errorf("unchanged HEAD should not deploy, calls=%d", len(rd.calls))
	}
}
