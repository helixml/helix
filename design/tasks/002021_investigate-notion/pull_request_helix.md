# Notion integration: webhook trigger + embed-in-row

Connect Notion databases to Helix projects. Each row = one spectask. When the user flips the row's action column (a `select` like `Go/NoGo`, or a `status` workflow column — type-agnostic) Notion's Database Automation POSTs a webhook to Helix; Helix creates a spectask in the configured project and appends a Notion `embed` block to the row's page body so the live Helix UI renders inline. Same flow drives kanban-board views (drag a card across lanes = same property change = same Automation).

Design: https://github.com/helixml/helix/tree/helix-specs/design/tasks/002021_investigate-notion (requirements, design, tasks, findings).

## Summary

- **Backend**: new `api/pkg/trigger/notion/` package with HMAC signature verification, dispatch on `X-Helix-Action` header (`create`/`cancel`), Notion HTTP client (`GetPage`, `PatchRichTextProperty`, `AppendEmbedBlock`, `DeleteBlock`), and lifecycle hooks named generically (`OnExternalCreate`, `OnSpecTaskCompleted`, `OnSpecTaskCancelled`) so a future `SpecTaskSource` interface can lift them out cleanly when Sentry / GitHub Issues land.
- **Types**: `NotionTrigger`, `NotionColumnMap`, `TriggerTypeNotion`, `OAuthProviderTypeNotion`, generic `ExternalTriggerRef` with opaque `json.RawMessage` payload (Notion stores `{page_id, database_id, embed_block_id}`).
- **OAuth**: Notion is now a first-class OAuth2 provider type. HTTP-Basic on the token endpoint, `Notion-Version: 2025-09-03` header on every request, parser for `/v1/users/me` (handles the bot-owner-user nesting OAuth integrations return).
- **Frontend**: per-app Notion trigger config form with auto-generated shared secret and copy-paste webhook URL; OAuth provider preset auto-fills the Notion endpoints.

## Changes

### Backend
- `api/pkg/types/types.go`: `NotionTrigger`, `NotionColumnMap`, `TriggerTypeNotion`; `Trigger.Notion` field.
- `api/pkg/types/oauth.go`: `OAuthProviderTypeNotion`.
- `api/pkg/types/simple_spec_task.go`: `ExternalTriggerRef` (generic, JSONB column on `SpecTask`), `ExternalTriggerSourceType` discriminator (Notion / Sentry-future / GitHub-future), `NotionTriggerPayload`.
- `api/pkg/oauth/notion.go`: `parseNotionUserInfo`, `notionAPIVersion` constant.
- `api/pkg/oauth/oauth2.go`: HTTP-Basic auth style for Notion-typed providers; `Notion-Version` header on `MakeAuthorizedRequest` and on the user-info fetch; Notion case in `getUserInfo`.
- `api/pkg/trigger/notion/`: `notion.go`, `events.go`, `client.go` + 13 unit tests.
- `api/pkg/trigger/trigger.go`: `Manager.ProcessWebhook(ctx, cfg, headers, body)` (extended signature), Notion handler registration, `defaultEmbedURLBuilder`.
- `api/pkg/server/webhook_trigger_handlers.go`: pass `r.Header` through.

### Frontend
- `frontend/src/components/app/TriggerNotion.tsx`: new per-app trigger config UI.
- `frontend/src/components/app/Triggers.tsx`: register `TriggerNotion`.
- `frontend/src/components/settings/OAuthSettings.tsx`: Notion preset (auto-fills authorize/token/user-info URLs on type select).
- `frontend/src/api/api.ts` + `frontend/swagger/swagger.yaml` + `api/pkg/server/swagger.{json,yaml,docs.go}`: regenerated.

## Tests

```
$ CGO_ENABLED=0 go test -v ./api/pkg/trigger/notion/ -count=1
PASS: 13/13 — signature verification (good/bad/mismatched body/mismatched token/empty),
              automation event parsing,
              dispatch (create + idempotency, cancel + embed cleanup, no-op when no live spectask),
              result writeback (Result column only, no-op when ResultColumn empty),
              embed-append-failure-doesn't-block-spectask creation
```

Frontend `tsc --noEmit` passes.

## Deferred — explicit follow-ups (see findings.md for details)

1. **Spectask completion hook wiring** — `Notion.OnSpecTaskCompleted` exists and is tested; needs a ~30-LOC patch in `api/pkg/services/git_http_server.go` (two call sites near `task.Status = types.TaskStatusDone`) to invoke it. The `GitHTTPServer` constructor needs a `*trigger.Manager`.
2. **Idempotency lookup** — `notion.SpecTaskByExternalRefLookup` interface is defined; needs a `GetSpecTaskByExternalRef` method on the spec-task store (JSONB column search) and wiring in `trigger.go`.
3. **Cancel by external ref** — same shape as #2 for `CancelTaskByExternalRef`.
4. **Polished setup wizard** — current form is functional; database-picker and column-name dropdowns sourced live from Notion are a UX win.
5. **Secondary path dispatch** — verification works, dispatch logs and returns nil. Needed if/when the use case extends beyond database-row events.
6. **Embed iframe verification** — uses existing `EmbedTaskPage` route (Gatewaze pattern); needs manual screenshot verification inside a Notion `/embed` block.

## What needs human verification before GA

- Notion webhook payload shape against `events.go AutomationEvent` (real workspace, webhook.site capture).
- Custom headers (`X-Helix-Webhook-Secret`, `X-Helix-Source`, `X-Helix-Action`) actually pass through Notion's Send-webhook action.
- Notion `/embed` block accepts the Helix embed URL via Iframely.
- Full end-to-end demo (see findings.md "What needs human verification").

## Coordination

Sentry workstream owner (Priya) was notified before code landed. The `OnExternalCreate` / `OnSpecTaskCompleted` / `OnSpecTaskCancelled` hook names + `ExternalTriggerRef.Payload` opaque-RawMessage pattern are designed so the second implementation (Sentry) can lift them into a `SpecTaskSource` Go interface mechanically — see design.md "Generalisation".
