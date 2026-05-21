# Step 1 — Reconnaissance and Inventory

Factual map of `helix-org/` as it sits on `main` (commit `ee0ecf976`). No
recommendations here; just what is in the box.

A note up-front about git churn: `helix-org/` was merged into the
parent `helix` monorepo on 2026-05-19 (PR #2467, "consolidate helix-org
into a single module"). Before that it lived in a separate repo whose
history is not in this tree. The `--since` churn map in §5 is therefore
mostly flat — it reflects the last week of this directory's life under
this `.git`, not the file's true age or activity. I call out the files
that look load-bearing based on size + structural centrality instead.

---

## 1. Entry points

The single process binary `helix-org` has four CLI subcommands. Only
`serve` opens long-lived sockets; the others are short-lived clients.

### 1.1 CLI subcommands

| Subcommand   | File / line                       | What it does |
|---           |---                                |---           |
| `serve`      | `cmd/helix-org/serve.go:36`       | Opens SQLite, runs `bootstrap.Run` once, builds spawner + dispatcher + transports + UI handler + chat backend, listens on `--addr`. The whole "production" wiring. |
| `chat`       | `cmd/helix-org/chat.go:33`        | `syscall.Exec`s the `claude` CLI with an inline `--mcp-config` pointing at `<server-url>/workers/<id>/mcp`. Default worker `w-owner`. Restores latest per-cwd claude session via `latestClaudeSessionID()` (`chat.go:121`). |
| `bootstrap`  | `cmd/helix-org/bootstrap.go:16`   | Currently has one target, `helix-runtime`: opens the DB directly (`openRegistry`), reads `helix.url`/`helix.api_key`, pings `/api/v1/whoami`, validates `chat.provider`/`chat.model` against Helix. Otherwise a no-op. The dead `sandboxStartupSh` constant + `persistString` are kept alive by a `_ = …` line (`bootstrap.go:139-140`). |
| `config`     | `cmd/helix-org/config.go`         | `set`/`get`/`list`/`delete` against the in-DB `configs` table via `config.Registry`. |

### 1.2 HTTP routes

All HTTP routes are mounted via the single `http.ServeMux` built in
`server.Server.Handler` (`server/server.go:73`), plus the "extras"
passed in from `cmd/helix-org/serve.go:187-197`. There is no central
router file; the route list is split across three packages.

| Method + pattern                  | Mounted at (file:line)             | Handler                                  | Notes |
|---                                |---                                 |---                                       |---    |
| `ANY  /workers/{id}/mcp`          | `server/server.go:75`              | `Server.mcpHandler` → `mcp.NewStreamableHTTPHandler` (`server/mcp.go:25`) | The headline endpoint. Per-request: read worker, read grants, build a fresh `*mcp.Server` with only the granted tools registered (`server/mcp.go:67`). Stateless. `Authorization: Bearer` and `X-Helix-Org-User-Id` are hoisted onto request ctx via `helixclient.WithBearerToken` / `helixclient.WithUserID`. |
| `POST /webhooks/{streamID}`       | `server/server.go:76`              | `Server.webhookHandler` (`server/webhook.go:33`) | Body becomes one `domain.Event` on the stream. Stream must have `transport.kind == webhook`. Calls `broadcaster.Notify` + `dispatcher.Dispatch`. 1 MiB body cap (`webhook.go:23`). |
| `POST /email/postmark`            | `serve.go:188`                     | `postmark.Transport.HandleInbound` (`transports/postmark/postmark.go:239`) | Postmark inbound JSON → Stream lookup by alias → append Event. Outbound is registered the other way: `dispatcher.SetEmailEmitter(emailTransport)` (`serve.go:112`). |
| `POST /github/webhook`            | `serve.go:189`                     | `github.Transport.HandleInbound` (`transports/github/github.go:137`) | HMAC-SHA256 verify, fan out to every Stream whose `repo` + `events` match `X-GitHub-Event`. Inbound only — outbound deliberately not supported (`github.go:21`). |
| `GET  /ui/chat/stream`            | `serve.go:190`                     | `chat.Backend.StreamHandler` (`server/chat/chat.go:296` or `helix_bridge.go`) | SSE; HTML fragment per claude stream-json line. |
| `POST /ui/chat/send`              | `serve.go:191`                     | `chat.Backend.SendHandler` (`server/chat/chat.go:466`) | Form POST; rewrites `/<prompt>` to the rendered prompt body before forwarding. |
| `POST /ui/chat/commands`          | `serve.go:192`                     | `chat.Backend.CommandsHandler` (`server/chat/chat.go:556`) | Slash-command typeahead. |
| `POST /ui/chat/new`               | `serve.go:193`                     | `chat.Backend.NewHandler` (`server/chat/chat.go:357`) | Kill subprocess, fresh session. |
| `POST /ui/chat/switch`            | `serve.go:194`                     | `chat.Backend.SwitchHandler` (`server/chat/chat.go:425`) | Resume a particular `--resume` sid. |
| `GET  /ui/{$}`                    | `server/ui/ui.go:72`               | `handleChat` | Renders chat page shell. |
| `GET  /ui/org`                    | `ui.go:73`                         | `handleOrg` | Org chart page. |
| `GET  /ui/org/chart`              | `ui.go:74`                         | `handleOrgChart` | Standalone chart fragment. |
| `GET  /ui/settings`               | `ui.go:75`                         | `handleSettings` | Settings page using `SettingsView`. |
| `POST /ui/settings/set`           | `ui.go:76`                         | `handleSettingsSet` | Calls `config.Registry.Set`. |
| `POST /ui/settings/delete`        | `ui.go:77`                         | `handleSettingsDelete` | |
| `POST /ui/org/roles/set`          | `ui.go:78`                         | `handleOrgRoleSet` | Mutates roles directly through the store — bypasses MCP (see findings). |
| `GET  /ui/org/detail`             | `ui.go:79`                         | `handleOrgDetail` | |
| `POST /ui/org/identity/set`       | `ui.go:80`                         | `handleOrgIdentitySet` | Mutates identity directly through the store — bypasses MCP. |
| `GET  /ui/streams`                | `ui.go:81`                         | `handleStreams` | |
| `GET  /ui/streams/events`         | `ui.go:82`                         | `handleStreamsEventsSSE` | SSE feed reading via store + broadcaster. |
| `POST /ui/streams/publish`        | `ui.go:83`                         | `handleStreamsPublish` | Owner-side publish — also bypasses MCP. |
| `GET  /{$}`                       | `serve.go:196`                     | `http.Redirect → /ui/` | |

### 1.3 MCP tool registrations

`tools.RegisterBuiltins` (`tools/builtins.go:77`) registers a fixed
list. The catalogue is exactly this:

Mutations: `create_role`, `update_role`, `update_identity`,
`create_position`, `hire_worker`, `grant_tool`, `revoke_tool`,
`create_stream`, `stream_members`, `subscribe`, `unsubscribe`,
`invite_workers`, `publish`, `dm` (14 tools).

Reads: `list_roles`, `get_role`, `list_positions`, `get_position`,
`list_position_children`, `list_workers`, `get_worker`,
`list_worker_grants`, `get_worker_environment`, `list_streams`,
`get_stream`, `list_stream_events`, `get_grant`, `read_events`,
`worker_log` (15 tools).

29 tools total. Each tool name is a package-level `const` so the
bootstrap grant list and the registry stay in lockstep
(`tools/*.go` and `bootstrap/bootstrap.go:109-141`).

`tools.Deps` (`tools/builtins.go:51`) is the dependency bundle every
tool receives:

```
Store, Now, NewID, EnvsDir, Broadcaster, Dispatcher (EventDispatcher), Workspace (agent.WorkspaceSync)
```

`Dispatcher` is declared as a local `EventDispatcher` interface
(`tools/builtins.go:27`) specifically to dodge the `dispatch` →
`tools` → `dispatch` cycle (dispatcher itself imports tools? It does
not — see §4).

MCP prompts (slash-commands) are registered separately via
`prompts.RegisterBuiltins` (`prompts/builtins.go:13`). Today there
are exactly two: `role` and `help` — i.e. this surface has not been
exercised much.

### 1.4 Goroutines, queues, scheduled jobs

| Source                                       | What it does |
|---                                           |---           |
| `dispatch.Dispatcher.queues sync.Map` of `*workerQueue` (`dispatch/dispatcher.go:63-79`) | Per-Worker activation queue. A single runner goroutine per Worker drains the queue and calls the Spawner; new triggers arriving while running are coalesced into the next batch (`enqueue` / `run`). |
| Outbound webhook POSTs `go d.postOutbound(...)` (`dispatcher.go:295`) | One goroutine per outbound webhook delivery. Background context. 5 s `http.Client` timeout. |
| Outbound email emit `go func() { d.emailEmitter.Emit(...) }` (`dispatcher.go:300`) | One goroutine per outbound email. |
| `claude.Spawner` stderr drain (`agent/claude/spawner.go:169`) | Per-activation goroutine reading claude's stderr. |
| `helix.Spawner` (`agent/helix/spawner.go:98`) | Bounded semaphore (`MaxInflight`, default 8) gating concurrent Helix activations. Each activation opens a Helix WebSocket and polls for completion. |
| `broadcast.Broadcaster` subscriber channels (`broadcast/broadcaster.go`) | In-process pub/sub keyed by `domain.StreamID`. Long-poll readers in tools (e.g. `read_events` with `wait=…`) and UI SSE handlers block on these. |
| `chat.Bridge` claude subprocess pump (`server/chat/chat.go`) | One long-lived `claude` subprocess for the owner-chat UI, in the server's cwd. SSE fan-out from its stream-json stdout. |
| `chat.HelixBridge` per-session WebSocket pump (`server/chat/helix_bridge.go`) | Same as above but driven via a Helix chat session WebSocket instead of a subprocess. |

There is no cron, no background scheduler, no in-process tick generator
beyond the per-runtime poll loops in `agent/helix/spawner.go`.

### 1.5 Webhook receivers (summary)

Three inbound HTTP receivers (per §1.2): the generic
`POST /webhooks/{streamID}`, the Postmark inbound `POST /email/postmark`,
and the GitHub inbound `POST /github/webhook`. All three terminate in
`store.Events.Append` + `broadcaster.Notify` + `dispatcher.Dispatch`.

---

## 2. External integrations

| Integration      | SDK / client | Constructed in (file:line) | Called from (callers) | Touches domain code? |
|---               |---           |---                         |---                    |---                   |
| `claude` CLI (Anthropic, local subprocess) | `os/exec` | `agent/claude/spawner.go:124` (per activation), `server/chat/chat.go` (long-lived for owner UI), `cmd/helix-org/chat.go:106` (`syscall.Exec`) | Spawner for AI Workers, Bridge for owner UI, `helix-org chat` for interactive humans | Yes — the spawner is part of `agent/claude` and the UI bridge is part of `server/chat`. The domain knows nothing about claude, but the spawner reads `store.Workers/Roles/Positions/Environments` directly to project files (`agent/claude/spawner.go:197`). |
| Helix parent API (`helixclient`) | Hand-rolled REST + WebSocket client in `helix/helixclient/client.go` (1308 LOC, 13 methods on the `Client` interface) | Once per process in `cmd/helix-org/serve.go:291` (spawner path) and `:388` (chat backend path). A second instance also constructed inside `cmd/helix-org/bootstrap.go:63` for pre-flight ping. | `agent/helix/spawner.go`, `agent/helix/project.go`, `agent/helix/workspace.go`, `server/chat/helix_bridge.go`, `server/mcp.go` (only via `helixclient.WithBearerToken`/`WithUserID` context helpers), `tools/hire_worker.go`. | Yes — `tools/hire_worker.go` imports both `agent/helix` and `helix/helixclient` directly. So does `server/chat/helix_bridge.go`. The "domain layer" `tools/` package therefore has knowledge of the Helix runtime backend baked in. |
| SQLite via GORM | `github.com/glebarez/sqlite` + `gorm.io/gorm` | `store/sqlite/sqlite.go:Open` (called once in `serve.go:61`) | All `store/sqlite/*.go` repository implementations | No — `store/sqlite` is the only package that imports GORM. Domain types in `domain/` have GORM tags but the domain package itself does not import GORM (checked via `go list`). |
| GitHub webhooks (inbound) | `crypto/hmac`, `crypto/sha256` (no GitHub SDK) | `transports/github/github.go` | `POST /github/webhook` only | Verifies HMAC, builds Events directly. Does not call the GitHub API. The doc string explicitly defers outbound to Workers' own `gh` CLI. |
| Postmark (email, in + out) | Plain `net/http` against `api.postmarkapp.com` | `transports/postmark/postmark.go` | Inbound via the route; outbound via `dispatcher.SetEmailEmitter(emailTransport)` (`serve.go:112`) | Self-contained module, talks only to `config.Registry` + `store.Streams/Events` + `broadcast`. |
| `modelcontextprotocol/go-sdk` (MCP server) | `mcp.NewStreamableHTTPHandler`, `mcp.NewServer` | `server/mcp.go:25,86` | The `/workers/{id}/mcp` route. | Domain code does not import this SDK; only `server/` does. Good. |
| `gorilla/websocket` | `helix/helixclient/client.go` | Live-session WebSocket against Helix. | Only the helixclient. |
| `tylermmorton/tmpl` + `yuin/goldmark` | UI page rendering / markdown render | `server/ui/pages.go`, `server/chat/render.go` | UI handlers only. |

There is no direct integration with any LLM provider (OpenAI, Anthropic
API, etc.) — every model call is delegated to either the local `claude`
CLI subprocess or to the parent Helix server.

---

## 3. Deployment topology

One Go binary, one process, one SQLite file. From there the topology
fans out into subprocesses and external HTTP loops.

```
                       ┌────────────────────────────────────────────────┐
                       │             helix-org (one process)            │
                       │                                                │
   HTTP :8080  ───►    │  mux  ──► /workers/{id}/mcp  ──► tools/*       │
                       │       ──► /webhooks/{streamID}                 │
                       │       ──► /email/postmark, /github/webhook     │
                       │       ──► /ui/* (SSE chat, settings, org…)     │
                       │                                                │
                       │   dispatch.Dispatcher (per-Worker queues) ──►  │
                       │     agent.Spawner (claude OR helix backend)    │
                       │   broadcast.Broadcaster (in-proc pub/sub)      │
                       │                                                │
                       │   store.Store ──► SQLite file (helix-org.db)   │
                       └────────────────┬────────────────┬──────────────┘
                                        │                │
              ┌─────────────────────────┘                └─────────────────────┐
              ▼                                                                ▼
    (a) claude spawner backend                                  (b) helix spawner backend
        exec `claude -p` per activation in                          POST /sessions, WebSocket
        envs/<workerID>/  with --mcp-config                         to Helix parent API; runs
        pointing back at our own                                    a Helix session per Worker
        /workers/{workerID}/mcp                                     project (one project per Worker)
              │                                                                │
              │ (claude makes MCP tool calls back into our                     │
              │  /workers/{id}/mcp endpoint over HTTP)                         │
              ▼                                                                ▼
       loop closes inside one process                            loop closes via parent Helix
                                                                 + its in-sandbox agent → MCP
```

The `helix-org chat` subcommand is the human-driving equivalent of the
claude spawner path: it `syscall.Exec`s a `claude` CLI in the user's
terminal with an inline mcp-config pointing at
`http://localhost:8080/workers/w-owner/mcp`. So a human typing into
`claude` makes MCP calls back into the same running `helix-org serve`
process. (`cmd/helix-org/chat.go:56-110`.)

Identical loop in-process for the UI: `server/chat/chat.go` keeps a
long-lived `claude` subprocess as the owner-chat backend; its stdout is
SSE-fanned to browsers, its stdin is fed POSTs from `/ui/chat/send`. It
is essentially "the `helix-org chat` subprocess, but in a tab".

So `claude` is run in three different ways, all of which terminate in
MCP calls back to this same process:

1. AI Worker activation (one process per activation, exit-bound, in the
   Worker's env dir) — `agent/claude/spawner.go`.
2. Owner-chat UI bridge (one long-lived process in server cwd) —
   `server/chat/chat.go`.
3. `helix-org chat` interactive (foreground exec, in the user's cwd) —
   `cmd/helix-org/chat.go`.

There is no separate worker process, no message broker, no Redis. All
fan-out is in-process via goroutines + `sync.Map`. SQLite is the only
durable store; if the process dies, in-flight activations die with it
(no retry queue).

---

## 4. Static dependency overview

19 internal packages (excluding tests). Edges resolved via
`go list -deps -f '{{.ImportPath}} → {{.Imports}}'`.

### 4.1 Importer count (who is depended on by how many other helix-org packages)

| Package | Internal importers | Notes |
|---|---:|---|
| `domain`            | 16 | The shared kernel. Every package except `broadcast`-only-internal types imports it. Carries enum + types AND non-trivial transport logic (`domain/transport.go`, 315 LOC). |
| `store`             | 13 | Interface-only package; concrete impl in `store/sqlite` is imported by exactly one place (`cmd/helix-org/serve.go`). |
| `broadcast`         | 9  | Tiny pub/sub. Imported widely because nearly every write path notifies it. |
| `agent`             | 6  | Spawner contract + activation policy + the `WorkspaceSync` interface. |
| `helix/helixclient` | 5  | Anywhere that touches Helix. |
| `tools`             | 4  | `bootstrap`, `cmd`, `prompts`, `server`. (Yes, `prompts` imports `tools` for tool-name constants.) |
| `config`            | 4  | `cmd`, `server/ui`, `transports/{github,postmark}`. |
| `agent/helix`       | 3  | `cmd`, `tools` (hire_worker), `server/chat`. |
| `prompts`           | 3  | `cmd`, `server`, `server/chat`. |
| `server/chat`       | 2  | `cmd`, `server/ui`. |
| `dispatch`          | 2  | `cmd`, `server/ui`. |
| Everything else     | ≤1 | Leaf-ish. |

Nothing imports `cmd/helix-org` (entry point, as expected).

### 4.2 Importer count from the other side (how many packages each one pulls in)

| Package | Internal imports (raw count) |
|---|---:|
| `cmd/helix-org`        | 18 — the wiring god-set |
| `server/chat`          |  4 — but reaches into `agent/helix`, `helix/helixclient`, `prompts`, `domain` |
| `tools`                |  6 — including `agent/helix` and `helix/helixclient` (problematic; see findings) |
| `server/ui`            |  6 — `broadcast`, `config`, `dispatch`, `domain`, `server/chat`, `store` |
| `server`               |  5 — `broadcast`, `domain`, `helix/helixclient`, `prompts`, `store`, `tools` |
| `agent/helix`          |  4 — `agent`, `broadcast`, `domain`, `helixclient`, `store` |
| `bootstrap`            |  3 — `agent`, `domain`, `store`, `tools` |
| `dispatch`             |  3 — `agent`, `domain`, `store` |

### 4.3 Diagram (internal edges only, primary direction of control)

```
                  ┌─────────────────────────────────┐
                  │       cmd/helix-org (wiring)    │
                  └─────────┬──────────────┬────────┘
                            │              │
              ┌─────────────┼──────────────┼──────────────┐
              ▼             ▼              ▼              ▼
        ┌──────────┐  ┌──────────┐  ┌────────────┐  ┌──────────┐
        │  server  │  │ dispatch │  │ server/ui  │  │ transports│
        │          │  │          │  │            │  │ (gh, pm)  │
        └─┬───┬────┘  └──┬───────┘  └──┬───┬─────┘  └────┬──────┘
          │   │          │             │   │             │
          │   ▼          ▼             ▼   │             ▼
          │  ┌──────────────────┐  ┌──────────────┐  ┌────────┐
          │  │      tools       │  │  server/chat │  │ config │
          │  └─┬────────────┬───┘  └──┬───┬───────┘  └───┬────┘
          │    │            │         │   │              │
          ▼    ▼            ▼         ▼   ▼              ▼
        ┌──────────┐ ┌──────────────┐ ┌─────────────────┐
        │ prompts  │ │ agent/helix  │ │ helixclient     │
        └──────────┘ └────┬─────────┘ └─────────────────┘
                          ▼
                    ┌──────────┐
                    │  agent   │  (+ agent/claude)
                    └────┬─────┘
                         ▼
                  ┌─────────────────────┐
                  │  store / domain / broadcast  │  (shared kernel)
                  └─────────────────────┘
```

Notable shape: `tools` and `server/chat` both reach all the way down
into `agent/helix` + `helixclient`. The "domain-ish" mutations layer
already knows about a specific runtime backend. That edge — `tools →
agent/helix → helixclient` — is the single biggest violator of the
"keep the core generic, code is scaffolding" principle in `CLAUDE.md`.

### 4.4 Cycles

`go list` reports no import cycles. The dispatcher avoids cycling
through `tools` by declaring its own local `EventDispatcher` interface
in `tools/builtins.go:27`. Similarly `server/server.go:25` declares a
local `Dispatcher` interface so `server` doesn't have to import
`dispatch`. These interface-on-the-receiver patterns are doing real
work — without them the graph would knot up.

---

## 5. Size + churn map

### 5.1 LOC by top-level directory

| Directory             | Code LOC | Test LOC | Notes |
|---                    |---:      |---:      |---    |
| `tools/`              | 2536     | 1167     | 29 tool files + registry + schema helpers. `builtins_test.go` alone is 878 LOC. `hire_worker.go` (257) is the biggest individual tool. |
| `server/chat/`        | 2150     | 404      | Two parallel implementations: `chat.go` (claude bridge, 592) + `helix_bridge.go` (Helix bridge, 944). Plus `sessions.go`, `render.go`, `backend.go`. |
| `helix/helixclient/`  | 1637     | 544      | One file dominates: `client.go` at **1308 LOC**, plus `patches.go` (183) and `session_send.go` (146). |
| `server/ui/`          | 1444     | 0        | `ui.go` (878) + `pages.go` (350) + `orgchart.go` (216). **Zero tests in this package.** |
| `cmd/helix-org/`      | 1151     | 94       | `serve.go` (447) is the wiring god-file. `config.go` (215) + `chat.go` (172) + `bootstrap.go` (140). |
| `store/sqlite/`       | 1076     | 490      | One repo file per domain table. Largest individual file: `event.go` (153). |
| `agent/helix/`        | 984      | 595      | `spawner.go` (421) + `project.go` (285) + `workspace.go` (129) + `state.go` (149). |
| `domain/`             | 944      | 905      | Tests almost mirror code. Largest non-test file: `transport.go` (315) — does parsing/validation of three transport variants (local/webhook/email). |
| `agent/claude/`       | 482      | 334      | `spawner.go` (416) + `workspace.go` (66). |
| `transports/github/`  | 450      | 644      | Tests outweigh code. |
| `transports/postmark/`| 442      | 414      | |
| `server/`             | 417      | 708      | `mcp.go` (192) + `webhook.go` (101) + `server.go` (124). Light. |
| `agent/`              | 394      | 160      | `spawner.go` (125) + `prompt.go` (178) + `activations.go` (68) + `policy.go` (23) + embedded `policy.md`. |
| `dispatch/`           | 334      | 711      | Tests are 2× code. |
| `config/`             | 324      | 183      | One package, one file (324). |
| `prompts/`            | 253      | 285      | |
| `bootstrap/`          | 193      | 0        | Single file; **no tests**. |
| `store/`              | 150      | 0        | Pure interfaces. |
| `broadcast/`          | 108      |  89      | Trivial pub/sub. |
| **Total**             | **~15.5k** | **~7.5k** | |

### 5.2 Files I'd flag as load-bearing (large + central)

| File | LOC | Why |
|---|---:|---|
| `helix/helixclient/client.go` | 1308 | One file, 13 methods on the Client interface, JSON shapes mirror the parent Helix `api/pkg/types`. This is the entire integration with the parent product. |
| `server/chat/helix_bridge.go` | 944 | Owner-chat against a Helix session. SSE fan-out, frame translation, session lifecycle. Imports `agent/helix` directly — the chat surface is coupled to the agent backend abstraction. |
| `server/ui/ui.go` | 878 | Every UI HTML handler, settings mutation, orgchart, streams. Zero tests. |
| `tools/builtins_test.go` | 878 | Single file testing the whole tool surface. |
| `server/chat/chat.go` | 592 | Long-lived claude subprocess for owner UI. |
| `cmd/helix-org/serve.go` | 447 | The wiring file. Has both wiring AND the `buildSpawner` / `buildChatBackend` switch statements over config keys — choosing the runtime variant lives here in `case "claude":` / `case "helix":` blocks (`serve.go:247-326`). |
| `transports/github/github.go` | 450 | Self-contained but big. |
| `transports/postmark/postmark.go` | 442 | Self-contained but big. |
| `agent/helix/spawner.go` | 421 | The Helix spawner: builds prompt, takes semaphore, ensures session, opens WS, polls. |
| `agent/claude/spawner.go` | 416 | The claude spawner: project env files, exec claude, parse stream-json, publish per-segment events. |
| `dispatch/dispatcher.go` | 334 | Per-Worker queue, outbound webhook + email emit. Single goroutine model. |
| `domain/transport.go` | 315 | Encoding + decoding + validation for all 3 transport variants. Inside the domain package. |
| `agent/helix/project.go` | 285 | Per-Worker Helix project lifecycle. Has the keys `"session_id"`, `"project_id"`, `"agent_app_id"`, `"repo_id"` hard-coded in its sidecar `WorkerRuntimeState` writes. |
| `tools/hire_worker.go` | 257 | The most-coupled tool: imports `agent/helix`, `helixclient`, conditionally creates Helix projects at hire time. |

### 5.3 Churn

Since this directory was carved out of its original repo on
2026-05-19, every file shares a "last touched" date in the same
narrow window. The 3-months-back churn lens is therefore not useful —
ranges are 2-3 commits per file, dominated by the consolidation
commit. I have not chased this further; once we've redesigned, treat
churn from this point forward as the real signal.

Files that look recently active in the original-repo sense (touched
in `f722188f2 first alpha` and again in `b5226cac5 re-read worker
settings on every apply`):

- `agent/helix/state.go` (3 commits)
- `agent/helix/project.go` (3 commits)

Everything else is 1-2 commits.

---

## 6. Honest summary — what jumped out

1. **`helixclient/client.go` is the biggest file in the project (1308 LOC) and is the project's single most important external dependency.** It is a hand-rolled mirror of Helix's `api/pkg/types`. Any redesign that touches the Helix integration has to grapple with this one file. The fact that 13 distinct method groups (whoami, projects, secrets, git files, repos, branches, app lifecycle, models, providers, sessions, server status, WebSocket transcript) all live behind one `Client` interface is a sign the integration has been growing organically — there is no sub-grouping by responsibility.

2. **`tools/` knows about a specific runtime backend.** `tools/hire_worker.go` imports `agent/helix` and `helix/helixclient` directly, and at hire time it conditionally calls `ProjectApplier.Ensure(...)` to materialise a Helix project. This is the largest violation of `CLAUDE.md`'s "keep the core generic" rule that I see. The `agent.Spawner` abstraction was supposed to hide which backend is active; instead, hiring is backend-aware. (See `tools/hire_worker.go:12-15`.)

3. **`server/chat/` is effectively two parallel implementations stapled to a `Backend` interface.** `chat.go` (592 LOC) runs a local `claude` subprocess; `helix_bridge.go` (944 LOC) drives a Helix chat session. They share an SSE fan-out shape but not much code. The Backend interface has 5 methods (`StreamHandler`, `SendHandler`, `NewHandler`, `SwitchHandler`, `CommandsHandler`) — i.e. it's a thin HTTP-surface adapter, not a real abstraction over chat semantics. Combined LOC of this subtree (2150 + 404 tests) is the second biggest in the project. It also imports `agent/helix` directly, mirroring the `tools` problem.

4. **The MCP surface and the UI surface are not the same surface.** `CLAUDE.md` insists "every read and mutation of the org graph goes through MCP". But `server/ui/ui.go` exposes `POST /ui/org/roles/set`, `POST /ui/org/identity/set`, `POST /ui/streams/publish`, `POST /ui/settings/set` — these mutate the store directly via `store.Roles.Update` / `Config.Set` etc., without going through a tool. The owner UI bypasses the MCP gateway. That's not necessarily wrong (owner UI is server-side and trusted) but the architecture doc claims otherwise.

5. **`cmd/helix-org/serve.go` is a wiring god-file (447 LOC) AND it carries runtime-selection logic** (`buildSpawner` and `buildChatBackend` switch on `spawner.kind` / `chat.backend`). Each branch has its own config dance — read `helix.url`, `helix.api_key`, `helix.org_url`, `helix.activation_timeout`, `helix.max_inflight`, `chat.provider`, `chat.model`, `chat.session_role`, validate provider/model — and is duplicated near-verbatim between the two backends (serve.go lines 247-329 vs 343-437). When a third backend appears this gets worse, not better.

6. **`server/ui/` has 1444 LOC of code and zero tests.** Same for `bootstrap/` (193 LOC, 0 tests) and `store/` (150 LOC of interfaces, no tests is fine here). UI is genuinely untested; everything you see in `/ui/*` works because it's been manually exercised, not because anything pins it.

7. **`domain/transport.go` (315 LOC) is doing transport-shape validation inside the domain package**, including URL parsing for webhook outbound URLs and email-config object parsing. The domain package is supposed to be primitive types; transport is an integration concern. The fact it lives in `domain/` means new transports require editing the domain package's enum + parser — not a "data-driven" surface as `CLAUDE.md` envisions.

8. **There is no separation between "ops config" and "feature flags".** `config.Registry` is a flat key/value store keyed by dot-namespaced strings, registered en bloc in `cmd/helix-org/configspecs.go`. That file is the source of truth for what is configurable, mixed in with secret declarations. New subsystems must edit `configspecs.go`, registry registration, and the code that reads keys at three different sites — easy to drift. The doc says a future refactor "could push registration into each subsystem's package-level init" (`configspecs.go:13`) — agreed.

9. **The dispatcher has both fan-out-to-Workers AND outbound webhook/email emit** (`dispatch/dispatcher.go:128, 275`). These are two unrelated concerns sharing a class because both fire on `domain.Event` append. The result: `dispatcher.SetEmailEmitter(...)` (called from `serve.go:112`) exists specifically because the email transport also takes a dispatcher (for inbound), so constructor injection isn't an option — the comment at `dispatcher.go:98-103` flags this honestly. Smells like emit-on-publish should live elsewhere.

10. **The "human → claude CLI → MCP → server → claude (spawner) → MCP → server" loop is the central insight of the system** but it is implemented in three separate places — `cmd/helix-org/chat.go` (interactive), `server/chat/chat.go` (UI), `agent/claude/spawner.go` (AI Worker). Each rebuilds `mcp.json`, each handles claude session-resume differently, each parses claude stream-json (or doesn't). Owner-chat in the UI gets a label, AI Workers get triggers in the prompt, the CLI chat gets `--name "helix-org: <worker>"`. They are 90% the same shape with 100% the same dependency footprint. One place would do.

That is everything I observed without speculating about fixes.
