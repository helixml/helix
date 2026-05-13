# Implementation Tasks

## Discovery (do first)
- [ ] Register a Notion **internal integration** in a paid-plan test workspace; capture client ID, client secret, and an integration-token PAT for solo testing
- [ ] Manually create a Database Automation in a test database with action "Send webhook" → confirm payload shape (which fields land, what headers Notion supports adding) using webhook.site
- [ ] Manually add a Button property column with a "Send webhook" action → confirm payload shape and that we can distinguish it from the Automation payload via a custom header
- [ ] `POST` a hand-crafted secondary-path webhook payload at a local endpoint to confirm `X-Notion-Signature` HMAC scheme matches the docs
- [ ] Paste `https://app.helix.ml/embed/task/{task_id}?access_token={key}` into a Notion `/embed` block in a test page; screenshot the result to `screenshots/01-notion-embed-baseline.png`
- [ ] Inspect the response headers Helix returns on `/embed/*` (X-Frame-Options, CSP) and document in the findings doc

## Backend — types & OAuth
- [ ] Add `NotionTrigger`, `NotionColumnMap`, `NotionActionMap` structs + `Notion` field on `Trigger` in `api/pkg/types/types.go`
- [ ] Add `TriggerTypeNotion = "notion"` constant
- [ ] Add `ExternalTriggerRef` struct + JSONB column on `SpecTask`
- [ ] Add `OAuthProviderTypeNotion = "notion"` in `api/pkg/types/oauth.go`
- [ ] Extend `oauth/oauth2.go` `GetUserInfo` switch with the Notion case (parse `/v1/users/me`)
- [ ] Add `useBasicAuth` flag (or per-provider override) so the Notion token exchange sends client creds via HTTP Basic
- [ ] Set `Notion-Version: 2025-09-03` header on every authorized request

## Backend — trigger (primary path: Database Automation + Button)
- [ ] Create `api/pkg/trigger/notion/notion.go` with `New()` and `ProcessWebhook(ctx, cfg, headers, body)` that branches on `X-Helix-Source` header
- [ ] Implement shared-secret verification (constant-time compare on `X-Helix-Webhook-Secret` header)
- [ ] Implement `events.go` payload parser for the Automation/Button shape (Notion's webhook-action JSON)
- [ ] Implement `dispatch.go`: `Status → Ready` (or button press) creates spectask; `Status → Cancelled` cancels; idempotency via `ExternalTriggerRef` lookup
- [ ] Implement `client.go`: `GetPage`, `PatchPage` (status + URL + result write-back) using the OAuth connection
- [ ] Hook spectask completion in `api/pkg/services/spec_driven_task_service.go` to invoke `notion.WriteResultBack` when `ExternalTriggerRef.Type == "notion"`
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
- [ ] Unit test: dispatch — `Status → Ready` creates a spectask via mocked spectask service; idempotent on replay
- [ ] Unit test: dispatch — `Status → Cancelled` cancels in-flight spectask referenced by `ExternalTriggerRef`
- [ ] Unit test: spectask-completion hook PATCHes back `Status: Done` + `Result`
- [ ] Unit test: prompt template renders for each secondary-path event type
- [ ] Unit test: dedup collapses two secondary-path webhooks for the same page within the debounce window
- [ ] Unit test: writing `Status: Running` from Helix does NOT trigger a re-dispatch loop (validates the no-loop invariant for default action mapping)

## Frontend — trigger setup wizard
- [ ] Add a "Notion" option in the per-app trigger configuration form
- [ ] Wizard step 1: pick an OAuth connection (or "create one")
- [ ] Wizard step 2: pick the Notion database (call OAuth-authorized `POST /v1/search` filtered to databases)
- [ ] Wizard step 3: validate the database schema — fetch its properties, verify `Status` is a status type, `Helix Task` is a URL, etc.; show clear errors and a "Click here to add the missing columns" button (links to Notion docs)
- [ ] Wizard step 4: pick the target Helix project; set status-value mapping (defaults pre-filled)
- [ ] Wizard step 5: display the generated webhook URL, shared secret, and copy-paste instructions for creating the Database Automation in Notion (with a screenshot annotation)
- [ ] "Test setup" button that POSTs a synthetic Automation payload through the same webhook URL
- [ ] Render Notion entries in the trigger executions list

## Frontend — OAuth provider preset
- [ ] Add a "Notion" preset in `Providers.tsx` so admins don't have to fill in URLs by hand

## Embed verification
- [ ] If headers are restrictive, add server middleware to relax `X-Frame-Options` / set `Content-Security-Policy: frame-ancestors https://*.notion.so https://*.notion.site` for `/embed/*` only
- [ ] Add Open Graph meta tags to `EmbedTaskPage` so Iframely can extract a preview
- [ ] Re-test embed in Notion after any change; capture `screenshots/02-notion-embed-final.png`
- [ ] Verify desktop video stream works inside Notion's iframe (or document why it doesn't)

## End-to-end demo
- [ ] In the test Notion workspace: create a database with the convention columns
- [ ] Configure the Helix Notion trigger via the wizard end-to-end
- [ ] Create the Database Automation following the wizard's instructions
- [ ] Flip a row's `Status: Backlog → Ready` → confirm Helix spectask is created and `Helix Task` URL appears in the row
- [ ] Wait for spectask completion → confirm `Status: Done` and `Result` are populated back in the row
- [ ] Add a `Run` button column → press it on a separate row → confirm immediate spectask creation
- [ ] Paste the `Helix Task` URL into a Notion `/embed` block on the same page → confirm the live Helix UI renders inside Notion
- [ ] Capture a short screen recording (or screenshot sequence) for the demo

## Findings doc
- [ ] Write `findings.md` covering: end-to-end latency observed (Notion → Helix → row write-back round-trip), what worked, what didn't, embed result, recommended GA shape (drop secondary path? keep both?), friction in setup wizard, whether free-plan parity matters
- [ ] Include screenshots and the demo recording link
