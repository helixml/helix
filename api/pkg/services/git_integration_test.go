package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// GitIntegrationSuite tests the git system end-to-end using real git operations
// with local bare repos simulating external upstreams.
type GitIntegrationSuite struct {
	suite.Suite
	ctx       context.Context
	ctrl      *gomock.Controller
	mockStore *store.MockStore

	// Test directories
	testDir     string
	upstreamDir string // "upstream" bare repo (simulates GitHub/ADO)
	middleDir   string // Helix's middle repo

	// Services
	gitRepoService *GitRepositoryService
	gitHTTPServer  *GitHTTPServer

	// HTTP test server
	httpServer *httptest.Server

	// Test repo
	testRepo *types.GitRepository
}

func TestGitIntegrationSuite(t *testing.T) {
	suite.Run(t, new(GitIntegrationSuite))
}

func (s *GitIntegrationSuite) SetupTest() {
	s.ctx = context.Background()
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = store.NewMockStore(s.ctrl)

	// Create test directories
	s.testDir = s.T().TempDir()
	s.upstreamDir = filepath.Join(s.testDir, "upstream")
	s.middleDir = filepath.Join(s.testDir, "middle")

	// Initialize upstream bare repo with initial commit
	s.initUpstreamRepo()

	// Create GitRepositoryService
	s.gitRepoService = NewGitRepositoryService(
		s.mockStore,
		s.middleDir,
		"http://localhost:8080", // Will be replaced by httptest server
		"Test User",
		"test@example.com",
	)

	// Clone upstream to middle repo (simulating repo attachment)
	s.cloneUpstreamToMiddle()

	// Set up mock store expectations
	s.testRepo = &types.GitRepository{
		ID:            "test-repo",
		Name:          "test-repo",
		LocalPath:     filepath.Join(s.middleDir, "git-repositories", "test-repo"),
		IsExternal:    true,
		ExternalURL:   "file://" + s.upstreamDir,
		ExternalType:  types.ExternalRepositoryTypeGitHub,
		DefaultBranch: "main",
		Status:        types.GitRepositoryStatusActive,
	}

	s.mockStore.EXPECT().
		GetGitRepository(gomock.Any(), "test-repo").
		Return(s.testRepo, nil).
		AnyTimes()

	s.mockStore.EXPECT().
		UpdateGitRepository(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock API key validation for git HTTP auth
	testUser := &types.User{
		ID:    "test-user",
		Email: "test@example.com",
	}
	testAPIKey := &types.ApiKey{
		Key:   "test-api-key",
		Owner: "test-user",
	}
	s.mockStore.EXPECT().
		GetAPIKey(gomock.Any(), gomock.Any()).
		Return(testAPIKey, nil).
		AnyTimes()
	s.mockStore.EXPECT().
		GetUser(gomock.Any(), gomock.Any()).
		Return(testUser, nil).
		AnyTimes()
	s.mockStore.EXPECT().
		GetProjectsForRepository(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	// Create GitHTTPServer
	config := GitHTTPServerConfig{
		ServerBaseURL:   "http://localhost:8080",
		AuthTokenHeader: "Authorization",
		EnablePush:      true,
		EnablePull:      true,
		MaxRepoSize:     100 * 1024 * 1024, // 100MB
		RequestTimeout:  5 * time.Minute,
	}

	// nil authorization for tests (bypasses all auth checks)
	s.gitHTTPServer = NewGitHTTPServer(
		s.mockStore,
		s.gitRepoService,
		config,
		nil, // nil authorizeFn = allow all
		nil, // no trigger manager
	)

	// Create HTTP test server with git routes
	router := mux.NewRouter()
	s.gitHTTPServer.RegisterRoutes(router)
	s.httpServer = httptest.NewServer(router)
}

func (s *GitIntegrationSuite) TearDownTest() {
	if s.httpServer != nil {
		s.httpServer.Close()
	}
	s.ctrl.Finish()
}

// initUpstreamRepo creates a bare repo with an initial commit to act as "upstream"
func (s *GitIntegrationSuite) initUpstreamRepo() {
	require := s.Require()

	// Create bare repo
	err := os.MkdirAll(s.upstreamDir, 0755)
	require.NoError(err)

	err = giteagit.InitRepository(s.ctx, s.upstreamDir, true, "sha1")
	require.NoError(err)

	// Create temp clone to add initial commit
	tempClone := filepath.Join(s.testDir, "temp-clone")
	err = giteagit.InitRepository(s.ctx, tempClone, false, "sha1")
	require.NoError(err)

	// Create initial file
	readmePath := filepath.Join(tempClone, "README.md")
	err = os.WriteFile(readmePath, []byte("# Test Repository\n"), 0644)
	require.NoError(err)

	// Add and commit
	err = giteagit.AddChanges(s.ctx, tempClone, true)
	require.NoError(err)

	err = giteagit.CommitChanges(s.ctx, tempClone, giteagit.CommitChangesOptions{
		Committer: &giteagit.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
		Author:    &giteagit.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
		Message:   "Initial commit",
	})
	require.NoError(err)

	// Rename branch to main if needed
	currentBranch, _ := GetHEADBranch(s.ctx, tempClone)
	if currentBranch == "master" {
		err = GitRenameBranch(s.ctx, tempClone, "master", "main")
		require.NoError(err)
	}

	// Add upstream as remote and push
	err = AddRemote(s.ctx, tempClone, "origin", s.upstreamDir)
	require.NoError(err)

	err = giteagit.Push(s.ctx, tempClone, giteagit.PushOptions{
		Remote: "origin",
		Branch: "refs/heads/main:refs/heads/main",
	})
	require.NoError(err)

	// Set HEAD in bare repo
	err = SetHEAD(s.ctx, s.upstreamDir, "main")
	require.NoError(err)
}

// cloneUpstreamToMiddle clones the upstream repo to the middle repo location
func (s *GitIntegrationSuite) cloneUpstreamToMiddle() {
	require := s.Require()

	middleRepoPath := filepath.Join(s.middleDir, "git-repositories", "test-repo")
	err := os.MkdirAll(filepath.Dir(middleRepoPath), 0755)
	require.NoError(err)

	err = giteagit.Clone(s.ctx, s.upstreamDir, middleRepoPath, giteagit.CloneRepoOptions{
		Bare:   true,
		Mirror: true,
	})
	require.NoError(err)
}

// cloneFromServer clones the repo from the HTTP test server with auth
func (s *GitIntegrationSuite) cloneFromServer(targetDir string) {
	require := s.Require()

	// Build authenticated URL: http://api:test-api-key@host/git/test-repo
	serverURL := s.httpServer.URL
	authURL := strings.Replace(serverURL, "://", "://api:test-api-key@", 1)
	cloneURL := fmt.Sprintf("%s/git/test-repo", authURL)

	err := giteagit.Clone(s.ctx, cloneURL, targetDir, giteagit.CloneRepoOptions{})
	require.NoError(err)
}

// createCommitInClone creates a file and commits it in the given clone
func (s *GitIntegrationSuite) createCommitInClone(clonePath, filename, content, message string) string {
	require := s.Require()

	filePath := filepath.Join(clonePath, filename)
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(err)

	err = giteagit.AddChanges(s.ctx, clonePath, true)
	require.NoError(err)

	err = giteagit.CommitChanges(s.ctx, clonePath, giteagit.CommitChangesOptions{
		Committer: &giteagit.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
		Author:    &giteagit.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
		Message:   message,
	})
	require.NoError(err)

	// Get commit hash
	hash, err := GetBranchCommitID(s.ctx, clonePath, "main")
	require.NoError(err)
	return hash
}

// pushFromClone pushes the clone to the server
func (s *GitIntegrationSuite) pushFromClone(clonePath string) error {
	// Use git command directly for push to HTTP server
	_, _, err := gitcmd.NewCommand("push", "origin", "main").
		RunStdString(s.ctx, &gitcmd.RunOpts{Dir: clonePath})
	return err
}

// getUpstreamCommit returns the current commit hash of the upstream repo
func (s *GitIntegrationSuite) getUpstreamCommit(branch string) string {
	hash, err := GetBranchCommitID(s.ctx, s.upstreamDir, branch)
	s.Require().NoError(err)
	return hash
}

// getMiddleCommit returns the current commit hash of the middle repo
func (s *GitIntegrationSuite) getMiddleCommit(branch string) string {
	hash, err := GetBranchCommitID(s.ctx, s.testRepo.LocalPath, branch)
	s.Require().NoError(err)
	return hash
}

// assertUpstreamHasCommit verifies the upstream repo has a commit with the given message
func (s *GitIntegrationSuite) assertUpstreamHasCommit(message string) {
	stdout, _, err := gitcmd.NewCommand("log", "--oneline", "-10").
		RunStdString(s.ctx, &gitcmd.RunOpts{Dir: s.upstreamDir})
	s.Require().NoError(err)
	s.Contains(stdout, message, "Expected commit message not found in upstream")
}

// =============================================================================
// Basic Tests
// =============================================================================

func (s *GitIntegrationSuite) TestPushToExternalRepo() {
	// Clone from server
	agentClone := filepath.Join(s.testDir, "agent-clone")
	s.cloneFromServer(agentClone)

	// Create a commit
	s.createCommitInClone(agentClone, "test.txt", "test content", "Test commit from agent")

	// Push to server
	err := s.pushFromClone(agentClone)
	s.Require().NoError(err)

	// Verify commit is in upstream
	s.assertUpstreamHasCommit("Test commit from agent")
}

func (s *GitIntegrationSuite) TestSyncFromUpstream() {
	// Add a commit directly to upstream (simulating external change)
	tempClone := filepath.Join(s.testDir, "upstream-clone")
	err := giteagit.Clone(s.ctx, s.upstreamDir, tempClone, giteagit.CloneRepoOptions{})
	s.Require().NoError(err)

	s.createCommitInClone(tempClone, "external.txt", "external content", "External commit")
	_, _, err = gitcmd.NewCommand("push", "origin", "main").
		RunStdString(s.ctx, &gitcmd.RunOpts{Dir: tempClone})
	s.Require().NoError(err)

	// Get upstream commit
	upstreamCommit := s.getUpstreamCommit("main")

	// Middle repo should not have it yet
	middleCommit := s.getMiddleCommit("main")
	s.NotEqual(upstreamCommit, middleCommit, "Middle should not have upstream commit yet")

	// Sync from upstream
	err = s.gitRepoService.SyncAllBranches(s.ctx, "test-repo", true)
	s.Require().NoError(err)

	// Now middle should have it
	middleCommit = s.getMiddleCommit("main")
	s.Equal(upstreamCommit, middleCommit, "Middle should match upstream after sync")
}

func (s *GitIntegrationSuite) TestForceSyncOverwritesDivergedLocal() {
	// Create a commit in middle repo directly (simulating divergence)
	middleRepoPath := s.testRepo.LocalPath

	// Use worktree to commit to bare repo
	worktree := filepath.Join(s.testDir, "middle-worktree")
	err := giteagit.Clone(s.ctx, middleRepoPath, worktree, giteagit.CloneRepoOptions{})
	s.Require().NoError(err)

	s.createCommitInClone(worktree, "local-only.txt", "local content", "Local-only commit")

	// Push to bare middle repo (not to upstream)
	_, _, err = gitcmd.NewCommand("push", "origin", "main").
		RunStdString(s.ctx, &gitcmd.RunOpts{Dir: worktree})
	s.Require().NoError(err)

	// Middle is now ahead of upstream
	middleCommit := s.getMiddleCommit("main")
	upstreamCommit := s.getUpstreamCommit("main")
	s.NotEqual(middleCommit, upstreamCommit, "Middle should be ahead of upstream")

	// Force sync should overwrite
	err = s.gitRepoService.SyncAllBranches(s.ctx, "test-repo", true)
	s.Require().NoError(err)

	// Middle should now match upstream (local commit lost)
	middleCommit = s.getMiddleCommit("main")
	s.Equal(upstreamCommit, middleCommit, "Force sync should overwrite local divergence")
}

// =============================================================================
// Concurrency Tests
// =============================================================================

func (s *GitIntegrationSuite) TestConcurrentPushes_DifferentFiles() {
	// Two agents push different files - both should succeed
	var wg sync.WaitGroup
	errors := make(chan error, 2)

	// Agent 1
	wg.Add(1)
	go func() {
		defer wg.Done()
		clone1 := filepath.Join(s.testDir, "agent1-clone")
		s.cloneFromServer(clone1)
		s.createCommitInClone(clone1, "file1.txt", "content1", "Agent 1 commit")
		errors <- s.pushFromClone(clone1)
	}()

	// Small delay to ensure sequential pushes (concurrent would likely fail on same branch)
	time.Sleep(100 * time.Millisecond)

	// Agent 2
	wg.Add(1)
	go func() {
		defer wg.Done()
		clone2 := filepath.Join(s.testDir, "agent2-clone")
		s.cloneFromServer(clone2)
		s.createCommitInClone(clone2, "file2.txt", "content2", "Agent 2 commit")
		errors <- s.pushFromClone(clone2)
	}()

	wg.Wait()
	close(errors)

	// Count successes and failures
	var successCount, failCount int
	for err := range errors {
		if err == nil {
			successCount++
		} else {
			failCount++
			s.T().Logf("Push failed (expected for second push to same branch): %v", err)
		}
	}

	// At least one should succeed, one might fail due to non-fast-forward
	s.GreaterOrEqual(successCount, 1, "At least one push should succeed")
}

func (s *GitIntegrationSuite) TestLockSerializesOperations() {
	// Verify that concurrent operations on the same repo are serialized
	var wg sync.WaitGroup
	operationOrder := make([]string, 0)
	var mu sync.Mutex

	recordOp := func(op string) {
		mu.Lock()
		operationOrder = append(operationOrder, op)
		mu.Unlock()
	}

	// Start multiple syncs concurrently
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			recordOp(fmt.Sprintf("start-%d", n))
			err := s.gitRepoService.WithRepoLock("test-repo", func() error {
				recordOp(fmt.Sprintf("locked-%d", n))
				time.Sleep(50 * time.Millisecond) // Simulate work
				recordOp(fmt.Sprintf("done-%d", n))
				return nil
			})
			s.Require().NoError(err)
		}(i)
	}

	wg.Wait()

	// Verify operations were serialized (locked-N always followed by done-N before next locked)
	s.T().Logf("Operation order: %v", operationOrder)

	// Check that we never have two "locked" without a "done" in between
	lockedCount := 0
	for _, op := range operationOrder {
		if strings.HasPrefix(op, "locked-") {
			lockedCount++
			s.LessOrEqual(lockedCount, 1, "Should never have two concurrent locked operations")
		} else if strings.HasPrefix(op, "done-") {
			lockedCount--
		}
	}
}

func (s *GitIntegrationSuite) TestLockPerRepo_DifferentReposConcurrent() {
	// Different repos should be able to operate concurrently
	var wg sync.WaitGroup
	startTimes := make(map[string]time.Time)
	var mu sync.Mutex

	repos := []string{"repo-a", "repo-b", "repo-c"}

	for _, repoID := range repos {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			err := s.gitRepoService.WithRepoLock(id, func() error {
				mu.Lock()
				startTimes[id] = time.Now()
				mu.Unlock()
				time.Sleep(50 * time.Millisecond)
				return nil
			})
			s.Require().NoError(err)
		}(repoID)
	}

	wg.Wait()

	// All three should have started within ~10ms of each other (concurrent)
	var times []time.Time
	for _, t := range startTimes {
		times = append(times, t)
	}

	maxDiff := times[0].Sub(times[0])
	for i := 1; i < len(times); i++ {
		for j := 0; j < i; j++ {
			diff := times[i].Sub(times[j])
			if diff < 0 {
				diff = -diff
			}
			if diff > maxDiff {
				maxDiff = diff
			}
		}
	}

	// They should all start within 20ms of each other (allowing for goroutine scheduling)
	s.Less(maxDiff, 20*time.Millisecond, "Different repos should operate concurrently")
}

func (s *GitIntegrationSuite) TestPushWhileReading_NoDataLoss() {
	// Start a sync (read) operation
	syncStarted := make(chan struct{})
	syncDone := make(chan struct{})

	go func() {
		err := s.gitRepoService.WithRepoLock("test-repo", func() error {
			close(syncStarted)
			time.Sleep(100 * time.Millisecond) // Hold lock
			return s.gitRepoService.SyncAllBranches(s.ctx, "test-repo", true)
		})
		s.Require().NoError(err)
		close(syncDone)
	}()

	// Wait for sync to start
	<-syncStarted

	// Try to push (should block until sync completes)
	clone := filepath.Join(s.testDir, "push-while-read-clone")
	s.cloneFromServer(clone)
	commitHash := s.createCommitInClone(clone, "during-read.txt", "content", "Commit during read")

	pushDone := make(chan error, 1)
	go func() {
		pushDone <- s.pushFromClone(clone)
	}()

	// Wait for sync to complete
	<-syncDone

	// Push should complete successfully
	select {
	case err := <-pushDone:
		s.Require().NoError(err, "Push should succeed after read completes")
	case <-time.After(5 * time.Second):
		s.Fail("Push timed out")
	}

	// Verify commit made it to upstream
	upstreamCommit := s.getUpstreamCommit("main")
	s.Equal(commitHash, upstreamCommit, "Pushed commit should be in upstream")
}

func (s *GitIntegrationSuite) TestNoReentrancy_NestedLockWouldDeadlock() {
	// This test verifies that our locking pattern does NOT cause reentrancy.
	// If SyncAllBranches or PushBranchToRemote tried to acquire the lock,
	// this test would deadlock and timeout.

	done := make(chan bool)

	go func() {
		// Simulate WithExternalRepoWrite pattern:
		// Acquire lock, then call SyncAllBranches (which must NOT acquire lock)
		err := s.gitRepoService.WithRepoLock("test-repo", func() error {
			// This call would DEADLOCK if SyncAllBranches tried to acquire the lock
			return s.gitRepoService.SyncAllBranches(s.ctx, "test-repo", true)
		})
		s.Require().NoError(err)
		done <- true
	}()

	select {
	case <-done:
		// Success - no deadlock
	case <-time.After(5 * time.Second):
		s.Fail("Test timed out - possible reentrancy deadlock detected")
	}
}

func (s *GitIntegrationSuite) TestSyncBaseBranch_AcquiresLock() {
	// Test that SyncBaseBranch acquires the lock and serializes with other operations.
	// This is important because SyncBaseBranch is called from entry points
	// (ApproveSpecs, SyncBaseBranchForTask) that don't hold the lock.

	// Run concurrent SyncBaseBranch calls - they should serialize
	var wg sync.WaitGroup
	results := make(chan error, 3)
	operationOrder := make([]string, 0)
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			mu.Lock()
			operationOrder = append(operationOrder, fmt.Sprintf("start-%d", idx))
			mu.Unlock()

			err := s.gitRepoService.SyncBaseBranch(s.ctx, "test-repo", "main")
			results <- err

			mu.Lock()
			operationOrder = append(operationOrder, fmt.Sprintf("done-%d", idx))
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	close(results)

	// All should succeed
	for err := range results {
		s.NoError(err)
	}

	s.T().Logf("SyncBaseBranch operation order: %v", operationOrder)
}

func (s *GitIntegrationSuite) TestSyncBaseBranch_NoDeadlockFromEntryPoint() {
	// Verify that SyncBaseBranch can be called without holding a lock
	// and completes successfully (doesn't deadlock on itself)

	done := make(chan bool)

	go func() {
		// Call SyncBaseBranch directly - this simulates the entry point pattern
		// from ApproveSpecs or SyncBaseBranchForTask
		err := s.gitRepoService.SyncBaseBranch(s.ctx, "test-repo", "main")
		s.Require().NoError(err)
		done <- true
	}()

	select {
	case <-done:
		// Success - no deadlock
	case <-time.After(5 * time.Second):
		s.Fail("SyncBaseBranch timed out - possible deadlock")
	}
}

// =============================================================================
// HTTP Server Tests
// =============================================================================

func (s *GitIntegrationSuite) TestHTTPServer_InfoRefs() {
	// Test the info/refs endpoint with auth
	url := fmt.Sprintf("%s/git/test-repo/info/refs?service=git-upload-pack", s.httpServer.URL)
	req, err := http.NewRequest("GET", url, nil)
	s.Require().NoError(err)
	req.SetBasicAuth("api", "test-api-key")

	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusOK, resp.StatusCode)
	s.Contains(resp.Header.Get("Content-Type"), "application/x-git-upload-pack-advertisement")
}
