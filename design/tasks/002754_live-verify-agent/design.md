# Design: Live Verification of Agent Questions (ACP Elicitations) in the Helix UI

## Approach

This is a **manual, evidence-producing verification run**, not an engineering change. No
harness, no script, no test framework — the automated e2e already exists and is green. The
whole design is: bring the branch up, drive the real UI with browser automation, screenshot
each claim, and write down what actually happened.

Deliberately *not* built: a Python/Go verification driver, a new e2e phase, or any helper
tooling. Building a tool to prove a UI works to a human defeats the point of the task — the
gap being closed is precisely "no one has looked at it".

## What is under test (from `design/2026-08-11-agent-questions-elicitation.md`)

```
Claude Code AskUserQuestion
  → claude-agent-acp: session/request_elicitation (mode "form"), turn BLOCKS
  → Zed crates/external_websocket_sync: SyncEvent elicitation_requested
  → Helix API: websocket_external_agent_elicitation.go
       ├─ row   agent_elicitations   (authoritative status, indexed)
       └─ entry ResponseEntries[type=elicitation]  (renders in conversation order)
  → frontend ElicitationCardContainer / ElicitationCard (form built generically from schema)
  → POST /api/v1/sessions/{id}/elicitations/{id}/respond   {action, content}
  → ExternalAgentCommand{Type:"respond_elicitation"} → Zed → adapter → turn resumes
```

Key files to consult when interpreting evidence:

| Concern | File |
|---|---|
| Types, statuses, resolution reasons | `api/pkg/types/agent_elicitation.go` |
| Sync-event handlers, heartbeat, reaper | `api/pkg/server/websocket_external_agent_elicitation.go` |
| Respond endpoint | `api/pkg/server/session_handlers.go` (`respondToElicitation`) |
| Routes | `api/pkg/server/server.go:1062-1063` |
| Auto-wake gate | `api/pkg/server/auto_wake_stuck_interactions.go:228-246` |
| Follow-up dispatch while blocked | `api/pkg/server/websocket_external_agent_sync.go:3432-3460` |
| Card + schema rendering | `frontend/src/components/session/ElicitationCard*.tsx`, `elicitationSchema.ts` |

Statuses: `pending`, `submitting` (Helix-local, optimistic), then terminal `accepted`,
`declined`, `cancelled`, `completed`. Reasons: `answered`, `skipped`, `follow_up`,
`interrupted`, `agent_no_longer_holds`.

## Environment

- Inner dev stack: `docker compose -f docker-compose.dev.yaml` in `/home/retro/work/helix`,
  branch `feature/002731-end-to-end-agent`, `ZED_COMMIT` pinned to zed PR #83's head.
- Everything through **`http://localhost:8080`**. `api:8080` is the outer production stack
  and must never be touched.
- Readiness = `helix-api-1`, `helix-frontend-1`, `helix-postgres-1` all `Up` **and** `8080`
  returning `200`. Boot takes minutes; `000` is "booting".

### Polling, not sleeping

Every wait is a short-interval poll loop with a bounded deadline and per-iteration output,
e.g.

```bash
for i in $(seq 1 120); do
  code=$(curl -s -o /dev/null -w '%{http_code}' http://localhost:8080)
  echo "$i $code $(docker compose -f docker-compose.dev.yaml ps --status running --format '{{.Name}}' | tr '\n' ' ')"
  [ "$code" = "200" ] && break
  sleep 5
done
```

Rationale: a blind `sleep 600` hides both success and death. Three prior sandboxes died on
long operations; short loops surface state immediately.

## Driving the UI

- **Browser:** `chrome-devtools` MCP (`navigate_page`, `take_snapshot`, `click`, `fill`,
  `wait_for`, `list_console_messages`, `list_network_requests`).
  `take_snapshot` before every click — element uids change between renders.
- **Screenshots:** `helix-desktop` MCP — `list_windows` → `focus_window` (**required**) →
  `save_screenshot` into
  `/home/retro/work/helix-specs/design/tasks/002754_live-verify-agent/screenshots/`.
- **Console + network capture** at each step is corroborating evidence: the
  `respond` POST and its response status say what the DOM alone cannot.

## Ground truth beside the UI

The UI claim is the deliverable, but each screenshot is cross-checked against the database
and logs so the report can distinguish "looks right" from "is right":

```bash
docker compose -f docker-compose.dev.yaml exec -T postgres \
  psql -U postgres -d helix -c \
  "select id, status, resolution_reason, interaction_id, left(message,60) from agent_elicitations order by created desc limit 10;"
```

- Liveness of the spec task: `config->>'zed_thread_id'` non-empty UUID.
- Auto-wake behaviour: `docker compose -f docker-compose.dev.yaml logs api | grep AUTO_WAKE`.
- Elicitation handling: `... logs api | grep -i elicit`.

## Provoking a question

A **spec task**, not a chat session — a spec task provisions a git repo, so Zed's workspace
setup completes and the sync WebSocket opens. The agent must be Claude Code:
`AskUserQuestion` is a Claude Code built-in, so a zed-agent or qwen session cannot produce an
elicitation at all. **If no Claude-backed agent exists in the inner Helix, that is a
blocker and is reported as such — not worked around.**

Prompt: *"Before you write any code, ask me which caching backend to use — Redis, in-memory,
or none."* Bounded retries (~3) with rephrasing if the model answers without calling the
tool.

### Fallback, and how it is labelled

If the model will not call `AskUserQuestion`, the report says so **explicitly**. Only then
may an `elicitation_requested` sync event be injected to exercise the UI paths — and every
screenshot so produced is captioned **"INJECTED — not a real model tool call"**. An injected
screenshot never counts as evidence that Claude Code triggers the flow; it is evidence only
about the Helix rendering and respond path.

## Evidence map

| # | Claim | Artefact |
|---|---|---|
| 1 | Question renders with all options, labels, descriptions, "Other" field | `01-question-rendered.png` |
| 2 | Answer accepted, turn resumes reflecting the choice | `02-answer-accepted.png`, `03-turn-resumed.png` |
| 3 | Persistence across reload | `04-after-reload.png` |
| 4 | Normal follow-up works | `05-followup-reply.png` |
| 5 | Second question, uncontaminated | `06-second-question.png`, `07-second-answered.png` |
| 6 | Skip path settles cleanly | `08-skip.png` |
| 7 | Interrupt locks the card | `09-interrupt-locked.png` |
| 8 | Blocked indicator + auto-wake leaves it alone | `10-blocked-indicator.png`, log excerpt |

Screenshots are numbered in capture order so the report reads as a narrative. Each one is
accompanied in the report by the matching DB row state.

## Report and Git

Report: `design/2026-08-12-agent-questions-live-verification.md` in the **helix** repo,
branch `feature/002731-end-to-end-agent`. Structure: what was run, per-claim
observed/expected/verdict with screenshot references, defects found (described, **not
fixed**), and a blunt overall verdict. A failed verification is a useful result and is
reported as one — a false pass is the only true failure of this task.

Commit and push **after every meaningful chunk** (stack up, task created, each evidence
group, report sections). Screenshots land in the helix-specs task folder and are pushed to
`helix-specs`; the report is pushed to the helix branch.

Finally, a summary comment on https://github.com/helixml/helix/pull/3009 via `gh pr comment`.
**No merges, no new PRs, no code changes.**

## Implementation Notes (written during the run — read these first if you are repeating this)

**Outcome: NOT VERIFIED.** The loop was not proven to work and not proven broken. Details in
`helix:design/2026-08-12-agent-questions-live-verification.md`.

Discoveries that will save the next agent hours:

- **The Zed pin does not apply locally.** `sandbox-versions.txt` sets
  `ZED_COMMIT=859325b38f`, but that is consumed by **`.drone.yml` only**. `./stack build-zed`
  builds from whatever `/home/retro/work/zed` has checked out — which was `main`, with no
  elicitation support. Symptom: a real `AskUserQuestion` renders as a **dead tool-call stub**
  with `agent_elicitations` empty and zero `elicit` lines in the API log. Check first:
  `cd /home/retro/work/zed && git log --oneline -1` and
  `grep -c elicitation_requested /home/retro/work/helix/zed-build/zed`.
- **Rebuilding just the binary is enough.** `docker-compose.dev.yaml` bind-mounts
  `./zed-build:/helix-dev/zed-build:ro` into the sandbox, so no sandbox image rebuild is
  needed after `./stack build-zed release` (~37 min).
- **The database is `postgres`, not `helix`**: `docker exec helix-postgres-1 psql -U postgres -d postgres`.
  Use `docker exec` directly rather than `docker compose exec` — the compose CLI spawns
  buildx/compose plugin-metadata children on every call and adds real load on a busy box.
- **Fresh stacks cannot create projects** until
  `PUT /api/v1/system/settings` sets `default_new_project_agent_provider` +
  `default_new_project_agent_model` (error text points at "Admin > System Settings", but there
  is no `/admin/settings` route). Used `anthropic` / `claude-opus-5`.
- **Claude Code is present** in the project-creation agent picker, with real Anthropic models.
  Note the inner `/v1/models` aggregation lists **no** Anthropic models even so — don't use it
  to decide whether Claude is available.
- **`chrome-devtools` `fill` does not trigger React onChange** in the composer; the Send button
  stays disabled. Use `type_text` (real keystrokes) instead.
- **`/proc/<pid>/root/...` did not resolve into the desktop container** — it returned the
  host's own file (same inode). Don't trust it for reading a container's logs.
- **The killer: `sandbox-nvidia`'s healthcheck wedges the box.** It is `docker info` every 30 s
  with a 5 s timeout that does not kill the hung process. Once the inner dockerd wedges, one
  leaks every 30 s; 51 were measured stacked, load 30→150, iowait ~50 %, inner API dead on 8080
  with no logs. Recovery is `docker restart helix-sandbox-nvidia-1` (exits 137, then needs
  `docker compose up -d sandbox-nvidia`). Killing the processes alone does not work. This
  recurred **five times** on a ~25 minute cycle, each triggered by a desktop cold-start
  (extracting the 7.67 GB desktop image). Watch it with `pgrep -c -f '^docker info'`; anything
  above ~3 means restart the sandbox now.
- **Liveness gate before prompting:** `config->>'zed_thread_id'` must be a non-empty UUID. If
  it stays empty and the API log repeats `[CONNECT] … zed_thread_id=` alongside
  `websocket: close 1006`, the transport is flapping and no prompt will ever run.

Repo state left behind: `/home/retro/work/zed` is at detached HEAD `859325b38f` (the pinned
commit) with no local commits — that is the correct state for this sandbox. No code was
changed in any repo; the only commits are the report in `helix` and these docs.

## Risks

| Risk | Handling |
|---|---|
| No Claude-backed agent in inner Helix | Report as blocker immediately; labelled-injection fallback for the UI-only paths |
| Model won't call `AskUserQuestion` | Bounded retries, then state it plainly |
| Stack slow/failed boot | Poll loop with per-iteration output; `docker compose logs` on timeout |
| Sandbox death mid-run | Push after every chunk; report is written incrementally, not at the end |
| Temptation to fix a found defect | Out of scope — record it in the report and in the PR comment |
