# Implementation Tasks

## Backend — types & config
- [ ] Add `api/pkg/types/feedback.go` with `FeedbackReport`, `FeedbackContext`, `SessionContext`, `SpecTaskContext`, `BrowserInfo`, `FrontendError`, `InteractionSummary`, `FeedbackReportRequest`, `SubmitResult` per design.md.
- [ ] Add `Feedback` block to `api/pkg/config/config.go`: `URL` (`HELIX_FEEDBACK_URL`, default `https://feedback.helix.ml/v1/report`), `Disabled` (`HELIX_FEEDBACK_DISABLED`, default false).

## Backend — collector
- [ ] Create `api/pkg/feedback/collector.go` with `Collector` struct holding store, hydra client, log buffer, version + edition + license-key-hash.
- [ ] Implement `Collect(ctx, req)` that builds the deployment ID, populates spec-task / session / server-log / sandbox-log fields, and enforces truncation caps (4 KB per message, 16 KB per artifact, 1 MB total).
- [ ] Add a small in-memory ring buffer to the existing zerolog setup that captures the last ~200 stdout lines for `ServerLogs`.
- [ ] Reuse `hydra.SandboxOps.Logs` via `client_sandbox.go` to fetch `SandboxLogs` when a sandbox is attached.
- [ ] Strip env-var-shaped secrets (`*_KEY`, `*_TOKEN`, `*_SECRET`, `*_PASSWORD`) from any embedded config dump.

## Backend — uploader
- [ ] Create `api/pkg/feedback/uploader.go` with `Submit(ctx, report)` returning `{Mode: "uploaded", ReferenceID}` or `{Mode: "bundle", Bundle: []byte}`.
- [ ] If `cfg.Feedback.Disabled` → always return Bundle.
- [ ] Else POST JSON to `cfg.Feedback.URL`, 10 s timeout, 1 retry, `X-Helix-Deployment-ID` header. Any failure → return Bundle with the error appended to `warnings`.

## Backend — handlers
- [ ] Add `api/pkg/server/feedback_handlers.go` with:
  - `POST /api/v1/feedback/preview` → returns assembled `FeedbackReport` without submitting.
  - `POST /api/v1/feedback/report`  → assembles + submits, returns `SubmitResult`.
- [ ] Wire both routes into `api/pkg/server/server.go` behind `requireUser` middleware.
- [ ] When the request includes `session_id` / `spec_task_id`, run them through `authorizeUserToResource()` so a user can't pull another org's data.
- [ ] Add swagger annotations + run `./stack update_openapi`.

## CLI
- [ ] Add `api/pkg/cli/support/report.go` with `helix support report --task <id> --session <id> --output <file>`.
- [ ] CLI calls `POST /api/v1/feedback/preview` and writes the JSON to the output path; never auto-uploads in v1.
- [ ] Register the `support` command tree in the root cobra command.

## Frontend — context & dialog
- [ ] Add `frontend/src/contexts/ReportIssueContext.tsx` with `ReportIssueProvider` + `useReportIssue()` hook (`openReportDialog({ specTaskId?, sessionId?, frontendErrors? })`).
- [ ] Mount `ReportIssueProvider` at the app root.
- [ ] Add `frontend/src/components/feedback/ReportIssueDialog.tsx`: description textarea, include-checkboxes (spec task / session / server logs / sandbox logs / frontend errors), collapsible JSON preview populated by `/feedback/preview`, Send button hitting `/feedback/report`.
- [ ] Use the generated API client (run `./stack update_openapi` first); handle both `mode: "uploaded"` (snackbar with reference ID) and `mode: "bundle"` (trigger browser download via `application/json` Blob).

## Frontend — trigger surfaces
- [ ] Add "Report Issue" entry to the global help/user menu (top bar). Opens dialog with no pre-attached context.
- [ ] Add "Report Issue with this task" button on the spec-task detail page; pre-attaches `specTaskId`.
- [ ] Add "Report Issue" affordance on the session/chat view; pre-attaches `sessionId`.
- [ ] Extend `frontend/src/components/system/ErrorBoundary.tsx` (don't fork): add a "Report this error" button on the crash overlay that opens the dialog with `frontendErrors` pre-attached from the existing sessionStorage buffer.

## Helm
- [ ] In `charts/helix-controlplane/values.yaml`, add `controlplane.feedback.url` and `controlplane.feedback.disabled` (defaults match Go defaults). Wire them into the controlplane Deployment env vars.

## Tests
- [ ] Go unit tests for `collector.Collect`: spec-task path, session path, both, neither; truncation caps; secret-stripping.
- [ ] Go unit tests for `uploader.Submit`: disabled, success, network error → bundle, non-2xx → bundle.
- [ ] Handler tests using the existing `test_helpers.go` pattern: auth required, cross-org access denied, preview ≠ submit.
- [ ] Frontend: smoke-test the dialog opens from each trigger surface and the preview round-trip works (verify in the inner Helix browser per CLAUDE.md "Never Give Up on Testing").

## Docs
- [ ] Add a short section to `docs/` describing the feature and the `HELIX_FEEDBACK_*` env vars for self-hosted operators.
