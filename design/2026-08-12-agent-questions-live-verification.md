# Live verification: agent questions (ACP elicitations) in the Helix UI

**Verdict: NOT VERIFIED.** The user-facing loop was not proven to work, and it was not proven
broken either. One real blocker was found and fixed; after that the sandbox could not hold an
agent WebSocket open long enough to reach a question, so the UI evidence was never produced.

Everything below is what was observed. Nothing here is a prediction.

Branch: `feature/002731-end-to-end-agent` (same commits as helix PR #3009, zed PR #83).
Scope: **verification only** — no code was changed. Defects are described, not fixed.
Screenshots referenced live in `helix-specs@helix-specs:design/tasks/002754_live-verify-agent/screenshots/`.

## Summary of what was and was not established

| # | Claim | Verdict |
|---|---|---|
| — | A Claude Code harness agent exists in the inner Helix | **Confirmed** |
| — | The model does call `AskUserQuestion` when asked to | **Confirmed** |
| — | A spec task provisions a repo and gets a `zed_thread_id` | **Confirmed once** (run 1) |
| — | With Zed lacking elicitation support, the question renders as a dead stub | **Confirmed** (`00-wrong-zed-dead-stub.png`) |
| 1 | Question renders with all options, labels, descriptions, "Other" field | **Not verified** |
| 2 | Answer accepted, turn resumes reflecting the choice | **Not verified** |
| 3 | Answered card survives a page reload | **Not verified** |
| 4 | Normal follow-up still works | **Not verified** |
| 5 | Second question in the same session is uncontaminated | **Not verified** |
| 6 | Skip path settles cleanly | **Not verified** |
| 7 | Interrupt makes the card stop being answerable | **Not verified** |
| 8 | Blocked indicator + auto-wake gate | **Not verified** |

No elicitation was ever injected. There are no injected screenshots in this report — the two
UI screenshots taken after the Zed fix show connection errors, not simulated questions.

## Environment

Inner dev stack from this branch, `docker compose -f docker-compose.dev.yaml`, everything via
`http://localhost:8080`. Bring-up was polled, never blind-slept. Ready state reached at
07:29 BST: `helix-api-1`, `helix-frontend-1`, `helix-postgres-1` all `Up`, 8080 → `200`.

Registered `test@helix.ml` / "Test User", created org `testorg`, project `cache-demo`.

Two environment facts worth recording:

- The database is **`postgres`**, not `helix` — `psql -U postgres -d postgres`.
- Project creation first failed with *"default new project agent provider and model are not
  configured in Admin > System Settings"* (`api/pkg/server/app_handlers.go:653`). There is no
  admin UI route at `/admin/settings`; it was set via
  `PUT /api/v1/system/settings` with `default_new_project_agent_provider=anthropic`,
  `default_new_project_agent_model=claude-opus-5`. After that, project creation succeeded.

### Claude Code is available — not a blocker

The project-creation dialog offers Zed Agent, Qwen Code, **Claude Code**, Codex and Goose.
Selecting Claude Code lists real Anthropic models (`claude-opus-5`, `claude-sonnet-5`,
`claude-fable-5`, …). `.env` routes both APIs through the outer Helix
(`ANTHROPIC_BASE_URL=http://host.docker.internal:8081`), and querying that endpoint from
inside the API container returns `claude-opus-5`. All runs used **Claude Code / claude-opus-5**.

Note: the inner `/v1/models` aggregation lists 393 models owned by `togetherai`, `system`,
`openai`, `openai-internal` and **no Anthropic ones**, even though the Anthropic provider
works. The Claude Code model picker gets them from elsewhere. Harmless here, but it is why an
early check for "is there a Claude model" wrongly came back empty.

## Run 1 — the real blocker: the sandbox was running the wrong Zed

Spec task created, repo provisioned, `config->>'zed_thread_id'` =
`dab7327a-171f-49d7-ac05-4a32241dc72e` — a non-empty UUID, so workspace setup completed and
the sync WebSocket opened. Prompt:

> Add a caching layer to this project. Before you write any code, use your AskUserQuestion
> tool to ask me which caching backend to use - Redis, in-memory, or none. Wait for my answer
> before doing anything else.

**The model did call `AskUserQuestion`.** Helix rendered it as a **dead tool-call stub**: a
collapsible row reading "Which caching backend should I use for the caching layer in
cache-demo-2?" which expands to exactly the same sentence and nothing else. No options, no
descriptions, no "Other" field, no answer control, no Skip. The turn sat at
"Working for 3m 53s" and never progressed.

Corroboration, not just DOM:

- `select … from agent_elicitations` → **0 rows**.
- `docker compose logs api | grep -ic elicit` over ten minutes → **0**.

Evidence: `screenshots/00-wrong-zed-dead-stub.png`.

This is precisely the pre-change behaviour the design doc describes ("`AgentThreadEntry::Elicitation`
was silently dropped and Helix showed only the dead tool-call stub the adapter emitted just
before it").

### Root cause

`/home/retro/work/zed` was checked out on **`main`**, and `./stack build-zed` builds from that
working tree:

```
# on main
git grep -c elicitation_requested -- crates/external_websocket_sync/src/types.rs   → no match
# at the pinned commit
git grep -c elicitation_requested 859325b38f -- crates/…/types.rs                  → 2
# the binary that was actually running (built 22:51 the previous day)
grep -c elicitation_requested zed-build/zed                                        → 0
```

`sandbox-versions.txt` pins `ZED_COMMIT=859325b38fa519c28b55157039e7a9fe4990189a`, but that
pin is consumed by **`.drone.yml` only** — CI clones Zed at the pinned commit. The local
`./stack build-zed` path has no such pin and builds whatever the worktree happens to be. **A
local sandbox is not pinned by `sandbox-versions.txt` at all.**

### Fix applied (environment only)

Checked `/home/retro/work/zed` out at `859325b38f` and rebuilt (`./stack build-zed release`,
37m10s). `docker-compose.dev.yaml` bind-mounts `./zed-build:/helix-dev/zed-build:ro` into the
sandbox, so rebuilding the binary alone is sufficient — the sandbox image does not need
rebuilding. The new binary (08:15) contains `elicitation_requested`; the old one did not.

### Recommendation (not a change made here)

The local build path silently ignores the `ZED_COMMIT` pin. Anyone verifying this feature from
a dev stack will hit the same dead stub and is likely to read it as "the feature is broken".
Worth either making `./stack build-zed` honour `sandbox-versions.txt`, or warning when the Zed
worktree HEAD differs from the pinned commit.

## Runs 2, 3, 4 — could not reach a question with the correct Zed

Three attempts after the rebuild, none of which got as far as a question.

| Run | Outcome |
|---|---|
| 2 | `zed_thread_id` never set. Interaction ended `error`: *"Agent never connected after auto-wake cold-start retries (no WebSocket — see helixml/helix#2397)"*. UI: "Retried 2× · upstream ACP buffering" + "The system has encountered an error". `screenshots/01-acp-error-after-zed-rebuild.png` |
| 3 | Same. Session sat `waiting`, `zed_thread_id` stayed empty for 15+ min. |
| 4 | Restarted the desktop on run 3's task (reusing the already-extracted image). Still no `zed_thread_id` after 8 min. `screenshots/02-third-task-acp-error-desktop-paused.png` |

Zed itself was running each time — `/zed-build/zed /home/retro/work/cache-demo-2 …` plus
`claude-agent-acp` were both live in the desktop container. The problem was the transport. The
API log shows a reconnect loop roughly every two minutes:

```
WRN Server connection error, attempting reconnection
    error="websocket: close 1006 (abnormal closure): unexpected EOF"
INF Stream WebSocket connection closed
INF [CONNECT] Session loaded for reconnect agent_type=zed_external
    session_id=ses_01kztj5kpgq4dh0nr6y621s5j0 zed_thread_id=          ← still empty
DBG RevDial WebSocket ping failed, connection closing
    error="write tcp …: i/o timeout" / "broken pipe"
```

Zed reconnects, workspace setup never completes, `zed_thread_id` is never written, so no turn
ever runs and no question can be asked. This is transport instability on an overloaded host,
not anything to do with elicitations.

## Why the host was overloaded: the sandbox healthcheck wedge

This cost roughly 45 minutes and looked exactly like "the API is broken", so it is worth
recording. `sandbox-nvidia`'s healthcheck is:

```yaml
healthcheck:
  test: ["CMD-SHELL", "docker info > /dev/null 2>&1"]
  interval: 30s
  timeout: 5s
```

The 5 s timeout marks the check failed but does **not** kill the hung `docker info`. Once the
sandbox's inner dockerd wedges (here, under I/O pressure from the Zed rebuild and from
extracting the 7.67 GB desktop image on each task cold-start), one process leaks every 30 s.

Observed at the worst point: **51 `docker info` processes** in uninterruptible sleep (oldest
45 min), 32 buildx/compose plugin-metadata children, 38 zombies, load average oscillating
30→150, `iowait` pinned near 50 %, CPU pressure `some avg10=98.8`. The inner Helix API stopped
answering on 8080 and stopped logging entirely. Disk was **not** the issue (822 G free).

Recovery: `docker restart helix-sandbox-nvidia-1` — it exits 137 and then needs
`docker compose up -d sandbox-nvidia` to come back. Afterwards the pile-up drops to 0 and 8080
returns 200 immediately. Killing the stacked `docker info` processes alone does **not** work;
they respawn every 30 s until the container is restarted. This recurred on roughly a
25-minute cycle, each time triggered by a desktop cold-start.

## What still needs verifying

Everything in the evidence list. The specific unknown is unchanged from before this task: **no
one has yet seen a question rendered as a real card, answered it, and watched the turn
resume.** Runs 2–4 do not count as evidence against the feature — they never got far enough to
exercise it.

For whoever picks this up, the path is now shorter:

1. Confirm `/home/retro/work/zed` is at `859325b38f` (**not** `main`) before building, or the
   dead stub from run 1 is what you will see.
2. Bring the stack up and let the sandbox settle before creating a task; the desktop
   cold-start is what saturates the box.
3. Watch for the `docker info` pile-up (`pgrep -c -f '^docker info'`). Anything above ~3 means
   the sandbox needs restarting; the API will go unreachable shortly after.
4. Liveness gate before prompting: `config->>'zed_thread_id'` must be a non-empty UUID.
   If it is empty and the API log shows repeating `[CONNECT] … zed_thread_id=` with
   `close 1006`, the transport is flapping and no prompt will run.
