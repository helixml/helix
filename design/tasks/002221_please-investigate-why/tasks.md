# Implementation Tasks: Unify Bot Creation Flow So Slack Auto-Router Reconciles MCP-Created Bots

- [ ] Write the TDD **red** test first: in `api/pkg/org/interfaces/mcptools`, drive `create_bot` over an in-memory store with a spy `OrgReconciler` and assert its `Reconcile` runs once. Confirm it FAILS on current HEAD.
- [ ] Add an injected `Lifecycle *lifecycle.Service` seam to `mcptools.Config`; make `Config.Build()` / `lifecycleService()` use it when set and fall back to the current construction when nil.
- [ ] Reorder `api/pkg/server/helix_org.go` so `buildOrgServices`, `slackRouteReconciler`, and the reconciler-complete `lifecycleSvc` (with `OrgReconcilers`) are assembled BEFORE `mcptools.RegisterBuiltins(reg, deps.Build())`.
- [ ] Inject the shared `lifecycleSvc` into the MCP `Config` (`deps.Lifecycle = lifecycleSvc`) and remove the now-redundant second lifecycle construction; verify REST `apiDeps.Lifecycle` and the MCP `Deps.Lifecycle` are the same instance.
- [ ] Confirm nothing between old lines 642–776 of `helix_org.go` depends on `reg` being built first; adjust ordering if it does.
- [ ] Run the red test — confirm it now PASSES. Add the optional structural parity guard test in the `server` package (same-instance / non-empty `OrgReconcilers`).
- [ ] Build & unit-test: `go build ./api/pkg/...`; `CGO_ENABLED=1 go test ./pkg/org/... -count=1` (mcptools, lifecycle, slackrouting suites).
- [ ] Manual E2E at `localhost:8080`: with an Automated Slack router present, create a bot via MCP/chat AND via the UI; verify both produce a managed route (`Output.ManagedFor == botID`) + subscription; check API logs for `slackrouting: added route for bot`.
- [ ] Verify CI is green after pushing (Drone / `gh pr checks`).
