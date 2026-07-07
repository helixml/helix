# Spec-task attachment lost: upload races start-planning, never staged into the repo

**Date:** 2026-07-07
**Task that surfaced it:** `spt_01kwxcg8vk3fsy35x247brmr46` (Helix project)
**Attachment:** `att_01kwxcgae…` — `Screenshot 2026-07-07 at 05.13.29.png` (2.25 MB)
**Instance:** meta.helix.ml (localhost dev stack)

## Symptom

The agent reported it could not find an attachment the user is sure they added.
The attachment row and filestore blob exist, but the file never appears in the
agent's workspace at `design/tasks/<taskDir>/attachments/<name>`.

## Root cause — a race between three independent client requests

`CreateTaskFromPrompt` deliberately does **not** auto-start planning ("start in
backlog, wait for explicit user action"). So the UI create-with-attachment flow
issues three separate HTTP requests:

| Time (UTC) | Request | Effect |
|---|---|---|
| 04:13:42.643 | `POST /spec-tasks/from-prompt` | task created |
| 04:13:42.716 | `POST /spec-tasks/{id}/start-planning` | planning begins (**73 ms later**) |
| 04:13:42.774 | — | planning kickoff prompt **built**; `stageAttachmentsAndBuildPromptSection` runs here |
| 04:13:44.293 | `POST /spec-tasks/{id}/attachments` | attachment upload completes — **1.5 s too late** |

The 2.25 MB PNG upload is slower than the `start-planning` call that was fired
right behind the create. `start-planning` is the **only** place attachments get
staged into the repo (`stageAttachmentsAndBuildPromptSection` →
`commitAttachmentsToHelixSpecs`, in `pkg/services/spec_task_attachments.go`). At
staging time `ListSpecTaskAttachments` returned **zero** rows, so:

- nothing was committed to the helix-specs branch → `committed_sha` stays `""`;
- the prompt's "attachments" section was empty → the agent was never told;
- the file never reached `design/tasks/002234_we-basically-need-to/attachments/`.

### Evidence

- `spec_task_attachments.committed_sha = ''` for `att_01kwxcgae…`.
- No `Committed attachments to helix-specs branch` log for this task (there **is**
  one for `spt_01kwwcrew…`, proving the mechanism works when ordering is right).
- helix-specs branch has `design/tasks/002234_we-basically-need-to/{design,requirements,tasks}.md`
  and a `screenshots/` dir, but **no `attachments/` dir**.
- `uploadSpecTaskAttachments` writes to filestore + creates the DB row only — it
  does **not** stage into the repo. Staging is exclusively at planning time.

### Why the CLI is immune

`helix spectask start --attach` runs `createSpecTask` → `uploadSpecTaskAttachments`
→ **then** `triggerStartPlanning` (synchronous, in that order). The attachment
always exists before staging runs. So CLI-attached tasks work (that's why the
de-dup task's attachment staged fine). The bug lives in the UI create+attach
flow, where the three requests race and the slow upload loses.

## Fix direction

Two complementary fixes; the second is the robust one:

1. **Ordering (UI):** upload attachments *before* calling `start-planning`, or
   have `start-planning` wait until in-flight uploads for the task have settled.
   Fragile on its own (depends on client sequencing) but cheap.

2. **Server-side robustness (preferred):** make attachment staging independent of
   the planning race. Two sub-parts:
   - **`uploadSpecTaskAttachments` stages the blob into helix-specs immediately**
     (reuse `commitAttachmentsToHelixSpecs`), so the file lands in
     `design/tasks/<taskDir>/attachments/` regardless of when it's uploaded — even
     after planning started. `commitAttachmentsToHelixSpecs` is already idempotent
     (skips rows whose `CommittedSHA` is set), so double-staging is safe.
   - **Make the agent aware of late arrivals:** if an attachment lands after the
     planning prompt was built, either re-inject an "attachment added" note into
     the session (queue path — see #2808 / the sender-unification work) or ensure
     the agent is instructed to always check the attachments dir. Without this, the
     file is in the workspace but the agent isn't told to look.

Prefer (2). It removes the race entirely: once uploaded, the file is in the repo,
and the agent is notified — no dependence on request ordering.

## Files

| File | Role |
|---|---|
| `api/pkg/server/*` `uploadSpecTaskAttachments` | upload handler — currently filestore + row only; should stage into repo |
| `api/pkg/services/spec_task_attachments.go` | `stageAttachmentsAndBuildPromptSection`, `commitAttachmentsToHelixSpecs` (idempotent) |
| `api/pkg/server/spec_driven_task_handlers.go` | `startPlanning` (where staging currently happens) |
| frontend create+attach flow | issues create / start-planning / upload as separate races |

## Note on testing this fix

Reproduce the UI ordering (create → start-planning → slow upload) and assert the
file lands in the repo and the agent sees it. The **CLI `--attach` path will NOT
reproduce the bug** (correct order) — so a fix verified only via CLI attach is not
verified. Drive the racing UI/API sequence directly.
