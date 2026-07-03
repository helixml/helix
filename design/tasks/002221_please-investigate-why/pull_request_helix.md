# fix(api): unify bot creation so MCP-created bots reconcile the Slack auto-router

## Summary
The Slack auto-router was not reconciling when a bot was added **via MCP**
(the chat-driven `create_bot` tool), while bots added via the UI/REST worked.

Root cause: the MCP tool surface and the REST `POST /bots` handler each held a
**separate** `lifecycle.Service` instance. Only the REST one had the
`slackrouting` reconciler in its `OrgReconcilers`; the MCP one (built by
`mcptools.Config.Build()`) had an empty slice and no seam to wire one. Both
callers ran `lifecycle.Create` → `runOrgReconcilers`, but for the MCP instance
that iterated nothing — a silent no-op — so MCP-created bots never got a managed
route or subscription on the Automated Slack router.

The fix makes both surfaces share **one** reconciler-complete
`lifecycle.Service`, so the create semantics (Slack auto-router, and by
extension Dispatcher/Helix/Mirror) can never drift between the UI and chat paths
again.

## Changes
- `api/pkg/org/interfaces/mcptools/builtins.go`: add an optional
  `Lifecycle *lifecycle.Service` field to `Config`; `lifecycleService()` returns
  it verbatim when set, else falls back to the previous standalone construction
  (so tests / reconciler-free runtimes are unchanged).
- `api/pkg/server/helix_org.go`: build the MCP tool registry **after** the
  shared `lifecycleSvc` is fully assembled (with `OrgReconcilers`), and set
  `deps.Lifecycle = lifecycleSvc` before `deps.Build()`. The small registry
  block moved down (its `reg`/`orgServer` are only consumed much later), which
  is lower-risk than moving the services block up.
- `api/pkg/org/interfaces/mcptools/create_bot_slackrouting_test.go` (new):
  TDD red→green regression test — drives `create_bot` with a spy `OrgReconciler`
  injected via the new seam and asserts it runs once for the org.

## Testing
- Unit: red on HEAD (`deps.Lifecycle undefined`), green after the fix. Full
  `./pkg/org/...` suite (38 packages) passes.
- Live E2E (inner stack, `HELIX_ORG_ENABLED=true`): created bots via the
  helix-org UI with an Automated Slack router present; the router gained a
  managed route (`ManagedFor` + `mentions` predicate) and a subscription per
  bot; logs show `slackrouting: added route for bot`.

## Screenshots
![Bots created](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002221_please-investigate-why/screenshots/01-bots-created.png)
