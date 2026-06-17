# Switch agent framework in place (no fork)

## Summary

Adds a second, default path for changing the agentic framework on a running
session: switch the agent **in place** inside the existing Zed environment
instead of forking to a new session/container. The session keeps its id, desktop
container, workspace, and git state — only the agent changes, and the new agent
is repopulated with the prior conversation as context.

The fork-and-pause path (`POST /sessions/{id}/fork`) is left fully intact for
callers that want a clean-slate fork; only the chat-panel dropdown is rewired to
the new switch path, and "fork" jargon is dropped from that UI.

Design, spike, and debugging notes:
`helix-specs:design/tasks/002111_so-we-recently-added-a/`.

## How it works

1. `POST /sessions/{id}/switch-agent {helix_app_id}` repoints the session's agent
   in place: sets `ParentApp` / `CodeAgentRuntime` / `ZedAgentName`, **repoints
   the spec task's `HelixAppID`**, clears `ZedThreadID`, sets `AgentSwitchedAt`,
   and seeds a `fork_seed` (transcript) + a Waiting `fork_handoff` interaction.
2. It publishes `config_changed{field:"agent"}` to the session's settings-sync
   daemon.
3. **Fast path (no restart):** the daemon re-fetches `zed-config` and rewrites
   `settings.json` (new `agent_servers` + MCP `context_servers` + the claude_code
   `managed-settings.json` model). Zed **hot-reloads** it live — its
   `SettingsStore` observers reconcile agent servers and MCP context servers
   without a process restart. The daemon then calls `POST
   /sessions/{id}/agent-config-applied`, and Helix delivers the handoff over the
   **live** Zed WebSocket: empty `ZedThreadID` → new thread bound to the new
   agent, `maybePrependTranscript` injects the transcript.
4. **Restart fallback:** if no new thread appears within a short timeout (e.g. a
   brand-new custom agent_server didn't register from the hot-reload, or the
   callback was lost), the API publishes `config_changed{field:"agent_restart"}`
   and the daemon restarts Zed (`pkill -x zed`; the desktop's existing
   `run_zed_restart_loop` respawns it), delivering the handoff on reconnect.

No Zed (Rust) changes were needed: `chat_message` + `acp_thread_id:null` already
creates a new thread bound to the supplied `agent_name`.

## Why "switch only the current agent" (not "configure all agents")

A spike on the real Zed binary showed 100 `agent_servers` are ~free (0 procs,
flat RSS) but 100 MCP `context_servers` cost ~3.9 GB / 100 processes at startup,
and Zed shares context servers per-project (no per-agent isolation). So Zed only
ever holds the current agent's config and swaps it on switch.

## Notable fix (caught in live testing)

The underlying claude_code model initially didn't actually change on switch
(Zed's native `default_model` flipped, but `managed-settings.json` — the model
the agent really sends — stayed on the old model). Root cause: `getZedConfig`
resolves `code_agent_config` from `specTask.HelixAppID` first, which the switch
wasn't repointing. Fixed via `repointSpecTaskForSwitch`, with a regression test
that asserts on the resolved config source rather than the agent's self-report.

## Changes

- `api/pkg/server/session_switch_agent_handlers.go` — new switch endpoint,
  in-place mutation, spec-task repoint, live hot-reload delivery + restart
  fallback (reuses the fork transcript machinery).
- `api/pkg/server/server.go` — register `POST /sessions/{id}/switch-agent` and
  `POST /sessions/{id}/agent-config-applied`.
- `api/pkg/types/types.go` — add `SessionMetadata.AgentSwitchedAt`.
- `api/pkg/server/transcript_serializer.go` — prepend transcript on in-place
  switch too (not just forks).
- `api/cmd/settings-sync-daemon/main.go` — on `config_changed{field:"agent"}`
  hot-reload + `notifyAgentConfigApplied`; `field:"agent_restart"` → `restartZed`.
- `frontend/…` — `useSwitchAgent`, `SwitchAgentControl` (renamed from
  `ForkAgentControl`, simplified), wired into `SpecTaskDetailContent`; regenerated
  OpenAPI client.
- Unit tests for the in-place switch, transcript prepend, and the spec-task
  repoint regression.

## Testing

- Go unit tests pass; existing fork/transcript tests still pass; `go build ./...`
  and frontend `tsc -b` clean.
- Validated end-to-end on a live desktop: opus→sonnet switch keeps files +
  conversation, and `managed-settings.json` / spec-task `HelixAppID` flip to the
  new model (confirmed via container + DB ground truth, not the agent's
  self-report).
- Settings-sync-daemon change ships in the desktop image (`./stack build-ubuntu`).
