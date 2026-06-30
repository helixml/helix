# Design: Add Helix-Org MCP Tools for Workers to Manage Spec Tasks

## Architecture Overview

The helix-org MCP server (`api/pkg/org/interfaces/server/mcp.go`) builds a
per-Worker `*mcp.Server` whose tools come from `Role.Tools` resolved against
`s.registry` (`mcptools.Registry`). Org tools never touch `store.Store`
directly — they delegate to **application services** or **runtime ports**
defined in `api/pkg/org/infrastructure/runtime/runtime.go` and implemented by
the in-proc adapter `api/pkg/org/infrastructure/runtime/helix/`.

The precedent we follow for reaching the helix world is `ProjectConfig`: a
port (`runtime.ProjectConfig`) + an in-proc impl
(`runtimehelix.ProjectConfig`, `project_config.go`) that resolves
`workerID → projectID` via the Worker's runtime state (`LoadState`) and then
reads/writes the helix project through the in-proc client.

Per review feedback we add one more layer on top of that precedent: a
**dedicated org application service** (`org/application/spectasks`) as the
front-of-house the MCP tools consume — the same role `roles`, `workers`,
`topics`, and `publishing` play for their tools. The layering, top to bottom:

```
interfaces/mcptools  (tools)                 implement tool.Tool
        │ depends on
application/spectasks.Service  (front-of-house use cases, scoping, views)
        │ depends on (port)
infrastructure runtime.SpecTasks  (interface)        ← "interface to infra"
        │ implemented by
infrastructure/runtime/helix.SpecTasks               ← calls out to core
        │ calls
core spectask code  (shared helix spectask service + SpecDrivenTaskService + store)
```

## Key Decisions

### 1. Dedicated org application service `spectasks.Service` (front-of-house)
New package `api/pkg/org/application/spectasks/`. Like `application/workers`
and `application/roles`, it owns the org-side use cases the tools call and
holds the policy that does **not** belong in a tool or in infrastructure:
extracting `orgID`/`workerID` from the caller, enforcing "a Worker only
touches its own project", mapping infra errors to friendly ones, and shaping
the tool-facing views. It depends only on the `runtime.SpecTasks` port (an
interface), never on `store.Store` or the helix services directly — so org
stays decoupled from the helix core exactly as it is today.

### 2. Infrastructure port `runtime.SpecTasks` — modelled on a human reviewer
The port lives beside `ProjectConfig` in
`api/pkg/org/infrastructure/runtime/runtime.go`. Its verbs mirror what a human
project manager does in the UI (review a spec, approve it, request changes,
approve PR creation) rather than generic CRUD:
```go
type SpecTasks interface {
    // Authoring
    Create(ctx, orgID, workerID, CreateSpecTaskInput) (SpecTaskView, error)
    List(ctx, orgID, workerID, ListSpecTasksFilter) ([]SpecTaskView, error)
    Get(ctx, orgID, workerID, taskID) (SpecTaskView, error)
    StartPlanning(ctx, orgID, workerID, taskID) (SpecTaskView, error) // begin spec generation

    // Reviewing — the human-in-the-loop actions
    ReviewSpec(ctx, orgID, workerID, taskID) (SpecReviewView, error)            // read generated requirements/design/tasks
    ApproveSpec(ctx, orgID, workerID, taskID) (SpecTaskView, error)             // spec_review -> spec_approved -> implementation
    RequestChanges(ctx, orgID, workerID, taskID, comment string) (SpecTaskView, error) // -> spec_revision
    CreatePullRequests(ctx, orgID, workerID, taskID) (SpecTaskView, error)      // happy with the code → tell the system to open PR(s)
}
```
The port resolves the project internally from worker state, so neither the
application service nor the tools handle a `projectID`. Plus `NoopSpecTasks`
(returns `ErrSpecTasksUnsupported`) and the view/input structs, all defined in
the `runtime` package so the application service and the impl share them.

### 3. In-proc impl `runtimehelix.SpecTasks` — reuse the canonical services, touch nothing
New file `api/pkg/org/infrastructure/runtime/helix/spectasks.go`. Mirrors
`project_config.go`: holds `*store.Store` + `*services.SpecDrivenTaskService`,
loads worker state to get `ProjectID` (error if empty), enforces that any
referenced task belongs to that project, then delegates each verb to the
**already-tested canonical `services` layer the REST UI uses** — so each is a
thin delegation, not a reimplemented state machine, and no existing code is
modified:

| Port method | Reuses (existing, unmodified helix code) |
|-------------|------------------------------------------|
| `Create` | `SpecDrivenTaskService.CreateTaskFromPrompt` (assigns task number + `services.GenerateDesignDocPath`) |
| `StartPlanning` | `SpecDrivenTaskService.StartSpecGeneration` / `StartJustDoItMode` (`startPlanning` handler) |
| `List` / `Get` | `store.Store` read methods (`ListSpecTasks` / `GetSpecTask`) directly |
| `ReviewSpec` | `getTaskSpecs` read logic (`/spec-tasks/{id}/specs`) + `listDesignReviews` |
| `ApproveSpec` | `SpecDrivenTaskService.ApproveSpecs` (`approveSpecs` handler) |
| `RequestChanges` | `submitDesignReview` with `Decision: "request_changes"` → `spec_revision` |
| `CreatePullRequests` | `approveImplementation` handler logic + the `EnsurePRsFunc` callback |

`CreatePullRequests` is the "I'm happy with the code, open the PR(s)" step —
it does **not** approve/merge on GitHub (that happens on GitHub itself). It
authorizes the system to open pull requests, and via `EnsurePRsFunc` it opens
**one PR per external repo attached to the project**, so a single call can
produce multiple PRs. The result view therefore lists all created PRs
(`RepoPullRequests`).

Wire it in `helix_org.go` next to `NewProjectConfig` (~line 427), inject it
into `spectasks.New(...)`, and set `deps.SpecTasks` (the application service)
before `RegisterBuiltins` (~line 590).

### 4. Do NOT touch the Optimus skill or the helix `spectask` code; TDD the new path
Earlier drafts proposed extracting the skill's create/list/get logic into a
shared package and refactoring `api/pkg/agent/skill/project/` to call it. We
**drop that** — it edits working code for no functional gain and risks
regressing the Optimus agent. Instead:

- The org port impl reuses the **already-canonical** `services` layer
  (`SpecDrivenTaskService`, `services.GenerateDesignDocPath`, `store.Store`) —
  the same code the REST UI drives. The Optimus skill package keeps its
  existing local copy of `generateDesignDocPath` (it deliberately duplicates it
  "to avoid import cycles" today); we leave it exactly as-is.
- **Zero edits** to `api/pkg/agent/skill/project/*` and **zero edits** to
  `api/pkg/services/spec_*`/`api/pkg/store` behaviour. The only allowed
  additive change to existing code is the optional `Publisher` sink on
  `AttentionService` (decision 7), which is purely additive and nil-guarded.
- **TDD:** write tests first at every new layer — the `runtimehelix.SpecTasks`
  impl (against the canonical services/store), the `spectasks.Service`
  application layer (fake port), and each tool (fake service). The acceptance
  bar is that existing helix/Optimus tests are untouched and still green
  because their code is untouched.

The minor cost is that the skill's local `generateDesignDocPath` copy and
`services.GenerateDesignDocPath` remain two copies — but that duplication
already exists by design and is not made worse by this work. Consolidating it
is a separate, optional cleanup, not a prerequisite here.

### 5. Org MCP tools
One file per verb in `api/pkg/org/interfaces/mcptools/`, each implementing
`tool.Tool` (`Name`, `Description`, `InputSchema`, `Invoke`), following
`get_worker_project.go`. They read the caller from `inv.Caller`, unmarshal
args, and call the **application service** (`deps.SpecTasks.<Verb>`) — not the
port directly — then JSON-marshal the result. Add `SpecTasks
*spectasks.Service` to `mcptools.Deps` (alongside `Roles`, `Workers`, …) and
build it in `Config.Build()` from the injected `runtime.SpecTasks` port
(default `runtime.NoopSpecTasks{}` in `DefaultDeps`). Register all of them in
`RegisterBuiltins`.

### 6. Authorization & scoping
The caller worker *is* the subject — there is no `workerId` argument. The
application service takes `orgID`/`workerID` from `inv.Caller` and the port
resolves the project from that Worker's own state, so a Worker can only manage
tasks in the project it is assigned to. Mutating/approving tools stay out of
`BaseReadTools`; owners grant them per-Role via `create_role` / `update_role`.

### 7. Spec-task event transport → topic → worker trigger
The tools above are the Worker's *hands*; this is its *ears*. Workers must be
triggered when spec tasks change state, reusing the existing eventing
machinery rather than inventing a new path.

**Event source = the UI notification system (`AttentionService`).** The
Helix UI's notifications are driven by `services.AttentionService`, which emits
a curated, idempotent `AttentionEvent` on each "human action needed" moment
(`specs_pushed`, `pr_ready`, `spec_failed`, `implementation_failed`,
`ci_passed`, `ci_failed`, `agent_interaction_completed`). These are exactly the
moments we want to trigger Workers on. We hook `AttentionService` rather than
the raw `store.SubscribeForTasks` pubsub (which fires on every field change and
is what the Slack *project-updates* trigger uses). Note `AttentionService`
already does a fire-and-forget Slack thread reply via `notifySlack`, so we add
the topic publish as a **third side-effect of the same emit**, alongside the
UI-row write and the Slack reply.

**New transport `KindSpecTask`.** Add `transport.KindSpecTask = "spectask"`
to `org/domain/transport/` (strategy map + `kindOrder`), inbound-only,
project-scoped. New infra package
`api/pkg/org/infrastructure/transports/spectask/` modelled on the Slack
transport (a topic whose events come from a long-lived server-side source, not
a per-topic external webhook).

**Emit → publish → dispatch.** Add a publish hook to `AttentionService` (a
narrow `Publisher`/sink interface, optional like its Slack dependency) so each
newly-created `AttentionEvent` is mapped to a `streaming.Message` and sent via
`Publishing.Publish(orgID, topicID, "", msg)` to the project's `KindSpecTask`
topic. Skip the publish on the idempotency-dedup path (same guard that already
skips Slack). `Publishing` persists the `streaming.Event` and hands it to the
`dispatch.Dispatcher`, which fans out **one activation per subscribed Worker**
— the identical trigger path Slack/GitHub topics use. No new dispatch code.

**Wiring.** Resolve `project → KindSpecTask topic` in the hook (auto-create the
topic with the project, as Slack auto-creates its workspace topic). Inject the
publisher into `AttentionService` in `server.go` where it is constructed
(~line 608), pointing at the org `Publishing` service. The event payload
carries task id + event type + new status so the activated Worker can act
through the spec-task MCP tools.

**Connecting a Worker.** A Worker subscribes to the project's spec-task topic
through the existing `subscribe` tool / `Subscriptions` service — no new
subscription mechanism. Owners (or a hire-time default) subscribe the Worker;
from then on every state change triggers it.

## Tool Surface (summary)

Named to read like the actions a human reviewer takes:

| Tool | Args | Effect |
|------|------|--------|
| `create_spectask` | name, description, type?, priority?, original_prompt?, skip_planning?, depends_on? | Create task in caller's project (status `backlog`) |
| `list_spectasks` | status?, priority?, type? | List tasks in caller's project |
| `get_spectask` | task_id | Read one task (must belong to project) |
| `start_spectask_planning` | task_id | Begin spec generation (or queue implementation if `skip_planning`) |
| `review_spectask_spec` | task_id | Read the generated requirements/design/tasks for review |
| `approve_spectask_spec` | task_id | Approve the spec → advances to implementation |
| `request_spectask_changes` | task_id, comment | Send the spec back for revision (`spec_revision`) |
| `create_spectask_prs` | task_id | Happy with the code → tell the system to open PR(s); one per attached repo (GitHub merge-approval still happens on GitHub) |

## Files Touched

- New: `api/pkg/org/application/spectasks/spectasks.go` (+ test) — the
  front-of-house application service the tools consume; depends only on the
  `runtime.SpecTasks` port.
- New: `api/pkg/org/infrastructure/runtime/helix/spectasks.go` (+ test) — port
  impl that reuses the canonical `services.SpecDrivenTaskService` + `store.Store`
  (no existing code changed). Workflow verbs delegate to that service + the
  design-review / approve-implementation paths.
- New: one tool file per verb in `api/pkg/org/interfaces/mcptools/` (+ tests).
- Edit: `runtime.go` (port + Noop + structs), `builtins.go` (`Deps.SpecTasks`
  application service + `Config.Build()` + RegisterBuiltins), `helix_org.go`
  (composition: build impl, inject into the application service).
- **Not touched:** `api/pkg/agent/skill/project/*` (the Optimus skill) and the
  existing `api/pkg/services/spec_*` / `api/pkg/store` behaviour — reused as-is,
  not refactored. No new `api/pkg/spectask/` extraction package.
- New (eventing): `api/pkg/org/domain/transport/spectask.go`
  (`KindSpecTask` + strategy/`kindOrder` entry) and
  `api/pkg/org/infrastructure/transports/spectask/` (maps `AttentionEvent` →
  `streaming.Message` and resolves the project's topic).
- Edit (eventing): `api/pkg/services/attention_service.go` — add an optional
  `Publisher` sink, published from `EmitEvent` after the idempotency check
  (beside `notifySlack`).
- Edit (eventing): `server.go` — inject the org `Publishing`-backed publisher
  into `AttentionService` at construction (~line 608).

## Risks / Gotchas

- **No regression by construction:** existing helix/Optimus code is reused
  unmodified (canonical `services` + `store`), so the design-doc/task-number
  behaviour cannot drift — there is no second implementation to keep in sync.
  New layers are TDD'd; existing tests stay green because their code is
  untouched.
- **Typed-nil ports:** `Config.Build()` must construct the `spectasks.Service`
  over a non-nil port, defaulting to `runtime.NoopSpecTasks{}` to avoid
  nil-interface panics (same care taken for `ProjectConfig`/`Dispatcher`).
- **Cross-project leakage:** never read a `projectID` from tool args; always
  derive from worker state and re-check task ownership on every verb.
- **Approver identity / GitHub OAuth (open decision):** `approveSpecs`,
  `submitDesignReview`, and `approveImplementation` validate the *human*
  approver's GitHub OAuth (`ValidateUserGitHubOAuth`) so their credentials
  drive commits and push during implementation. A Worker has no GitHub OAuth
  identity. The port must decide whose credentials are used when a Worker
  approves — most likely the Worker's hiring user (already persisted on the
  Worker's runtime state via `SaveHiringUser`) or the task creator. Resolve
  this before implementing `ApproveSpec` / `CreatePullRequests`; do not let an
  approval silently fall back to the wrong identity.
- **Multiple PRs:** `CreatePullRequests` opens one PR per external repo
  attached to the project (`EnsurePRsFunc`), so a single call can create
  several PRs. The tool's response must list every PR created, not assume one.
- **Trigger loops:** a Worker triggered by a spec-task event may itself mutate
  the task (e.g. `request_spectask_changes`), emitting another event. Ensure
  the event payload / Worker prompt distinguishes "act" vs "no-op" states so a
  Worker doesn't ping-pong. The dispatcher already serialises one activation
  per Worker, but the trigger semantics must avoid self-perpetuating cycles.
- **Source choice:** trigger off `AttentionService` (curated, idempotent,
  typed event set), **not** the raw `store.SubscribeForTasks` pubsub. The
  latter fires on every field write — noisy and loop-prone. AttentionService
  already gates on idempotency and represents "needs attention" moments, which
  is precisely the trigger semantics we want.
- **Optional dependency wiring:** the `AttentionService` publisher must be
  optional (nil → skip publish), matching how its Slack dependency is handled,
  so non-org deployments and tests don't require the org `Publishing` service.

## Follow-on (out of scope, noted): unify the two notification paths

There are currently **two independent spec-task notification systems** that
overlap:

1. The **Slack project-updates trigger** (`slack_project_updates.go`) — a
   `store.SubscribeForTasks` subscriber that fires on every change, with its
   own formatting (`buildProjectUpdateAttachment`, `humanizeSpecTaskStatus`)
   and thread bookkeeping.
2. The **AttentionService** (UI notifications) — curated `AttentionEvent`s with
   their own formatting (`buildTitle`, `buildDescription`, `eventEmoji`), which
   *also* posts Slack thread replies (`notifySlack`) — and in fact depends on
   the thread the path-1 flow created.

So spec-task changes drive two event sources, two formatting code paths, and
two interdependent Slack posters. This work adds a *third* consumer
(worker triggers) but deliberately hangs it off AttentionService so we don't
add a fourth source.

A worthwhile **follow-on** (separate task) is to make AttentionService the
single curated source and re-express the Slack project-updates trigger and the
worker-trigger transport (and the UI) as consumers of the same
`AttentionEvent` stream — collapsing the duplicated event detection and
formatting. Not in scope here; flagged so it isn't lost.
