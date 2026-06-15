# Switch agent framework in place (no fork) via Zed restart

## Summary

Adds a second, default path for changing the agentic framework on a running
session: switch the agent **in place** inside the existing Zed environment
instead of forking to a new session/container. The session keeps its id,
desktop container, workspace, and git state — only the agent changes. The new
agent is repopulated with the prior conversation as context.

The fork-and-pause path (`POST /sessions/{id}/fork`) is left fully intact for
callers that still want a clean-slate fork; only the chat-panel dropdown is
rewired to the new switch path. "Fork" jargon is removed from that UI.

Design + spike: `helix-specs:design/tasks/002111_so-we-recently-added-a/`.

## How it works

1. `POST /sessions/{id}/switch-agent {helix_app_id}` repoints the session's
   agent in place: sets `ParentApp` / `CodeAgentRuntime` / `ZedAgentName`,
   clears `ZedThreadID` (so the next message opens a new Zed thread), sets
   `AgentSwitchedAt`, and seeds a `fork_seed` (transcript) + a Waiting
   `fork_handoff` interaction.
2. It publishes `config_changed{field:"agent"}` to the session's settings-sync
   daemon.
3. The daemon re-fetches `zed-config` (the new agent's `agent_servers` + MCP
   `context_servers`), rewrites `settings.json`, then **restarts Zed**
   (`pkill -x zed`; the desktop's existing `run_zed_restart_loop` respawns it).
   A full restart is required because Zed's MCP context servers are
   per-project/shared and can't be hot-swapped per-thread.
4. On reconnect, the existing `pickupWaitingInteraction` path delivers the
   handoff to the **new** agent — new thread (empty `ZedThreadID`), correct
   `agent_name`, transcript prepended by `maybePrependTranscript`.

No Zed (Rust) changes are needed: `chat_message` + `acp_thread_id:null` already
creates a new thread bound to the supplied `agent_name`, and Zed already emits
`agent_ready` on startup. No new WebSocket event was required — the
reconnect-after-restart flow provides the ordering gate.

## Spike result (why "switch", not "configure all agents")

Measured real Zed under Xvfb: 100 `agent_servers` ≈ free (0 procs, flat RSS),
but 100 MCP `context_servers` = ~3.9 GB / 100 processes at startup. So the
chosen design configures **only the current agent** in Zed and swaps it on
switch, rather than pre-loading every agent.

## Changes

- `api/pkg/server/session_switch_agent_handlers.go` — new switch endpoint +
  in-place mutation + config-change publish (reuses fork transcript machinery).
- `api/pkg/server/server.go` — register `POST /sessions/{id}/switch-agent`.
- `api/pkg/types/types.go` — add `SessionMetadata.AgentSwitchedAt`.
- `api/pkg/server/transcript_serializer.go` — prepend transcript on in-place
  switch too (not just forks).
- `api/cmd/settings-sync-daemon/main.go` — `restartZed()` on `field:"agent"`.
- `frontend/...` — `useSwitchAgent`, `SwitchAgentControl` (renamed from
  `ForkAgentControl`, simplified), wired into `SpecTaskDetailContent`;
  regenerated OpenAPI client.
- Unit tests for the in-place switch + transcript prepend.

## Testing

- Go unit tests (switch mutation, seed/handoff, transcript prepend) pass;
  existing fork/transcript tests still pass.
- `go build ./...` and frontend `tsc -b` clean.
- ⚠️ Not yet validated end-to-end on a live desktop — the daemon change ships in
  the desktop image and needs `./stack build-ubuntu` + a new session. The
  inner-Helix desktop did not provision in the dev sandbox during this work.
