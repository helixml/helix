# Implementation Tasks: Unify Bot Creation Flow So Slack Auto-Router Reconciles MCP-Created Bots

- [~] Write the TDD **red** test first: in `api/pkg/org/interfaces/mcptools`, drive `create_bot` over an in-memory store with a spy `OrgReconciler` injected via the new `Config.Lifecycle` seam and assert its `Reconcile` runs once. Confirm it FAILS on current HEAD.
- [ ] Add an injected `Lifecycle *lifecycle.Service` seam to `mcptools.Config`; make `Config.Build()` / `lifecycleService()` use it when set and fall back to the current construction when nil.
- [ ] Reorder `api/pkg/server/helix_org.go`: move the small MCP registry block (`reg := NewRegistry()` + `RegisterBuiltins` + `orgServer`) DOWN to after `lifecycleSvc` is fully assembled (after `OrgReconcilers` append), and set `deps.Lifecycle = lifecycleSvc` before `deps.Build()`. (`reg`/`orgServer` are only used far below at lines 831/953, so moving down is lower-risk than moving `buildOrgServices` up.)
- [ ] Verify REST `apiDeps.Lifecycle` and the MCP `Deps.Lifecycle` are the same instance; the MCP path no longer builds its own lifecycle in production.
- [ ] Run the red test — confirm it now PASSES.
- [ ] Build & unit-test: `go build ./api/pkg/...`; `CGO_ENABLED=1 go test ./pkg/org/... -count=1` (mcptools, lifecycle, slackrouting suites).
- [ ] Manual E2E at `localhost:8080`: with an Automated Slack router present, create a bot via MCP/chat AND via the UI; verify both produce a managed route (`Output.ManagedFor == botID`) + subscription; check API logs for `slackrouting: added route for bot`.
- [ ] Verify CI is green after pushing (Drone / `gh pr checks`).
