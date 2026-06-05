# Requirements — In-App Error Reporting (SaaS + Self-Hosted)

## Problem

Today, users in the SaaS web app and on Helm/k8s installs have no first-class way to send a bug report back to the Helix team. The only existing solution is the macOS app's "Report Issue" button (`for-mac/app.go:1120` `CollectDiagnostics()` + `for-mac/frontend/src/components/SettingsPanel.tsx:173` `handleReportIssue()`), which collects system info, VM logs, and container logs, then ships them via Crisp chat.

Web users have to copy/paste from DevTools or describe issues in Slack/Crisp from memory; self-hosted operators have to ssh into pods and grep logs themselves. Both paths lose the structured context (which spec-task stage, which agent, which session) that we'd need to triage quickly.

## User Stories

**1. SaaS user reporting a stuck spec task**
> As a SaaS user whose spec task is stuck in `spec_generation`, I want to click "Report Issue" from the task page so that the Helix team receives the task ID, current stage (`Status` + `StatusUpdatedAt`), the agent (`HelixAppID` / `ExternalAgentID`), the planning session's recent interactions, and any error message — without me having to assemble that context manually.

**2. SaaS user reporting a generic UI bug**
> As a SaaS user, I want to click "Report Issue" from the global menu so I can describe what went wrong with the current page; the report should auto-include browser info, the current URL, the most recent ErrorBoundary captures (`frontend/src/components/system/ErrorBoundary.tsx` already buffers them in sessionStorage), and my user/org IDs.

**3. Self-hosted (k8s/Helm) user with internet egress**
> As an operator running Helix on a Helm install with outbound internet, I want the same "Report Issue" button to work and route my report into the Helix support Crisp queue (the same queue the Mac app and SaaS web users already file into — `frontend/index.html:68-79` ships the Crisp widget in every install), tagged with a deployment ID hashed from my license key, so I don't have to open a separate ticket and attach files.

**4. Self-hosted (air-gapped) user**
> As an operator running Helix without outbound internet (so `client.crisp.chat` won't load), I want "Report Issue" to fall back to producing a downloadable `.json` bundle that I can attach to an email or upload to a support portal, so the same workflow works offline.

**5. CLI / on-call operator**
> As an operator debugging a broken install over ssh, I want `helix support report --task <id>` (or `--session <id>`) to write the same bundle to a file, so I can grab it without needing browser access.

**6. Frontend crash recovery**
> As a user whose React app just crashed (ErrorBoundary triggered), I want a "Report this error" button on the crash overlay that pre-fills the report with the error stack trace and component stack.

## Acceptance Criteria

### Trigger surfaces (frontend)
- [ ] A "Report Issue" entry exists in the global help/user menu, available on every page.
- [ ] The spec task detail page has a "Report Issue with this task" button that auto-attaches `spec_task_id`.
- [ ] The session/chat view has a "Report Issue" affordance that auto-attaches `session_id`.
- [ ] `ErrorBoundary.tsx` overlay gets a "Report this error" button that auto-attaches the most recent error.

### Report contents (always included)
- [ ] Helix version (commit SHA from build), edition (`HELIX_EDITION`), deployment ID (sha256 of `LICENSE_KEY`, matching `PingService`).
- [ ] Timestamp, reporter user ID + email + org ID, browser User-Agent + URL.
- [ ] User-typed description (free-text, optional but encouraged).

### Report contents (contextual, when available)
- [ ] **Session context**: session ID, last N interactions (prompts + responses, truncated per-message), agent app ID + model + name, token usage.
- [ ] **Spec task context**: task ID, name, type, priority, current `Status`, `StatusUpdatedAt`, `HelixAppID`, `ExternalAgentID`, `BranchName`, `LastPushCommitHash`, the three artifacts (`RequirementsSpec`, `TechnicalDesign`, `ImplementationPlan`) truncated, and recent activity from `WorkSessions` / `ZedThreads`.
- [ ] **Server-side logs**: last ~200 lines of API container logs visible to the API process itself (read from stdout buffer or `/var/log` mount). Sandbox container logs if a sandbox is attached.
- [ ] **Recent frontend ErrorBoundary captures** from sessionStorage (already buffered, max 20).

### Privacy / safety
- [ ] Before submission, the user sees a preview of the JSON bundle and can edit or remove fields.
- [ ] Prompts/responses and log lines are truncated (per-line cap + last-N-lines cap) to keep payload <1 MB.
- [ ] License key is **hashed**, never sent in clear text.
- [ ] Known secret env-var names (e.g. `*_API_KEY`, `*_TOKEN`, `*_SECRET`) are stripped from any included config dump.
- [ ] An admin can disable Crisp outreach entirely via env var (`HELIX_FEEDBACK_DISABLED=true`); the UI then only offers the "download bundle" path and never calls `$crisp`.

### Transport
- [ ] On Send, the dialog opens the existing Crisp widget (`$crisp.push(['do', 'chat:open'])`), sends a compact text summary (≤9 KB — same budget the Mac app uses at `for-mac/frontend/src/components/SettingsPanel.tsx:226`), and triggers a browser download of the full bundle as `helix-feedback-{report_id}.json` for the user to drag into the chat.
- [ ] When `$crisp` is undefined (script blocked or air-gapped) or `HELIX_FEEDBACK_DISABLED=true`, the dialog skips the Crisp calls and just downloads the bundle, with a toast explaining where to send it.
- [ ] User identity (`user:email`, `user:nickname`) is pushed to Crisp from the existing `useAccount()` data, matching `frontend/src/contexts/account.tsx:312-315`.
- [ ] CLI `helix support report` writes the bundle to a file path and prints the size; never auto-sends from the CLI in v1 (operators attach manually).

### Out of scope (v1)
- Programmatic file upload via Crisp's `message:send`/`file` API — needs a publicly-fetchable URL we'd have to host. Drag-drop into the Crisp chat panel is the v1 attachment flow.
- Two-way conversation tracking — Crisp already handles that natively once the report message lands.
- Auto-creating GitHub issues from reports.
- Redacting log lines for arbitrary regex secrets — only the env-var-name strip is in scope.
- Configurable Crisp website ID for self-hosted (helm installs use the same hard-coded ID in `frontend/index.html` so reports land in the Helix support queue).
