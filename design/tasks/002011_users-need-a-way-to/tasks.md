# Implementation Tasks

## Backend — types & config
- [ ] Add `api/pkg/types/feedback.go` with `FeedbackReport`, `FeedbackContext`, `SessionContext`, `SpecTaskContext`, `BrowserInfo`, `FrontendError`, `InteractionSummary`, `FeedbackReportRequest` per design.md.
- [ ] Add `Feedback` block to `api/pkg/config/config.go`: `Disabled` (`HELIX_FEEDBACK_DISABLED`, default false). Expose this on the existing `/api/v1/config` bootstrap so the frontend can branch on it.

## Backend — collector
- [ ] Create `api/pkg/feedback/collector.go` with `Collector` struct holding store, hydra client, log buffer, version + edition + license-key-hash.
- [ ] Implement `Collect(ctx, req)` that builds the deployment ID, populates spec-task / session / server-log / sandbox-log fields, and enforces truncation caps (4 KB per message, 16 KB per artifact, 1 MB total).
- [ ] Add a small in-memory ring buffer to the existing zerolog setup that captures the last ~200 stdout lines for `ServerLogs`.
- [ ] Reuse `hydra.SandboxOps.Logs` via `client_sandbox.go` to fetch `SandboxLogs` when a sandbox is attached.
- [ ] Strip env-var-shaped secrets (`*_KEY`, `*_TOKEN`, `*_SECRET`, `*_PASSWORD`) from any embedded config dump.

## Backend — handler
- [ ] Add `api/pkg/server/feedback_handlers.go` with `POST /api/v1/feedback/preview` → returns assembled `FeedbackReport`. Submission happens browser-side via Crisp; there is no `/report` endpoint.
- [ ] Wire the route into `api/pkg/server/server.go` behind `requireUser` middleware.
- [ ] When the request includes `session_id` / `spec_task_id`, run them through `authorizeUserToResource()` so a user can't pull another org's data.
- [ ] Add swagger annotations + run `./stack update_openapi`.

## CLI
- [ ] Add `api/pkg/cli/support/report.go` with `helix support report --task <id> --session <id> --output <file>`.
- [ ] CLI calls `POST /api/v1/feedback/preview` and writes the JSON to the output path; never auto-sends in v1 (operator attaches the file to whatever support channel they're using).
- [ ] Register the `support` command tree in the root cobra command.

## Frontend — Crisp helper
- [ ] Add `frontend/src/utils/crispReport.ts` exporting `sendReportToCrisp(report, opts)` that:
  - returns `{ ok: false, reason: 'unavailable' }` if `(window as any).$crisp` is undefined or `HELIX_FEEDBACK_DISABLED=true` (read from config bootstrap);
  - pushes user identity (`user:email`, `user:nickname`) — same calls as `frontend/src/contexts/account.tsx:312-315`;
  - builds the compact summary per design.md "Frontend → Crisp Transport" step 1 (capped at 9 KB);
  - calls `$crisp.push(['do', 'chat:show'])`, `['do', 'chat:open']`, `['do', 'message:send', ['text', summary]]`;
  - triggers browser download of the full `FeedbackReport` JSON as `helix-feedback-{report_id}.json` via Blob + `URL.createObjectURL`.

## Frontend — context & dialog
- [ ] Add `frontend/src/contexts/ReportIssueContext.tsx` with `ReportIssueProvider` + `useReportIssue()` hook (`openReportDialog({ specTaskId?, sessionId?, frontendErrors? })`).
- [ ] Mount `ReportIssueProvider` at the app root.
- [ ] Add `frontend/src/components/feedback/ReportIssueDialog.tsx`: description textarea, include-checkboxes (spec task / session / server logs / sandbox logs / frontend errors), collapsible JSON preview populated by `/feedback/preview`, Send button that calls `sendReportToCrisp` and shows a toast describing the next step ("drag the downloaded file into the chat" vs "attach the downloaded file to your support ticket").
- [ ] Use the generated API client (run `./stack update_openapi` first) for the preview call.

## Frontend — trigger surfaces
- [ ] Add "Report Issue" entry to the global help/user menu (top bar). Opens dialog with no pre-attached context.
- [ ] Add "Report Issue with this task" button on the spec-task detail page; pre-attaches `specTaskId`.
- [ ] Add "Report Issue" affordance on the session/chat view; pre-attaches `sessionId`.
- [ ] Extend `frontend/src/components/system/ErrorBoundary.tsx` (don't fork): add a "Report this error" button on the crash overlay that opens the dialog with `frontendErrors` pre-attached from the existing sessionStorage buffer.

## Tests
- [ ] Go unit tests for `collector.Collect`: spec-task path, session path, both, neither; truncation caps; secret-stripping.
- [ ] Handler tests using the existing `test_helpers.go` pattern: auth required, cross-org access denied, preview returns the expected shape.
- [ ] Frontend: smoke-test the dialog opens from each trigger surface, preview round-trip works, and `sendReportToCrisp` is called with the right summary (mock `window.$crisp`). Verify end-to-end in the inner Helix browser per CLAUDE.md "Never Give Up on Testing".

## Docs
- [ ] Add a short section to `docs/` describing the feature, the Crisp + drag-drop flow, the air-gapped fallback, and the `HELIX_FEEDBACK_DISABLED` env var.
