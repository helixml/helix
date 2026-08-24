# Agent restart-required banner

## Problem

Editing a running org Bot's configuration does not reach its sandbox. The
clearest case is the MCP tool list: the helix-org MCP handler is
`Stateless: true` and rebuilds the server per request from the Bot's live
`Tools` (`interfaces/server/mcp.go`), but the coding agent inside the
container fetched `tools/list` once at MCP client init and cached it. Grant
a tool and the agent never sees it; revoke one and the agent calls it and
gets an unknown-tool error.

This is a staleness gap, not an authorization hole — `registerToolForBot`
only binds granted names, so a revoked-but-cached tool is simply not
registered and the call fails.

There is no signal anywhere that a restart is needed. `notifications/tools/list_changed`
has zero occurrences in the repo, the settings-sync-daemon switches only on
`agent` and `agent_restart` (`api/cmd/settings-sync-daemon/main.go`), and no
frontend surface mentions restarting.

## Scope

Org Bots only. Helix Agents (Apps), project settings, and provider
endpoints are out of scope.

## Not every setting needs a restart

Instrumenting every edit would make the banner fire mostly on changes that
need nothing, and people would learn to ignore it. What a running bot
sandbox actually consumes:

| Setting | How it reaches the sandbox | Restart? |
|---|---|---|
| `Tools` | Agent caches `tools/list` at MCP client init; nothing pushes an update | **Yes** |
| `Content` (instructions) | `SyncAgentProfile` writes `AGENTS.md`/`CLAUDE.md` into the live container — but only on *activation*, never on save (`server/helix_org_inproc.go`) | **Yes**, for the chat session |
| Runtime / model / provider / effort | Already hot-switched: the bot page calls `useSwitchAgent`, and the daemon's `field=agent` path hot-reloads `settings.json` | No |
| Worker secrets | Fetched live through the `get_secret` MCP tool, not injected as container env | No |
| Project allowlist, preserve-context, triggers, name | Evaluated server-side per request/dispatch | No |

So the fingerprint covers exactly `Tools` + `Content`. Adding a field later
is a one-line edit in one function.

## Mechanism

Derived, computed from state that already exists. Nothing is added to
`SessionMetadata` or any other core Helix type, hydra is untouched, and
nothing is written during a GET. The only new fields are the two org-layer
DTO fields that carry the result to the UI.

### The fingerprint

```go
// api/pkg/org/domain/orgchart/restart.go
// RestartFingerprint hashes exactly the Node config a running sandbox
// consumes at startup and that nothing hot-applies afterwards.
func RestartFingerprint(n Node) string // sha256 over sorted Tools + Content
```

Sorting `Tools` makes reordering a no-op. Excluding `Name`,
`PreserveContext`, and `ProjectIDs` means editing them raises no banner.

### Stamp on save

`Nodes.Update`, `AttachTools`, and `DetachTools` already funnel into
`notifyToolsChanged` → `publishAgentToolChange` →
`publishAgentConfigChange(session, "tools")`. That event is already
published for every session linked to the bot's app, and currently goes
into a void because the daemon ignores `"tools"`.

`Nodes` holds both `existing` and `updated`, so it compares fingerprints
directly — no stored previous value. When they differ it calls a new narrow
injected hook:

```go
OnRestartRequired func(context.Context, string, orgchart.NodeID)
```

`Nodes` depends only on narrow interfaces, never `*store.Store`, so the hook
is wired at the composition root (`server/helix_org.go`) to a function that
reads the bot's session and writes into `NodeRuntimeState`:

```
restart_required_container = session.Metadata.ContainerID
```

Comparing fingerprints (rather than "was `Tools` in the patch") also tightens
the existing `if p.Tools != nil` guard: `sameToolList` compares element by
element, so it suppresses only a byte-identical resubmit. A pure reorder of
the same tool set still publishes `onToolsChanged` (fingerprint hashing is
what makes the *restart* signal reorder-insensitive; the older tools-changed
notifier is not).

### Read is pure

In `orgWorkerRuntime.State()` (`server/helix_org.go`), which every bot read
already goes through and which already loads the session:

```
RestartRequired = AgentStatus == "running"
               && stamped != ""
               && stamped == session.Metadata.ContainerID
```

### Why `ContainerID`

`Metadata.ContainerID` is the Docker container ID, already maintained by the
existing stack: set on container create (`hydra_executor.go:639`) and on
reconcile/resume (`:1895`), cleared on teardown (`:2073`, `:2232`). Docker
never reuses an ID, so it is a per-container generation marker.

That gives self-clearing behaviour on every path, with no clearing code
anywhere:

| Event | ContainerID | Result |
|---|---|---|
| Edit while running (C1) | C1 | stamp C1 == C1 → **banner** |
| Full restart (banner action) | C2 | C1 ≠ C2 → clears |
| Stop → start | `""` → C2 | clears |
| Idle reap → resume | `""` → C2 | clears |
| Crash → reconcile | C2 | clears |
| Edit while stopped | `""` | stamp `""`, read requires non-empty → no banner |

The last row makes the write self-guarding: the save hook needs no
"is it running" check.

### Alternatives rejected

- **Explicit dirty flag set by each mutation handler.** Every current and
  future write path has to remember. That is the failure mode that produced
  the stale tool list.
- **Timestamp comparison on `UpdatedAt`.** Any no-op edit (renaming a bot)
  falsely nags.
- **Adopt-on-observe** (stamp written lazily during `State()`). Robust, but
  it writes during a GET.
- **Clear-on-stop boolean.** `ExternalAgentStatus` is written at ~10
  scattered sites (idle reaper, crash reconciler, stop handler, project
  handler, hydra executor). Clearing on the org stop path alone misses
  idle-reap-then-resume, leaving a permanently wrong banner.
- **New `ExternalAgentStartedAt` on `SessionMetadata`.** Works, but requires
  editing hydra. `ContainerID` already carries the same generation signal.

### Known coupling

This ties an org-layer flag to a Helix core metadata field. If `ContainerID`
is repurposed or stops being per-container, the banner breaks silently. Pin
it with a test asserting the invariant and a comment at both ends.

Verify during implementation that `ContainerID` is reliably populated for org
bot sessions. If it is ever empty on a running sandbox the banner silently
never fires — the failure direction this work exists to eliminate.

## Restart semantics: full fresh session

The banner's action reuses the existing `restart-agent` endpoint
(`ResetSession` → stop desktop, delete session row, clear session pointer,
then Activate), which mints a brand-new session and thread.

A preserved thread would be wrong here. Its transcript still contains
successful `tool_use`/`tool_result` blocks for a tool that no longer exists.
The restarted container does fetch a correct `tools/list`, so availability is
fine — but the model reads its own history as proof of capability, retries
the removed tool, and errors. Restarting to fix a stale tool list and then
handing the agent a transcript that re-teaches the stale list defeats the
purpose.

This also matches the runtime's existing norm: `ensureSession` already calls
`ClearSession` before every re-activation unless `PreserveContext` is set
(`infrastructure/runtime/helix/spawner.go`). Chat history is already treated
as disposable.

Blast radius: a bot's durable state lives in the per-Worker Helix project's
git repo at `workers/<id>/.context/` on the `helix-specs` branch
(`infrastructure/runtime/helix/workspace.go`). `ResetSession` deletes the
session row and the container; the project and repo survive. A restart costs
the transcript and uncommitted container-local scratch, not the workspace.

### Guard rails

The one thing genuinely at risk is an in-flight turn killed mid-work, so the
guard rails carry that weight:

- **Never restart automatically.** The banner only offers.
- **Mid-turn restart is gated, within what the browser tab can see.** The
  button is disabled with a tooltip while `streaming.currentResponses` shows
  a turn in flight for the open session — the local streaming context of
  *this* tab. There is no server-side "agent is working" signal to key off,
  so a turn started by a Slack/topic trigger, by another operator, or in
  another browser tab leaves the button enabled: the gate only covers what
  this tab is actively streaming. The confirm dialog is the backstop for the
  turns the gate cannot see.
- **Confirm dialog states the cost**: "Restarts the sandbox with a fresh
  conversation. The workspace and committed work are kept; the current chat
  history is discarded."
- For `PreserveContext` bots an explicit banner restart deliberately
  overrides that setting — correct, and another reason it stays
  user-initiated.

## UI

`BotDTO.RestartRequired bool`, populated in `getBot` and `listBots` from a
new `BotRuntimeInfo.RestartRequired`.

One component, `AgentRestartRequiredBanner`, modelled on the existing
`components/session/AgentOfflineNotice.tsx`. It reads `restart_required` from
`useHelixOrgBot(botId)` — both mount points already call that hook, so it is
a prop-free drop-in.

1. **`pages/HelixOrgBotDetail.tsx`** — under the header. Appears on save; the
   page already refetches, and the server stays the single source of truth.
2. **`components/helix-org/HelixOrgChatPanel.tsx`** — above the composer, to
   catch people who never return to the config page.

A low-profile, non-blocking info strip: "Tool and instruction changes apply
after a restart", a **Restart sandbox** button, and a **Not now** that hides
it for the current browser tab only. It is not permanently dismissible — the point is to
keep catching people who have not restarted.

## Files

| Piece | Where |
|---|---|
| `RestartFingerprint(Node)` | new `api/pkg/org/domain/orgchart/restart.go` |
| Fingerprint comparison + `OnRestartRequired` hook | `api/pkg/org/application/nodes/nodes.go` |
| Hook wiring → `NodeRuntimeState` stamp | `api/pkg/server/helix_org.go` |
| Stamp accessors | `api/pkg/org/infrastructure/runtime/helix/state.go` |
| `RestartRequired` derivation | `orgWorkerRuntime.State()` in `api/pkg/server/helix_org.go` |
| `BotRuntimeInfo.RestartRequired` | `api/pkg/org/interfaces/server/api/api.go` |
| `BotDTO.RestartRequired` | `api/pkg/org/interfaces/server/api/dto.go`, `bots.go` |
| `AgentRestartRequiredBanner` | new `frontend/src/components/helix-org/` |
| Mount points | `HelixOrgBotDetail.tsx`, `HelixOrgChatPanel.tsx` |

## Testing

- `orgchart`: fingerprint stable across `Tools` reordering; changes on tool
  add/remove and on content edit; unchanged when `Name`, `PreserveContext`,
  or `ProjectIDs` change.
- `nodes`: `Update`/`AttachTools`/`DetachTools` fire `OnRestartRequired` only
  when the fingerprint changed; a no-op tool patch fires nothing.
- `server`: running + stamp matching `ContainerID` → `RestartRequired=true`;
  stopped → false; stamp from a previous container → false; no stamp → false.
- `bots.go`: `RestartRequired` reaches `BotDTO` on both `GET /bots/{id}` and
  `GET /bots`.
- Frontend (vitest): banner renders iff `restart_required`; the restart
  handler calls `restart-agent`; it is never called without confirmation; the
  button is disabled mid-turn.
- End-to-end in the inner Helix: create a bot, start it, edit its tools,
  confirm the banner appears on both surfaces, restart, confirm it clears and
  the agent sees the new tool.

## Out of scope

`notifications/tools/list_changed`. That is the real fix for tool staleness
and would make the banner unnecessary for tools, but it needs agent-side
support in Zed and Claude Code. Worth its own issue.

## Verification

Run 2026-08-24 against the live dev stack at `localhost:8080` (API confirmed
serving this branch's code by grepping the running binary for
`restart_required_container` and the Task-4 fix log string). Exercised through
the real REST API against real sandbox containers — not seeded rows, not mocks.

### The load-bearing assumption holds

Every org-bot session in the database carries a populated, unique 64-hex Docker
container id in `Metadata.ContainerID`. This is what the whole mechanism rests
on; had it been empty on a running sandbox, the banner would silently never
fire and the design would need revisiting.

### Every branch, verified live

| Case | Observed | Verdict |
|---|---|---|
| Edit tools, sandbox stopped | stamp written = that session's container id; `restart_required` absent | running-only gate holds |
| Sandbox started, new container | stamp `4e610047…` ≠ live `a0a1ae02…`; `restart_required` false | **self-clears on recreate, with no clearing code** |
| Edit tools while running | stamp becomes live `a0a1ae02…`; `restart_required: true` | true case |
| Tools restored to original | tool list byte-identical to before | no test residue |

The self-clearing row is the important one: a genuine container recreate cleared
a stale stamp without any code clearing it, which is the design's central claim.

### Known limitation found during verification

**Reverting a change does not clear the banner.** Edit config → revert it to
exactly what the running sandbox already has → the banner still shows until a
restart. The comparison asks "did restart-sensitive config change since this
container started", not "does config differ from what the container holds".

This fails toward nagging rather than toward silence, and a restart clears it,
so it is left as-is. Fixing it would mean stamping a fingerprint of the applied
config at container start — which requires the container to report what it
booted with, the heavier design deliberately deferred above.

## App-side instructions: only the REST update path is wired

A Bot's instructions are actually read from the App, not the Node —
`canonicalAgentProfile` overwrites `BotDTO.Content` with
`AgentProfile.Instructions`, which maps to
`app.Config.Helix.Assistants[0].SystemPrompt`. That meant editing
instructions from the Agent settings page (`/orgs/:org_id/agents/:app_id`)
changed the App only: the Node's `Content` fingerprint never moved, so the
mechanism above never armed. Fixed by adding a second, narrower hook,
`HelixAPIServer.orgAgentInstructionsChanged`, called from
`api/pkg/server/app_handlers.go` after `updateAgent`'s `Store.UpdateApp`
succeeds, gated on the system prompt actually changing. It resolves the
App to its backing Node (`stampRestartRequiredForApp` in
`api/pkg/server/helix_org.go`, matching `Node.AgentID == app.ID` across the
App's org) and reuses the same `stampRestartRequiredContainer` the Node
fingerprint path calls — one stamping implementation, two arming paths.

**Only the `s.updateAgent` HTTP handler is hooked — every path that funnels
through it is covered, everything that calls `Store.UpdateApp` directly is
not.** `Store.UpdateApp` has nine call sites across `api/pkg/server/`.
Checked each for whether it can touch `Assistants[0].SystemPrompt`:

- `app_handlers.go`'s `updateAgent` (REST `PUT /api/v1/agents/{id}`) — the
  hooked path this change adds.
- `helix_org_inproc.go`'s `inProcHelixClient.UpdateAgent` /
  `UpdateAgentContent` — used by the `set_bot_content` MCP tool among
  others — do **not** bypass the hook: both build an HTTP request in-process
  and call `c.server.updateAgent(nil, r)` directly, so they run through the
  exact same code this change instruments. (`set_bot_content` also updates
  `Node.Content` first, which was already covered by the pre-existing
  fingerprint mechanism — this call is belt-and-suspenders, not the
  interesting case.)
- `helix_org_inproc.go`'s `UpdateAppConfig` (`app.Config = cfg;
  Store.UpdateApp(...)`) **is** a real bypass: it writes the whole config,
  including `SystemPrompt`, straight to the store with no hook and no
  diff check. Its only current caller, `RemoveHelixOrgMCP`
  (`org/infrastructure/runtime/helix/mcp.go`), only ever touches `MCPs`, so
  nothing reachable today silently slips through — but the seam itself is
  unguarded. A future caller of `UpdateAppConfig` that changes the system
  prompt will not arm the restart-required flag.
- `helix_org_inproc.go`'s `ApplyAgentDefaults` and `markAgentAppAsOrgKind`,
  `app_handlers.go`'s avatar upload/delete handlers, `skills.go`'s MCP-skill
  writer, `zed_config_handlers.go`'s provider-ref healer, and
  `project_handlers.go`'s skill-preservation sync never write
  `SystemPrompt` at all (they touch `CodeAgentRuntime`/`Provider`/`Model`,
  `AgentKind`, `Avatar`, `MCPs`, provider refs, and `Tools` respectively) —
  they carry no restart-staleness risk today, hooked or not.

So the honest scope of the remaining gap is exactly `UpdateAppConfig`, not
every other `UpdateApp` call site. Closing it would mean either adding the
same prompt-diff check there, or moving the check into `Store.UpdateApp`
itself so no call site can bypass it structurally; neither was attempted —
out of scope for this change.

### Not verified by this run

- **The browser click-path.** The banner's rendering, the confirm dialog, the
  mid-turn gate, and the restart button were verified by component and panel
  tests (73 frontend tests) and by the API returning `restart_required` on both
  the list and detail endpoints — but no one drove the actual UI in a browser.
- **The post-restart agent behaviour** — that an agent picks up a newly granted
  tool after the banner's restart — was not exercised.
