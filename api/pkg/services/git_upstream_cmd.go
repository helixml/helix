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
	"time"

	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/rs/zerolog/log"

	helixgit "github.com/helixml/helix/api/pkg/git"
	"github.com/helixml/helix/api/pkg/netguard"
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
//   - followRedirects=false: the destination was vetted against the outbound
//     policy, and a redirect would send git (and its credentials) elsewhere
var upstreamGitConfig = [][2]string{
	{"http.lowSpeedLimit", "1000"},
	{"http.lowSpeedTime", "60"},
	{"http.postBuffer", "524288000"},
	{"http.followRedirects", "false"},
}

// upstreamRemote is an external git URL plus credentials. Credentials are sent
// as an HTTP Authorization header via environment-provided git config, so they
// never appear in process argv (/proc/<pid>/cmdline is world-readable).
type upstreamRemote struct {
	URL      string
	Username string
	Password string

	// resolve holds http.curloptResolve entries ("host:port:addr,...") that
	// pin the remote's host to addresses the outbound policy approved, so
	// git cannot re-resolve it to something else (DNS rebinding).
	resolve string
	// blocked is the policy's rejection of the remote; commands using this
	// remote fail with it instead of running.
	blocked error
}

// pin resolves the remote's host under policy. An allowed host is pinned to
// the vetted addresses; a disallowed one marks the remote blocked. A host
// that does not resolve locally (proxy-only DNS) is left to the proxy.
func (r upstreamRemote) pin(ctx context.Context, policy netguard.Policy) upstreamRemote {
	r.resolve, r.blocked = helixgit.ResolvePin(ctx, policy, r.URL)
	return r
}

// newUpstreamRemote builds an upstreamRemote. Credentials embedded in rawURL
// (https://user:token@host/...) are moved out of the URL so they never reach
// argv; explicit credentials take precedence over embedded ones.
func newUpstreamRemote(rawURL, username, password string) upstreamRemote {
	u, err := url.Parse(rawURL)
	if err != nil || u.User == nil {
		return upstreamRemote{URL: rawURL, Username: username, Password: password}
	}
	urlPassword, hasPassword := u.User.Password()
	if !hasPassword {
		return upstreamRemote{URL: rawURL, Username: username, Password: password}
	}
	if password == "" {
		username, password = u.User.Username(), urlPassword
	}
	u.User = nil
	return upstreamRemote{URL: u.String(), Username: username, Password: password}
}

// env returns the git config environment for a command that contacts r:
// credentials, redirect handling and address pinning.
func (r upstreamRemote) env() []string {
	if r.blocked != nil {
		return []string{upstreamBlockedEnv + "=" + r.blocked.Error()}
	}
	cfg := upstreamGitConfig
	if r.resolve != "" {
		cfg = append(cfg[:len(cfg):len(cfg)], [2]string{"http.curloptResolve", r.resolve})
	}
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

// upstreamGitAllowedProtocols are the transports an upstream command may use.
// Remote URLs come from users, so local paths, file://, ext:: (which runs a
// command) and the other transports git would otherwise allow are refused by
// git itself, including for redirects and submodules. It is set for every
// upstream command, overriding any GIT_ALLOW_PROTOCOL in the server's
// environment. A variable only so tests can use local bare repositories.
var upstreamGitAllowedProtocols = helixgit.AllowedProtocols

// upstreamBlockedEnv marks the environment of a remote the outbound policy
// rejected; runUpstreamGit refuses to run with it.
const upstreamBlockedEnv = "HELIX_UPSTREAM_BLOCKED"

// runUpstreamGit runs a git command that contacts an external remote. The
// command (and every child it spawns) is killed when ctx is done or timeout
// elapses; cancellation and the subsequent pipe wait are both bounded.
func runUpstreamGit(ctx context.Context, dir string, timeout time.Duration, env []string, args ...string) (string, string, error) {
	if timeout <= 0 {
		timeout = upstreamGitDefaultTimeout
	}
	for _, kv := range env {
		if reason, ok := strings.CutPrefix(kv, upstreamBlockedEnv+"="); ok {
			reason = strings.TrimPrefix(reason, netguard.ErrBlockedDestination.Error()+": ")
			return "", "", fmt.Errorf("git %s: upstream refused by outbound policy: %w: %s", args[0], netguard.ErrBlockedDestination, reason)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	// The allowed transports are always set: even for a caller env without
	// them (e.g. a fetch with no remote credentials), an inherited
	// GIT_ALLOW_PROTOCOL must not widen what a user-supplied URL may use.
	cmd.Env = append(append(append(os.Environ(), gitcmd.CommonGitCmdEnvs()...), "GIT_ALLOW_PROTOCOL="+upstreamGitAllowedProtocols), env...)
	configureUpstreamGitProcess(cmd)
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
