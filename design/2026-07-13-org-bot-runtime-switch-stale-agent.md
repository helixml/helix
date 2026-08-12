# Org bot runtime edits can leave a stale agent binding

## Symptom

Interaction `int_01kxdbr9jvreb7xw7vydq9m1fy` failed with:

```text
Thread load failed: Failed to load thread: agent connect failed: Other("Custom agent server `claude-acp` is not registered")
```

The session still stored `zed_agent_name=claude`, while its bot app had been
changed to `code_agent_runtime=codex_cli`. The app update and failing turn were
created within milliseconds of each other on 2026-07-13.

## Root cause

`HelixOrgBotDetail` saved runtime changes by updating the app only. The settings
daemon then correctly generated a current-runtime-only Zed configuration for
Codex, which removed the Claude registration. The durable bot session was not
put through `/sessions/{id}/switch-agent`, so its next turn still requested the
removed `claude-acp` server.

The switch handler also considered the session already switched when its
`code_agent_runtime` matched the target, even if `zed_agent_name` was stale.
That prevented the endpoint from repairing this partially updated state.

A restart exposed a second durability bug in the Bot's Helix MCP setup. The
long-lived app persisted the bearer from the activation request instead of the
org service key. Stopping or replacing that session revoked the bearer, so the
external MCP proxy could discover the `helix` server but its upstream
handshake failed with HTTP 401.

## Fix

- After an org bot runtime edit updates the app, switch its existing exploratory
  session through the canonical switch-agent lifecycle.
- Treat a session as already using a runtime only when both
  `code_agent_runtime` and the derived `zed_agent_name` match.
- Reconcile a durable external-agent session against its current app before
  every queued user turn, repairing stale runtime/thread metadata without a
  synthetic handoff interaction.
- Persist the long-lived org service key on Bot app MCP configuration; never
  persist an activation request's session-scoped bearer.
- Add org MCP tools to update spec-task metadata and stop a running spec-task
  desktop through the existing Helix store and executor paths.

## Verification

- Focused Go switch-handler tests pass, including a stale Claude agent name on
  a session whose runtime and app already say Codex.
- Frontend TypeScript and production builds pass.
- Live connected-agent test passed on `localhost:8080`: a real Claude thread
  switched in place to the affected bot's Codex runtime, created a new Codex
  thread, and the immediately following message completed with `SWITCH_OK`.
- Live `helix org bots chat` verification passed with Macroplane Engineer on
  the Codex subscription. It discovered the Macroplane project and spec-task
  list, created task `spt_01kxdmnx9az4qbfxkv3ss8ek1z`, updated and read it
  back, started a real connected Codex planning desktop, and stopped that
  desktop through `stop_spectask_agent`.
