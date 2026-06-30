# Design: Per-Bot Toggle to Preserve Conversation Context Across Triggers

## Where the wipe happens (the one line to gate)

`api/pkg/org/infrastructure/runtime/helix/spawner.go`, `ensureSession`:

```go
// Re-activation of an existing session: clear the prior conversation ...
if state.SessionID != "" {
    if err := c.Client.ClearSession(ctx, state.SessionID); err != nil {
        return "", fmt.Errorf("clear session %s before re-activation: %w", state.SessionID, err)
    }
    ...
}
```

This is the only place context is wiped per trigger. The fix is to gate
this block on a per-bot boolean. Everything else (`EnsureAndSend`, polling,
the mirror) is unchanged.

## Decision: per-Bot flag, not per-stream

The user floated "a bot config (or a stream config?)". A bot has exactly
**one** durable Helix session reused across all its activations (see the
`ensureSession` doc comment and `sessions.go`). Context lives in that single
session, not per-stream and not per-thread. So "preserve context" is
inherently a property of the bot's session, and a per-stream toggle would
have no place to apply itself — multiple subscribed streams all feed the
same session. **Therefore: one boolean on the Bot aggregate.** This is also
the simplest thing that matches the helix-org "data over code" philosophy.

Name: `PreserveContext` (Go field) / `preserve_context` (JSON + DB column).

## Layer-by-layer changes

The merged Bot model from spectask 002185 is already in `main`. The
symbols below are **verified present** (2026-06-30): `orgchart.Bot` +
`With*` builders (`domain/orgchart/bot.go`), `botRow`/`org_bots`
(`infrastructure/persistence/gorm/bot.go`), `store.Bots` with
`Get/List/Create/Update` (`domain/store/store.go`), and `BotDTO` /
`CreateBotRequest` / `UpdateBotRequest` (`interfaces/server/api/dto.go`).

### 1. Domain — `api/pkg/org/domain/orgchart/bot.go`
- Add `PreserveContext bool` to the `Bot` struct (zero value `false` =
  today's behaviour, so no constructor changes are forced).
- Add a `WithPreserveContext(bool) Bot` immutable builder, mirroring
  `WithContent`/`WithTools`.
- No new validation rule (a bool is always valid). Extend `bot_test.go`
  for the builder + round-trip.

### 2. Persistence — gorm + memory
- `infrastructure/persistence/gorm/bot.go` (the merged role/worker row):
  add `PreserveContext bool \`gorm:"not null;default:false"\`` to the row
  struct; map it in `ToRow`/`ToDomain`; add `"preserve_context"` to the
  `Update(...)` `WithUpdates` map (booleans serialise fine as a plain map
  value — unlike the json `tools`/`topics` columns, no pre-marshal needed).
- `infrastructure/persistence/memory/memorystore.go`: carry the field in
  the in-memory bot mapper/copy.
- **Migration:** none needed beyond AutoMigrate adding the column. Existing
  rows default to `false`. (helix-org is pre-release; the 002185 refactor
  already wipes+recreates these tables.)

### 3. Application — bot update path
- The bot update use case (`application/...` — the merged
  roles/update_identity service) must accept and persist `PreserveContext`.
  Thread it through whatever `UpdateBot`/`Patch` command 002185 produces,
  using `WithPreserveContext`. Treat an omitted value on update as "leave
  unchanged" if the update model is patch-style; otherwise carry it
  explicitly.

### 4. Interfaces — REST DTO (required) + MCP (optional)
- `interfaces/server/api/dto.go`: add `PreserveContext bool
  \`json:"preserve_context"\`` to `BotDTO` and `CreateBotRequest`. On
  `UpdateBotRequest` use a **pointer** — `PreserveContext *bool
  \`json:"preserve_context,omitempty"\`` — to match the existing patch
  style (`Content *string`, "nil = leave unchanged"). Map it in the
  handler ↔ domain conversions (nil → don't call `WithPreserveContext`).
- MCP (`interfaces/mcptools`): optional. If exposed, add an optional
  `preserve_context` arg to `update_bot` (and/or `create_bot`) in
  `schema.go` + the handler. Per the requirements this is optional; the
  REST/UI surface is the must-have. Keep the MCP surface small (helix-org
  philosophy) — prefer NOT adding it unless an agent genuinely needs to
  set it.

### 5. Runtime — honour the flag (the actual behaviour change)
In `spawner.go::ensureSession`, after `LoadState`, load the bot and gate
the clear:

```go
// workerID is already typed orgchart.BotID in the merged code.
bot, err := c.Store.Bots.Get(ctx, orgID, workerID)
if err != nil {
    return "", fmt.Errorf("load bot %s: %w", workerID, err)
}
if state.SessionID != "" && !bot.PreserveContext {
    if err := c.Client.ClearSession(ctx, state.SessionID); err != nil { ... }
    ...
}
```

Notes:
- `SpawnerConfig` already holds `Store *store.Store` and `workerID` is
  `orgchart.BotID`, so no new constructor wiring or interface is required —
  just one `c.Store.Bots.Get` read. The `ensureSession` signature in `main`
  is `func (c SpawnerConfig) ensureSession(ctx, orgID, workerID orgchart.BotID, prompt string, _ func(string))`.
- When `PreserveContext` is true we deliberately skip the clear. The
  follow-up `EnsureAndSend` then `SendMessage`s onto the existing session
  and (for Zed/ACP) the **same Zed thread**, which is exactly what removes
  the per-trigger cold-start cost.
- Log which branch was taken (`cleared` vs `preserved`) for observability.

### 6. Frontend — bot detail toggle
- In `HelixOrgBotDetail.tsx` (merged role/worker detail from 002185), add a
  toggle bound to `preserve_context`, wired through `helixOrgService.ts`'s
  bot-update call and the `Bot` type in `types.ts`. Use the generated API
  client (repo rule). Label + help text covering the trade-off.

## Trade-off to document in the UI help text

Preserving context keeps the session warm and context-rich (faster,
smarter follow-ups) but lets the transcript grow toward the model's context
limit, where Helix's existing compaction kicks in (slower/lossy at that
point). Durable bot state still belongs in the git workspace markdown, not
the chat history. This is an opt-in experiment, hence default-off.

## Testing

- Go unit: extend `spawner_test.go` — assert `ClearSession` is **not**
  called on re-activation when the bot has `PreserveContext=true`, and
  **is** called (once, before `SendMessage`) when false. The existing
  fake client already records `ClearSession` calls (`clearedBeforeSend`,
  `lastClearSID`).
- Persistence/DTO round-trip tests for the new field.
- End-to-end (per repo rules, mandatory for lifecycle changes): in the
  inner Helix, create a bot, enable the toggle, trigger it twice via the
  Slack auto-router (or a manual re-activation), and confirm the second
  turn continues the **same** session/Zed thread (context preserved) rather
  than a reset one. Compare against a default-off bot to confirm no
  regression.

## Risks

- **Context-limit blowups:** a busy preserved-context bot will eventually
  compact. Acceptable for an experiment; the default-off keeps it contained.
- **Refactor coupling:** field/accessor names depend on 002185's final
  shape. Mitigate by implementing strictly after it lands and grepping for
  the real post-refactor symbols (`Bot`, `org_bots`, `store.Bots`,
  `BotDTO`, `ensureSession`).
