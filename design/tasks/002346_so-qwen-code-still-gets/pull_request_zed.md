# Honour tool_permissions for external ACP agents

## Summary

External ACP agents (Qwen Code, Gemini, any custom `agent_servers` entry) stalled
indefinitely on tool permission prompts in headless sessions. Zed's external-ACP
`handle_request_permission` routed **every** `session/request_permission` straight
to the interactive "Allow / Reject" dialog, without ever consulting the
`agent.tool_permissions` setting that Zed's **native** agent short-circuits on in
`run_authorization_loop`.

The result: a Helix spec task driving Qwen Code would set
`tool_permissions.default = "allow"` (and launch qwen with `--yolo`), then hang
forever on "Awaiting Confirmation" the first time the agent wrote a file —
because nobody is present in a headless sandbox to click the button.

This makes the external-agent path honour the same setting the native path
already does.

## Changes

- `crates/agent_servers/src/acp.rs`: `handle_request_permission` now reads
  `AgentSettings::tool_permissions.default` and auto-answers `allow` / `deny`
  decisions. `confirm` (the default) falls through to the existing interactive
  prompt, so normal interactive behaviour is unchanged.
- Added `auto_selected_permission_option`, which picks the option to answer with
  by matching on `acp::PermissionOptionKind` rather than the option id (ids are
  agent-specific strings; kinds are part of the ACP schema). Prefers
  `AllowAlways` when allowing and `RejectOnce` when denying, with fallbacks, and
  returns `None` when the agent offered no option of a usable kind — in which
  case we fall through to the prompt rather than guessing.
- The auto-answer is issued inside the same `update` closure as the request, so
  no frame is ever rendered showing the prompt, while the tool call still appears
  in the thread view with the correct status via the existing
  `authorize_tool_call` transitions.
- Added `agent_settings` as a dependency of `agent_servers`.
- 4 unit tests covering the option-selection preference order and the
  no-matching-kind case.

## Notes for reviewers

- **Only the global `tool_permissions.default` is consulted.** Per-tool rules are
  keyed by Zed's *native* tool names (`edit_file`, `terminal`, …), and an
  external ACP `tool_call` carries no equivalent tool name to match against —
  only `tool_call_id` / `title` / `kind` / `raw_input`. Matching agent-specific
  titles against native rule keys would be guesswork. This mirrors what the
  native path does when a tool has no rules entry.
- The helper could not reuse `agent::tool_permissions::decide_permission_from_settings`:
  `crates/agent` already depends on `agent_servers`, so importing it would be a
  dependency cycle. Reading the setting from `agent_settings` avoids that.
