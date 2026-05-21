# Step 02 — Behavioural Mapping

End-to-end traces of the 4–6 most important user-visible capabilities,
read by inspection. Cites are `path/file.go:NN` relative to
`/home/phil/helix/helix-org/`.

Sources of truth used to pick capabilities:
`CLAUDE.md` (project philosophy + architecture), `TODO.md` (live pain
points), `demos/getting-started/README.md`, `demos/webhook/README.md`,
`demos/github/README.md`.

The cast across every flow is the same handful of objects: a SQLite
store wrapping the `domain` types (`Worker`, `Role`, `Position`,
`Stream`, `Subscription`, `Event`, `ToolGrant`, `Environment`), a
`tools.Registry`, a `*broadcast.Broadcaster` (long-poll wake-ups), a
`*dispatch.Dispatcher` (event-fan-out + activation runner), and a
`agent.Spawner` (either `agent/claude` or `agent/helix`).

The architectural claim from `CLAUDE.md` — "one HTTP endpoint,
`/workers/{id}/mcp`" — is *almost* true. In practice the server also
mounts `POST /webhooks/{streamID}` (`server/server.go:76`), the
transport-specific inbound paths `POST /email/postmark` and
`POST /github/webhook` (`cmd/helix-org/serve.go:188-189`), and the
whole `/ui/` HTML chat surface (`cmd/helix-org/serve.go:190-196`).
Worth flagging because the docs say otherwise.

---

## Capability 1 — Bootstrap an Org

### Trigger

First invocation of `helix-org serve` against an empty SQLite file.
**Not** `helix-org bootstrap` — despite both `cmd/helix-org/main.go:31`
and the `CLAUDE.md` docs claiming bootstrap seeds the owner Worker, the
real bootstrap target (`cmd/helix-org/bootstrap.go:21`) is `helix-runtime`
and does no seeding at all. It only pings the Helix server
(`bootstrap.go:69`) and optionally validates `chat.provider`/`chat.model`
(`bootstrap.go:81-89`). Owner seeding happens unconditionally at the
start of `runServe`.

### Timeline

1. `cmd/helix-org/main.go:25` dispatches `serve` → `runServe`.
2. `cmd/helix-org/serve.go:61` opens SQLite via `sqlite.Open(*dbPath)`;
   GORM `AutoMigrate` runs every table.
3. `cmd/helix-org/serve.go:69-72` makes `<envsDir>/w-owner` on disk.
4. `cmd/helix-org/serve.go:73-87` calls `bootstrap.Run`. On the second
   start `bootstrap.Run` returns `ErrAlreadyInitialised` — the normal
   case — and the rest of `runServe` continues.
5. `bootstrap/bootstrap.go:63-69` short-circuits if any Worker exists.
6. `bootstrap/bootstrap.go:72` constructs role `r-owner` from the
   embedded `templates/owner_role.md` (a hand-written playbook telling
   the LLM how to hire — see "Pain points" below).
7. `bootstrap/bootstrap.go:80-86` creates `p-root` Position (no parent).
8. `bootstrap/bootstrap.go:88-96` mints `w-owner` as a **human**
   Worker. `domain.NewHumanWorker` returns a Worker whose `Kind() ==
   WorkerKindHuman` — this is what causes the dispatcher to skip
   activation for the owner (`dispatch/dispatcher.go:166`).
9. `bootstrap/bootstrap.go:98-104` creates the Environment row pointing
   at the just-`MkdirAll`'d path. The directory is empty until an AI
   Worker activates (which the owner never does).
10. `bootstrap/bootstrap.go:149-164` mints the owner's
    `s-activations-w-owner` stream and self-subscribes the owner to it
    (`:165-171`). Comment in code: "the owner-chat bridge publishes
    activation events to this stream via `agent.PublishActivationEvent`"
    — but `agent.PublishActivationEvent` does not exist as a public
    symbol (`publishActivationEvent` is lowercase in
    `agent/claude/spawner.go:249`); only Spawners use it, and the owner
    is never spawned. The owner's activation stream is therefore
    perpetually empty.
11. `bootstrap/bootstrap.go:173-185` grants the owner every structural
    tool in the `defaults` slice (lines 109-141). This is the root of
    trust — the only grants in the system not issued by another Worker.
12. `cmd/helix-org/serve.go:89-107` wires the broadcaster, the
    `tools.Deps`, the dispatcher, then `deps.Dispatcher = dispatcher`
    (which the tools captured at registration time pick up via the
    shared `Deps` value — Go-pointer semantics).
13. `cmd/helix-org/serve.go:111-121` constructs the inbound transports
    (`postmark`, `github`) and hooks the email emitter back into the
    dispatcher via `dispatcher.SetEmailEmitter` — circular construction
    avoided by setter injection.
14. `cmd/helix-org/serve.go:123-126` instantiates the `tools.Registry`
    and `RegisterBuiltins(reg, deps)`. Each tool closes over `deps`.
15. `cmd/helix-org/serve.go:131-134` registers MCP prompts.
16. `cmd/helix-org/serve.go:145-148` builds the owner-chat bridge
    (`chat.Backend`) — either the long-lived `claude` subprocess
    bridge (`server/chat/chat.go:94`) or a `Helix` chat session
    (`server/chat/helix_bridge.go`). Selection is by `chat.backend`
    config key.
17. `cmd/helix-org/serve.go:185-199` mounts the routes and starts the
    HTTP server.

### State changes

- 4 row inserts in `roles`, `positions`, `workers`, `environments`.
- 1 row in `streams` (the owner's activation stream).
- 1 row in `subscriptions` (owner → own activation stream).
- ~31 rows in `grants` (one per tool in `defaults`).

### Outputs / side effects

- Owner Environment directory exists on disk but empty.
- HTTP server listening on `:8080` with `/workers/w-owner/mcp` ready.
- A `/ui/` chat page that, on first GET, lazily spawns `claude` in the
  server's cwd (`server/chat/chat.go:303`) — the chat brain is **not**
  pre-warmed.

### Pain points

- **Two different "bootstraps".** `helix-org bootstrap helix-runtime`
  is unrelated to seeding the owner. The CLI docstring at
  `cmd/helix-org/main.go:54-57` and `CLAUDE.md` both still describe
  bootstrap as the owner-seeding command. The `--install-claude-mcp`
  flag and "register the owner's MCP endpoint with the local claude
  CLI" mentioned in `CLAUDE.md` do not exist in the code.
- **Owner activation stream is dead by design but advertised as
  alive.** `bootstrap.go:148` says owner chat publishes to it — no
  code path does. The stream sits empty forever, but `/ui/streams`
  will still render it.
- **Owner Role markdown encodes workflow.** `owner_role.md` walks
  through `create_role → create_position → hire_worker (with grants)
  → create_stream → subscribe` step by step. The architecture note
  in `CLAUDE.md` insists "no workflow in code" — but workflow has
  simply moved to a markdown file that the bootstrap embeds, then
  unconditionally seeds. It is workflow-in-data, which is the design
  goal, but every owner gets the same hard-coded text and there is no
  way to override it at bootstrap.

---

## Capability 2 — Owner Hires a Worker

### Trigger

Human types in the `/ui/` chat textarea, e.g. the
`demos/getting-started/README.md` prompt that asks the chat brain to
"set up a small echo worker." The chat brain is a `claude` subprocess
(or Helix session) whose MCP config points at `/workers/w-owner/mcp`,
so every tool call runs as `w-owner`.

### Timeline

1. Browser POSTs to `/ui/chat/send` → `server/chat/chat.go` SendHandler
   writes a `user` stream-json frame to claude's stdin.
2. `claude` consults its system prompt + the MCP tool list it fetched
   from `/workers/w-owner/mcp` and emits tool calls. The MCP server at
   `server/mcp.go:67` rebuilds a fresh `mcp.Server` per request,
   filtered by `s.store.Grants.ListByWorker(workerID)` (`mcp.go:80`).
3. Tool call 1: `create_role` → `tools/create_role.go`. Inserts a row
   in `roles`.
4. Tool call 2: `create_position` → `tools/create_position.go`. Insert.
5. Tool call 3: `create_stream` (`s-general`) → `tools/create_stream.go`.
   Insert into `streams`. Transport defaults to `local`.
6. Tool call 4: `hire_worker` — the meaty one.
   - `tools/hire_worker.go:117-145` parses args. `id` must be supplied
     in `w-<name>` form per the description (`hire_worker.go:63-69`).
     If absent, falls back to `w-<uuid>` (`:139`).
   - `:166` makes the env directory `<EnvsDir>/<id>` (idempotent).
   - `:170` `Store.Workers.Create`. `domain.NewAIWorker` flips
     `Kind` to `WorkerKindAI` so the dispatcher will spawn it.
   - `:178` `Store.Environments.Create`.
   - `:185-197` issues every grant in `args.Grants` (one row per).
     The bundle-grants-with-hire is deliberate
     (`hire_worker.go:31-34`): without it, the activation races the
     follow-up `grant_tool` calls.
   - `:199-203` `createActivationStream` creates
     `s-activations-<workerID>` and **subscribes the hiring Worker
     (`inv.Caller.ID()` — here `w-owner`) to it**. The newly-hired
     Worker is intentionally not subscribed (`hire_worker.go:46`).
   - `:210-218` optionally persists the hiring user's Helix identity
     for SaaS-embedded operation.
   - `:220-222` fires `Dispatcher.DispatchHire`.
   - Returns `{"id":"w-echo"}`.
7. `Dispatcher.DispatchHire` (`dispatch/dispatcher.go:111`) enqueues a
   `Trigger{Kind: TriggerHire}` and the per-Worker runner goroutine
   takes it (`:191-205`). Spawner runs in its own goroutine with
   `context.Background` — outlives the HTTP request that posted to
   `hire_worker`.
8. Claude runtime path (`agent/claude/spawner.go:93`):
   - `projectEnv` writes `role.md`, `identity.md`, `agent.md` into the
     env directory (`:197-227`). This is the disk projection of the DB.
   - `writeMCPConfig` writes `mcp.json` pointing at
     `<publicURL>/workers/<workerID>/mcp` (`:276-296`).
   - `agent.BuildPrompt` (`agent/prompt.go:24`) assembles the
     activation prompt. For a hire, the literal text is *"You have
     just been hired. This is your first activation. Complete any
     one-time setup your role describes, then exit."* (`prompt.go:37`).
     Everything else the new Worker does — subscribing, creating
     streams, publishing — is **driven by the role markdown that the
     hiring manager wrote**.
   - `:124-128` `exec.CommandContext` claude with `-p <prompt>
     --permission-mode bypassPermissions --output-format stream-json
     --mcp-config <path> --strict-mcp-config` plus `--model`/`--effort`.
   - `:139` publishes `=== activation: hire ===` to the activation
     stream.
   - `:178` `streamTranscript` parses stream-json line-by-line; each
     atomic segment (assistant text, tool_use, tool_result, system
     init, result) becomes one Event on
     `s-activations-<workerID>` via `publishActivationEvent`
     (`:249-271`). That call appends directly to the store and calls
     `Broadcaster.Notify` — it **does not** go through the dispatcher
     (`:240-247`), avoiding an infinite recursion since the new Worker
     would otherwise be triggered by every line it speaks.
9. The new Worker's claude process, running the hire prompt, reads
   `role.md` (the `r-echo` content the owner just wrote) which says
   "on hire, subscribe to s-general." Claude calls
   `mcp__helix__subscribe(streamId=s-general)`. That hits
   `tools/subscribe.go:36`, which inserts a Subscription row.
10. Claude exits. `cmd.Wait` returns; publish
    `=== exit: ok ===` (`spawner.go:182`). Activation runner loops:
    finds the queue empty, sets `running = false`, returns
    (`dispatcher.go:213-216`).
11. Meanwhile the owner's chat brain has been doing follow-up calls
    (`worker_log` with `wait=30`) to read the new Worker's activation
    transcript until it sees `=== exit: ok ===`.

### State changes

- 1 role, 1 position, 1 stream (s-general), 1 worker, 1 environment,
  N grants, 1 activation stream, 1 hiring-worker subscription, 1
  self-subscription by the new Worker to `s-general`, plus 5-20 events
  on the activation stream (one per claude stream-json line).

### Outputs / side effects

- New AI Worker process spawned and exited. Disk: role/identity/agent
  markdown + mcp.json in `<envsDir>/<workerID>/`.
- Activation transcript readable via `worker_log` or `read_events` on
  the activation stream.
- Browser receives streamed HTML fragments via SSE on `/ui/chat/stream`
  reflecting the chat brain's tool calls and replies.

### Pain points

- **Control flow is split.** `hire_worker` does NOT subscribe the new
  Worker to its streams (`hire_worker.go:36-38`). The hiring manager's
  prompt must do that. The owner Role explicitly tells the LLM to
  chain `create_stream → subscribe` (owner_role.md:81-86). If the
  LLM forgets, the worker is "half-hired — they have nothing to listen
  to" (owner_role.md:89-91). The invariant "every hired Worker has
  the subscriptions their Role declares" is not enforced anywhere.
  TODO.md's first bullet acknowledges this.
- **Grants must be bundled.** `owner_role.md:58-63` is explicit that
  granting tools after hire means the Worker can't see them until
  their session restarts. This is enforced socially via the role
  markdown — not in code.
- **The hiring activation has no return value.** The owner has no
  causal "did the Worker finish hire?" signal; the role markdown tells
  the owner to poll with `worker_log` and a 30-second wait
  (`getting-started/README.md:52-53`). TODO.md item 2 ("owner chat
  isn't able to poll or check on status properly") reflects fragility
  here.

---

## Capability 3 — Inbound Event → Worker Activation

The dispatcher's job. Three triggers feed it: in-process `publish`
calls from a Worker, the generic `/webhooks/{streamID}` HTTP endpoint,
and the typed transports (`postmark`, `github`).

### Trigger A — Worker calls `publish`

1. AI Worker's claude makes the MCP call `mcp__helix__publish` with
   `{streamId, body}`.
2. `tools/publish.go:53` parses args, looks up the Stream
   (`:62`). If the stream is GitHub-typed it rejects loudly
   (`:71-73`) — outbound writes belong to `gh` in the env.
3. `:74-91` builds the `domain.Message`, mints an `EventID`,
   appends via `Store.Events.Append`.
4. `:100-101` `Broadcaster.Notify(streamID)` wakes any `read_events`
   long-poll.
5. `:103-105` `Dispatcher.Dispatch(ctx, event)`.

### Trigger B — generic webhook

1. External POST → `/webhooks/{streamID}` →
   `server/webhook.go:33`.
2. Looks up Stream; rejects if not `TransportWebhook`
   (`webhook.go:51`).
3. Wraps raw body bytes in a `domain.Message{Body: ...}` with
   **empty `From`** (`webhook.go:70-77`) — webhook callers have no
   helix identity. `Source` on the Event is also empty
   (`webhook.go:74`).
4. `Events.Append`, `broadcaster.Notify`, `dispatcher.Dispatch`.

### Trigger C — GitHub webhook

1. `POST /github/webhook` → `transports/github/github.go:137`.
2. HMAC-verifies via `transport.github.webhook_secret` config key
   (`:159`). Bad signature → 401.
3. Re-reads its config from `config.Registry` on every delivery
   (`:110-119`) — live updates without restart.
4. `matchingStreams` (`:258`) does a linear scan of all streams,
   filters to `TransportGitHub` with `cfg.Repo` matching `repository.
   full_name` AND `cfg.Events` containing `X-GitHub-Event`.
5. For each matching stream, builds a canonical `Message` with
   `From=sender.login`, `Subject=issue.title|pull_request.title|
   release.name`, `Body=<the user-typed text>`, `Extra=<full payload
   with event header injected>` (`:212-219`). `Source` is empty
   (system-emitted).
6. Append, broadcast, dispatch, per stream (`:222-247`).

### Dispatcher fan-out (shared)

1. `dispatch/dispatcher.go:128` `Dispatch`.
2. `:129` `emitOutbound(ctx, e)` first — if the Stream has a
   `webhook` Transport with `outbound_url`, fires a goroutine to
   POST `event.Body` to that URL with `X-Helix-Stream` /
   `X-Helix-Event` headers (`:314-334`). If the Stream is `email`-
   typed, calls the registered `emailEmitter.Emit` (`:296-305`).
   `Source == ""` (transport-inbound) is **not** re-emitted — see
   the loop-prevention comment (`:266-271`).
3. `:137-141` parses the canonical `Message` envelope from
   `event.Body`; a parse failure logs and falls back to a synthetic
   `Message{Body: e.Body}`.
4. `:142-145` `Subscriptions.ListForStream(streamID)`.
5. `:148-156` resolves the source Worker's kind (human vs AI) once,
   for the `SourceKind` field on the Trigger — used by `agent.md` to
   teach Workers to de-prioritise AI-origin events.
6. `:157-184` loops subscribers:
   - skip the publisher (`:158`)
   - skip if not AI (`:166`) — human Workers don't activate
   - look up env path (`:169`)
   - build a `Trigger{Kind: TriggerEvent, EventID, StreamID, Source,
     SourceKind, Message, CreatedAt}` and `enqueue`.
7. `enqueue` (`:191-205`) appends to a per-Worker `pending` slice. If
   no runner is active, spawns one. **Critical**: if a runner is
   active, the new trigger is just appended; the next `run` iteration
   drains everything in one Spawner call (coalescing). This is the
   defence against webhook cascades quoted in the package comment.
8. `run`/`activate` (`:211-253`) calls the configured Spawner
   synchronously with the batch. Spawner does its thing
   (`agent/claude/spawner.go` or `agent/helix/spawner.go`), publishes
   transcript events to `s-activations-<workerID>`, and returns.
9. While that Spawner runs, more events may arrive and pile up in
   `pending`. When the Spawner returns, the loop picks them up and
   batches them as `len(triggers) > 1`. `agent.BuildPrompt`
   (`agent/prompt.go:27-29`) renders them as a numbered list and tells
   the agent: *"Read all of them before deciding what to do — often
   the latest supersedes earlier ones, and most cascades resolve to
   a single response or to silence."*

### State changes

- One Event row per dispatched event (the trigger).
- Many Event rows on the receiving Worker's activation stream as the
  Spawner streams stream-json output.

### Outputs / side effects

- Outbound HTTP POSTs (5s timeout) for `webhook` streams with
  `outbound_url`.
- Outbound emails via Postmark for `email` streams.
- New process: one `claude` per activation (claude backend) or one
  Helix session re-engaged per activation (helix backend).
- Long-poll observers on the source stream and on the activation
  stream wake.

### Pain points

- **`Dispatch` is invoked from inside a 5s outbound HTTP POST goroutine
  AND from the Spawner's transcript publish path... wait, no — the
  transcript publisher (`agent/claude/spawner.go:249`) deliberately
  does not call dispatch.** The naming is awkward: `publish` (the
  tool) dispatches; `publishActivationEvent` (the spawner internal)
  does not. Two functions doing similar things with different
  semantics, distinguishable only by which package called them.
- **TODO.md item 6**: "quite a lag when multiple events are published
  at the same time. Each agent goes through events one by one. When
  there is a queue of events, they should be batched into one spawn."
  The coalescing logic in `enqueue` *does* batch — but only events
  that arrive **while a Spawner is already running**. A burst of N
  events that lands when no Spawner is running produces N sequential
  Spawner calls (each blocking the queue). The first event blocks the
  Spawner; events 2..N coalesce. So worst-case is one spawn followed
  by one batched spawn, not N. The "lag" in the TODO is the cold-start
  cost of that first spawn.
- **Source attribution is empty for everything inbound.** The Event
  has `Source = ""` for plain webhook, GitHub, email. Roles can't
  distinguish "from outside" via the `source` field — they have to
  branch on `Stream.Transport.Kind`, which `read_events` does not
  return. The Message envelope's `From` is the only carry-through for
  external sender identity. Mixed naming again: `Source` (event-
  level, identity-of-helix-Worker) vs `From` (message-level, free-
  form string).
- **Outbound emit has no retry.** `dispatcher.go:329-333` — 5xx and
  timeout are logged and dropped. Confirmed by `webhook` demo's
  "what this doesn't cover" section.

---

## Capability 4 — Worker Reads the Org Graph (`subscribe` + `read_events`)

### Trigger

An AI Worker activation. Could be a hire (`TriggerHire`) or an event
(`TriggerEvent`). Either way the Worker's claude process is running in
the env directory with `mcp.json` pointing at its own MCP endpoint.

### Timeline

1. Claude fetches the tool list. `server/mcp.go:24` mounts an
   `mcp.NewStreamableHTTPHandler` with **stateless mode** (each
   request is independent). Per request, `buildMCPServer`
   (`mcp.go:67`) rebuilds a tools-filtered `mcp.Server`.
2. `mcp.go:74` `Store.Workers.Get(workerID)` — 404 the worker if it
   doesn't exist. `mcp.go:80` `Store.Grants.ListByWorker(workerID)`.
3. `mcp.go:91-102` loops grants, looks up each tool in
   `s.registry.Get(toolName)`. Unknown tool names log and skip
   silently — "removing the grant is the owner's job." Each tool is
   registered with a handler that closes over the caller Worker.
4. `mcp.go:104-111` registers any MCP prompts gated by a tool the
   Worker holds (`p.RequiresTool()`).
5. Claude's tool-call → SDK → `registerToolForWorker` handler
   (`mcp.go:121`) → `tool.Invoke(ctx, Invocation{Caller, Args})`.
6. Worker calls `subscribe` (`tools/subscribe.go:36`).
   - Looks up the Stream (`:45`).
   - Idempotency: `Subscriptions.Find(workerID, streamID)` — if
     already subscribed, returns the success shape without inserting
     (`:51-54`).
   - Otherwise inserts a Subscription row.
7. Worker calls `read_events` (`tools/read_events.go:128`).
   - Tolerant int parsing for `limit` / `wait` (`:75-117`) — Sonnet
     sometimes returns string-encoded ints.
   - `fresh` (`:188-202`) calls
     `Events.ListForWorker(workerID, limit)` — which returns events
     across **all** streams the Worker subscribes to, newest-first.
     `since` is applied client-side by truncating to the first match.
   - If `fresh` returns non-empty, OR `wait == 0`, OR no broadcaster,
     return immediately (`:154-156`).
   - Otherwise: `Subscriptions.ListForWorker(workerID)` to get the
     full set of stream IDs (`:158-165`), subscribe a wake channel
     for that set via the broadcaster (`:166`), and `select` on
     `wake | timer | ctx.Done` (`:172-177`). The timer caps at 60s
     (`readEventsMaxWaitSecs`).
   - On wake, re-runs `fresh` (`:179-183`) and returns. This is
     "edge-triggered with re-poll" — the channel only fires once per
     publish; if N events publish during the wait, the second wake
     still finds them because `fresh` reads from the store, not from
     the channel.

### State changes

- Subscription insert (idempotent).
- No event mutation. Read-only otherwise.

### Outputs / side effects

- MCP JSON-RPC response with `{events: [...]}` shape (`:204-216`).

### Pain points

- **`read_events` is union-of-streams.** A Worker subscribed to 20
  streams has no way to ask `read_events` for events on just one of
  them. The CLAUDE.md says "humans drive the org through claude" —
  every owner-side filter must be expressed in claude's prompt
  ("ignore everything not on `s-general`"). For an AI Worker this is
  fine; for the owner using `helix-org chat` to read a specific
  stream, it is awkward.
- **No causal `since`.** `since=<eventId>` is a positional skip-list,
  not a causal cursor — if the event is older than the most recent
  `limit` events, it's never found and the loop returns everything.
- **The MCP server is rebuilt per request.** `mcp.go:67-114` does
  three DB calls (`Workers.Get`, `Grants.ListByWorker`, plus
  `Streams.Get` and `Grants.ListByWorker` inside tool handlers). For a
  long-running Worker pumping `read_events` with `wait=60`, the
  outer rebuild is a one-time cost; subsequent tool calls inside the
  same stateless request still pay per-call lookups. Acceptable today,
  fragile if any Worker holds hundreds of grants.

---

## Capability 5 — End-to-End Demo: `demos/webhook`

A condensed trace because it stitches every previous capability
together.

### Trigger sequence (per `demos/webhook/README.md`)

1. Operator runs `helix-org serve --db /tmp/helix-webhook.db
   --envs-dir /tmp/helix-webhook-envs`. Bootstrap creates `w-owner`
   (Capability 1).
2. Operator runs `helix-org chat --new` → claude session opens against
   `/workers/w-owner/mcp`.
3. Operator types the long delegation prompt
   (`demos/webhook/README.md:64-71`): read `roles/secretary.md`,
   `create_role`, `create_stream` for `s-inbox`/`s-outbox`,
   `create_position`, `hire_worker w-secretary` with grants for
   `subscribe`/`dm`/`publish`, then `worker_log` until exit.
4. Claude (running as `w-owner`) makes those MCP calls in order
   (Capability 2). The `hire_worker` call fires `TriggerHire`; the
   secretary's claude reads its role markdown (which presumably says
   "subscribe to `s-inbox`"), subscribes, and exits.
5. Operator: `curl -X POST http://localhost:8080/webhooks/s-inbox
   --data 'Mistral released...'`. Hits `server/webhook.go:33`. Event
   appended on `s-inbox` with empty `Source`.
6. Dispatcher fan-out (Capability 3): `s-inbox`'s `outbound_url` is
   unset, so no outbound POST. The dispatcher loops subscribers,
   finds `w-secretary` (AI), enqueues a `TriggerEvent` with the
   parsed Message.
7. Secretary claude is spawned. Prompt
   (`agent/prompt.go:48`): identity hint + `agent.Policy` mandate +
   the rendered trigger (`renderTrigger`, `prompt.go:71`). The
   trigger renders all fields:
   ```
   A new event arrived on a Stream you subscribe to.
     stream:      s-inbox
     event:       e-<uuid>
     time:        2026-...
     body:
       Mistral released a new 3B model this morning. ...
   ```
   `source` and `source_kind` are omitted because Source is empty.
8. Role tells secretary to summarise and publish to `s-outbox` and
   DM the owner. Secretary calls
   `publish(streamId=s-outbox, body=<summary>)` and
   `dm(workerId=w-owner, body=<summary>)`.
9. `publish` (`tools/publish.go`) appends to `s-outbox`. Dispatcher
   sees the Stream is `TransportWebhook` with `outbound_url=
   http://localhost:9000`. Goroutine fires POST to `nc` (Capability
   3, Trigger A → outbound emit).
10. `dm` (`tools/dm.go`) lazily creates the per-pair DM stream,
    subscribes both parties, publishes. Owner is subscribed → no
    AI activation (owner is human).
11. Owner's chat brain, blocked in `read_events(s-outbox, wait=30)`,
    wakes via the broadcaster, gets the summary event, renders it
    in the UI.

### Pain points

- **The role markdown carries all the orchestration.** The Go code
  does not "subscribe the secretary to `s-inbox`" — the secretary's
  `roles/secretary.md` markdown tells the LLM to do it, and the LLM
  may or may not. If the LLM skips that step, the demo silently fails
  (no events delivered).
- **Outbound to `nc -lk 9000` always times out** because `nc` doesn't
  speak HTTP back. The README admits this. The 5s timeout in the
  dispatcher is what makes the demo workable.
- **No idempotency on the inbound POST.** Same curl run twice
  produces two events, two activations, two outbound POSTs. There is
  no notion of a delivery ID.

---

## Capability 6 — `helix-org chat`

### Trigger

Operator runs `helix-org chat` in a directory.

### Timeline

1. `cmd/helix-org/chat.go:33` `runChat`. Default worker is `w-owner`,
   default server is `http://localhost:8080`.
2. `:56-67` builds the MCP config inline:
   ```json
   {"mcpServers":{"helix":{"type":"http","url":"http://localhost:8080/workers/w-owner/mcp"}}}
   ```
3. `:72-104` builds the claude argv. Notable flags:
   `--permission-mode bypassPermissions`, `--strict-mcp-config`
   (only the helix MCP is visible — user's machine-wide claude MCP
   config is ignored), `--mcp-config <json blob>`.
4. Session continuity (`:88-104`): unless `--new` or `--resume` is
   passed, calls `latestClaudeSessionID()` (`:121`). That function
   reads `~/.claude/projects/<cwd-with-slashes-replaced-by-hyphens>/`
   and picks the newest `.jsonl`, then parses the first line for
   `sessionId`. If found, claude is invoked with `--resume <sid>`. If
   no prior session, claude starts fresh.
5. `:106-112` `syscall.Exec` — replaces the helix-org process with
   claude. helix-org itself exits.
6. Claude now talks to `/workers/w-owner/mcp`. Every tool call runs
   as `w-owner`.

### Pain points

- **The docstring (`:32`) says `helix-org chat` defaults to "continue
  the most recent session" via `--continue`. The code does not use
  `--continue`** (`:97-103` explains why: `--continue` refuses some
  resumable sessions). Instead, it manually picks the newest jsonl
  and passes `--resume <uuid>`. Functional but undocumented.
- **`CLAUDE.md` says `bootstrap --install-claude-mcp` "register[s]
  the owner's MCP endpoint with the local `claude` CLI; from then on
  plain `claude` sessions can drive the org."** That code path does
  not exist. `chat.go` always builds an inline MCP config. There is
  no persistent registration.
- **The chat brain in `/ui/` and the `helix-org chat` CLI both pick
  the latest jsonl in the same `cwd`**, which is the directory where
  `helix-org serve` was launched (`server/chat/chat.go:140`) and the
  directory where `helix-org chat` was launched, respectively. These
  are typically different directories, so "the UI and terminal share
  a conversation" (`server/chat/chat.go:8-12`) only holds when the
  operator was disciplined enough to run both from the same cwd. The
  comment elides this constraint.

---

## Cross-cutting Observations

Patterns and rough edges seen in every flow above.

1. **Workflow lives in role markdown, not code — but the markdown is
   embedded.** The CLAUDE.md design philosophy ("no workflow in code")
   is held to *for downstream Roles* — they are `.md` files in
   `demos/*/roles/`. But the **owner** Role is hard-coded into the
   binary via `//go:embed templates/owner_role.md`
   (`bootstrap/bootstrap.go:28`). The owner cannot be customised at
   bootstrap. Effectively the project has one prompt-driven layer
   (downstream Roles) and one shipped layer (owner), and the seams
   between them aren't documented.

2. **Documentation drifts from code in three concrete places.**
   - `CLAUDE.md` describes `helix-org bootstrap` as the owner-seeding
     command; in reality `serve` does it (`cmd/helix-org/serve.go:73`)
     and `bootstrap` is unrelated (`cmd/helix-org/bootstrap.go:21`).
   - `CLAUDE.md` describes `--install-claude-mcp` and a persistent
     claude MCP registration — none of that exists.
   - `cmd/helix-org/chat.go:32` docstring says `--continue` is used;
     the code says otherwise.

3. **Same concept, different names per layer.**
   - `Source` (event-level, helix `WorkerID`) vs `From` (message-
     level, free-form string). Inbound transports set `Source = ""`
     and `From = <external sender>`; in-org publishes set both.
     Roles must know which to read.
   - "Activation" vs "Trigger" vs "Spawn": one trigger fires one
     spawn; one activation can carry multiple triggers (coalescing).
     The owner Role just says "activation."
   - `agent.PublishActivationEvent` is referenced in bootstrap.go
     comments but doesn't exist as a public symbol; the actual
     function is `publishActivationEvent` in two different package-
     private locations (one per backend).
   - `TriggerHire` is generated server-side in `hire_worker` and
     consumed by the dispatcher. There is no analogous "fire" or
     "leave" trigger — the Worker lifecycle isn't symmetric.

4. **Invariants enforced socially via the owner Role prompt.** Three
   important rules are not enforced by code:
   - Grants must accompany hire (`hire_worker` tolerates an empty
     grant list silently).
   - Hired Workers must be subscribed to streams declared by their
     Role (`hire_worker` does no subscribing).
   - Workers should use `gh` for outbound GitHub action, not
     `publish` (this one IS enforced, `tools/publish.go:71-73`).
   TODO.md item 1 captures the first two and proposes moving them
   into code.

5. **Control flow is split between code and prompt in every
   capability except bootstrap.** Hiring chains 4-5 tool calls that
   the LLM is told to make. Reading new events on a particular stream
   requires the LLM to ignore events from other subscribed streams.
   Responding to a webhook requires the LLM to know its role
   markdown wants a `dm` followed by a `publish`. The Go code does
   exactly one thing per tool. The "second tool call" in any chain
   is the LLM's responsibility. This is the prompt-driven design
   working as intended, but it makes the system non-trivial to debug
   without reading the role markdown alongside the code.

6. **Two backends, one transcript shape.** Both `agent/claude` and
   `agent/helix` publish to `s-activations-<workerID>` with the same
   `assistant: …` / `tool_use …: …` / `tool_result: …` / `=== exit:
   ok ===` line shapes (`agent/claude/spawner.go:368-416` and the
   helix equivalent at `agent/helix/spawner.go:371-385`). The
   `worker_log` tool reads the activation stream and is therefore
   backend-agnostic. This is the clean part.

7. **The dispatcher's coalescing is the only piece of orchestration
   in the codebase.** Everything else is either a thin tool over a
   GORM repo, a transport wrapper, or a prompt. The dispatcher is
   load-bearing for cost predictability under bursty traffic and
   would be the first thing to break if anyone added a "fan-out to
   multiple Spawner processes" feature.

8. **The owner is treated specially in five places**:
   `bootstrap.Run` (seeds them), `dispatcher.Dispatch` (skips them
   for AI activation, `dispatcher.go:166`), `chat.go` (`w-owner`
   default), `serve.go:174` (`Owner: "w-owner"` in SettingsView), and
   `helix_bridge.go` (`OwnerID: "w-owner"`). If multi-owner ever
   matters, those five sites are the audit list.
