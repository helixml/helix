package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

// A stuck upstream sync holds the repo lock. Clones must still be served from
// the local mirror instead of queueing behind it.
func (s *GitIntegrationSuite) TestPullServedWhileRepoLockHeld() {
	prev := uploadPackLockWait
	uploadPackLockWait = 500 * time.Millisecond
	defer func() { uploadPackLockWait = prev }()

	lock := s.gitRepoService.GetRepoLock(s.testRepo.ID)
	lock.Lock()
	defer lock.Unlock()

	start := time.Now()
	s.cloneFromServer(filepath.Join(s.testDir, "clone-while-locked"))
	s.Less(time.Since(start), 10*time.Second)
}

// Pushes fail fast with a retryable error instead of hanging forever.
func (s *GitIntegrationSuite) TestPushRejectedWhileRepoLockHeld() {
	clone := filepath.Join(s.testDir, "push-while-locked")
	s.cloneFromServer(clone)
	s.createCommitInClone(clone, "busy.txt", "busy", "Push while busy")

	prev := receivePackLockWait
	receivePackLockWait = 500 * time.Millisecond
	defer func() { receivePackLockWait = prev }()

	lock := s.gitRepoService.GetRepoLock(s.testRepo.ID)
	lock.Lock()
	start := time.Now()
	_, stderr, err := gitcmd.NewCommand("push", "origin", "main").
		RunStdString(s.ctx, &gitcmd.RunOpts{Dir: clone, Timeout: 30 * time.Second})
	lock.Unlock()

	s.Require().Error(err)
	s.Contains(stderr, "503")
	s.Less(time.Since(start), 10*time.Second)
}

// A helix-specs push must return to the agent even when publishing it upstream
// stalls (the original incident: a proxy swallowing a chunked upload).
func (s *GitIntegrationSuite) TestHelixSpecsPushDoesNotWaitForStalledUpstream() {
	agentClone := filepath.Join(s.testDir, "agent-specs-stalled")
	s.cloneFromServer(agentClone)
	_, _, err := gitcmd.NewCommand("checkout", "-b", SpecsBranchName).
		RunStdString(s.ctx, &gitcmd.RunOpts{Dir: agentClone})
	s.Require().NoError(err)

	task := &types.SpecTask{
		ID:            "spt_stalled",
		Name:          "Stalled",
		DesignDocPath: "spt_stalled",
		Status:        types.TaskStatusSpecGeneration,
		// No acting user: publication goes straight to the (stalled) upstream
		// instead of failing early on a missing OAuth connection.
	}
	taskDir := filepath.Join(agentClone, "design", "tasks", task.DesignDocPath)
	s.Require().NoError(os.MkdirAll(taskDir, 0755))
	s.Require().NoError(os.WriteFile(filepath.Join(taskDir, "requirements.md"), []byte("# Requirements: Stalled\n"), 0644))
	s.Require().NoError(giteagit.AddChanges(s.ctx, agentClone, true))
	s.Require().NoError(giteagit.CommitChanges(s.ctx, agentClone, giteagit.CommitChangesOptions{
		Committer: &giteagit.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
		Author:    &giteagit.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
		Message:   "Add plan",
	}))

	s.testAPIKey.SpecTaskID = task.ID
	s.mockStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil).AnyTimes()
	s.mockStore.EXPECT().ListOAuthConnections(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	s.mockStore.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	s.mockStore.EXPECT().ListSpecTaskDesignReviews(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	s.mockStore.EXPECT().CreateSpecTaskDesignReview(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	s.mockStore.EXPECT().GetUnresolvedCommentsForTask(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	addr, release := stallingListenerWithClose(s.T())
	s.testRepo.ExternalURL = fmt.Sprintf("http://%s/stalled.git", addr)

	start := time.Now()
	_, stderr, err := gitcmd.NewCommand("push", "origin", SpecsBranchName).
		RunStdString(s.ctx, &gitcmd.RunOpts{Dir: agentClone, Timeout: 30 * time.Second})
	s.Require().NoError(err, stderr)
	s.Less(time.Since(start), 10*time.Second, "push waited for the stalled upstream")

	// While publication is stuck, the repo stays usable for pulls.
	prev := uploadPackLockWait
	uploadPackLockWait = 500 * time.Millisecond
	defer func() { uploadPackLockWait = prev }()
	s.cloneFromServer(filepath.Join(s.testDir, "clone-during-stalled-publish"))
	s.Less(time.Since(start), 45*time.Second, "pull stalled behind the upstream")

	release()
	s.gitHTTPServer.wg.Wait()

	commit, commitErr := GetBranchCommitID(context.Background(), s.testRepo.LocalPath, SpecsBranchName)
	s.Require().NoError(commitErr)
	s.False(strings.TrimSpace(commit) == "")
}

// After an upstream failure, pulls skip the pre-sync for a while instead of
// stalling on the same broken upstream again.
func (s *GitIntegrationSuite) TestPullSkipsPreSyncAfterRecentUpstreamFailure() {
	addr := stallingListener(s.T())
	s.testRepo.ExternalURL = fmt.Sprintf("http://%s/stalled.git", addr)
	_ = s.gitRepoService.trackUpstreamSync(s.ctx, s.testRepo.ID, func() error { return fmt.Errorf("upstream timed out") })
	s.True(s.gitRepoService.UpstreamFailedWithin(s.testRepo.ID, time.Minute))

	start := time.Now()
	s.cloneFromServer(filepath.Join(s.testDir, "clone-after-failure"))
	s.Less(time.Since(start), 10*time.Second)
}

// A process that died mid-sync leaves sync_in_progress_since set; startup
// recovery must clear exactly that stale marker.
func (s *GitIntegrationSuite) TestRecoverClearsStaleSyncInProgress() {
	stale := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	s.testRepo.SyncInProgressSince = &stale

	s.mockStore.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return([]*types.GitRepository{s.testRepo}, nil)
	s.mockStore.EXPECT().ClearStaleGitRepositorySync(gomock.Any(), s.testRepo.ID, stale, "upstream sync interrupted by API restart").Return(nil)

	s.gitRepoService.recoverIncompletePushes(s.ctx)
}
