# Lightweight tasks: optional PRs, no specs, many per project

**Date:** 2026-09-24
**Status:** proposal — a map of what has to change, nothing implemented or
tested.
**Related:** `design/2026-09-24-untrusted-bot-mode.md` (the untrusted key and
tool surface this reuses; its Org Bot parts are superseded by this doc),
`design/2026-09-24-browser-support-bot-evals.md` (timings).

## Why

Two needs, one change:

1. **PRs should be optional for tasks.** Not every task produces code: research,
   operations, form filling, document checks.
2. **One task per end-customer case**, hundreds at once in one project. The
   first user is a WhatsApp document-checking bot:
   - it collects a passport and business licence;
   - it checks them against each other;
   - it goes back and forth with the user;
   - it submits the details on the user's behalf through a website.

   A third-party gateway talks to Helix over the existing APIs.

A task already gives each case its own sandbox, attachments, a status on the
board, and a chat. Org Bots would need all of that built (the previous
proposal did exactly that). The bot's identity maps onto the **project**:

- the repo holds `AGENTS.md`, `.agents/skills/` and scripts;
- the project holds the runtime, model and MCP servers;
- every task in the project inherits all of it.

## The model change

One new task field, alongside the existing `just_do_it_mode` (skip planning):

```
spec_tasks.delivery   text not null default ''   -- '' | 'none'
```

- `''` is today's behaviour, unchanged: a PR for external repos, a fast-forward
  merge for internal ones, chosen by repo type (`spec_task_workflow_handlers.go:147-374`).
- `'none'`: the task produces no branch, no push and no PR. It is done when
  someone marks it done.

A **lightweight task** is `just_do_it_mode = true` + `delivery = none`. It
needs no `helix-specs` worktree at all.

**Why a field and not a new `Type` value.** `Type` is free-form
("feature/bug/refactor", `types/simple_spec_task.go:305`) and already overloaded
by `isAutonomousRun` (`git_http_server.go:1292-1307`). Delivery is a real
property that coding tasks can use too (for example a planning task whose
output is only the specs), so it should not be tied to one use case.

## Map of what changes

Every entry comes from reading the code on `feat/support-bot-fast-start`.
File paths are under `api/pkg/` unless shown otherwise.

### 1. Creating and starting

| Where | Today | For `delivery = none` |
|---|---|---|
| `POST /spec-tasks/from-prompt` (`server/spec_driven_task_handlers.go:109-294`, `types/simple_spec_task.go:224-278`) | no `delivery`, no `labels` field | add `delivery` and `labels` (labels at create avoid a second call and a race) |
| `StartJustDoItMode` (`services/spec_driven_task_service.go:805-1193`) | always generates `BranchName` (`:867-892`) | skip when `delivery = none` |
| same | git identity sync (`:982`), base-branch sync (`:1016`) | skip |
| same, prompt (`:1042-1088`, `services/spec_task_prompts.go:222-234`) | adds branch and push instructions, repo list, `helix-specs/.helix/startup.sh` hint | send the user's prompt verbatim plus the attachment note. `PromptMessage` at `:1088` is the single place to change |
| `DesignDocPath` / `HELIX_SPEC_DIR_NAME` (`:325-332`, `external-agent/hydra_executor.go:387-411`) | always set | don't emit the env var |
| Launch env (`hydra_executor.go:1503-1512`) | `HELIX_BRANCH_MODE=new` + working branch | empty branch mode: setup stays on the default branch (`desktop/shared/helix-workspace-setup.sh:492-494`) |

Planning tasks (`just_do_it_mode = false`) with `delivery = none` keep
`helix-specs`: specs are written there. Only the implementation-end steps
change for them.

### 2. Sandbox

| Where | Today | Change |
|---|---|---|
| `helix-specs` worktree (`helix-workspace-setup.sh:501`) | created whenever `HELIX_PRIMARY_REPO_NAME` is set; `git worktree add` failure is fatal (`:543-565`) | new env flag `HELIX_NO_SPECS=1`, set by Hydra for lightweight tasks, checked at `:501`. `HELIX_PRIMARY_REPO_NAME` cannot be the gate: it also drives branch checkout, skill linking and the Zed folder order |
| Repo-root `AGENTS.md` | ACP session cwd is `/home/retro/work` for every harness (`hydra_executor.go:1461`). Zed's native agent reads `AGENTS.md` from worktree roots; OpenCode walks up from cwd and never reaches `/home/retro/work/<repo>/AGENTS.md` | setup links the primary repo's `AGENTS.md` / `CLAUDE.md` into `/home/retro/work/`. This is a general fix: coding tasks' repo `AGENTS.md` is invisible to OpenCode today too. Harness rules-file discovery beyond Zed is not verified |
| Skills | `<repo>/.agents/skills` already linked for every harness (`helix-workspace-setup.sh:744-765`) | none |
| Startup script (`helix-workspace-setup.sh:1067`, `:1096`) | first choice is `helix-specs/.helix/startup.sh`; UI-saved scripts live only there (`server/project_handlers.go:293, 910, 3001-3009`) | the API resolves the script server-side and passes it as `HELIX_STARTUP_SCRIPT`, so it doesn't depend on the worktree |
| Git hooks (`helix-workspace-setup.sh:607-619`) | conventional-commit hook in every repo | harmless; nothing is committed |
| `helix-tasks` MCP (`external-agent/zed_config.go:399-406`) | present when agent tools are granted, including a bound org agent's tools (`server/mcp_backend_spectask.go:127-171`) | none for lightweight tasks; untrusted projects never get it (see §6) |

### 3. Attachments

Today the only way a task attachment reaches the sandbox is a commit into
the `helix-specs` bare repo at `design/tasks/<dir>/attachments/`
(`services/spec_task_attachments.go:137-259`). It is gated on
`project.DefaultRepoID` (`:44`, `:106`). The prompt path
(`spec_task_prompts.go:201-220`) and the late-upload note (`spec_task_attachments.go:116-132`)
both hard-code that location. A late upload is not pulled into the worktree:
setup pulls only once (`helix-workspace-setup.sh:579`).

Change: without a specs worktree, deliver attachments through the existing
sandbox upload path. That is the one `/external-agents/{id}/upload` uses
(`server/external_agent_handlers.go:815` → `desktop/upload.go:19`,
`/home/retro/work/incoming/`), and it resumes a stopped sandbox first. The
prompt note and the late-upload note then point at `incoming/`. That is
three call sites, all in `spec_task_attachments.go` / `spec_task_prompts.go`.

**Images work on `main`.** OpenCode reads an image into the model's context
only if the code-agent config declares image input
(`api/cmd/settings-sync-daemon/opencode.go:133-144`). The node06 endpoint
doesn't advertise modalities, so they come from `model_info.json`.
https://github.com/helixml/helix/pull/3281 added `z-ai/glm-5.3-flash`
(text/image/video, from OpenRouter), and the lookup also resolves for the
node06 endpoint id.

Verified live on 2026-09-24 with an org bot (OpenCode + `glm-5.3-flash`):

- on a pre-#3281 API, OpenCode refused the PNG with "this model does not
  support image input";
- with #3281 applied, it read "MARIA K SILVA" off a sample passport image
  in 8.4 s.

### 4. Lifecycle

| Where | Today | For `delivery = none` |
|---|---|---|
| WIP limits (`services/spec_task_orchestrator.go:471-517`, `:670-685`) | every just-do-it task takes an implementation slot; default limit 5 (`:578`). The two counts disagree about `queued_implementation` (`:481-485` vs `:676-679`) | **not exempt** (decided): limits stay and are raised to a sane value per project. Fix the two counts so they agree. Lightweight cases share the limit with coding tasks, so a project that mixes both needs a limit sized for the cases |
| Slot leak after stop (`server/spec_task_workflow_handlers.go:971-1063`, `external-agent/idle_checker.go:44-86`) | stopping leaves status `implementation` and keeps the slot | must be fixed now that limits apply: a stopped, idle case would otherwise hold a slot for days. Release the slot on idle stop, or count only tasks with a running sandbox |
| Finishing | `done` is set only by PR merge, fast-forward merge, push to main, or a raw `PUT status=done`, which skips `CompletedAt` (`spec_driven_task_handlers.go:1361-1404`) | new `POST /spec-tasks/{id}/complete`: sets `done` + `CompletedAt`. `handleDone` already stops the sandbox (unless `keep_alive`) and auto-archives if the project enables it (`spec_task_orchestrator.go:1756-1813`) |
| `approve-implementation` (`spec_task_workflow_handlers.go:127-130`) | requires a repo and pushed work | refused for `delivery = none` |
| PR pollers (`spec_task_orchestrator.go:1361`, `:1508`) and the push hook (`git_http_server.go:1087-1177`) | select by `pull_request` status or `BranchName` | already skip a task with neither. No change |
| Idle stop | container deleted after 1 h; workspace kept; next message restarts via `startDevContainerForSession` (`spec_task_design_review_handlers.go:1367-1486`) | none. Nothing durable lives in the workspace (see Storage) |
| Stale-archive sweep (`spec_task_orchestrator.go:1919-1926`) | never archives `implementation` | none. Conversations stay until completed |
| GC (`gc_reaper.go:17-20`) | keeps workspaces of non-terminal tasks | none. The workspace is disposable, so GC may remove it at any time |

### 5. Scale: hundreds of tasks per project

These are the real costs, independent of PRs:

- **The orchestrator lists every non-archived task in the database every 10
  s**, with dependencies loaded (`spec_task_orchestrator.go:281-283`).
- A burst of queued tasks takes a per-project lock and lists the whole
  project once per task (`:782-806`), which is roughly N² reads.
- Every push lists every task in every project using the repo
  (`git_http_server.go:1081-1085`).
- The label filter is JSONB containment with no index I could find
  (`store/store_spec_tasks.go:591-595`).

Before hundreds per project:

- filter the orchestrator scan by status;
- count WIP with one query instead of listing;
- add a GIN index on `labels`.

Sizing and caps:

- **Sandbox cap:** 10 concurrent headless sandboxes per org by default
  (`types/system_settings.go:243`, always enforced,
  `sandbox/controller_billing.go:159-194`). Raise
  `max_concurrent_headless_sandboxes` to the number of customers active
  within one idle timeout.
- **Default size:** 12 vCPU / 24 GB (`types/simple_spec_task.go:101-103`).
  Lightweight tasks should default to headless and the 1 vCPU / 2 GB preset
  (`:143-149`).

### 6. Keys

- **Sandbox key.** Task session keys already carry `SpecTaskID`, so git
  pushes are limited to the task branch plus `helix-specs`
  (`git_http_server.go:899-932`). For `delivery = none` that becomes no
  receive-pack at all. The rest of the API is still fully open, so untrusted
  work needs the untrusted key from `design/2026-09-24-untrusted-bot-mode.md`
  §1. That key's trust setting moves from the bot to the **project**: every
  session of an untrusted project gets it, and a gateway can't create a
  trusted task there.
- **Gateway key.** No key type today limits a caller to one project; embed
  keys are one task, minted by admins only (`server/handlers.go:1022-1117`).
  Add a project-bound key, fail-closed like the embed key
  (`server/auth_embed_key.go`):

  | Method | Path | Allowed when |
  |---|---|---|
  | POST | `/api/v1/spec-tasks/from-prompt` | `project_id` == key's project; the server forces `just_do_it_mode`, `delivery = none`, headless, `auto_start` |
  | GET | `/api/v1/spec-tasks?labels=` | key's project only |
  | POST | `/api/v1/spec-tasks/{id}/attachments` | task in key's project |
  | POST | `/api/v1/spec-tasks/{id}/complete`, `PATCH …/archive` | same |
  | POST | `/api/v1/sessions/chat`, `/api/v1/sessions/{id}/messages` | session is the canonical session of a task in key's project |
  | GET | `/api/v1/sessions/{id}/interactions` | same |

## Storage

**The sandbox workspace is ephemeral** (decided). If it disappears, nothing
of value is lost. Durable state lives in Helix, not in the sandbox:

- **Documents:** task attachments in the filestore. The sandbox gets copies
  in `incoming/`, re-delivered after a restart.
- **Conversation:** the session's interactions.
- **Result:** the submission itself happens on the target website.

So a case must survive a sandbox that comes back empty. That means the
agent's context is rebuilt from the interactions (or the thread Zed keeps),
and attachments are re-copied into `incoming/` on start. Neither is verified
today; both belong in the end-to-end test. No `case.md` and no retention
policy for workspaces.

## Frontend

A lightweight task today would sit in "In Progress" forever with a disabled
Accept/Open PR button. It would show a generated branch and a specs folder,
and read "Merged" once done (`frontend/src/components/tasks/TaskCard.tsx:1039-1049`).
Changes:

- **Board** (`SpecTaskKanbanBoard.tsx:141-197`, `:980-1072`): lightweight tasks
  go Backlog → In Progress → Done. Rename "Merged" to "Done" for tasks with no
  PR (`TaskCard.tsx:1039-1049`).
- **Actions** (`SpecTaskActionButtons.tsx:694-887`): "Mark done" instead of
  Accept / Open PR.
- **Detail page** (`SpecTaskDetailContent.tsx:2228-2262`,
  `TaskChatMetadata.tsx:58-195`): hide branch, base, specs folder and the Diff
  tab.
- **Create form** (`NewSpecTaskForm.tsx:248-313`): a "No pull request" option
  that hides the branch fields. A labels field.

## Gateway contract

Using only existing endpoints plus `complete`:

1. **New customer:** `POST /spec-tasks/from-prompt` with the case's first
   message, `labels: ["wa:<number>"]`, and any documents as `attachments`.
   Store the returned task id and its `planning_session_id`.
2. **Later messages:** `POST /api/v1/sessions/chat` with that `session_id`
   (blocking; returns the final text). Documents go first through
   `POST /spec-tasks/{id}/attachments`.
3. Send the final text back to WhatsApp.
4. Send one message per task at a time. What `/sessions/chat` does when a
   turn is already running is not verified. `POST /sessions/{id}/messages`
   queues safely but is asynchronous: read replies from
   `GET /sessions/{id}/interactions`.
5. **Case finished:** `POST /spec-tasks/{id}/complete`.

The gateway keeps its own number → task map. The `wa:` label is for operators
and recovery. URL-encode `+` as `%2B`. Nothing stops two tasks from having
the same label.

## Findings on the way

- **`POST /prompt-history/sync` does not authorize `spec_task_id`**
  (`server/prompt_history_handlers.go:71-128`). The queue processor then
  delivers every pending entry for that task, whoever wrote it
  (`store/store_prompt_history.go:253`). From the code, any authenticated user
  who knows a task id can queue prompts into another tenant's agent. **Not
  tested**; fix and test before anything external uses tasks.
- **A repo-root `AGENTS.md` is invisible to OpenCode** in every task (§2).
- **`PUT status=done` doesn't set `CompletedAt`.**
- **The orchestrator's per-tick full-table scan** (§5).

## Implementation plan

Each step is its own PR, tested end to end in the inner Helix.

1. **Security fix:** authorize `prompt-history/sync`.
2. **`delivery` field + complete endpoint + start path + WIP slot release on idle stop**
   (§1, §4), with the prompt sent verbatim. PRs become optional for coding
   tasks from this step on.
3. **Sandbox:**
   - `HELIX_NO_SPECS`;
   - the `AGENTS.md` link;
   - the startup script resolved server-side;
   - attachments to `incoming/` (§2, §3).
4. **Frontend** for PR-less tasks.
5. **Scale fixes** (§5).
6. **Untrusted project mode:** the untrusted key and tool surface from
   `design/2026-09-24-untrusted-bot-mode.md` §1–2, keyed on the project.
7. **Gateway key.**
8. **End-to-end test:** a gateway-shaped script runs two customers at once
   on a mock form site.
   - Each sends a sample passport and licence.
   - The bot flags a name mismatch, gets a correction, and submits after an
     explicit yes.
   - One case is idle-stopped mid-way and must resume intact.
   - Negative key tests for the sandbox key and the gateway key.

## Decisions (2026-09-24)

- **The workspace is ephemeral**; durable state lives in Helix (see Storage).
- **Limits are not skipped**; WIP and sandbox limits are raised to sane
  values per project and org.

## Open questions

1. `delivery` naming and values: is `none` enough, or do we want
   `direct_push` as an explicit third value now?
