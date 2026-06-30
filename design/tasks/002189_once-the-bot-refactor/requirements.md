# Requirements: Per-Bot Toggle to Preserve Conversation Context Across Triggers

## Background

This is an **experiment** for Slack support in the helix-org subsystem
(`api/pkg/org/`). It **depends on the bot refactor** (spectask
`002185_i-need-to-merge-the` — "Merge Role and Worker into a single Bot
concept") having landed first. Write/implement in terms of the merged
**Bot** aggregate, not the old Role/Worker split.

### Current behaviour (the thing we want to make optional)

When an org bot is triggered (e.g. a Slack message routed to it by the
Slack auto-router → topic subscription → activation), the Helix spawner
runs the bot's prompt on its **one long-lived chat session**. Before every
re-activation the spawner **wipes** the prior conversation:

- `api/pkg/org/infrastructure/runtime/helix/spawner.go:468-476` —
  `ensureSession` calls `c.Client.ClearSession(...)` whenever a persisted
  session already exists.
- `ClearSession` (documented at
  `api/pkg/org/infrastructure/runtime/helix/sessions.go:59-65`) wipes the
  DB interactions and, for a Zed/ACP session, resets the Zed thread, so
  each turn starts on a fresh context window.

The wipe exists to avoid the long-lived session growing until it hits the
model context limit and triggers expensive, lossy compaction. The trade-off
is that **every trigger starts from scratch**: the Zed thread is reset, the
agent re-reads its workspace, and there is no in-session memory of the
previous exchange — which makes Slack replies slower and more forgetful.

## Goal

Add a per-bot config toggle (default **off** = preserve today's
wipe-every-trigger behaviour) that, when **on**, makes the spawner **skip
`ClearSession`** so the bot keeps its conversation/context across triggers.
The expected payoff for Slack: faster responses (no per-trigger Zed-thread
reset / cold re-read) and better cross-message understanding.

## User Stories

### US-1 — Operator enables "keep context" on a bot
As an org operator, I want a toggle on a bot's config to keep its
conversation context across triggers, so its Slack replies are faster and
context-aware.
- **AC1** The bot detail page has a labelled toggle (e.g. "Preserve context
  across triggers" / "Don't wipe context on every trigger") with help text
  explaining the speed-vs-compaction trade-off.
- **AC2** The toggle defaults to **off** for new and existing bots —
  current wipe-on-every-trigger behaviour is unchanged unless explicitly
  enabled.
- **AC3** Toggling it persists via the bot update endpoint and survives a
  reload (round-trips through `BotDTO`).

### US-2 — Spawner honours the toggle
As the runtime, I want the activation path to respect the bot's setting.
- **AC1** When the bot's flag is **off** (default), `ensureSession` clears
  the session before re-activation exactly as today.
- **AC2** When the flag is **on**, `ensureSession` **skips `ClearSession`**;
  the follow-up prompt lands on the existing session/Zed thread, preserving
  prior context.
- **AC3** First activation (no persisted session yet) is unaffected — there
  is nothing to clear either way.

### US-3 — Field is persisted and exposed end-to-end
As the client and DB, I want the new flag threaded through every layer.
- **AC1** The Bot aggregate carries the boolean; persistence (gorm +
  memory) stores and round-trips it; `BotDTO` + the bot create/update
  request bodies expose it as JSON (default false when omitted).
- **AC2** Existing rows without the column read back as `false` (AutoMigrate
  adds the new boolean column; no data migration / re-bootstrap needed —
  helix-org is pre-release).

## Out of Scope

- The standalone app-based Slack trigger (`api/pkg/trigger/slack/`,
  `slack_threads.go` thread→session mapping) — that is a different code
  path from org-bot activations and is not changed here.
- Any new compaction/summarisation strategy. When the toggle is on we
  simply accept Helix's existing context-limit/compaction behaviour.
- Per-stream / per-thread granularity. A bot has **one** durable session,
  so the setting is naturally per-bot. (We considered a stream-config
  toggle — see design.md "Decision" — and chose per-bot.)
- MCP exposure of the flag is optional (REST/UI is the required surface).

## Constraints / Notes

- Follow helix-org philosophy: minimal Go, small interfaces, change the
  domain aggregate first then ripple outward (domain → app/infra →
  interfaces → frontend). Extend the existing `_test.go` suites (TDD).
- Default-off is load-bearing: this must be a strictly additive, opt-in
  experiment that cannot regress existing bots.
