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

## Fix

- After an org bot runtime edit updates the app, switch its existing exploratory
  session through the canonical switch-agent lifecycle.
- Treat a session as already using a runtime only when both
  `code_agent_runtime` and the derived `zed_agent_name` match.

## Verification

- Focused Go switch-handler tests pass, including a stale Claude agent name on
  a session whose runtime and app already say Codex.
- Frontend TypeScript and production builds pass.
- Live connected-agent test passed on `localhost:8080`: a real Claude thread
  switched in place to the affected bot's Codex runtime, created a new Codex
  thread, and the immediately following message completed with `SWITCH_OK`.
