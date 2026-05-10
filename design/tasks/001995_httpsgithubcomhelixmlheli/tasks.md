# Implementation Tasks

- [ ] Extract a new `startDevContainerForSession(ctx, *types.Session) error` helper in `api/pkg/server/spec_task_design_review_handlers.go` that builds a `DesktopAgent` from the session (handling spec-task, exploratory `Metadata.ProjectID`, and legacy `session.ProjectID` shapes) and calls `externalAgentExecutor.StartDesktop`.
- [ ] Refactor `startDevContainerForSpecTask` (`spec_task_design_review_handlers.go:910`) into a thin wrapper over the new helper — load the spec task, then delegate.
- [ ] Refactor the agent-build + `StartDesktop` block of `resumeSession` (`session_handlers.go:~1923-2070`) to delegate to the new helper. Keep HTTP concerns (auth, response writing, post-StartDesktop metadata refresh) in the handler.
- [ ] Rewrite `autoStartDevContainerForSession` (`websocket_external_agent_sync.go:3222`) to call the new helper for any `zed_external` session — remove the `if SpecTaskID == "" { return }` early-out.
- [ ] Extend Gate 1 in `auto_wake_stuck_interactions.go:237` so that when no WS exists, the worker calls `autoStartDevContainerForSession` (bounded by `autoWakeMaxRetries` via the existing `AutoWakeCount` column) instead of returning silently. Add a targeted column update to bump `AutoWakeCount` for the no-WS branch — do not use `Save` (see file header comment at lines 75-86).
- [ ] Mark stuck interactions as `state=error` after `autoWakeMaxRetries` no-WS attempts so the scan stops re-trying.
- [ ] Add INFO logging in `autoStartDevContainerForSession`: one line on entry (`session_id`, `agent_type`, `has_spec_task`), one on success, one on no-op (no project context), and an ERROR line on failure with the underlying error.
- [ ] Extend `SessionMessagesHandlerSuite.TestQueuesWhenNoWS` (`session_messages_handler_test.go:90`) to assert the mocked `externalAgentExecutor.StartDesktop` is invoked exactly once when no WS is connected. Will require wiring a mock executor into the suite's `SetupTest`.
- [ ] Add a unit test for `startDevContainerForSession` that covers all three session shapes (spec-task, exploratory `Metadata.ProjectID`, legacy `session.ProjectID`) and the no-project-no-spec-task no-op case.
- [ ] Add a unit test for the auto-wake worker's new no-WS branch: stuck interaction + no WS triggers `autoStartDevContainerForSession` once, increments `AutoWakeCount`, and after `autoWakeMaxRetries` marks the interaction `state=error`.
- [ ] Verify `cd api && CGO_ENABLED=1 go test -run TestSessionMessagesHandlerSuite ./pkg/server/ -count=1` passes locally (CGo required for tree-sitter — see CLAUDE.md "Go Local Tests").
- [ ] Run `cd api && go build ./pkg/server/` to confirm clean compile.
- [ ] End-to-end test against the inner Helix at `http://localhost:8080` using the issue's reproducer: register `test@helix.ml`, create project, POST /sessions/chat (zed_external, no spec task), then POST /sessions/{id}/messages — confirm the queued message is delivered within ~30s without opening the desktop URL.
- [ ] Confirm an existing spec-task session still auto-starts correctly (no regression in the spec-task flow).
- [ ] Tail `docker compose -f docker-compose.dev.yaml logs api` and check the new auto-start INFO logs are visible.
- [ ] Commit, push, and check Drone CI (`drone_build_info`) — fix any failures before declaring done.
