# Implementation Tasks

## Discovery (do first)
- [ ] Register a Notion **internal integration** in a test workspace; capture client ID, client secret, and an integration-token PAT for solo testing
- [ ] Manually `POST` a hand-crafted "page edited" webhook payload at a local endpoint to confirm the HMAC signature scheme matches Notion's docs
- [ ] Paste `https://app.helix.ml/embed/task/{task_id}?access_token={key}` into a Notion `/embed` block in a test page; screenshot the result to `screenshots/01-notion-embed-baseline.png`
- [ ] Inspect the response headers Helix returns on `/embed/*` and document them in the findings doc (X-Frame-Options, CSP)

## Backend — types & OAuth
- [ ] Add `NotionTrigger` struct + `Notion` field on `Trigger` in `api/pkg/types/types.go`
- [ ] Add `TriggerTypeNotion = "notion"` constant
- [ ] Add `OAuthProviderTypeNotion = "notion"` in `api/pkg/types/oauth.go`
- [ ] Extend `oauth/oauth2.go` `GetUserInfo` switch with the Notion case (parse `/v1/users/me`)
- [ ] Add `useBasicAuth` flag (or per-provider override) so the Notion token exchange sends client creds via HTTP Basic
- [ ] Set `Notion-Version: 2025-09-03` header on every authorized request

## Backend — trigger
- [ ] Create `api/pkg/trigger/notion/notion.go` with `New()` and `ProcessWebhook(ctx, cfg, headers, body)`
- [ ] Implement HMAC-SHA256 signature verification against `verification_token`
- [ ] Implement webhook subscription handshake helper (verify-token paste flow)
- [ ] Implement event dispatch for `page.properties_updated`, `page.content_updated`, `comment.created`
- [ ] Implement page fetch via stored OAuth connection (or trigger-config-level token for internal-integration mode)
- [ ] Implement in-memory dedup keyed by page ID with a debounce window (default 10s)
- [ ] Implement `render.go` prompt formatter with `PromptTemplate` (Go `text/template`)
- [ ] Wire `notion.ProcessWebhook` into `trigger.ProcessWebhook` switch
- [ ] Update `webhookTriggerHandler` to pass request headers through to `trigger.ProcessWebhook`
- [ ] Optional `ReplyAsComment`: implement Notion `POST /v1/comments` call; tag bot's own author so we skip looped events

## Backend — tests
- [ ] Unit test: HMAC verify accepts good signature, rejects bad signature, rejects mismatched body
- [ ] Unit test: dedup collapses two webhooks for the same page within the debounce window
- [ ] Unit test: prompt template renders for each event type
- [ ] Unit test: own-bot comment is ignored to break reply loops

## Frontend — trigger config UI
- [ ] Add a "Notion" option in the per-app trigger configuration form
- [ ] Form fields: verification token (paste-from-Notion), OAuth connection picker, optional page IDs / database IDs / event-type filters, prompt template, "Reply as comment" toggle
- [ ] Display the generated webhook URL with copy-to-clipboard (already done generically — confirm Notion case renders it)
- [ ] Render Notion entries in the trigger executions list

## Frontend — OAuth provider preset
- [ ] Add a "Notion" preset in `Providers.tsx` so admins don't have to fill in URLs by hand

## Embed verification
- [ ] If headers are restrictive, add server middleware to relax `X-Frame-Options` / set `Content-Security-Policy: frame-ancestors https://*.notion.so https://*.notion.site` for `/embed/*` only
- [ ] Add Open Graph meta tags to `EmbedTaskPage` so Iframely can extract a preview
- [ ] Re-test embed in Notion after any change; capture `screenshots/02-notion-embed-final.png`
- [ ] Verify desktop video stream works inside Notion's iframe (or document why it doesn't)

## End-to-end demo
- [ ] In the test Notion workspace: create a database with a `Status` property
- [ ] Configure a Helix App with a Notion trigger pointing at that workspace
- [ ] Toggle a row's `Status` → confirm Helix session is created and visible
- [ ] (If `ReplyAsComment` enabled) confirm a reply lands as a comment on the row
- [ ] Embed the resulting Helix task URL inside the same Notion page; confirm both directions work in one view
- [ ] Capture a short screen recording (or screenshot sequence) for the demo

## Findings doc
- [ ] Write `findings.md` covering: webhook latency observed, what worked, what didn't, embed result, recommended GA shape, rate-limit estimate, scope of polling work if needed
- [ ] Include screenshots and the demo recording link
