# Implementation Tasks: Unify Bot Creation Flow So Slack Auto-Router Reconciles MCP-Created Bots

- [x] Write the TDD **red** test first: in `api/pkg/org/interfaces/mcptools`, drive `create_bot` over an in-memory store with a spy `OrgReconciler` injected via the new `Config.Lifecycle` seam and assert its `Reconcile` runs once. Confirmed RED on HEAD (`deps.Lifecycle undefined`).
- [x] Add an injected `Lifecycle *lifecycle.Service` seam to `mcptools.Config`; make `Config.Build()` / `lifecycleService()` use it when set and fall back to the current construction when nil.
- [x] Reorder `api/pkg/server/helix_org.go`: move the small MCP registry block (`reg := NewRegistry()` + `RegisterBuiltins` + `orgServer`) DOWN to after `lifecycleSvc` is fully assembled (after `OrgReconcilers` append), and set `deps.Lifecycle = lifecycleSvc` before `deps.Build()`. (`reg`/`orgServer` are only used far below at lines 831/953, so moving down is lower-risk than moving `buildOrgServices` up.)
- [x] Verify REST `apiDeps.Lifecycle` and the MCP `Deps.Lifecycle` are the same instance; the MCP path no longer builds its own lifecycle in production.
- [x] Run the red test — now PASSES (seam + reorder).
- [x] Build & unit-test: `go build ./api/pkg/...`; `CGO_ENABLED=1 go test ./pkg/org/... -count=1` (mcptools, lifecycle, slackrouting suites).
- [x] Manual E2E at `localhost:8080`: with an Automated Slack router present, create a bot via MCP/chat AND via the UI; verify both produce a managed route (`Output.ManagedFor == botID`) + subscription; check API logs for `slackrouting: added route for bot`.
- [x] Local verification complete: `go build ./...` clean, full `./pkg/org/...` suite (38 pkgs) green, live E2E passed. (Drone CI runs on the external repo when the platform opens the PR — no build number available from the inner stack to poll here.)
