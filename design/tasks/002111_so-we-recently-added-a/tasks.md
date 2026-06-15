# Implementation Tasks: In-Place Agent Framework Switching on Running Sessions

## Backend — switch endpoint & session mutation
- [ ] Add `POST /api/v1/sessions/{id}/switch-agent` handler (new file next to `session_fork_handlers.go`) taking `{ helix_app_id }`, with swagger annotations
- [ ] Validate request: session is running (not paused), target app has a `zed_external` assistant, target `CodeAgentRuntime` is Zed-compatible (`zed_agent`/`claude_code`/`qwen_code`/`goose_code`)
- [ ] No-op guard when target agent equals current agent
- [ ] Update session in place: set `ParentApp`, `Metadata.ZedAgentName` = new runtime's `ZedAgentName()`; clear `ZedThreadID`
- [ ] Clear the acp_thread_id↔session mapping in `ExternalAgentWSManager` so the next message opens a new Zed thread
- [ ] Reuse the fork transcript serializer to snapshot the current thread's messages
- [ ] Cancel any in-flight turn (`cancel_current_turn`) before resetting the thread binding

## Backend — repopulate the new thread
- [ ] Queue a handoff `chat_message` over the external-agent WS with `acp_thread_id: null`, new `agent_name`, and the serialized transcript as the seed (reuse `maybePrependTranscript`/`fork_seed`-style interaction)
- [ ] On `thread_created` from Zed, map the new acp_thread_id to the session and persist `ZedThreadID`
- [ ] Confirm `getZedConfig`/`buildCodeAgentConfig` resolve the new agent from `ParentApp` after the switch (no code change expected — verify)

## settings-sync daemon — config ordering
- [ ] Publish `config_changed` from the switch handler so the daemon re-syncs the new agent's `agent_servers` + `context_servers` + credentials into `settings.json`
- [ ] Add a "config applied (version N)" signal in the daemon and have the switch flow await it before sending the handoff message
- [ ] Add a `claude`/`zed-agent` fast path that skips the wait (registry/native agents resolve immediately)

## Zed (Rust) — verify / minimal change
- [ ] Verify `chat_message` + `acp_thread_id: null` creates a new thread bound to the supplied `agent_name` (dispatch in `websocket_sync.rs:400`); no new command type if so
- [ ] (Optional) Add an explicit `switch_agent` command only if reuse of `chat_message` proves insufficient
- [ ] If touched, bump `ZED_COMMIT` in `sandbox-versions.txt` per the repo's ordering rule

## Frontend — rewire the dropdown
- [ ] Point `ForkAgentControl` confirm action at the new `switch-agent` mutation instead of `useForkSession`
- [ ] Add the generated API client method (`./stack update_openapi`)
- [ ] Reword the confirm dialog for in-place switching (no fork, no new session)
- [ ] Remove/rework the "child clones fresh" + commit/push warnings (workspace is preserved now); keep dirty state purely informational
- [ ] Keep the `AGENT_TYPE_ZED_EXTERNAL` eligible-agents filter and the paused-session guard

## Fork path preservation
- [ ] Leave `POST /sessions/{id}/fork` and all `fork_*` handlers/markers intact and working
- [ ] Confirm no other caller of the dropdown silently loses fork behaviour it depended on

## Testing
- [ ] Go unit/HTTP test for `switch-agent`: validation, session mutation, transcript seed, mapping reset
- [ ] E2E in inner Helix: start a session, write a scratch file in the container, switch agent, confirm the file survives and the new agent has prior context
- [ ] E2E: switch mid-turn cancels cleanly; switch to same agent is a no-op; switch on paused session is blocked
- [ ] Verify settings.json carries the new agent before the new thread is created (no unresolved-agent race)
