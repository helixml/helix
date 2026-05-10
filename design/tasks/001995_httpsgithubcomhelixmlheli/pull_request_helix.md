# Wake dev container for exploratory zed_external sessions on /messages

Fixes https://github.com/helixml/helix/issues/2397

## Summary

`POST /api/v1/sessions/{id}/messages` (introduced in https://github.com/helixml/helix/pull/2375) persisted a `Waiting` interaction durably, but on a session whose Zed agent had never WebSocket-connected, the interaction sat in `waiting` forever. External callers (helix-org, design-review automation, anything driving sessions without a human in the loop) could not rely on `/messages` for first contact.

The wiring `sendMessageToSession → sendChatMessageToExternalAgent → sendCommandToExternalAgent` already fires `autoStartDevContainerForSession` when there's no live WebSocket — but `autoStartDevContainerForSession` silently no-op'd for any session without a `SpecTaskID`, which is exactly the shape exploratory `/sessions/chat?agent_type=zed_external` creates.

## Changes

- **New helper `startDevContainerForSession(ctx, *types.Session)`** in `api/pkg/server/spec_task_design_review_handlers.go`. Resolves project context in priority order: `Metadata.SpecTaskID` → `Metadata.ProjectID` → `session.ProjectID`. Calls `externalAgentExecutor.StartDesktop`. Returns nil when no project context exists (logged) — we cannot invent project config.
- **`startDevContainerForSpecTask` reduced to a thin wrapper** around the new helper. Same observable behaviour, no duplicated 100-line agent-build block.
- **`autoStartDevContainerForSession` rewritten** to call the new helper for any `zed_external` session. Removes the `if SpecTaskID == "" { return }` early-out that was the root of the bug. Adds explicit `agent_type` gate so non-zed_external sessions still skip cleanly.
- **Auto-wake worker (`auto_wake_stuck_interactions.go`) gets a cold-start kick**. When a stuck `Waiting` interaction is on a session with no live WS, `maybeKickColdStart` runs `autoStartDevContainerForSession` (bounded by the existing `autoWakeMaxRetries` cap via `IncrementInteractionAutoWakeCount`). After exhaustion, the interaction is marked `state=error` so the scan stops re-trying. Defensive net for any future code path that creates a Waiting interaction without going through `sendCommandToExternalAgent`.
- **Tests.** New `StartDevContainerForSessionSuite` covers all three session shapes plus no-project / no-executor / nil-session edges. New `AutoWakeColdStartSuite` covers the no-WS branch (kicks once, marks error after retries, skips young interactions). Existing `SessionMessagesHandlerSuite.TestQueuesWhenNoWS` extended to assert `StartDesktop` is invoked exactly once when no WS is connected.

## Why this is the right fix

The issue reporter's preferred Option A ("have `sendMessageToSession` call `autoStartDevContainerForSession`") would be a hack — that wiring already exists. The real defect is one level deeper. Per CLAUDE.md "If you find yourself adding hacks or workarounds, **stop** — root cause the issue", the helper extraction is the proper fix.

## Test plan

- [x] `CGO_ENABLED=1 go test ./api/pkg/server/ -count=1` — full suite passes
- [x] `go build ./api/pkg/server/` — clean
- [x] New `TestStartDevContainerForSessionSuite` — 6/6 pass
- [x] New `TestAutoWakeColdStartSuite` — 3/3 pass
- [x] Extended `TestQueuesWhenNoWS` asserts StartDesktop fires
- [x] Inner Helix API smoke test — restarted cleanly with new binary
- [ ] CI (Drone)

## Out of scope

- Refactor of `resumeSession` to also use the new helper (deferred — working HTTP handler with subtle 500 semantics; not required for the fix).
- The "Zed accepts a keystroke but never relays it" failure mode (known coverage gap in `auto_wake_stuck_interactions.go:96-104`).
