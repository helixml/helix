# Notion integration: webhook trigger + embed-in-row + writeback

Connect Notion databases to Helix projects. Each row = one spectask. When the user flips the row's action column (a `select` like `Go/NoGo`, or a `status` workflow column — type-agnostic) Notion's Database Automation POSTs a webhook to Helix; Helix creates a spectask in the configured project, writes the spec-task URL back into a column on the row, writes an initial status into a Result column, and appends a live `embed` block to the row's page body. The user sees everything happen inside Notion without leaving. Same flow drives kanban-board views (drag a card across lanes = same property change = same Automation).

Design + findings: https://github.com/helixml/helix/tree/helix-specs/design/tasks/002021_investigate-notion

## What you'll see in Notion when Helix picks up a row

![Notion page with all four writebacks](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002021_investigate-notion/screenshots/10-notion-fish-page-with-embed.png)

That single Notion page shows:
- **Helix Task** URL column — clickable link to the live Helix task page
- **Result** column — initial `🟡 Helix picked this up — see the task page or the embed below.` status (overwritten with the agent's summary on completion when the completion hook lands)
- **Embed block in the page body** — the live Helix UI rendering inside Notion (fully interactive, including "Start Planning" button)
- **Go/NoGo** column — left untouched, owned by the user

## Helix side

![Helix kanban with Notion-triggered spec tasks](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002021_investigate-notion/screenshots/11-helix-kanban-final-state.png)

## Summary

- **Backend**: new `api/pkg/trigger/notion/` package with shared-secret + HMAC signature verification, dispatch on `X-Helix-Action` header (`create`/`cancel`), Notion HTTP client (`GetPage`, `PatchRichTextProperty`, `PatchURLProperty`, `AppendEmbedBlock`, `DeleteBlock`), and lifecycle hooks named generically (`OnExternalCreate`, `OnExternalCancel`, `OnSpecTaskCompleted`, `OnSpecTaskCancelled`) so a future `SpecTaskSource` interface can lift them out cleanly when Sentry / GitHub Issues land.
- **Auth**: dual-path. Either an OAuth connection (full Notion OAuth flow), OR a direct `IntegrationToken` (`ntn_…`) for the simpler internal-integration setup. `resolveAccessToken` picks whichever is set.
- **URL writeback**: on create, Helix patches the configured `Helix Task` URL column with `${PublicURL}/task/{spectask_id}` so the row becomes a clickable link the moment the agent picks it up.
- **Initial Result writeback**: on create, Helix patches the configured Result column with `🟡 Helix picked this up — see the task page or the embed below.` so the table immediately shows acknowledgement.
- **Embed block insertion**: on create, Helix appends an `embed` block to the row's page body pointing at `${PublicURL}/embed/task/{spectask_id}?access_token=…` — the live Helix UI renders inside Notion.
- **PublicURL override**: trigger config carries an optional `PublicURL` field that overrides `WebServer.URL` for URLs written into Notion. Required when the deployment URL isn't reachable from Notion (e.g. localhost in dev, internal-only deployment).
- **Types**: `NotionTrigger`, `NotionColumnMap`, `TriggerTypeNotion`, `OAuthProviderTypeNotion`, generic `ExternalTriggerRef` with opaque `json.RawMessage` payload.
- **OAuth**: Notion is a first-class OAuth2 provider type. HTTP-Basic on the token endpoint, `Notion-Version: 2022-06-28` header on every request (pinned to the pre-data-sources version that matches our property-create / property-patch payloads), parser for `/v1/users/me` (handles both bot-owner-user and bot-owner-workspace shapes).
- **Frontend**: per-app Notion trigger config form with auto-generated shared secret and copy-paste webhook URL; OAuth provider preset auto-fills the Notion endpoints.

## Live demo verified end-to-end

Real Notion Business workspace → real cloudflared tunnel → real Helix. Captured timestamps:

| Time | Event |
|------|-------|
| 21:11:25 | wiped Result + Helix Task columns via Notion API |
| 21:11:25 | flipped `Go/NoGo: Backlog → NoGo` (no automation fires — wrong direction) |
| 21:11:30 | flipped `Go/NoGo: NoGo → Go` (fires Database Automation) |
| 21:11:29 (Helix) | `webhookTriggerHandler` received, `notion: prompt column empty, using row name as prompt`, `Created spec task spt_01ks62m7qrsb3sy5d3p5k5g4pk` |
| 21:11:30 | Notion `Helix Task` column populated with `https://elimination-course-scenario-short.trycloudflare.com/task/spt_01ks62m7qrsb3sy5d3p5k5g4pk` |
| 21:11:30 | Notion `Result` column populated with initial status |
| 21:11:30 | Notion page body: embed block appended pointing at the tunnel URL |
| (immediate) | Helix kanban shows the new spec task |

## Bugs caught and fixed during live testing

Each bug below was hit on a real Notion fire and fixed in the same session:

1. **`store_trigger_configurations.go` switch missed `Trigger.Notion`** — `POST /api/v1/triggers` returned 500 "trigger type not specified". Added the case.
2. **`app_trigger_handlers.go` only populated `WebhookURL` for AzureDevOps** — Notion triggers came back with empty `webhook_url`. Added the Notion case in both list and create handlers.
3. **`AutomationEvent.Source` was `string`** in my best-guess parser; Notion actually sends an object with `type`/`automation_id`/`action_id`/`event_id`/`attempt`. Updated struct + added `TestParseAutomationEvent_LiveFixture` that reads the captured real webhook body.
4. **API version `2025-09-03` silently dropped database properties** because of a new "data sources" concept; pinned to `2022-06-28`.
5. **Prompt column empty caused 500** — single-column UX ("make me a fish" as title, nothing else) was the natural interaction and we should honour it. Fall back to row Name when Prompt is empty.
6. **OAuth connection ID required** even for internal-integration token customers — added direct `IntegrationToken` path via `resolveAccessToken`.
7. **URL writeback wasn't wired** — only the embed block was supposed to land in Notion; nothing showed in the table. Added `writeHelixTaskURL` + `HelixTaskURLColumn` + initial Result PATCH so the user sees acknowledgement immediately.
8. **`localhost:8080` URLs in writebacks** — Notion can't reach the deployment's default URL in dev. Added `PublicURL` override on the trigger config; URLs Helix writes to Notion now use it.
9. **Notion auto-pauses automations on 5 consecutive 500s** (gotcha to document): once the dispatch was buggy long enough, Notion's circuit breaker disabled the automation — user must re-enable it in the Notion UI.

## Changes

### Backend
- `api/pkg/types/types.go`: `NotionTrigger` (now with `IntegrationToken`, `PublicURL`), `NotionColumnMap` (now with `HelixTaskURLColumn`), `TriggerTypeNotion`; `Trigger.Notion` field.
- `api/pkg/types/oauth.go`: `OAuthProviderTypeNotion`.
- `api/pkg/types/simple_spec_task.go`: `ExternalTriggerRef` (generic, JSONB column on `SpecTask`), `ExternalTriggerSourceType` discriminator, `NotionTriggerPayload`.
- `api/pkg/oauth/notion.go`: `parseNotionUserInfo`, `notionAPIVersion = "2022-06-28"`.
- `api/pkg/oauth/oauth2.go`: HTTP-Basic auth style for Notion-typed providers; `Notion-Version` header on `MakeAuthorizedRequest` and user-info fetch; Notion case in `getUserInfo`.
- `api/pkg/store/store_trigger_configurations.go`: Notion case in trigger-type switch.
- `api/pkg/server/app_trigger_handlers.go`: populate `WebhookURL` for Notion triggers (list + create).
- `api/pkg/server/webhook_trigger_handlers.go`: pass `r.Header` through to the trigger manager.
- `api/pkg/trigger/notion/`: `notion.go` (lifecycle hooks + dual auth + URL writeback + embed + name fallback), `events.go`, `client.go` (with `PatchURLProperty`), `testdata/automation_webhook_create.json` (real captured webhook).
- `api/pkg/trigger/trigger.go`: `Manager.ProcessWebhook(ctx, cfg, headers, body)` extended signature; Notion handler registration; `defaultEmbedURLBuilder`.

### Frontend
- `frontend/src/components/app/TriggerNotion.tsx`: new per-app trigger config UI.
- `frontend/src/components/app/Triggers.tsx`: register `TriggerNotion`.
- `frontend/src/components/settings/OAuthSettings.tsx`: Notion preset (auto-fills authorize/token/user-info URLs on type select).
- API client + swagger regenerated.

### Dev environment
- `CLAUDE.md`: note about cloudflared quick tunnels for inner-Helix webhook testing (no signup, no auth token).

## Tests

```
$ CGO_ENABLED=0 go test -v ./api/pkg/trigger/notion/ -count=1
PASS: 15/15
- VerifySharedSecret good/bad/empty
- VerifyNotionSignature good/bad-body/bad-token/empty
- ParseAutomationEvent + ParseAutomationEvent_LiveFixture (real Notion payload)
- ProcessWebhook rejects bad secret + unknown source
- OnExternalCreate creates spectask + appends embed + writes URL column (IntegrationToken path)
- OnExternalCreate idempotent on replay (existing-spectask path)
- OnExternalCreate append-embed failure doesn't block spectask
- OnExternalCancel removes embed + cancels in-flight
- OnExternalCancel is no-op when no live spectask
- OnSpecTaskCompleted patches Result column only
- OnSpecTaskCompleted is no-op when ResultColumn empty
```

Frontend `tsc --noEmit` passes.

## Deferred — explicit follow-ups

1. **Spectask completion hook wiring** — `Notion.OnSpecTaskCompleted` exists, tested, and overwrites the initial `🟡` status with the final summary. Needs a ~30-LOC patch in `api/pkg/services/git_http_server.go` (two call sites near `task.Status = types.TaskStatusDone`) to invoke it. The `GitHTTPServer` constructor needs a `*trigger.Manager`.
2. **Idempotency lookup** — `notion.SpecTaskByExternalRefLookup` interface defined; needs a `GetSpecTaskByExternalRef` method on the spec-task store (JSONB column search) and wiring in `trigger.go`.
3. **Cancel by external ref** — same shape as #2 for `CancelTaskByExternalRef`.
4. **Polished setup wizard** — current form is functional; database-picker and column-name dropdowns sourced live from Notion are a UX win.
5. **Auto-re-enable paused automations** — Notion auto-pauses after 5 consecutive 500s; ideally Helix should detect this (e.g. a `Test setup` button that POSTs synthetic events) and the trigger config UI should surface a "paused in Notion" state.
6. **Secondary path dispatch** — verification works, dispatch logs and returns nil. Needed if/when the use case extends beyond database-row events.

## Coordination

Sentry workstream owner (Priya) was notified before code landed. The `OnExternalCreate` / `OnSpecTaskCompleted` / `OnSpecTaskCancelled` hook names + `ExternalTriggerRef.Payload` opaque-RawMessage pattern are designed so the second implementation (Sentry) can lift them into a `SpecTaskSource` Go interface mechanically — see design.md "Generalisation".

## Screenshots (all in `helix-specs/design/tasks/002021_investigate-notion/screenshots/`)

| # | What |
|---|------|
| 03 | New Notion trigger card rendered in Helix Triggers tab |
| 06 | Helix kanban after Notion `DEMO @ 18:58:27` fire — direct path, no relay |
| 07 | Pure Notion → cloudflared → Helix end-to-end (no relay) |
| 08 | "make me a fish" with empty Prompt — name fallback kicks in |
| 09 | Notion table view showing `Helix Task` URL column populated |
| 10 | **Notion fish page with all four writebacks: URL, Result, embed iframe rendering live Helix UI** |
| 11 | Helix kanban final state |
