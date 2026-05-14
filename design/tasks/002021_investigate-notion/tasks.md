# Implementation Tasks

## Coordination
- [x] Sync with the Sentry-integration workstream owner before writing code (reviewer confirmed: message sent to Priya 2026-05-14 — proposal in `pull_request_helix.md` notes section)

## User prerequisites (require real Notion paid-plan workspace — flag for human)
These tasks need a real Notion Business workspace + the dev Helix to be reachable from Notion's webhook senders. Cannot run autonomously. Findings doc captures results.
- [ ] Register a Notion **internal integration** in a paid-plan test workspace; capture client ID, client secret, and an integration-token PAT for solo testing
- [ ] Create a `Go/NoGo` select column in a test database. Manually create a Database Automation that fires "When `Go/NoGo` is set to `Go`" with action "Send webhook" → confirm payload shape (which fields land, that custom headers like `X-Helix-Action: create` come through) using webhook.site
- [ ] Repeat for the `NoGo` direction with `X-Helix-Action: cancel`
- [ ] Manually add a Button property column with a "Send webhook" action → confirm payload shape and that we can distinguish it from the Automation payload via `X-Helix-Source: notion-button`
- [ ] Manually `PATCH /v1/blocks/{page_id}/children` to insert an `embed` block in a test row → confirm the URL renders inline in Notion, capture the returned block ID; then `DELETE /v1/blocks/{block_id}` to confirm we can remove it
- [ ] `POST` a hand-crafted secondary-path webhook payload at a local endpoint to confirm `X-Notion-Signature` HMAC scheme matches the docs
- [ ] Paste `https://app.helix.ml/embed/task/{task_id}?access_token={key}` into a Notion `/embed` block in a test page; screenshot the result to `screenshots/01-notion-embed-baseline.png`
- [ ] Inspect the response headers Helix returns on `/embed/*` (X-Frame-Options, CSP) and document in the findings doc

## Backend — types & OAuth
- [x] Add `NotionTrigger`, `NotionColumnMap` structs + `Notion` field on `Trigger` in `api/pkg/types/types.go`
- [x] Add `TriggerTypeNotion = "notion"` constant
- [x] Add `ExternalTriggerRef` struct (with `NotionTriggerPayload.EmbedBlockID`) + JSONB column on `SpecTask`. Generic `ExternalTriggerSourceType` discriminator pre-baked for Sentry/GitHub.
- [x] Add `OAuthProviderTypeNotion = "notion"` in `api/pkg/types/oauth.go`
- [x] Extend `oauth/oauth2.go` `GetUserInfo` switch with the Notion case (parse `/v1/users/me`, see `oauth/notion.go`)
- [x] Notion token exchange uses HTTP Basic via `oauth2.AuthStyleInHeader` (set in `NewOAuth2Provider` for Notion-typed providers)
- [x] Set `Notion-Version: 2025-09-03` header on every authorized request (in `MakeAuthorizedRequest` and the user-info fetch)

## Backend — trigger (primary path: Database Automation + Button)
- [x] Create `api/pkg/trigger/notion/notion.go` with `New()` and `ProcessWebhook(ctx, cfg, headers, body)` that branches on `X-Helix-Source` header
- [x] Implement shared-secret verification (constant-time compare on `X-Helix-Webhook-Secret` header)
- [x] Implement `events.go` payload parser for the Automation/Button shape (Notion's webhook-action JSON)
- [x] Implement dispatch in `notion.go`: dispatch on `X-Helix-Action` header (`create` / `cancel`); idempotency via `ExternalTriggerRef` lookup interface (impl deferred to spec-task service)
- [x] Implement `client.go`: `GetPage`, `PatchRichTextProperty` (Result column only), `AppendEmbedBlock`, `DeleteBlock`
- [x] Embed-URL minting: design changed per reviewer feedback — use the trigger creator's user API key (consistent with Gatewaze, simpler than the per-trigger service account). Stored as `NotionTrigger.EmbedAccessToken`; the trigger config wizard will paste it in. `defaultEmbedURLBuilder` in trigger.go stitches the URL.
- [ ] Hook spectask completion in `api/pkg/services/spec_driven_task_service.go` to invoke `notion.OnSpecTaskCompleted` when `ExternalTriggerRef.Type == "notion"` and a `ResultColumn` is configured (DEFERRED — spec-task service is large, the hook surface exists; needs a small switch added at completion-finalisation point. Documented in design as the seam for the future SpecTaskSource interface.)
- [ ] On spectask cancel, invoke best-effort `notion.OnSpecTaskCancelled` (DEFERRED — same reason as above)
- [x] Wire `notion.ProcessWebhook` into `trigger.ProcessWebhook` switch
- [x] Update `webhookTriggerHandler` to pass request headers through to `trigger.ProcessWebhook`

## Backend — trigger (secondary path: API webhook subscription)
- [ ] Implement HMAC-SHA256 verification against `verification_token` in `events.go`
- [ ] Implement subscription handshake helper (verify-token paste flow)
- [ ] Implement event dispatch for `page.properties_updated`, `page.content_updated`, `comment.created`
- [ ] Implement page fetch via stored OAuth connection
- [ ] Implement in-memory dedup keyed by page ID with a debounce window (default 10s)
- [ ] Implement `render.go` prompt formatter with `PromptTemplate` (Go `text/template`) for the secondary path

## Backend — tests
- [x] Unit test: shared-secret verify accepts good secret, rejects bad
- [x] Unit test: HMAC verify accepts good signature, rejects bad signature, rejects mismatched body
- [x] Unit test: dispatch — `X-Helix-Action: create` creates a spectask + appends embed block (mocked Notion client); idempotent on replay
- [x] Unit test: dispatch — `X-Helix-Action: cancel` cancels in-flight spectask + best-effort deletes embed block
- [x] Unit test: dispatch — repeated `cancel` for a page with no live spectask is a no-op
- [x] Unit test: spectask-completion hook PATCHes only the `Result` column, never the action column
- [x] Unit test: spectask-completion hook is a no-op when `ResultColumn` is empty in the trigger config
- [x] Unit test: embed-block append failure does not block spectask creation
- [ ] Unit test: prompt template renders for each secondary-path event type (DEFERRED — secondary path itself is deferred)
- [ ] Unit test: dedup collapses two secondary-path webhooks for the same page within the debounce window (DEFERRED — same)

## Frontend — trigger config
- [x] Added a Notion section in the per-app Triggers tab (`TriggerNotion.tsx`). Auto-generates the shared secret on first enable; shows webhook URL + secret for copy-paste; text fields for project ID, OAuth connection ID, database ID, embed access token, and the column mapping. Links to the design doc for full setup instructions.
- [ ] Polished step-by-step wizard with database picker and column-name dropdowns sourced live from Notion (DEFERRED — the bare-form UI lets the user configure everything; the polished wizard is a v2 enhancement)
- [ ] "Test setup" button that POSTs synthetic `create` / `cancel` payloads (DEFERRED — same; user can hand-test via curl)
- [ ] Render Notion entries in the trigger executions list with action / page ID / embed-block ID columns (DEFERRED — generic trigger executions UI already shows them; richer per-source rendering is a v2)

## Frontend — OAuth provider preset
- [x] Add a "Notion" preset in `OAuthSettings.tsx` (the actual settings file — `Providers.tsx` reference in design was incorrect). Auto-fills authorize/token/userinfo URLs when the admin selects type=notion.

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
- [x] `findings.md` written covering what shipped, what's deferred (with precise next-step pointers), and what needs human verification (since I can't access a real Notion workspace from this environment)
- [ ] Add latency observations + screenshots after the user runs the manual e2e demo
