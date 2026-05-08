# Requirements: Auto-Approve ACP Permission Requests for Helix

## User Story

As a Helix platform running autonomous spectasks, I need all ACP permission requests (including plan mode exit) to be auto-approved so that Claude Code agent sessions run without user interaction.

## Problem

When Claude Code enters and exits plan mode (via `EnterPlanMode`/`ExitPlanMode` tools), the ACP protocol sends a `request_permission` call to the Zed client. Zed renders Allow/Deny buttons and waits for a human to click. In Helix spectasks, there is no human — the session blocks forever.

This applies to **all** ACP `request_permission` calls, not just plan mode. Any permission prompt blocks autonomous execution.

## Acceptance Criteria

- [ ] When `external_websocket_sync` feature is enabled, all ACP `request_permission` calls are auto-approved without rendering UI buttons
- [ ] The auto-approval selects the first `AllowOnce` option from the permission request's options list
- [ ] If no `AllowOnce` option exists, falls back to `AllowAlways`, then to the first available option
- [ ] The tool call transitions directly to `InProgress` status (no `WaitingForConfirmation` state)
- [ ] Normal (non-Helix) Zed builds are unaffected — permission prompts work as before
- [ ] Existing tests continue to pass
