# Multi-Agent E2E Tests & Session Integrity Fixes

**Date:** 2026-03-20
**PRs:** #1967 (merged), #1975 (open)

## Problem

Spectask `spt_01km320kmz9x3b1w1z92y8cmmd` failed with "Thread load failed: no thread found with ID" on follow-up messages. Investigation uncovered multiple related bugs in agent routing, session management, and the E2E test infrastructure.

## Bugs Found & Fixed

### 1. Missing `agent_name` in `sendChatMessageToExternalAgent` (PR #1967)

**Root cause:** `sendChatMessageToExternalAgent` was the only code path sending `chat_message` commands without `agent_name`. For Claude Code sessions, Zed received `agent_name=null`, defaulted to `NativeAgent`, and tried to load the thread from local SQLite instead of via the Claude Code ACP agent.

**Fix:** Added `agent_name` (from `getAgentNameForSession`) to the command data. Also added `agent_name` to `sendOpenThread`.

### 2. Multi-agent E2E test coverage (PR #1967)

**Problem:** E2E tests only covered `zed-agent` (native). The `agent_name` bug would have been caught if we tested Claude Code.

**Fix:** Refactored the E2E test driver to run "rounds" — each agent gets all 9 test phases. CI now tests both `zed-agent` and `claude`. Added nodejs to Docker images for Claude Code auto-install. All 9 phases pass for both agents including mid-stream interrupt and rapid cancel.

### 3. Stale `ZedThreadID` after container restart (PR #1975)

**Root cause:** When a container restarts, Claude Code's in-process session state is lost. The old `ZedThreadID` stored in the Helix session points to a session that no longer exists, causing "Resource not found" errors on every subsequent `open_thread`.

**Fix:** When Zed reports `thread_load_error`, clear the stale `ZedThreadID` from the session so the next message creates a fresh thread.

### 4. Slack creating duplicate sessions per spectask (PR #1975)

**Root cause:** `slack_project_updates.go:postProjectUpdateNew` created a new session for every Slack project update notification without checking if a session already existed for the spectask. This broke the 1:1 session-to-spectask invariant.

**Fix:** Reuse the spectask's existing `PlanningSessionID` when available.

### 5. `handleThreadCreated` creating duplicate sessions for same Zed thread (PR #1975)

**Root cause:** When Zed reports `thread_created` for a user-initiated thread (no `request_id` mapping), `handleThreadCreated` fell through to creating a new session without checking if a session already existed with the same `ZedThreadID`. This happened on reconnects when Zed re-reported existing threads.

**Fix:** Added a "PRIORITY 3" check: before creating a new session, call `findSessionByZedThreadID` to see if one already exists. If so, reuse it.

### 6. Thread dropdown showing duplicate entries for same session (PR #1975)

**Root cause:** The thread selector dropdown in `SpecTaskDetailContent.tsx` showed "Main thread" + "New Conversation" even when both pointed to the same underlying session (the work session's `helix_session_id` equaled the planning session). Switching appeared broken because both options loaded the same data.

**Fix:** Filter out threads whose `helix_session_id` matches the `planning_session_id`. Only show the dropdown when there are genuinely separate threads.

## Architecture Notes

### Session-to-Spectask Invariant

Each Zed thread should map to exactly one Helix session. Multiple Zed threads on the same spectask = multiple sessions (this is fine — e.g., context exhaustion creates a new thread). But the same Zed thread should never create duplicate sessions.

### Claude Code Session Persistence

Claude Code stores sessions in `~/.claude/projects/<hash>/`. Inside desktop containers, `~/.claude` is symlinked to `/home/retro/work/.claude-state`, which is on the persistent workspace mount. However, the `projects/` subdirectory (where actual session histories live) was empty in tested containers — sessions appear to be in-memory only in the `claude-agent-acp` process. When the process restarts, sessions are lost despite the persistent mount being available.

### E2E Test Infrastructure

- **Go test server:** `zed-repo/crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go`
- **Imports real Helix code** via `replace` directive — tests the currently checked out versions of both repos
- **Multi-agent rounds:** Controlled by `E2E_AGENTS` env var (default: `zed-agent`)
- **CI config:** `.drone.yml` sets `E2E_AGENTS="zed-agent,claude"`
- **Local runs:** `cd ~/pm/zed/crates/external_websocket_sync/e2e-test && E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh`
- **Dev builds:** Use `./stack build-zed dev` (~3min) instead of `release` (~12min) for iteration

## Roadmap

### Session integrity
- [x] ~~**Audit all session creation paths**~~: Done. Slack `postProjectUpdateNew` and `handleThreadCreated` were the two sources of duplicates. Both fixed.
- [x] ~~**Data cleanup**~~: Done. 122 duplicate sessions soft-deleted, 1091 sessions backfilled with `zed_agent_name`.
- [ ] **Investigate Claude Code session persistence**: Sessions exist in `~/.claude/projects/` on newer containers (confirmed working) but not on older ones. The `~/.claude.json` symlink fix may resolve this for future containers. Monitor.
- [ ] **Per-session prompt queue worker**: Currently `processPromptQueue`, `processAnyPendingPrompt`, and `processPendingPromptsForIdleSessions` all run as independent goroutines with no coordination. This caused a race where the same prompt was sent twice (fixed with atomic `UPDATE ... FOR UPDATE SKIP LOCKED`). The proper fix: a single goroutine per session fed by a channel, which deduplicates triggers, serializes prompt sends, and coalesces multiple "check for pending" signals into one check.

### Thread dropdown / multi-thread UX
- [ ] **Threads not showing in Helix session list**: Sessions created by `handleThreadCreated` (user-initiated Zed threads) may not appear in the session list UI due to missing `project_id` or other metadata. Investigate and fix.
- [ ] **Better thread naming**: "New Conversation" comes from Zed's default thread title. Should use a more meaningful name — either from the first message content or the spectask name.
- [ ] **Thread switching should show different content**: Verify that when there are genuinely separate threads, selecting one in the dropdown actually loads its interactions (not just the planning session's interactions).

### E2E test coverage
- [ ] **Qwen Code agent round**: Check if upstream Qwen ACP now supports `session_capabilities.resume` (our fork added this before the protocol officially supported it). If so, drop our fork and add Qwen as a third E2E test round.
- [ ] **Test stale ZedThreadID recovery**: Add an E2E phase that simulates a stale thread ID and verifies the error recovery path (clear + retry).
- [ ] **Codex agent**: Add as fourth E2E test round once available.
