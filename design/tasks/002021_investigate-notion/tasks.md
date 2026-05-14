# Implementation Tasks

## Coordination (do first)
- [ ] Sync with the Sentry-integration workstream owner before writing code: agree on the names of the lifecycle methods (`OnExternalCreate` / `OnSpecTaskCompleted` / `OnSpecTaskCancelled`), the shape of `ExternalTriggerRef.Payload` (`json.RawMessage` opaque to the spectask service, owned by each source's own package), and the discriminator value (`type: "notion"` vs `"sentry"`) so the future `SpecTaskSource` interface extraction is mechanical. Drop a note in #engineering linking both spec tasks. Capture the agreement at the top of the findings doc.

## Discovery (do first)
- [ ] Register a Notion **internal integration** in a paid-plan test workspace; capture client ID, client secret, and an integration-token PAT for solo testing
- [ ] Create a `Go/NoGo` select column in a test database. Manually create a Database Automation that fires "When `Go/NoGo` is set to `Go`" with action "Send webhook" → confirm payload shape (which fields land, that custom headers like `X-Helix-Action: create` come through) using webhook.site
- [ ] Repeat for the `NoGo` direction with `X-Helix-Action: cancel`
- [ ] Manually add a Button property column with a "Send webhook" action → confirm payload shape and that we can distinguish it from the Automation payload via `X-Helix-Source: notion-button`
- [ ] Manually `PATCH /v1/blocks/{page_id}/children` to insert an `embed` block in a test row → confirm the URL renders inline in Notion, capture the returned block ID; then `DELETE /v1/blocks/{block_id}` to confirm we can remove it
- [ ] `POST` a hand-crafted secondary-path webhook payload at a local endpoint to confirm `X-Notion-Signature` HMAC scheme matches the docs
- [ ] Paste `https://app.helix.ml/embed/task/{task_id}?access_token={key}` into a Notion `/embed` block in a test page; screenshot the result to `screenshots/01-notion-embed-baseline.png`
- [ ] Inspect the response headers Helix returns on `/embed/*` (X-Frame-Options, CSP) and document in the findings doc

## Backend — types & OAuth
- [ ] Add `NotionTrigger`, `NotionColumnMap` structs + `Notion` field on `Trigger` in `api/pkg/types/types.go`
- [ ] Add `TriggerTypeNotion = "notion"` constant
- [ ] Add `ExternalTriggerRef` struct (with `NotionEmbedBlockID` field) + JSONB column on `SpecTask`
- [ ] Add `OAuthProviderTypeNotion = "notion"` in `api/pkg/types/oauth.go`
- [ ] Extend `oauth/oauth2.go` `GetUserInfo` switch with the Notion case (parse `/v1/users/me`)
- [ ] Add `useBasicAuth` flag (or per-provider override) so the Notion token exchange sends client creds via HTTP Basic
- [ ] Set `Notion-Version: 2025-09-03` header on every authorized request

## Backend — trigger (primary path: Database Automation + Button)
- [ ] Create `api/pkg/trigger/notion/notion.go` with `New()` and `ProcessWebhook(ctx, cfg, headers, body)` that branches on `X-Helix-Source` header
- [ ] Implement shared-secret verification (constant-time compare on `X-Helix-Webhook-Secret` header)
- [ ] Implement `events.go` payload parser for the Automation/Button shape (Notion's webhook-action JSON)
- [ ] Implement `dispatch.go`: dispatch on `X-Helix-Action` header (`create` / `cancel`); idempotency via `ExternalTriggerRef` lookup keyed on `NotionPageID`
- [ ] Implement `client.go`: `GetPage`, `PatchPageProperty` (Result column only — never the action column), `AppendEmbedBlock`, `DeleteBlock` using the OAuth connection
- [ ] Implement embed-URL minting: per-trigger service-account API key (read-only on the target project); stored on the trigger config; used in `?access_token=`
- [ ] Hook spectask completion in `api/pkg/services/spec_driven_task_service.go` to invoke `notion.WriteResultBack` when `ExternalTriggerRef.Type == "notion"` and a `ResultColumn` is configured
- [ ] On spectask cancel, invoke best-effort `notion.DeleteBlock(EmbedBlockID)`
- [ ] Wire `notion.ProcessWebhook` into `trigger.ProcessWebhook` switch
- [ ] Update `webhookTriggerHandler` to pass request headers through to `trigger.ProcessWebhook`

## Backend — trigger (secondary path: API webhook subscription)
- [ ] Implement HMAC-SHA256 verification against `verification_token` in `events.go`
- [ ] Implement subscription handshake helper (verify-token paste flow)
- [ ] Implement event dispatch for `page.properties_updated`, `page.content_updated`, `comment.created`
- [ ] Implement page fetch via stored OAuth connection
- [ ] Implement in-memory dedup keyed by page ID with a debounce window (default 10s)
- [ ] Implement `render.go` prompt formatter with `PromptTemplate` (Go `text/template`) for the secondary path

## Backend — tests
- [ ] Unit test: shared-secret verify accepts good secret, rejects bad
- [ ] Unit test: HMAC verify accepts good signature, rejects bad signature, rejects mismatched body
- [ ] Unit test: dispatch — `X-Helix-Action: create` creates a spectask + appends embed block (mocked Notion client); idempotent on replay
- [ ] Unit test: dispatch — `X-Helix-Action: cancel` cancels in-flight spectask + best-effort deletes embed block
- [ ] Unit test: dispatch — repeated `cancel` for a page with no live spectask is a no-op
- [ ] Unit test: spectask-completion hook PATCHes only the `Result` column, never the action column
- [ ] Unit test: spectask-completion hook is a no-op when `ResultColumn` is empty in the trigger config
- [ ] Unit test: embed-block delete failure is logged and does not block cancellation
- [ ] Unit test: prompt template renders for each secondary-path event type
- [ ] Unit test: dedup collapses two secondary-path webhooks for the same page within the debounce window

## Frontend — trigger setup wizard
- [ ] Add a "Notion" option in the per-app trigger configuration form
- [ ] Wizard step 1: "Connect to Notion" OAuth button (or pick existing connection)
- [ ] Wizard step 2: pick the Notion database (call OAuth-authorized `POST /v1/search` filtered to databases)
- [ ] Wizard step 3a: pick the action column — Helix lists every `select` and `status` column in the database
- [ ] Wizard step 3b: pick the create-option and cancel-option from that column's options dropdown
- [ ] Wizard step 3c: optionally pick a `Prompt` rich-text column and a `Result` rich-text column
- [ ] Wizard step 4: pick the target Helix project; mint the embed-URL service-account token
- [ ] Wizard step 5: display the generated webhook URL, shared secret, and copy-paste instructions for creating **two** Database Automations in Notion (create + cancel) plus optional Run-button column, with screenshots
- [ ] "Test setup" button that POSTs synthetic `create` and `cancel` payloads through the same webhook URL
- [ ] Render Notion entries in the trigger executions list (action: create/cancel, page ID, embed-block ID)

## Frontend — OAuth provider preset
- [ ] Add a "Notion" preset in `Providers.tsx` so admins don't have to fill in URLs by hand

## Embed verification
- [ ] If headers are restrictive, add server middleware to relax `X-Frame-Options` / set `Content-Security-Policy: frame-ancestors https://*.notion.so https://*.notion.site` for `/embed/*` only
- [ ] Add Open Graph meta tags to `EmbedTaskPage` so Iframely can extract a preview
- [ ] Re-test embed in Notion after any change; capture `screenshots/02-notion-embed-final.png`
- [ ] Verify desktop video stream works inside Notion's iframe (or document why it doesn't)

## End-to-end demo (mirrors the customer's Go/NoGo flow)
- [ ] In the test Notion workspace: create a database with a `Go/NoGo` select column (options: `Go`, `NoGo`) and an optional `Result` rich-text column
- [ ] Add a **kanban board view** of the same database grouped by `Go/NoGo` so we can demo the drag-card UX as well as the table-row-edit UX
- [ ] Configure the Helix Notion trigger via the wizard end-to-end (action column = `Go/NoGo`, create option = `Go`, cancel option = `NoGo`, result column = `Result`)
- [ ] Create both Database Automations in Notion following the wizard's instructions
- [ ] **Table view demo:** flip a row's `Go/NoGo: NoGo → Go` → confirm Helix spectask is created and an `embed` block is appended to the row's page body
- [ ] **Kanban view demo:** drag a card from the `NoGo` lane to the `Go` lane → confirm the same outcome (verifies that the same Automation fires regardless of view)
- [ ] Click into the row/card → confirm the live Helix UI renders inside the row's page body, including video stream
- [ ] Wait for spectask completion → confirm the `Result` column is populated; confirm `Go/NoGo` is **untouched** in both views
- [ ] Drag the card back to `NoGo` (or flip the row) → confirm Helix cancels the spectask and removes the embed block
- [ ] Add a `Run` button column → press it on a separate row → confirm immediate spectask creation regardless of `Go/NoGo`
- [ ] Capture a short screen recording showing both the table and kanban views driving the same agent

## Findings doc
- [ ] Write `findings.md` covering: end-to-end latency observed (Notion → Helix → row write-back round-trip), what worked, what didn't, embed result, recommended GA shape (drop secondary path? keep both?), friction in setup wizard, whether free-plan parity matters
- [ ] Include screenshots and the demo recording link
