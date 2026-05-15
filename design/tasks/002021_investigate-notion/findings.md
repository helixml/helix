# Findings — Notion Integration MVP

Status as of 2026-05-15 (live verification done in Luke's Notion, Business plan). This file records what shipped in the MVP, what was deferred, and what's confirmed against a real Notion workspace.

## Live verification — what we proved

On 2026-05-15 Luke upgraded his personal Notion to Business and ran end-to-end verification with a `Helix Tasks (legacy)` test database (`Name`/`Go/NoGo`/`Prompt`/`Result` columns, integration shared with the parent page). All four Notion API operations + the Database Automation webhook were exercised against the live API. Results:

1. **OAuth/integration token** (`POST /v1/oauth/token` not exercised, but the equivalent **internal-integration token** `ntn_…` works). `GET /v1/users/me` returns the shape parsed by `parseNotionUserInfo` — for workspace-scoped tokens `bot.owner = {"type":"workspace","workspace":true}`, no `user` sub-object; the parser's bot-identity fallback handles this.
2. **Embed-block insert + delete** (`PATCH /v1/blocks/{page_id}/children` with `embed` block, then `DELETE /v1/blocks/{block_id}`) — both returned the documented shapes; delete archives the block.
3. **Result-column writeback** (`PATCH /v1/pages/{page_id}` with `properties.Result.rich_text`) — succeeded; the `Go/NoGo` column was untouched, validating the no-loop invariant by direct observation.
4. **Database Automation `Send webhook` action** — created via the Notion UI (⚡ icon → New automation → "When Go/NoGo is set to Go" → Send webhook). All three custom headers (`X-Helix-Webhook-Secret`, `X-Helix-Source`, `X-Helix-Action`) passed through verbatim. Body contains the page ID + the user-selected property values + a `source` object with `automation_id`/`action_id`/`event_id`/`attempt`. Captured at `api/pkg/trigger/notion/testdata/automation_webhook_create.json`.

## Findings that changed the code

- **API version pin**: changed from `2025-09-03` → `2022-06-28`. The newer version introduces a breaking "data sources" concept where database properties live on a nested data source rather than on the database itself. `POST /v1/databases` with `properties` inline is silently dropped on the new version (only the title property landed in our first attempt). 2022-06-28 still works for everything we need. One-line change in `api/pkg/oauth/notion.go` + `api/pkg/trigger/notion/client.go` with a comment explaining the rationale.
- **Webhook payload shape**: my best-guess `AutomationEvent.Source` was `string`. **Actual:** `Source` is an object (`type`, `automation_id`, `action_id`, `event_id`, `attempt`). Updated `events.go` and added a fixture-driven test (`TestParseAutomationEvent_LiveFixture`) that reads the captured payload and asserts the parser handles it.
- **Notion-Automation User-Agent**: `User-Agent: NotionAutomation`. Useful for filtering / observability.
- **Parent shape**: even on 2022-06-28, the page's `parent` block carries both `database_id` AND `data_source_id`. We extract `database_id` (existing behaviour) and ignore the data-source field for now.
- **OAuth `bot.owner` shape varies**: workspace-scoped internal-integration tokens return `bot.owner = {"type":"workspace","workspace":true}` with no email; OAuth tokens (different flow, not exercised live) return `bot.owner.user.person.email`. Parser's fallback path handles both.
- **API errors are friendly**: 400 responses include a clear `message` field — e.g. `"Go/NoGo is not a property that exists. Prompt is not a property that exists."` (when the database was created on the new API version and properties were silently dropped). Worth surfacing to the user verbatim in the trigger config UI's error toast.

## Coordination

The Sentry workstream owner (Priya) was notified before code landed. Async-collab pitch lives in this task's chat history; the agreement we proposed:

- **Lifecycle method names**: `OnExternalCreate(ctx, cfg, payload) → spectask`, `OnSpecTaskCompleted(ctx, ref, spectask, result)`, `OnSpecTaskCancelled(ctx, ref, spectask)`. Notion shipped with these names verbatim — see `api/pkg/trigger/notion/notion.go`.
- **`ExternalTriggerRef.Payload`** is `json.RawMessage`, opaque to the spectask service. Each source unmarshals its own shape. See `api/pkg/types/simple_spec_task.go` `ExternalTriggerRef` + `NotionTriggerPayload`.
- **Discriminator strings**: `"notion"`, `"sentry"`, `"github_issue"`. Defined in `api/pkg/types/simple_spec_task.go` as `ExternalTriggerSourceType` constants.

Confirm + amend if Priya pushes back on any of the above.

## Coordination

The Sentry workstream owner (Priya) was notified before code landed. Async-collab pitch lives in this task's chat history; the agreement we proposed:

- **Lifecycle method names**: `OnExternalCreate(ctx, cfg, payload) → spectask`, `OnSpecTaskCompleted(ctx, ref, spectask, result)`, `OnSpecTaskCancelled(ctx, ref, spectask)`. Notion shipped with these names verbatim — see `api/pkg/trigger/notion/notion.go`.
- **`ExternalTriggerRef.Payload`** is `json.RawMessage`, opaque to the spectask service. Each source unmarshals its own shape. See `api/pkg/types/simple_spec_task.go` `ExternalTriggerRef` + `NotionTriggerPayload`.
- **Discriminator strings**: `"notion"`, `"sentry"`, `"github_issue"`. Defined in `api/pkg/types/simple_spec_task.go` as `ExternalTriggerSourceType` constants.

Confirm + amend if Priya pushes back on any of the above.

## What shipped

### Backend
- `api/pkg/types/`: `NotionTrigger`, `NotionColumnMap`, `TriggerTypeNotion`, `OAuthProviderTypeNotion`, `ExternalTriggerRef` (generic), `NotionTriggerPayload` (Notion-specific, source-owned).
- `api/pkg/oauth/notion.go`: parser for Notion's `/v1/users/me` (handles bot-owner-user nesting). `oauth/oauth2.go` augmented to use HTTP-Basic on Notion's token endpoint and to send `Notion-Version: 2025-09-03` on every request.
- `api/pkg/trigger/notion/`: full webhook ingest, signature verification (shared secret + HMAC for the secondary path), dispatch on `X-Helix-Action` header (`create` / `cancel`), Notion HTTP client (`GetPage`, `PatchRichTextProperty`, `AppendEmbedBlock`, `DeleteBlock`), lifecycle hooks (`OnExternalCreate`, `OnExternalCancel`, `OnSpecTaskCompleted`, `OnSpecTaskCancelled`).
- `api/pkg/trigger/trigger.go`: `Manager.ProcessWebhook` extended to take request headers (Azure-DevOps path is unchanged); Notion handler registered; `defaultEmbedURLBuilder` stitches `${WebServer.URL}/embed/task/{id}?access_token=...`.
- `api/pkg/server/webhook_trigger_handlers.go`: passes `r.Header` through to the manager.
- 13 passing unit tests (`go test ./api/pkg/trigger/notion/`): signature verification (good / bad / mismatched body / mismatched token / empty), automation-event parsing, dispatch (create + idempotency, cancel + embed cleanup, cancel-with-no-spectask no-op), result writeback (Result column only, no-op when ResultColumn empty), embed-append-failure-doesn't-block-spectask.

### Frontend
- `frontend/src/components/settings/OAuthSettings.tsx`: Notion preset auto-fills authorize/token/user-info URLs when admin picks `type=notion`.
- `frontend/src/components/app/TriggerNotion.tsx`: per-app trigger config UI. Auto-generates the shared secret on first enable; shows webhook URL + secret with copy buttons; text fields for project ID, OAuth connection ID, database ID, embed access token, and the column mapping (action column name + type, create/cancel options, prompt/result columns).
- API client regenerated (`frontend/src/api/api.ts`, `frontend/swagger/swagger.yaml`, `api/pkg/server/swagger.{json,yaml,docs.go}`).

## Deferred — explicit next steps

### 1. Spec-task completion hook (small wiring patch)
The `Notion.OnSpecTaskCompleted` and `Notion.OnSpecTaskCancelled` methods exist and are tested. They need to be invoked at the spectask lifecycle transition points. Two call sites:

- `api/pkg/services/git_http_server.go:1131` (`task.Status = types.TaskStatusDone` after auto-merge)
- `api/pkg/services/git_http_server.go:1180` (`task.Status = types.TaskStatusDone` in `handleMainBranchPush`)

After each `task.Status = types.TaskStatusDone` block, add:
```go
if task.ExternalTriggerRef != nil && task.ExternalTriggerRef.Type == types.ExternalTriggerSourceNotion {
    if cfg, err := s.store.GetTriggerConfiguration(ctx, &store.GetTriggerConfigurationQuery{ID: task.ExternalTriggerRef.TriggerConfigID}); err == nil {
        if err := s.triggerManager.Notion().OnSpecTaskCompleted(ctx, cfg, task, "Spectask complete — see Helix for details"); err != nil {
            log.Warn().Err(err).Str("task_id", task.ID).Msg("notion: writeback failed")
        }
    }
}
```

GitHTTPServer does not currently hold a reference to the trigger manager — passing one in via the constructor is the surgery. This is roughly 30 LOC across the constructor + two call sites and was kept out of this PR to keep the diff focused on the Notion package itself.

### 2. Idempotency lookup wiring
`Notion.OnExternalCreate` calls `n.lookup.GetSpecTaskByExternalRef(...)` if the lookup interface is supplied. The MVP wires `nil` so re-fired create events would create duplicates. To wire it: implement `GetSpecTaskByExternalRef(ctx, ref *types.ExternalTriggerRef) (*types.SpecTask, error)` on the spec-task store (search the JSONB column for matching type + page_id), then re-construct the Notion handler in `trigger.go SetSpecTaskCreator` passing it in.

### 3. Cancel wiring
Same shape — implement `CancelTaskByExternalRef` on the spec-task service and pass it into `notion.New`.

### 4. Polished setup wizard
`TriggerNotion.tsx` is a flat form, not a wizard. The user has to know what to put in each field. A polished wizard would:
- Call `POST /v1/search` with the OAuth token to populate a database picker.
- Fetch the picked database's schema and present action-column / option dropdowns.
- Surface a "Test setup" button that POSTs synthetic create + cancel payloads.

The current form is sufficient to configure a working trigger — the wizard is UX polish, not a correctness gap.

### 5. Secondary path (API webhook subscription)
Verification + handshake code for `X-Notion-Signature` is implemented and tested. Dispatch (page-content-updated, comment-created → agent prompt) is stubbed — the handler logs and returns nil. Wire when there's a real use case beyond the primary database-row flow.

### 6. Embed iframe verification
Not tested against a real Notion `/embed` block yet. The existing `EmbedTaskPage` route (`frontend/src/router.tsx:480`) and its `?access_token=` flow are reused from the Gatewaze pattern (`frontend/src/hooks/useApi.ts:62`). Likely outcomes (per design.md):
- Just works (Helix doesn't currently set restrictive frame-ancestors).
- Iframely refuses metadata extraction → add OG tags to `EmbedTaskPage`.
- CSP blocks → relax for `/embed/*` only, allowlist `*.notion.so` + `*.notion.site`.

Capture screenshots in `screenshots/` after manual verification.

## What needs human verification

These cannot be done autonomously — each requires a real Notion Business workspace with the integration installed:

1. **Webhook payload shape**: confirm the actual JSON Notion sends from a Database Automation matches `events.go AutomationEvent`. Best-effort guess based on Notion's docs as of May 2026; needs webhook.site capture.
2. **Custom headers on Notion's webhook actions**: confirm `X-Helix-Webhook-Secret`, `X-Helix-Source`, `X-Helix-Action` actually pass through (Notion's webhook UI advertises arbitrary headers; needs verification).
3. **Embed block insertion**: `PATCH /v1/blocks/{page_id}/children` should accept an `embed` block with `{url}` body — confirmed in docs but verify the response shape (we extract `results[0].id`).
4. **Iframely + Notion embed**: paste `https://app.helix.ml/embed/task/{id}?access_token={key}` into a Notion `/embed` block and screenshot.
5. **End-to-end demo**: see tasks.md "End-to-end demo" section.

## Auth tradeoff (resolved per reviewer feedback)

Reviewer flagged the per-trigger service-account-mint approach as overengineered. Adopted the simpler **trigger-creator's user API key** model (consistent with Gatewaze): admin pastes their own Helix API key into the trigger config's `embed_access_token` field. All Notion viewers see Helix as the trigger creator. Cookie-based auth (correct viewer identity) was considered but deferred to v2 — depends on third-party-cookie support which is increasingly unreliable in modern browsers.

## Generalisation note

Notion is the first of an expected sequence (Sentry, GitHub Issues, Linear, …). The lifecycle hook names + opaque `ExternalTriggerRef.Payload` are deliberately generic; the second implementation should lift them into a `SpecTaskSource` Go interface (see design.md "Generalisation"). Until then, the spectask service's dispatch is one tiny `switch ref.Type` per lifecycle hook (see "Deferred — 1." above for the exact shape).
