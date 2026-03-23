# ACP Interrupt Hang & Multi-Repo PR Issues

**Date**: 2026-03-23
**Task**: spt_01kkrerndw1dbxggp9pre93kfs (feature/001554-merge-latest-zed)
**Session**: ses_01kkrerqd3rs5tps034eqvsqj0

## Summary

Two issues discovered while testing multi-repo PR creation on a spectask that spanned helix and zed repositories.

---

## Issue 1: ACP Agent Hangs After Mid-Stream Interrupt

### Symptoms

- User interrupted the Claude agent mid-stream (while it was outputting a `<thinking>` block)
- Zed's bottom-right spinner stuck indefinitely — UI believes the agent turn is still in progress
- No way to recover without killing the session

### Root Cause Analysis

Timeline from Zed.log inside container `ubuntu-external-01kkrerqd3rs5tps034eqvsqj0`:

1. `08:16:19` — Last websocket-out message: agent streaming `<thinking>` block (message_id 439)
2. `08:16:21` — ACP stderr: `"Session b573fc87-...: consuming background task result"` — then **silence**
3. No further websocket sync events — Zed stopped receiving messages from the ACP bridge entirely
4. `08:19:37` — Unrelated LSP activity confirms Zed itself is still running, just the agent panel is stuck

Process state at time of investigation:
- `claude` process (PID 10942): main thread on `ep_poll`, 9 worker threads on `futex_do_wait` — **idle**, not deadlocked
- The ACP bridge (`claude-agent-acp` v0.22.2 at `/home/retro/.local/share/zed/external_agents/claude-agent-acp/0.22.2/`) consumed the background task result but **never sent a turn-complete or cancellation-complete event** back to Zed
- Zed has no timeout for agent turn completion, so the spinner hangs forever

### What We Think Happened

The interrupt arrived while the Anthropic API was streaming a response (specifically during a `<thinking>` block). The `claude` CLI process received the cancel signal and stopped producing output. The ACP bridge Node.js process logged "consuming background task result" — indicating it handled the cancellation internally — but failed to propagate a turn-completion signal back to Zed over the ACP protocol. Zed's agent panel state machine is stuck in "agent turn in progress" with no event to transition it out.

### Relation to E2E Tests

The E2E test suite has a "mid-stream interrupt" test (phase 8 of the 9-phase test), but this tests the **Go server side** (websocket sync handler). The bug is in the **Zed/ACP-side** — the `claude-agent-acp` npm package that bridges between the `claude` CLI and Zed's ACP protocol. The Go server correctly handles the interrupt; it's the local ACP bridge that drops the ball.

### Potential Fixes

- The `claude-agent-acp` package needs to guarantee a turn-complete/cancellation-complete event is sent to Zed after any interrupt, even if the underlying CLI process goes silent
- Zed could add a timeout: if no ACP events are received for N seconds after an interrupt, forcibly transition the UI out of "agent running" state
- The E2E tests should be extended to cover the Zed-side ACP interrupt flow, not just the Go server side

---

## Issue 2: Multi-Repo PR — Only One PR Created

### Symptoms

- Spectask spanned helix and zed repos, but "Open PR" only created PR #1998 on helix, not on zed
- Logs showed: `pr_count=1`

### Root Cause

The multi-PR code (`ensurePullRequestsForAllRepos` in `spec_task_workflow_handlers.go`) is working correctly. It iterates all project repos and checks if the task's feature branch exists in each before creating a PR. The logs confirmed:

```
Branch does not exist in repo, skipping PR creation  branch=feature/001554-merge-latest-zed  repo_name=zed-4
Branch does not exist in repo, skipping PR creation  branch=feature/001554-merge-latest-zed  repo_name=qwen-code-4
Branch does not exist in repo, skipping PR creation  branch=feature/001554-merge-latest-zed  repo_name=docs
Ensuring pull request for repo  branch=feature/001554-merge-latest-zed  repo_name=helix-4
```

The agent simply never created the feature branch in the zed-4 repo — it only committed to helix-4.

---

## Issue 3: Manual Push to Middle Git Server Silently Fails to Sync to GitHub

### Symptoms

User manually pushed the branch from inside the sandbox to the API's middle git server:
```
git push origin origin/feature/001554-merge-latest-zed
```
Git reported success (`[new reference]`), but the branch never appeared on GitHub.

### Root Cause

The push command `git push origin origin/feature/001554-merge-latest-zed` pushes the local remote-tracking ref to the server. This creates a ref at `refs/remotes/origin/feature/001554-merge-latest-zed` on the API server — **not** at `refs/heads/feature/001554-merge-latest-zed`.

The git HTTP server's branch detection (`getBranchHashes` at `git_http_server.go:662`) only scans `refs/heads/`. Since the push went to `refs/remotes/origin/`, it saw zero changed branches, reported `pushed_branches=[]`, and never triggered the sync-to-GitHub step.

The pre-receive hook also silently skips non-`refs/heads/` refs (`case "$refname" in refs/heads/*) ;; *) continue ;; esac`), so the push was accepted without error.

Server logs confirmed:
```
Receive-pack completed  pushed_branches=[]  repo_id=prj_01kg02vqqyg178c1n2ydscn5fb-zed-4
```

The ref ended up at:
```
refs/remotes/origin/feature/001554-merge-latest-zed 0039f77a68
```

### Correct Command

```
git push origin origin/feature/001554-merge-latest-zed:refs/heads/feature/001554-merge-latest-zed
```

Or better: create a local branch first, then push normally:
```
git branch feature/001554-merge-latest-zed origin/feature/001554-merge-latest-zed
git push origin feature/001554-merge-latest-zed
```

### Potential Fix

The pre-receive hook or the git HTTP server should **reject** pushes to refs outside `refs/heads/` and `refs/tags/`. Silently accepting them and then doing nothing is a trap. A clear error message ("refusing push to refs/remotes/ — push to refs/heads/ instead") would have saved significant debugging time.
