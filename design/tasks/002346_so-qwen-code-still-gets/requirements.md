# Requirements: Auto-Approve Qwen Code Tool Permission Prompts in Zed

## Overview

When a Helix spec-task runs the **Qwen Code** runtime, qwen executes inside a
headless sandbox as a custom ACP agent driven by Zed
(`LLM ←(OpenAI API)→ Qwen Code ←(ACP)→ Zed IDE`). Even though Helix already
launches qwen with `--yolo` / `default_mode: "yolo"` and sets
`always_allow_tool_actions: true` in Zed's settings, the agent **still stalls on
interactive permission prompts** — the "Awaiting Confirmation / Allow all edits /
Reject" dialog seen in the attached screenshots. No human is present in the
sandbox to click, so the task hangs indefinitely.

### Confirmed root cause

1. **Zed ignores the auto-approve setting for external agents (primary).** Zed's
   external-ACP handler `handle_request_permission`
   (`zed/crates/agent_servers/src/acp.rs`) never reads
   `agent.tool_permissions` / `always_allow_tool_actions`. Those settings only
   short-circuit Zed's **native** agent (`run_authorization_loop` in
   `crates/agent/src/thread.rs`). For a custom ACP agent like qwen, Zed
   unconditionally routes every `session/request_permission` to the interactive
   `WaitingForConfirmation` UI and blocks on a click. Helix's
   `always_allow_tool_actions: true` is therefore a **no-op for qwen**.

2. **Qwen still sends `request_permission` despite `--yolo` (contributing).** In
   qwen's ACP `Session.ts`, `requestPermission` is still reached when: (a) the
   session's approval mode is flipped off YOLO after `new_session` (the likely
   trigger for the *edit-tool* "Allow all edits / Reject" prompt), or (b) the
   model calls `ask_user_question`, which is intentionally never YOLO-exempt
   (a permanent stall vector `--yolo` can never fix).

Because Zed always prompts for external agents (fact 1), the durable fix is to
make Zed honour the existing "always allow" intent for external ACP agents.

## User Stories

### US1 — Autonomous tool execution
**As** a Helix user running a spec-task with the Qwen Code runtime,
**I want** qwen's file edits, shell commands, and other tool calls to run
without a permission prompt,
**so that** the task completes autonomously in the headless sandbox instead of
hanging on "Awaiting Confirmation".

### US2 — Consistent behaviour across code agents
**As** a Helix operator,
**I want** the existing `always_allow_tool_actions` / `tool_permissions.default`
setting to actually take effect for external ACP agents (qwen, and any future
custom agent),
**so that** external agents behave like Zed's native agent and Claude Code,
which already auto-approve.

## Acceptance Criteria

- [ ] A spec-task using the `qwen_code` runtime runs an edit/write tool (e.g.
  writing `requirements.md`) end-to-end **without** surfacing an "Allow all edits
  / Reject" prompt, and without a human clicking anything.
- [ ] When Zed's effective agent setting is `tool_permissions.default = "allow"`
  (equivalently `always_allow_tool_actions: true`), an inbound external-agent
  `session/request_permission` is auto-answered by selecting an "allow" option
  (prefer allow-always) instead of rendering the interactive dialog.
- [ ] When the setting is not "allow" (e.g. a user explicitly wants prompts),
  the interactive dialog still appears — existing behaviour is preserved.
- [ ] The fix is not qwen-specific: it applies to any custom ACP agent_server.
- [ ] Verified live in the inner Helix (`localhost:8080`) with a real qwen
  spec-task, not just unit tests — screenshots/logs show no stall.

## Out of Scope

- Changing whether `ask_user_question` is auto-answered. Genuine clarifying
  questions are a distinct concern (a question, not a tool-permission) — see
  Open Questions. This spec targets the tool-action permission prompts.
- Reworking qwen's internal YOLO/approval-mode logic. The qwen-side paths are
  documented for context, but the primary fix is Zed-side.

## Open Questions

1. **`ask_user_question` handling.** Should Zed also auto-answer
   `ask_user_question` requests in the headless/automation context (e.g. pick a
   default option so the agent proceeds), or is leaving those interactive
   acceptable? They are a separate stall vector `--yolo` cannot fix. Current
   assumption: **leave out of scope** for this task; treat separately.
2. **Allow-once vs allow-always.** When auto-approving, should Zed select the
   "allow always" option (persisting qwen's mode) or "allow once" (per-call)?
   Assumption: prefer **allow-always** when offered, fall back to allow-once.
3. **Confirm the qwen mode-flip.** Should we also instrument qwen's
   `config.setApprovalMode` to confirm whether Zed is flipping qwen off YOLO
   after `new_session` (root-cause fact 2a)? The Zed-side fix makes qwen's mode
   irrelevant, but confirming would validate the diagnosis. Assumption:
   optional, log-only, not required for the fix.
4. **Scope of "allow".** Should the auto-approve honour per-tool
   `tool_permissions` rules (deny/confirm for specific tools) for external
   agents too, or only the top-level `default`? Assumption: reuse the same
   decision helper the native agent uses so per-tool rules are respected.
