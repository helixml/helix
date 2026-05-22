# 07 — SOLID as a Local Lens

Class / file / struct-level findings. The architectural moves
(contexts, runtime port, dispatcher split) belong to steps 4–6;
this file is the line-by-line companion. Citations use
`relative/path:NN` and the line numbers match `main @ ee0ecf976`.

A pattern that recurs in nearly every violation: the package has a
clean small interface defined locally (`Backend`, `EventDispatcher`,
`Dispatcher`, `ProjectEnsurer`, `EmailEmitter`, `Spawner`) but the
constructors / wiring around it then import a concrete from a sibling
package anyway. The interfaces are present; nobody is using them as
the seam.

---

## 1. SRP — Single Responsibility

The flagged files, each with the unrelated jobs they currently
bundle and the smallest split that breaks them.

| File | LOC | Concerns it mixes today | Smallest split |
|---|---:|---|---|
| `server/ui/ui.go` | 878 | 12 HTTP handlers spanning four resources: **chat shell** (`handleChat`, `:118`), **org graph reads + mutations** (`handleOrg`, `handleOrgRoleSet:`78, `handleOrgIdentitySet:`80, `handleOrgDetail:79`), **settings** (`handleSettings:`75, `handleSettingsSet:`76, `handleSettingsDelete:`77 — direct `config.Registry` writes), **streams** (`handleStreams:`81, `handleStreamsEventsSSE:`82, `handleStreamsPublish:`83 — direct `store.Events.Append` + `broadcaster.Notify`). Also owns `ownerSidebar` (chat sessions) and the `uiHandler` struct itself. Settings + streams mutations also bypass MCP (`04-bounded-contexts §4 item 6`). | One file per resource: `ui_chat.go`, `ui_org.go`, `ui_settings.go`, `ui_streams.go`, plus `ui_sidebar.go`. The `Handler` constructor at `:69` stays as the only wiring point and gets a 4-line body. |
| `server/chat/helix_bridge.go` | 944 | One struct, 21 methods, doing five things: (a) **HTTP surface** (`StreamHandler`, `SendHandler`, `NewHandler`, `SwitchHandler`, `CommandsHandler`, `:327-872`); (b) **SSE fan-out** (`subscribe`, `unsubscribe`, `broadcast`, `broadcastLocked`, `:297-650`); (c) **Helix-session lifecycle** (`send`, `sendAppOnly`, `attachSession`, `:453-689`); (d) **WebSocket pump + frame translation** (`runWebsocket:`690, `renderEvent:`729, `broadcastInteractions:`615); (e) **slash-command expansion** (`expandSlashCommand:`873). | `chat_http.go` (handlers), `chat_sse.go` (fan-out, shared with `Bridge`), `helix_session.go` (lifecycle), `helix_ws.go` (pump + translation). Slash-command expansion already exists in `Bridge` (`chat.go:512`) and `HelixBridge` (`helix_bridge.go:873`) as near-duplicates — extract once into `prompts_expand.go`. |
| `helix/helixclient/client.go` | 1308 | 13 unrelated REST surfaces on **one interface** (`:42-133`): whoami, server status, providers, models, projects, secrets, files, repos, branches, apps, sessions, messages, websocket. Every caller imports the whole god-interface to use any one method. | Split the interface into `AuthClient`, `ProjectClient`, `RepoClient`, `AppClient`, `SessionClient`, `ModelCatalogClient`. Keep the concrete `realClient` implementing all of them (or composing). Callers depend only on the slice they use — `agenthelix.WorkerProject` only needs `ProjectClient + RepoClient`; `agenthelix.Workspace` only needs `RepoClient` (PutFile); `chat.HelixBridge` only needs `SessionClient + AppClient`. |
| `cmd/helix-org/serve.go` | 447 | (a) **CLI flag parsing + flag-derived path resolution** (`:36-58`); (b) **store open + bootstrap** (`:61-87`); (c) **runtime selection** (`buildSpawner:`234, `buildChatBackend:`336 — 100 LOC each of switch-on-config); (d) **transport construction** + the post-hoc `dispatcher.SetEmailEmitter` dance (`:111-121`); (e) **HTTP server assembly + Route list** (`:185-199`); (f) **signal-handled lifecycle** (`:201-225`). | Pull `buildSpawner` / `buildChatBackend` into `agent/runtime/factory.go` and `server/chat/factory.go` respectively (each owning its own config-key block). `serve.go` then composes the pre-built objects and stays under ~150 LOC. The `errors.Is(err, bootstrap.ErrAlreadyInitialised)` switch is fine where it is. |
| `dispatch/dispatcher.go` | 334 | (a) **per-Worker activation queue + coalescing runner** (`enqueue:`191, `run:`211, `activate:`231); (b) **outbound webhook POSTs** (`postOutbound:`314); (c) **outbound email emit** via injected `EmailEmitter` (`emitOutbound:`275 case `TransportEmail`). The author flagged the smell honestly at `:98-103` — `SetEmailEmitter` is post-construction injection only because the email transport needs the dispatcher first. | Lift `emitOutbound` + `postOutbound` into a sibling `Outbox` type owned by Communication; the dispatcher subscribes to publishes that need outbound and forwards them. The two halves stop sharing a struct, the `SetEmailEmitter` setter goes away. (Matches `04 §4 cut #2`.) |
| `tools/hire_worker.go` | 257 | (a) **org-graph mutations** — Worker row, Environment row, Grants rows, Activation Stream + subscription (`:170-203`); (b) **Helix-runtime provisioning** — `helixclient.UserIDFromContext` + `agenthelix.SaveHiringUser` (`:210-218`); (c) **dispatcher kickoff** (`:220-222`). The tool also embeds a JSON shape patch (`UnmarshalJSON` tolerating a string-encoded array, `:89-115`) — an LLM-protocol concern that doesn't belong with hire mechanics. | Move the Helix bit out behind a `runtime.OnWorkerHired` callback that the wiring layer registers (matches `04 §4 cut #1`). Move the `UnmarshalJSON` tolerance into a `tools/jsonpatches.go` helper that other tools can share. The dispatcher kickoff stays — it is the lifecycle event hire genuinely owns. |

A pattern in 4 of the 6 cases: the SRP violation is the **same root
cause as a DIP violation** (concrete cross-package imports). Splitting
responsibility makes the dependency direction tractable.

---

## 2. OCP — Open/Closed (switch-ladders over a kind)

Every one of these should be a registry / strategy lookup so adding
a new value never reopens the same source file.

| Site | Switches over | Effect of adding a value |
|---|---|---|
| `cmd/helix-org/serve.go:247-326` `buildSpawner` | `spawner.kind` string ∈ {`"claude"`, `"helix"`} | Edit `serve.go`, add a new `case`, plumb each runtime's config keys inline (`helix.url`, `helix.api_key`, `helix.org_url`, `helix.activation_timeout`, `helix.max_inflight`, `chat.provider`, `chat.model` for helix — claude has its own block of `claudeBin`, `model`, `effort`). |
| `cmd/helix-org/serve.go:348-435` `buildChatBackend` | `chat.backend` string ∈ {`"claude"`, `"helix"`} | Same shape; near-duplicate of `buildSpawner`'s helix branch (lines 363-394 ≈ 263-300). |
| `dispatch/dispatcher.go:285-305` `emitOutbound` | `stream.Transport.Kind` ∈ {`TransportWebhook`, `TransportEmail`} (silently no-ops on `TransportLocal` and `TransportGitHub`) | Adding Slack / RSS / SMS means a new case here AND a new field on `Dispatcher` (`emailEmitter`-shape setter) AND a new constant in `domain/transport.go`. |
| `agent/prompt.go:31-46` activation prompt rendering | `Trigger.Kind` ∈ {`TriggerHire`, `TriggerEvent`} with a `default:` that quietly stringifies anything else | New TriggerKind (`TriggerFire`, `TriggerSchedule`, `TriggerInvite`…) silently degrades to `Activation kind: %q.\n` until somebody remembers this site. |
| `domain/transport.go:217-283` `Transport.Validate` | `TransportKind` four-way switch with per-kind config parser and per-kind validation rules | Adding a transport means editing the kernel package. The "data-driven transports" idea in `CLAUDE.md` is contradicted here. (Cross-cut `04 §4 #4`.) |
| `domain/transport.go:165-211` three `*Config()` methods | TransportKind | Same shape: three near-identical `if t.Kind != Transport<X>` guards, three near-identical Unmarshal blocks. |
| `agent/helix/state.go:23` `Backend = "helix"` + `store.WorkerRuntimeState`'s `backend` parameter (`store/store.go:60`) | Implicit kind discrimination | Today only "helix" exists. The contract is open-ended but no registry; a second runtime collides silently if it picks the same string. |

**Smallest fix common to all six**: a `map[K]Builder` populated at
package `init` in each transport / runtime / trigger package, registered
into a global registry import-`init`-side. The kernel keeps only the
discriminator type. Adding a kind means adding a file; no existing file
opens.

For `buildSpawner` / `buildChatBackend` the natural shape is a
`type Factory interface { Build(ctx, cfg) (Spawner, WorkspaceSync, error) }`
indexed by kind, registered from `agent/claude/init.go` and
`agent/helix/init.go`. `serve.go` then becomes `factory := registry.Get(kind); spawner, ws, err := factory.Build(...)`.

---

## 3. LSP — Liskov Substitution (impls that lie about their contract)

| Interface | Impls | Where they diverge |
|---|---|---|
| `agent.Spawner` (`agent/spawner.go:64`, function type) | `agent/claude/spawner.go:Spawner`, `agent/helix/spawner.go:Spawner` | (a) **envPath**: claude's `Spawner` is documented to exec in that directory (`spawner.go:124-128`); helix's signature accepts it but throws it away (`agent/helix/spawner.go:99` — `_ string`). Callers that count on "env files in envPath are visible to the agent" are silently wrong on helix. (b) **Cancellation**: claude waits for the subprocess via `cmd.Wait` and trusts process-tree kill on context cancel; helix has its own `ActivationTimeout` (`spawner.go:130`) layered *over* the dispatcher's context — a Worker can be activated with a 0 deadline and helix still runs for 5 minutes. (c) **MaxInflight**: helix has a global semaphore (`:98, 115-119`), claude has none. A burst that's safe on claude can starve on helix or vice versa. |
| `agent.WorkspaceSync` (`agent/spawner.go:91`) | `agent/claude/workspace.go:38`, `agent/helix/workspace.go:62` | The contract says "mirror canonical content into wherever the agent reads it." Concrete divergence: claude writes the file synchronously to disk (`os.WriteFile`, immediately visible). Helix's impl skips the write entirely when `state.RepoID == ""` (`workspace.go:73-79`) and returns nil **as if it succeeded**. The caller — `update_role`, `update_identity` — has no way to know whether the next activation will actually see the new content. Additionally, the helix impl has a side effect the claude impl doesn't: invalidates the warm chat session on `role.md`/`identity.md` writes (`:107-113`) — and swallows that side-effect's error too. Two impls that look the same in the type signature, behave differently. **Fix**: rename the no-op-on-unprovisioned case to an explicit `ErrNotProvisioned` return, and surface "did the publish actually land?" in the return type. Make the session-invalidation a separate observer on the publish, not buried in `PublishFile`. |
| `chat.Backend` (`server/chat/backend.go:24`) | `chat.Bridge` (`chat.go`), `chat.HelixBridge` (`helix_bridge.go`) | `Bridge.History(_ context.Context) []string { return nil }` (`chat.go:388`). The interface comment at `backend.go:43-50` admits the asymmetry — the claude bridge falls back to "the UI's separate ReadHistory path". So the interface promises a method whose nil return is a sentinel meaning "ask elsewhere". Classic LSP lie — the substitutable type doesn't actually substitute. `HistoryStartsFresh()` is similarly meaningful on claude only (it inspects path-based session detection, `chat.go:67`) — `HelixBridge.HistoryStartsFresh` exists but reflects a different concept (`freshFromNew` flag, `helix_bridge.go:75`). **Fix**: drop `History` and `HistoryStartsFresh` from the interface and let the UI handler hold a separate `HistorySource` it can wire per-backend. Alternative: make both methods return a typed `History` value where empty has unambiguous meaning. |
| `server.Dispatcher` (`server/server.go:25`) vs `tools.EventDispatcher` (`tools/builtins.go:27`) vs `*dispatch.Dispatcher` (concrete) | one concrete | The two narrower interfaces are not strict subsets of each other: `server.Dispatcher` has `Dispatch` only; `tools.EventDispatcher` has `Dispatch + DispatchHire`. Today both happen to be satisfied by the same concrete struct. If somebody ever returns a wrapper that implements only `Dispatch` to satisfy `server.Dispatcher`, it'll fail at the `tools` site at construction time — Go's structural typing makes this a compile error, not a runtime LSP bug, but it's still a maintenance hazard. **Fix**: collapse to one interface in `activation/` (per `04 §4 #7`). |
| `store.Streams.Get` semantics | `store/sqlite/stream.go` (only impl) | Single impl, no LSP risk today. Flagged only to note that `Streams.Create` does not call `Transport.Validate` (`store/store.go:80-84` interface; verified at the sqlite layer) — every caller is expected to validate first. A future impl that started rejecting invalid transports would tighten the contract incompatibly. Document the validate-before-write rule in the interface comment. |

---

## 4. ISP — Interface Segregation

| Interface | Surface | Forced-fake symptom |
|---|---|---|
| `helixclient.Client` (`client.go:42-133`) | 21 methods (13 conceptual groups) | Every caller imports the whole interface. `chat.HelixBridge` uses ~8 of them; `agenthelix.WorkerProject` ~7; `agenthelix.Workspace` exactly 1 (`PutFile`). A test fake of `Workspace` has to stub out 20 methods it never calls. Same for the `WhoAmI`-only `bootstrap.Run` (`bootstrap/bootstrap.go:63`). **Fix**: split per §1 above (`AuthClient`, `ProjectClient`, `RepoClient`, `AppClient`, `SessionClient`, `ModelCatalogClient`); the concrete `realClient` keeps all of them. |
| `chat.Backend` (`backend.go:24`) | 8 methods | `Bridge.History` returns `nil` unconditionally (`chat.go:388`). The interface advertises a method one impl can't honestly implement. **Fix**: drop `History`/`HistoryStartsFresh` (see LSP §3). |
| `store.Store` | A *struct of small interfaces* (`store.go:139-150`) | Not actually a god — each sub-interface is 4–6 methods. The pattern is fine. However, every test in `tools/` instantiates the **whole** `Store` via `sqlite.Open(":memory:")` even when only one sub-store is exercised (e.g. `tools/publish_test.go:21` opens the full DB to test `Publish` which only writes to `Events`+ reads `Streams`). Code-wise this is cheap (it's in-memory); it just means the per-sub-interface seams aren't being exercised by tests. **Fix**: provide a `memorystore` package with one struct per sub-interface; bench tests can wire what they need. |
| `tools.Tool` (`domain/tool.go:23`) | 4 methods (Name, Description, InputSchema, Invoke) | Honest small interface. No violations. |
| `agent.WorkspaceSync` | 1 method | Honest, see LSP for contract issue. |
| `agent.Spawner` | function type (one method) | Honest. |
| `store.WorkerRuntimeState` | 4 methods (Get/Set/SetMany/Clear) | Honest. Used only by `agent/helix/state.go` today. |

The two interfaces that are honestly big are `helixclient.Client` and
`chat.Backend`. The "fat" appearance of `store.Store` is illusion —
it's already segregated; just the wiring tests use the whole bundle.

---

## 5. DIP — Dependency Inversion (concrete imports where interface would do)

The headline list:

| Importer | Imports concretely | Should import |
|---|---|---|
| `tools/hire_worker.go:13-15` | `agent/helix`, `helix/helixclient` | Nothing runtime-specific. The Helix bits (`SaveHiringUser`, `UserIDFromContext`) belong behind a `runtime.OnWorkerHired(ctx, workerID, hiringUserID)` callback that the wiring layer plugs in. This is the **single biggest DIP violation in the project** and the headline cross-cut in `04 §4 #1`. |
| `server/chat/helix_bridge.go:15-17` | `agent/helix` (`agenthelix.AgentType`, `agenthelix.TranscriptBody` at `:514, 699`), `helix/helixclient` | The UI chat surface shouldn't know which Runtime drives it. Lift Helix-session driving behind a `runtime.ChatSession` port (start, send, subscribe, stop). Then `helix_bridge.go` collapses into "the Helix impl of ChatSession" and the UI wires whichever the operator configured. (Cross-cut `04 §4 #3`.) |
| `cmd/helix-org/serve.go:17-34` | All 18 internal packages, including `agentclaude`, `agenthelix`, `helixclient`, `postmark`, `githubtransport` | This is the wiring file and *some* concrete import here is unavoidable. The issue is that `serve.go` then passes the concretes through to subsystems instead of an interface — `dispatch.New(..., spawner, ...)` accepts the function type (fine), but `chat.NewHelix(chat.HelixConfig{Client: client, ...})` takes a concrete `helixclient.Client` interface value, and `chat.HelixBridge` reaches back into `agenthelix` from inside the package. Better: each subsystem accepts only the smallest interface it uses; the wiring file builds the concrete once and assigns it to N narrow types. |
| `agent/helix/project.go:24` `WorkerProject` | Concrete `helixclient.Client` field (`spawner.go:75`) | Acceptable inside the Runtime context — `helixclient` *is* the Helix runtime. But once `Client` is split per §4, `WorkerProject` should hold only `ProjectClient + RepoClient + AppClient`, not the whole god interface. |
| `dispatch/dispatcher.go:32-34` | `agent` (Spawner type), `domain`, `store` | Correct — `Spawner` is a function type, store and domain are kernel. No fix. |
| Tests: every `_test.go` listed above (`tools/builtins_test.go:34`, `server/server_test.go:26`, `dispatch/dispatcher_test.go:90`, `transports/.../*_test.go`, etc.) | `sqlite.Open(":memory:")` | The store sub-interfaces (`Roles`, `Streams`, …) cry out for fakes. Production code already depends only on the sub-interfaces — tests pinning the SQLite concrete defeats the abstraction. `CLAUDE.md:118` says "Prefer fakes over mocks" — there are neither. **Fix**: hand-roll a `memorystore` per sub-interface (each is 4–6 methods); use it in tests that don't need GORM's persistence behaviour. Keep `sqlite.Open(":memory:")` for integration tests of `store/sqlite/` itself. |
| `cmd/helix-org/bootstrap.go:63` | Constructs a second `helixclient.New(...)` just to call `WhoAmI` | Should accept an `AuthClient` interface; production wiring builds the concrete once. |

The `tools.EventDispatcher` and `server.Dispatcher` local-interface
pattern is doing real work (`01 §4.4`) — DIP applied correctly.
The fix for the cycle goes the other way: collapse to one interface
in Activation (`04 §4 #7`).

---

## 6. Go-idiom deviations from the project's own stated rules

`CLAUDE.md` lines ~100-135 list a specific software-engineering
contract. Where the code breaks its own rules:

### 6.1 Boolean parameters that switch behaviour

- `helixclient.Client.AttachRepoToProject(ctx, projectID, repoID string, primary bool)` (`client.go:92`, impl `:778`). `primary` flips the semantics — primary-attach vs auxiliary-attach are different operations the server treats differently. **Fix**: two methods, `AttachRepoAsPrimary` / `AttachRepo`. (`CLAUDE.md`: "Don't use a boolean to switch between fundamentally different behaviours".)
- `helixclient.Client.StartChatWithStatus(...) (Session, bool, error)` (`client.go:122`) — the `bool` is "did the SSE surface a transient no-agent error?" Different from the normal happy/error return. The fact that this exists in addition to `StartChat` is itself the smell (two methods doing nearly the same thing). **Fix**: return a typed `StartChatResult` with a discriminated outcome.
- `helixclient.SendMessageOptions{Interrupt bool, NotifyUserID string}` (`client.go:141`) — `Interrupt` is an **orthogonal modifier**, not behaviour-switch. Acceptable per the same rule. Flagged for completeness.
- `chat.Bridge.expandSlashCommand(ctx, msg) (string, bool)` (`chat.go:512`) and the identical `HelixBridge.expandSlashCommand` (`helix_bridge.go:873`) — the `bool` is "did it match a known slash command?" That's a presence flag, not a behaviour switch. Fine.

### 6.2 `interface{}` / `any` where a typed struct would do

Searched — the codebase actually does well here. `domain.Message.Extra json.RawMessage` (`message.go:36`) instead of `map[string]any`. `Transport.Config json.RawMessage`. Only smell: `transports/github/github.go:423` `numberFromAny(v any) (int64, bool)` — exists because the GitHub payload is `map[string]any` after generic JSON decode; reasonable inside the ACL boundary.

### 6.3 Globals / package-level vars

Mostly schemas (`mustSchema[...]`) and `errors.New` sentinels — both acceptable. Non-acceptable:

- `domain/transport.go:154` `knownGitHubEvents map[string]struct{}` — a package-private allowlist living in the kernel. Belongs in `transports/github/` (see OCP §2). Once moved it becomes private to that transport.
- `server/chat/render.go:18` `markdown = goldmark.New()` — a global parser configured once. Acceptable Go idiom; not flagged.
- 29 `<ToolName>Name domain.ToolName = "..."` consts spread across `tools/*.go` (e.g. `tools/hire_worker.go:51`, `tools/publish.go:28`) — `CLAUDE.md`: "No globals: prefer classes over public constants". The names are used in `bootstrap/bootstrap.go:109-141` to keep the default-grants list and the registry in sync; the const-per-file pattern is the cheapest way to express that today. Real fix: a single `tools.Names()` function or registry-derived list, populated by each tool's `Name()` method (which already exists). The consts can then go.

### 6.4 `-er` / `Manager` / `Handler` suffix names

Searched. The codebase is mostly disciplined. Surviving offenders:

- `server/ui/ui.go:87` `type uiHandler struct` — small, narrow, fine.
- `agent/helix/project.go:24` `type WorkerProject struct` — "Applier" is a verb-noun. Replace with `Project` or `HelixProject` once it's clear what it owns.
- `dispatch.EmailEmitter` (`dispatcher.go:46`) — narrow interface, the `-er` suffix is the Go idiom (`io.Reader`, `io.Writer`). Acceptable deviation per `CLAUDE.md`'s last bullet ("follow the language idiom").

No `Manager` or `Helper` exists. Good discipline.

### 6.5 Public constants where a small private type would fit better

- `agent/helix/state.go:23` `const Backend = "helix"` — used as the `backend` parameter to `store.WorkerRuntimeState` calls. Should be a private `runtimeBackend` type with that one value, so other Runtime packages can't accidentally collide on the same string.
- `transports/postmark/postmark.go:112` `const DefaultSendURL = "https://api.postmarkapp.com/email"` — a default value for an env-var-overridable config field. Could collapse into the `Config` struct's default. Minor.
- `prompts/help.go:13`, `prompts/role.go:16` — same shape as the tool-name constants. Same fix applies.

### 6.6 Other CLAUDE.md violations worth a note

- "Error on missing configuration — fail with an error, don't log a warning and continue." Mostly followed. Exception: `tools/hire_worker.go:210-218` "persist hiring user" is documented as "Non-fatal" *in the comment* (`:212`) but actually returns the error (`:216`). Comment is stale — code is right.
- "No fallbacks — one approach, no fallback code paths." Violated: `dispatch/dispatcher.go:137-141` — when `e.Message()` parse fails, the dispatcher silently falls back to `domain.Message{Body: e.Body}` and continues. Pre-migration safety net per the comment; the fallback should be removed once it's confirmed nothing produces bare-body events.
- "One primary constructor." Violated by `agent/claude.Spawner` (`spawner.go`) returning a function, while `agent/helix.Spawner(cfg)` also returns a function — both are constructors named `Spawner`, neither is a struct. The factories disguised as functions confuse the construction pattern. Less important than the splits above.

---

## 7. Prioritised fixes (six moves, payback-ordered)

Calibrated to whether each move unblocks an architectural cut from
step 4 §4 or is purely local hygiene.

| # | Fix | Unblocks | Cost (rough) |
|---|---|---|---|
| 1 | Remove `agent/helix` + `helixclient` imports from `tools/hire_worker.go` — put Helix provisioning behind a `runtime.OnWorkerHired` callback the wiring layer registers. | `04 §4 #1` (the headline cross-cut), simplifies DIP §5 row 1, and lets the Runtime context become "addable" per `04 §5`. | M — touches `tools/builtins.go` `Deps`, `cmd/serve.go` wiring, and `agent/helix`. |
| 2 | Split `helixclient.Client` (`client.go:42-133`) into per-concern sub-interfaces. Keep the concrete `realClient` implementing the union. | ISP §4 row 1, DIP §5 row 4, and shrinks the surface that `chat.HelixBridge` / `WorkerProject` / `Workspace` each see. | M — one file refactor in `helixclient/`; callers narrow their declared type. |
| 3 | Make Runtime selection (`buildSpawner` / `buildChatBackend` in `cmd/helix-org/serve.go`) a registry. Each runtime registers a factory at `init`. | OCP §2 rows 1–2, `04 §4 #5`, and cuts `serve.go` in half. Required *after* fix 1 — otherwise the registered factory still imports `tools/`. | M — new `agent/runtime/registry.go`, plus an `init()` in each runtime package. |
| 4 | Split `dispatch.Dispatcher` into an Activation queue and an Outbox. Drop `SetEmailEmitter`. | OCP §2 row 3 (`emitOutbound` switch goes with the Outbox), `04 §4 #2`, removes one of the three Dispatcher interfaces (LSP §3 row 4). | M — modest refactor, but the package has 711 LOC of tests that all reach into both halves, so test churn is non-trivial. |
| 5 | Pull transport-config parsing/validation out of `domain/transport.go` into each `transports/<x>/` package. Kernel keeps only the discriminator + `json.RawMessage`. Move `knownGitHubEvents` with it. | OCP §2 rows 4-5, `04 §4 #4`, kills the kernel-touches-on-new-transport problem. | S–M — straightforward; mostly moving code. Test fixtures in `domain/transport_test.go` follow. |
| 6 | Build an in-memory `memorystore` package implementing each `store.*` sub-interface. Cut over the ~10 tests currently using `sqlite.Open(":memory:")` for unit-level coverage. | DIP §5 row 6 (forced concrete in tests). Local hygiene, not a context cut — but cheap and accelerates iteration on cuts 1-5. | S — ~150-200 LOC of fakes; mechanical test edits. |

Local-only hygiene (do alongside, don't prioritise):

- Drop `chat.Backend.History` + `HistoryStartsFresh` (LSP §3 row 3 / ISP §4 row 2).
- Replace `AttachRepoToProject`'s `primary bool` with two methods (§6.1).
- Replace 29 per-file `<Name>Name` consts with a registry-derived `tools.Names()` (§6.3).

The bigger architectural cuts in `04 §4` (`#6` UI-bypasses-MCP, `#8`
two-bootstraps, `#9` chat-Backend-is-only-HTTP-adapter, `#10`
zero-tests-in-ui) are orthogonal to SOLID-at-class-scope and live in
later steps.
