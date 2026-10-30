package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/rs/zerolog/log"
)

// Upstream git commands talk to external providers through git-remote-https and
// send-pack/fetch-pack child processes. gitea's gitcmd only kills the top-level
// git process on timeout, so a stalled helper keeps the output pipes open and
// Wait() never returns. runUpstreamGit kills the whole process group and bounds
// Wait() so a stalled upstream can never hold a repo lock forever.
const (
	upstreamGitDefaultTimeout = 5 * time.Minute
	upstreamGitWaitDelay      = 10 * time.Second
)

// upstreamGitConfig is applied to every upstream command:
//   - lowSpeedLimit/Time abort transfers that stall (e.g. a proxy swallowing a chunked upload)
//   - postBuffer avoids chunked transfer encoding for packs under 500 MB
var upstreamGitConfig = [][2]string{
	{"http.lowSpeedLimit", "1000"},
	{"http.lowSpeedTime", "60"},
	{"http.postBuffer", "524288000"},
}

// upstreamRemote is an external git URL plus credentials. Credentials are sent
// as an HTTP Authorization header via environment-provided git config, so they
// never appear in process argv (/proc/<pid>/cmdline is world-readable).
type upstreamRemote struct {
	URL      string
	Username string
	Password string
}

func (r upstreamRemote) env() []string {
	cfg := upstreamGitConfig
	if r.Password != "" && isHTTPURL(r.URL) {
		username := r.Username
		if username == "" {
			username = "git"
		}
		token := base64.StdEncoding.EncodeToString([]byte(username + ":" + r.Password))
		cfg = append(cfg[:len(cfg):len(cfg)], [2]string{"http.extraHeader", "Authorization: Basic " + token})
	}

	env := []string{fmt.Sprintf("GIT_CONFIG_COUNT=%d", len(cfg))}
	for i, kv := range cfg {
		env = append(env,
			fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", i, kv[0]),
			fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", i, kv[1]),
		)
	}
	return env
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

// runUpstreamGit runs a git command that contacts an external remote. The
// command (and every child it spawns) is killed when ctx is done or timeout
// elapses; the call returns at most upstreamGitWaitDelay after that.
func runUpstreamGit(ctx context.Context, dir string, timeout time.Duration, env []string, args ...string) (string, string, error) {
	if timeout <= 0 {
		timeout = upstreamGitDefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), gitcmd.CommonGitCmdEnvs()...), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = upstreamGitWaitDelay

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil && err != nil {
		err = fmt.Errorf("git %s: %w", args[0], ctxErr)
	}
	return stdout.String(), stderr.String(), err
}

// upstreamGitError formats a failed upstream command, keeping stderr for diagnostics.
func upstreamGitError(op string, err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return fmt.Errorf("%s failed: %w", op, err)
	}
	return fmt.Errorf("%s failed: %w - %s", op, err, stderr)
}

// upstreamCloneTimeout bounds a full mirror clone. Stalled transfers are caught
// much earlier by http.lowSpeedLimit/lowSpeedTime; this only caps slow-but-moving ones.
const upstreamCloneTimeout = 30 * time.Minute

// cloneUpstreamMirror creates a bare mirror of the upstream at repoPath. A
// killed clone can leave a half-written directory behind, so it is removed on
// failure when it did not exist beforehand.
func cloneUpstreamMirror(ctx context.Context, remote upstreamRemote, repoPath string) error {
	_, statErr := os.Stat(repoPath)
	existed := statErr == nil

	_, stderr, err := runUpstreamGit(ctx, "", upstreamCloneTimeout, remote.env(),
		"clone", "--mirror", "--no-progress", "--", remote.URL, repoPath)
	if err != nil {
		if !existed {
			_ = os.RemoveAll(repoPath)
		}
		return upstreamGitError("clone", err, stderr)
	}
	return nil
}

// scrubRemoteCredentials removes credentials that older Helix versions embedded
// in the stored origin URL of mirror clones.
func scrubRemoteCredentials(ctx context.Context, repoPath string) error {
	stdout, _, runErr := gitcmd.NewCommand("config", "--get", "remote.origin.url").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if runErr != nil {
		return nil // no origin remote
	}
	u, err := url.Parse(strings.TrimSpace(stdout))
	if err != nil || u.User == nil {
		return nil
	}
	if _, hasPassword := u.User.Password(); !hasPassword {
		return nil
	}
	u.User = nil
	return SetRemoteURL(ctx, repoPath, "origin", u.String())
}

// trackUpstreamSync records sync health (in progress, last success, last error)
// on the repository around an upstream fetch or push.
func (s *GitRepositoryService) trackUpstreamSync(ctx context.Context, repoID string, fn func() error) error {
	// Record even when the caller's context was cancelled by the operation's failure.
	storeCtx := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	}

	sctx, cancel := storeCtx()
	if err := s.store.MarkGitRepositorySyncStarted(sctx, repoID, time.Now()); err != nil {
		log.Warn().Err(err).Str("repo_id", repoID).Msg("Failed to record upstream sync start")
	}
	cancel()

	err := fn()

	syncErr := ""
	s.upstreamFailedAtMu.Lock()
	if err != nil {
		syncErr = err.Error()
		if s.upstreamFailedAt == nil {
			s.upstreamFailedAt = make(map[string]time.Time)
		}
		s.upstreamFailedAt[repoID] = time.Now()
	} else {
		delete(s.upstreamFailedAt, repoID)
	}
	s.upstreamFailedAtMu.Unlock()
	sctx, cancel = storeCtx()
	defer cancel()
	if storeErr := s.store.MarkGitRepositorySyncFinished(sctx, repoID, time.Now(), syncErr); storeErr != nil {
		log.Warn().Err(storeErr).Str("repo_id", repoID).Msg("Failed to record upstream sync result")
	}
	return err
}

// UpstreamFailedWithin reports whether the last upstream fetch or push for the
// repo failed less than d ago.
func (s *GitRepositoryService) UpstreamFailedWithin(repoID string, d time.Duration) bool {
	s.upstreamFailedAtMu.Lock()
	defer s.upstreamFailedAtMu.Unlock()
	failedAt, ok := s.upstreamFailedAt[repoID]
	return ok && time.Since(failedAt) < d
}
