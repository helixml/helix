package services

import (
	"fmt"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"code.gitea.io/gitea/modules/git/gitcmd"
)

// serveUpstreamOverHTTP serves the upstream repo through git http-backend and
// points the test repo at it. The returned counter is the number of upstream
// fetches Helix has made.
func (s *GitIntegrationSuite) serveUpstreamOverHTTP() *atomic.Int32 {
	execPath, _, err := gitcmd.NewCommand("--exec-path").RunStdString(s.ctx, nil)
	s.Require().NoError(err)
	_, _, err = gitcmd.NewCommand("config", "http.receivepack", "true").
		RunStdString(s.ctx, &gitcmd.RunOpts{Dir: s.upstreamDir})
	s.Require().NoError(err)

	backend := &cgi.Handler{
		Path: filepath.Join(strings.TrimSpace(execPath), "git-http-backend"),
		Env: []string{
			"GIT_PROJECT_ROOT=" + filepath.Dir(s.upstreamDir),
			"GIT_HTTP_EXPORT_ALL=1",
		},
	}
	var fetches atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/info/refs") &&
			r.URL.Query().Get("service") == "git-upload-pack" {
			fetches.Add(1)
		}
		backend.ServeHTTP(w, r)
	}))
	s.T().Cleanup(srv.Close)

	s.testRepo.ExternalURL = srv.URL + "/" + filepath.Base(s.upstreamDir)
	return &fetches
}

// commitToUpstream adds a commit to upstream main behind Helix's back.
func (s *GitIntegrationSuite) commitToUpstream(name string) string {
	clone := filepath.Join(s.testDir, "upstream-writer-"+name)
	_, _, err := gitcmd.NewCommand("clone").AddDynamicArguments(s.upstreamDir, clone).RunStdString(s.ctx, nil)
	s.Require().NoError(err)
	s.createCommitInClone(clone, name+".txt", name, "Upstream "+name)
	_, _, err = gitcmd.NewCommand("push", "origin", "main").RunStdString(s.ctx, &gitcmd.RunOpts{Dir: clone})
	s.Require().NoError(err)
	return s.getUpstreamCommit("main")
}

// A burst of pulls of a mirrored repo costs one upstream fetch, not one per
// upload-pack; once the freshness window has passed, a pull fetches again.
func (s *GitIntegrationSuite) TestPullPreSyncDebouncedAfterSuccess() {
	fetches := s.serveUpstreamOverHTTP()

	for i := range 5 {
		s.cloneFromServer(filepath.Join(s.testDir, fmt.Sprintf("burst-%d", i)))
	}
	s.Equal(int32(1), fetches.Load(), "five clones (ten upload-packs) should fetch upstream once")

	// Inside the window the mirror is served as is.
	upstreamHead := s.commitToUpstream("later")
	s.cloneFromServer(filepath.Join(s.testDir, "within-window"))
	s.Equal(int32(1), fetches.Load())
	s.NotEqual(upstreamHead, s.getMiddleCommit("main"))

	// Once it has passed, the next pull refreshes the mirror.
	s.gitRepoService.recordMirrorSynced(s.testRepo.ID, time.Now().Add(-uploadPackSyncFreshness))
	s.cloneFromServer(filepath.Join(s.testDir, "after-window"))
	s.Equal(int32(2), fetches.Load())
	s.Equal(upstreamHead, s.getMiddleCommit("main"))
}

// Pulls that arrive together queue on the repo lock behind the first one's
// sync and are served from it instead of each fetching again.
func (s *GitIntegrationSuite) TestConcurrentPullsShareOnePreSync() {
	fetches := s.serveUpstreamOverHTTP()

	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.cloneFromServer(filepath.Join(s.testDir, fmt.Sprintf("concurrent-%d", i)))
		}()
	}
	wg.Wait()
	s.Equal(int32(1), fetches.Load())
}

// A failed sync still backs pulls off, and does not count as fresh: once the
// backoff has passed, the next pull fetches.
func (s *GitIntegrationSuite) TestPullPreSyncFailureBackoffUnchanged() {
	fetches := s.serveUpstreamOverHTTP()
	_ = s.gitRepoService.trackUpstreamSync(s.ctx, s.testRepo.ID, func() error { return fmt.Errorf("upstream timed out") })

	s.cloneFromServer(filepath.Join(s.testDir, "during-backoff"))
	s.Equal(int32(0), fetches.Load())

	s.gitRepoService.upstreamStateMu.Lock()
	s.gitRepoService.upstreamFailedAt[s.testRepo.ID] = time.Now().Add(-uploadPackSyncBackoff)
	s.gitRepoService.upstreamStateMu.Unlock()
	s.cloneFromServer(filepath.Join(s.testDir, "after-backoff"))
	s.Equal(int32(1), fetches.Load())
}

// A push always syncs from upstream before it is accepted, however fresh the
// mirror is for pulls.
func (s *GitIntegrationSuite) TestPushSyncsDespiteFreshMirror() {
	fetches := s.serveUpstreamOverHTTP()
	clone := filepath.Join(s.testDir, "pusher")
	s.cloneFromServer(clone)
	s.Equal(int32(1), fetches.Load())
	s.True(s.gitRepoService.UpstreamSyncedWithin(s.testRepo.ID, uploadPackSyncFreshness))

	s.createCommitInClone(clone, "pushed.txt", "pushed", "Pushed through Helix")
	s.Require().NoError(s.pushFromClone(clone))
	s.Equal(int32(2), fetches.Load())
	s.assertUpstreamHasCommit("Pushed through Helix")
}
