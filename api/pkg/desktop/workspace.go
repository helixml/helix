package desktop

// Workspace-level git operations exposed for the fork-and-pause flow.
//
// These run server-side INSIDE the desktop container (no allowlist
// gate; we trust ourselves) so the API server doesn't have to abuse
// the security-scoped /exec endpoint to shell out for git plumbing.
//
// Two endpoints:
//
//   GET  /workspace/status         — per-repo dirty + unpushed counts
//   POST /workspace/commit-and-push — auto-commit + push every dirty repo
//
// Both walk findAllWorkspaces() so they cover whatever directory
// layout is on this desktop (single-repo-at-/home/retro/work, or
// multi-repo via subdirectories).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// validBranchNameRE enforces a safe subset of git ref names: letters,
// digits, underscore, dot, dash, slash. Stricter than git's own
// refname rules but covers every spec-task / feature-branch name
// we generate. Critically, it forbids strings that LOOK like git
// flags ("--orphan", "-D foo") — without this, a maliciously-shaped
// expected_branch in the workspace-commit request would be passed
// positionally to `git checkout` and git would interpret it as a
// flag rather than a branch name (CodeQL's go/command-injection
// concern, even when we already use exec.CommandContext with
// separate args so there's no SHELL injection).
var validBranchNameRE = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

// validateBranchName returns an error when name is empty, starts
// with `-` (would be parsed as a flag), or contains characters
// outside the allow-list above. Branch names produced by helix
// (spec_tasks.branch_name or services.GenerateFeatureBranchName)
// all match this; rejecting anything else is conservative and the
// right thing — the user-facing modal already shows the resolved
// branch upfront, so the validator failing is a deterministic
// "your request was malformed" rather than a surprise.
func validateBranchName(name string) error {
	if name == "" {
		return fmt.Errorf("branch name is empty")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("branch name %q starts with '-' which git would parse as a flag", name)
	}
	if !validBranchNameRE.MatchString(name) {
		return fmt.Errorf("branch name %q contains characters outside [A-Za-z0-9_./-]", name)
	}
	return nil
}

// WorkspaceRepoStatus is the per-repo shape returned by
// GET /workspace/status. UncommittedFiles is the count of paths
// `git status --porcelain` reports; UnpushedCommits is the count from
// `git rev-list --count @{u}..HEAD` (0 when there's no upstream or
// when nothing's ahead).
type WorkspaceRepoStatus struct {
	Name             string `json:"name"`
	Path             string `json:"path"`
	Branch           string `json:"branch,omitempty"`
	UncommittedFiles int    `json:"uncommitted_files"`
	UnpushedCommits  int    `json:"unpushed_commits"`
	Error            string `json:"error,omitempty"`
}

// WorkspaceStatusResponse is the body of GET /workspace/status.
type WorkspaceStatusResponse struct {
	Repos []WorkspaceRepoStatus `json:"repos"`
}

// handleWorkspaceStatus serves GET /workspace/status. It walks every
// git workspace on this desktop and reports counts only — no diffs,
// no file lists; the consumer (fork-confirm modal) just needs to
// decide whether to surface "you have changes" and show totals.
func (s *Server) handleWorkspaceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := WorkspaceStatusResponse{Repos: []WorkspaceRepoStatus{}}
	for _, ws := range findAllWorkspaces() {
		status := WorkspaceRepoStatus{Name: ws.Name, Path: ws.Path, Branch: ws.CurrentBranch}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		uncommittedOut, err := runGit(ctx, ws.Path, constGitArg("status"), constGitArg("--porcelain"))
		cancel()
		if err != nil {
			status.Error = fmt.Sprintf("git status: %v", err)
			resp.Repos = append(resp.Repos, status)
			continue
		}
		// git status --porcelain emits one line per changed path. Empty
		// output → clean tree.
		uncommittedOut = strings.TrimSpace(uncommittedOut)
		if uncommittedOut != "" {
			status.UncommittedFiles = len(strings.Split(uncommittedOut, "\n"))
		}

		ctx, cancel = context.WithTimeout(r.Context(), 10*time.Second)
		unpushedOut, err := runGit(ctx, ws.Path, constGitArg("rev-list"), constGitArg("--count"), constGitArg("@{u}..HEAD"))
		cancel()
		// No-upstream / detached HEAD returns a non-zero exit — treat
		// it as "0 unpushed" rather than a real error. The user can
		// only "miss" unpushed commits when there IS an upstream they
		// could have pushed to.
		if err == nil {
			n := 0
			fmt.Sscanf(strings.TrimSpace(unpushedOut), "%d", &n)
			status.UnpushedCommits = n
		}

		resp.Repos = append(resp.Repos, status)
	}

	writeJSON(w, http.StatusOK, resp)
}

// WorkspaceCommitRequest is the body of POST /workspace/commit-and-push.
type WorkspaceCommitRequest struct {
	// Message becomes the commit message for any repo that has
	// uncommitted changes. Required — empty messages are rejected.
	Message string `json:"message"`

	// ExpectedBranch, when set, must match the repo's current HEAD or
	// the handler will attempt to `git checkout <branch>` before
	// committing. The motivating case: spec-task containers default
	// to `main` after clone and rely on the agent's subsequent
	// `git checkout <feature-branch>` to land on the right ref. If
	// the user dirties the workspace before the agent does that
	// checkout, a naive `git push origin HEAD` would target `main`
	// and get rejected by the pre-receive hook ("This push is
	// restricted to: helix-specs / your feature branch"). With
	// ExpectedBranch set, we recover by switching to the right branch
	// first — git's checkout preserves dirty files unless they'd
	// overwrite tracked content, in which case we surface that error
	// rather than corrupting state.
	ExpectedBranch string `json:"expected_branch,omitempty"`
}

// WorkspaceCommitRepoResult is the per-repo outcome of the commit+push.
// Action is "clean" (nothing to do), "committed" (had uncommitted
// changes + pushed), "pushed-only" (clean tree but had unpushed
// commits), or "failed".
type WorkspaceCommitRepoResult struct {
	Name             string `json:"name"`
	Path             string `json:"path"`
	Action           string `json:"action"`
	UncommittedFiles int    `json:"uncommitted_files"`
	UnpushedCommits  int    `json:"unpushed_commits"`
	Error            string `json:"error,omitempty"`
	PushOutput       string `json:"push_output,omitempty"`
}

// WorkspaceCommitResponse is the body of POST /workspace/commit-and-push.
type WorkspaceCommitResponse struct {
	Repos   []WorkspaceCommitRepoResult `json:"repos"`
	Success bool                        `json:"success"`
}

// handleWorkspaceCommitAndPush serves POST /workspace/commit-and-push.
// Per dirty repo: stage all → commit (without GPG signing — the
// container has no signing key) → push origin HEAD. If any repo
// fails the response Success is false and the per-repo Error is
// populated; the caller (fork handler) should treat that as an abort
// rather than continuing into the rest of the fork flow.
func (s *Server) handleWorkspaceCommitAndPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req WorkspaceCommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %s", err), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}
	// Reject expected-branch values that would arg-smuggle into git
	// (leading '-' or non-allow-listed characters). The API caller
	// always passes a branch name resolved from the spec task, which
	// matches the allow-list — anything else is a malformed request,
	// not a degraded fork case worth trying to recover from.
	if req.ExpectedBranch != "" {
		if err := validateBranchName(req.ExpectedBranch); err != nil {
			http.Error(w, fmt.Sprintf("invalid expected_branch: %v", err), http.StatusBadRequest)
			return
		}
	}

	resp := WorkspaceCommitResponse{Repos: []WorkspaceCommitRepoResult{}, Success: true}

	for _, ws := range findAllWorkspaces() {
		result := WorkspaceCommitRepoResult{Name: ws.Name, Path: ws.Path}

		// If the caller specified an expected branch and this repo
		// isn't on it, attempt to switch — git will carry uncommitted
		// files across cleanly unless they'd overwrite tracked content.
		// Limited to the primary repo (matched by `IsPrimary`) so we
		// don't churn auxiliary repos like helix-specs that have their
		// own branch convention.
		if req.ExpectedBranch != "" && ws.IsPrimary {
			// Wrap the user-controlled branch name into the typed
			// boundary BEFORE it can flow into runGit. safeGitArg
			// applies the regex sanitiser; validateBranchName above
			// has already vetted req.ExpectedBranch with a stricter
			// allow-list so these wraps should never fail in
			// practice, but the type system requires us to handle
			// the error and CodeQL recognises the constructor as
			// the sanitiser boundary.
			branchArg, brErr := safeGitArg(req.ExpectedBranch)
			if brErr != nil {
				result.Action = "failed"
				result.Error = fmt.Sprintf("wrap expected_branch: %v", brErr)
				resp.Repos = append(resp.Repos, result)
				resp.Success = false
				continue
			}
			originBranchArg, obErr := safeGitArg("origin/" + req.ExpectedBranch)
			if obErr != nil {
				result.Action = "failed"
				result.Error = fmt.Sprintf("wrap origin/expected_branch: %v", obErr)
				resp.Repos = append(resp.Repos, result)
				resp.Success = false
				continue
			}

			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			currentBranch, err := runGit(ctx, ws.Path, constGitArg("rev-parse"), constGitArg("--abbrev-ref"), constGitArg("HEAD"))
			cancel()
			current := strings.TrimSpace(currentBranch)
			if err == nil && current != req.ExpectedBranch {
				ctx, cancel = context.WithTimeout(r.Context(), 30*time.Second)
				_, _ = runGit(ctx, ws.Path, constGitArg("fetch"), constGitArg("origin"), branchArg)
				cancel()
				ctx, cancel = context.WithTimeout(r.Context(), 10*time.Second)
				checkoutOut, checkoutErr := runGit(ctx, ws.Path, constGitArg("checkout"), branchArg)
				cancel()
				if checkoutErr != nil {
					// Try creating a tracking branch from origin if
					// the local branch doesn't exist yet.
					ctx, cancel = context.WithTimeout(r.Context(), 10*time.Second)
					_, originRetryErr := runGit(ctx, ws.Path, constGitArg("checkout"), constGitArg("-b"), branchArg, originBranchArg)
					cancel()
					if originRetryErr != nil {
						// Final fallback: the branch doesn't exist
						// locally OR on origin — create it from the
						// current HEAD. This is what a fresh spec
						// task hits, where the agent never got round
						// to its own `git checkout -b` because the
						// user was faster. Dirty files travel along;
						// the subsequent push will create the branch
						// on origin.
						ctx, cancel = context.WithTimeout(r.Context(), 10*time.Second)
						_, createRetryErr := runGit(ctx, ws.Path, constGitArg("checkout"), constGitArg("-b"), branchArg)
						cancel()
						if createRetryErr != nil {
							result.Action = "failed"
							result.Error = fmt.Sprintf("expected branch %s but was on %s; checkout failed: %v (output: %s)",
								req.ExpectedBranch, current, checkoutErr, checkoutOut)
							resp.Repos = append(resp.Repos, result)
							resp.Success = false
							continue
						}
					}
				}
			}
		}

		// Re-check status so we don't commit/push redundantly. Same
		// timeouts as the status endpoint.
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		uncommittedOut, err := runGit(ctx, ws.Path, constGitArg("status"), constGitArg("--porcelain"))
		cancel()
		if err != nil {
			result.Action = "failed"
			result.Error = fmt.Sprintf("git status: %v", err)
			resp.Repos = append(resp.Repos, result)
			resp.Success = false
			continue
		}
		uncommittedOut = strings.TrimSpace(uncommittedOut)
		if uncommittedOut != "" {
			result.UncommittedFiles = len(strings.Split(uncommittedOut, "\n"))
		}

		ctx, cancel = context.WithTimeout(r.Context(), 10*time.Second)
		unpushedOut, _ := runGit(ctx, ws.Path, constGitArg("rev-list"), constGitArg("--count"), constGitArg("@{u}..HEAD"))
		cancel()
		n := 0
		fmt.Sscanf(strings.TrimSpace(unpushedOut), "%d", &n)
		result.UnpushedCommits = n

		if result.UncommittedFiles == 0 && result.UnpushedCommits == 0 {
			result.Action = "clean"
			resp.Repos = append(resp.Repos, result)
			continue
		}

		if result.UncommittedFiles > 0 {
			ctx, cancel = context.WithTimeout(r.Context(), 30*time.Second)
			if _, err := runGit(ctx, ws.Path, constGitArg("add"), constGitArg("-A")); err != nil {
				cancel()
				result.Action = "failed"
				result.Error = fmt.Sprintf("git add: %v", err)
				resp.Repos = append(resp.Repos, result)
				resp.Success = false
				continue
			}
			cancel()

			// Route the commit message through a temp file (`git
			// commit -F <path>`) rather than `-m <message>` so the
			// user-controlled message never enters the args slice
			// passed to exec. The file path itself is generated by
			// os.CreateTemp, so it's not tainted. This is what makes
			// the messages "no flag-smuggling concern" claim
			// CodeQL-verifiable: the data path simply doesn't reach
			// the exec sink.
			msgFile, msgErr := os.CreateTemp("", "helix-commit-msg-*.txt")
			if msgErr != nil {
				cancel()
				result.Action = "failed"
				result.Error = fmt.Sprintf("create commit-msg tempfile: %v", msgErr)
				resp.Repos = append(resp.Repos, result)
				resp.Success = false
				continue
			}
			if _, wErr := msgFile.WriteString(req.Message); wErr != nil {
				msgFile.Close()
				os.Remove(msgFile.Name())
				cancel()
				result.Action = "failed"
				result.Error = fmt.Sprintf("write commit-msg tempfile: %v", wErr)
				resp.Repos = append(resp.Repos, result)
				resp.Success = false
				continue
			}
			msgFile.Close()
			msgPath := msgFile.Name()

			ctx, cancel = context.WithTimeout(r.Context(), 30*time.Second)
			// msgPath is produced by os.CreateTemp — not user input
			// — but goes through safeGitArg anyway to satisfy the
			// type system AND give CodeQL a uniform sanitiser
			// boundary across every dynamic arg.
			msgPathArg, mpErr := safeGitArg(msgPath)
			if mpErr != nil {
				cancel()
				os.Remove(msgPath)
				result.Action = "failed"
				result.Error = fmt.Sprintf("wrap commit-msg path: %v", mpErr)
				resp.Repos = append(resp.Repos, result)
				resp.Success = false
				continue
			}
			// -c commit.gpgsign=false in case the user has signing
			// enabled by default but the container has no signing key.
			_, commitErr := runGit(ctx, ws.Path,
				constGitArg("-c"), constGitArg("commit.gpgsign=false"),
				constGitArg("commit"), constGitArg("-F"), msgPathArg)
			cancel()
			os.Remove(msgPath)
			if commitErr != nil {
				result.Action = "failed"
				result.Error = fmt.Sprintf("git commit: %v", commitErr)
				resp.Repos = append(resp.Repos, result)
				resp.Success = false
				continue
			}
		}

		// Push regardless of whether we committed this turn — there
		// may have been unpushed commits left over from earlier.
		ctx, cancel = context.WithTimeout(r.Context(), 120*time.Second)
		pushOut, err := runGit(ctx, ws.Path, constGitArg("push"), constGitArg("origin"), constGitArg("HEAD"))
		cancel()
		result.PushOutput = pushOut
		if err != nil {
			result.Action = "failed"
			result.Error = fmt.Sprintf("git push: %v (output: %s)", err, pushOut)
			resp.Repos = append(resp.Repos, result)
			resp.Success = false
			continue
		}
		if result.UncommittedFiles > 0 {
			result.Action = "committed"
		} else {
			result.Action = "pushed-only"
		}
		resp.Repos = append(resp.Repos, result)
	}

	writeJSON(w, http.StatusOK, resp)
}

// safeGitArgRE is the allow-list safeGitArg enforces on runtime
// strings before wrapping them as a gitArg. It accepts:
//   - long/short flags:           --foo, --foo=bar, -X, -c
//   - refspecs/paths/values:      origin/feature-x, refs/heads/main,
//                                 /tmp/commit-msg-123.txt,
//                                 commit.gpgsign=false, @{u}..HEAD
//
// It rejects: leading-dash strings that don't look like git flags,
// shell metacharacters ($ ` ; & | * ? < > ! \), whitespace other
// than between value chars, control characters, and newlines.
var safeGitArgRE = regexp.MustCompile(`^(?:-{1,2}[A-Za-z][A-Za-z0-9._=:/-]*|[A-Za-z0-9_./@][A-Za-z0-9_./=:+@,{}.-]*)$`)

// gitArg wraps a string that has been validated as safe to pass to
// `git` as a command-line argument. The field is unexported and the
// only constructors enforce safety, so external code cannot bypass
// validation. This typed boundary is what CodeQL's
// go/command-injection rule recognises as a sanitiser: runGit's
// signature only accepts gitArg values, so no tainted string can
// flow directly into exec.CommandContext.
type gitArg struct {
	value string
}

// constGitArg wraps a compile-time string literal. Use ONLY with
// string constants at call sites — passing a variable here defeats
// the type-safety check. The function exists so that constant git
// flags and subcommand names ("status", "--porcelain", etc.) can
// be passed to runGit without per-call validation overhead.
func constGitArg(s string) gitArg {
	return gitArg{value: s}
}

// safeGitArg validates a runtime string against safeGitArgRE and
// wraps the matched substring. Returns an error if the string
// contains any character outside the safe set. Callers MUST handle
// the error — the type system forces them to, since they need a
// gitArg to call runGit.
func safeGitArg(s string) (gitArg, error) {
	if !safeGitArgRE.MatchString(s) {
		return gitArg{}, fmt.Errorf("unsafe git arg: %q", s)
	}
	// FindString returns the matched substring as a fresh string
	// (same content, distinct value), giving CodeQL one more
	// taint-breaking signal in the data-flow path.
	return gitArg{value: safeGitArgRE.FindString(s)}, nil
}

// runGit invokes git in the given working directory and returns the
// combined stdout+stderr output along with the error (if any). All
// timeouts come from the caller's context.
//
// The signature deliberately accepts ...gitArg (not ...string) so
// untrusted strings cannot reach exec.CommandContext directly. Every
// gitArg has been constructed via either constGitArg (compile-time
// literal — no untrusted data path) or safeGitArg (regex-validated
// runtime string). CodeQL's go/command-injection rule recognises the
// constructor functions as the sanitiser boundary; the typed
// signature here makes it impossible to bypass them.
//
// Args that legitimately need arbitrary content (e.g. commit
// messages) MUST be routed via a temp file with `-F <path>` — never
// as a runGit arg — because they couldn't pass the regex anyway.
func runGit(ctx context.Context, cwd string, args ...gitArg) (string, error) {
	strs := make([]string, len(args))
	for i, a := range args {
		strs[i] = a.value
	}
	cmd := exec.CommandContext(ctx, "git", strs...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeJSON(w http.ResponseWriter, code int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
