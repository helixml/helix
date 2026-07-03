# Requirements: Unify Bot Creation Flow So Slack Auto-Router Reconciles MCP-Created Bots

## Background

The Slack **auto-router** (`api/pkg/org/application/slackrouting`) maintains one
"managed route" per org Bot on each Automated Slack router processor: a filter
`Output` that mentions the bot's id, plus a subscription so the bot receives
Slack messages addressed to it. Its `Reconcile` is designed to run "on every
hire/fire and at startup" (see the package doc), driven by the lifecycle
service's `OrgReconcilers`.

A user reported that after adding a new bot **via MCP** (the chat-driven
`create_bot` tool), the auto-router does not reconcile — the new bot never gets a
route/subscription. Bots added via the UI/REST work. The user suspected a
difference between the UI and MCP flows.

## Root Cause (confirmed)

There are **two separate `lifecycle.Service` instances**, and only the REST one
has the auto-router wired:

- **REST/UI** `POST /api/v1/orgs/{org}/bots` (`api/pkg/org/interfaces/server/api/bots.go:111`)
  uses `lifecycleSvc` built in `api/pkg/server/helix_org.go:669`, which at
  `helix_org.go:776` does
  `lifecycleSvc.OrgReconcilers = append(..., slackRouteReconciler)`. ✅
- **MCP** `create_bot` (`api/pkg/org/interfaces/mcptools/create_bot.go:101`) uses
  the instance from `mcptools.Config.lifecycleService()`
  (`api/pkg/org/interfaces/mcptools/builtins.go:221`), which sets only
  `BotReconcilers` and **never sets `OrgReconcilers`**. `mcptools.Config` has no
  field to inject one, and `deps.Build()` runs at `helix_org.go:642` — before the
  `slackRouteReconciler` is even constructed (line 769). ❌

Both callers invoke the same `lifecycle.Create`, which ends with
`s.runOrgReconcilers(...)` (`lifecycle.go:243`). But `runOrgReconcilers` iterates
`s.OrgReconcilers`, which is empty for the MCP instance — so it is a silent
no-op. That is the whole bug: not domain logic, but a **wiring divergence**
between two hand-built service instances.

## User Stories

### Story 1 — Bot added via MCP is routed on Slack
As an org owner who hires a bot from chat (MCP `create_bot`), I want the Slack
auto-router to immediately gain a managed route + subscription for the new bot,
exactly as when I add it from the UI, so the bot receives Slack messages that
mention it without a server restart or workspace re-connect.

**Acceptance criteria**
- Creating a bot through the MCP `create_bot` tool runs the same whole-org
  reconcilers (including `slackrouting`) that the REST `POST /bots` path runs.
- With an Automated Slack router present, a bot created via MCP gets a managed
  route (`Output.ManagedFor == botID`) and a subscription to its output topic.
- No behavioural difference remains between the UI and MCP creation paths.

### Story 2 — One implementation, no drift
As a maintainer, I want a single shared `lifecycle.Service` used by both the REST
handlers and the MCP tools, so collaborators (OrgReconcilers, and by extension
Dispatcher/Helix/Mirror) can never again be wired into one path but not the
other.

**Acceptance criteria**
- The REST `apiDeps.Lifecycle` and the MCP tool `Deps.Lifecycle` are the **same**
  `*lifecycle.Service` (single instance), or provably carry the same
  `OrgReconcilers`.
- No new business logic is added to interface/wiring layers; the reconcile
  orchestration stays in `lifecycle.Create` (the application/domain layer).

### Story 3 — TDD regression test (red first)
As a maintainer, I want an automated test that **fails on current `main`** and
passes after the fix, proving MCP-created bots trigger the whole-org reconcilers.

**Acceptance criteria**
- A test drives bot creation through the MCP `create_bot` path (or the MCP-built
  `lifecycle.Service`) with a spy/fake `OrgReconciler` and asserts its
  `Reconcile` is called once for the org.
- The test is confirmed red against current HEAD before the fix, green after.

### Story 4 — Manual end-to-end verification
As the reporting user, I want the fix verified live at `localhost:8080`: create a
bot via MCP/chat and via the UI, confirm both produce a managed route +
subscription for the new bot on the Automated Slack router.

## Non-Goals
- Changing the auto-router's routing algorithm, predicate, or thread-follow.
- Touching the classic app-trigger Slack system (`api/pkg/trigger/slack`) — that
  is a different subsystem and not the reported bug.
- Adding new MCP tools or changing `create_bot`'s arguments/semantics.

## Open Questions
- **Unification mechanism.** Preferred fix is a single shared `lifecycle.Service`
  injected into both REST and MCP deps (reordering the composition root so the
  reconciler-wired service is built before `deps.Build()`). An alternative is
  adding an `OrgReconcilers`/`Lifecycle` seam to `mcptools.Config` and wiring the
  same reconciler into it. Both remove the drift; the single-instance approach is
  recommended as it also unifies Dispatcher/Helix/Mirror. Is the single-instance
  approach acceptable given it requires reordering `helix_org.go`?
- **Red test location.** Recommended in the `mcptools` package (behavioural, via
  a spy `OrgReconciler` through the composition seam). Acceptable, or do you want
  it as a server-package wiring/pointer-equality test instead?
- Assumption: an Automated Slack router already exists in the org at create time
  (created on workspace-connect). If none exists, `Reconcile` is a correct no-op;
  the fix only ensures the reconcile *runs*. Confirm this matches the reported
  scenario.
