# Implementation Tasks: In-Place Agent Switching via New Zed Threads

## Spike — gates the config strategy (done; PAUSED for interactive decision)
- [x] Generate Zed `settings.json` variants with ~100 `agent_servers` and ~100 MCP `context_servers` (`spike/run_spike.sh`)
- [x] Measure CPU, process count, and memory (real Zed under Xvfb) — see `spike/RESULTS.md`
- [x] Record findings: **100 agent_servers ≈ free (0 procs, flat RSS); 100 MCP context_servers = ~3.9 GB / 100 procs (the real cost)**
- [x] **DECISION (reviewer-confirmed): Strategy B — configure ONLY the current agent in Zed.** No all-agents listing, no Zed-native multi-agent picking. Helix dropdown is the sole switch path; daemon rewrites config (new agent's agent_servers + MCP context_servers) and cleanly restarts Zed, then a new thread is created and repopulated.

## Settings-Sync daemon — Zed lifecycle ownership
- [ ] Add a Zed process supervisor to the daemon (launch, track PID, start/stop/restart, restart on crash)
- [ ] Move Zed launch out of `desktop/shared/start-zed-core.sh` so the daemon owns the process; shell scripts only start the daemon
- [ ] On `config_changed` for a switch: Strategy A verify-only / Strategy B rewrite `settings.json` then clean-restart Zed
- [ ] Keep theme-style hot reload for cheap changes; use full restart only for agent switches (Strategy B)

## WebSocket sync protocol — event-driven config load
- [ ] Add an `agent_config_loaded` event (daemon/Zed → Helix) signalling the target agent is resolvable (and Zed reconnected, in Strategy B)
- [ ] Fast-path `claude`/`zed-agent` (registry/native) so the event fires immediately for them
- [ ] Helix awaits this event before creating the new thread

## Backend — switch endpoint & session mutation
- [x] Add `POST /api/v1/sessions/{id}/switch-agent` handler (`session_switch_agent_handlers.go`), swagger-annotated, taking `{ helix_app_id }`
- [x] Validate: session running (not paused), `zed_external`, target resolvable via `resolveForkTarget`
- [x] No-op guard when target agent equals current agent (same app AND same runtime)
- [x] In-flight turn: torn down by the Zed restart — no explicit `cancel_current_turn` needed (documented)
- [x] Update session in place: set `ParentApp`, `Metadata.CodeAgentRuntime`, `Metadata.ZedAgentName`, `AgentSwitchedAt`; clear `ZedThreadID`
- [x] Thread binding lives in `session.Metadata.ZedThreadID` (DB), not an in-memory map — clearing it makes the next message open a new thread
- [x] Publish `config_changed {field:"agent"}` to the session topic to trigger the daemon

## Backend — repopulate the new thread
- [x] Reuse `serializeTranscript` to snapshot the current thread's messages onto a `fork_seed` interaction
- [x] Create a Waiting `fork_handoff` interaction; delivered by existing `pickupWaitingInteraction` on Zed reconnect (new `agent_name` from `getAgentNameForSession`, `ZedThreadID=""` → new thread, `maybePrependTranscript` injects transcript)
- [x] `maybePrependTranscript` extended to fire on in-place switch (`AgentSwitchedAt`), not just forks
- [x] `thread_created` mapping + `ZedThreadID` persistence already handled by existing handler — reused as-is
- [x] `getZedConfig`/`buildCodeAgentConfig` already resolve the agent from `ParentApp` — verified, no change needed

## Zed (Rust) — verify / minimal change
- [ ] Verify `chat_message` + `acp_thread_id: null` creates a new thread bound to the supplied `agent_name` (dispatch `websocket_sync.rs:400`); no new command type if so
- [ ] Emit `agent_config_loaded` if sourced from Zed (vs daemon)
- [ ] Bump `ZED_COMMIT` in `sandbox-versions.txt` if Zed is touched (per repo ordering rule)

## Frontend — rewire the dropdown to a toggle
- [ ] Point `ForkAgentControl` confirm action at the new `switch-agent` mutation instead of `useForkSession`
- [ ] Replace all "fork" wording with simple switch/toggle copy; remove the "child clones fresh" + commit/push warnings (workspace is preserved)
- [ ] Keep the `AGENT_TYPE_ZED_EXTERNAL` eligible-agents filter and the paused-session guard
- [ ] Add the generated API client method (`./stack update_openapi`)

## Fork path preservation
- [ ] Leave `POST /sessions/{id}/fork` and all `fork_*` handlers/markers intact and working
- [ ] Confirm no other caller of the dropdown silently loses fork behaviour it depended on

## Testing
- [ ] Go unit/HTTP test for `switch-agent`: validation, session mutation, transcript seed, mapping reset
- [ ] E2E in inner Helix: start a session, write a scratch file in the container, switch agent, confirm the file survives and the new agent has prior context
- [ ] E2E: switch mid-turn cancels cleanly; switch to same agent is a no-op; switch on paused session is blocked
- [ ] Verify the new thread is never created before `agent_config_loaded` (no unresolved-agent race)
- [ ] Verify daemon-driven Zed restart (Strategy B) recovers cleanly and the WS reconnects
