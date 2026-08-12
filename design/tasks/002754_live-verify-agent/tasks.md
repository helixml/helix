# Implementation Tasks: Live Verification of Agent Questions (ACP Elicitations) in the Helix UI

> Verification only. No feature work, no refactors, no new PRs, no merges.
> Commit and push after every chunk. Poll, never blind-`sleep`. Only `http://localhost:8080`.

## Bring the stack up

- [x] Confirm `/home/retro/work/helix` is on `feature/002731-end-to-end-agent` and `ZED_COMMIT` is pinned to zed PR #83's head
- [x] Start the inner dev stack (startup script was already running `./stack build` → `build-zed` → `build-sandbox` → `start`)
- [x] Poll readiness in a short loop for several minutes (treat `000` as "booting")
- [x] Record ready state: `helix-api-1`, `helix-frontend-1`, `helix-postgres-1` all `Up` and `8080` returning `200` (reached 07:29 BST)
- [x] Create `screenshots/`; commit and push the stack-up notes

## Get to a live spec task

- [x] Register at `http://localhost:8080` as `test@helix.ml` / `helixtest` ("Test User") — fresh DB, registration path used
- [x] Complete onboarding: org `testorg`, project `cache-demo`
- [x] **Unblock project creation** — failed with "default new project agent provider and model are not configured in Admin > System Settings"; no `/admin/settings` route exists, set via `PUT /api/v1/system/settings` (anthropic / claude-opus-5)
- [x] Create a **spec task** (not a bare chat session) so a git repo is provisioned
- [x] Verify liveness: `config->>'zed_thread_id'` = `dab7327a-171f-49d7-ac05-4a32241dc72e` (run 1)
- [x] Confirm the task's agent uses the **Claude Code harness** — Claude Code is available; **not a blocker**
- [x] Commit and push progress notes

## Evidence: the happy path

- [x] Prompt the agent to ask which caching backend to use — **the model called `AskUserQuestion` first try**
- [x] **Blocker found:** question rendered as a dead tool-call stub. Root-caused to the sandbox running Zed `main` (no elicitation support); `sandbox-versions.txt` pin is consumed by `.drone.yml` only, not by `./stack build-zed`. Screenshot `00-wrong-zed-dead-stub.png`
- [x] Fix the environment: check `/home/retro/work/zed` out at `859325b38f`, rebuild (37m). New binary contains `elicitation_requested`; old one had 0
- [ ] `01-question-rendered.png`: every option visible with label **and** description, plus "Other" free-text field — **NOT REACHED**
- [ ] `02-answer-accepted.png` / `03-turn-resumed.png` — **NOT REACHED**
- [ ] Cross-check DB: row `accepted` / `resolution_reason = answered` — **NOT REACHED**
- [ ] `04-after-reload.png` persistence — **NOT REACHED**

## Evidence: the session keeps working

- [ ] `05-followup-reply.png` normal follow-up — **NOT REACHED**
- [ ] `06-second-question.png` / `07-second-answered.png` uncontaminated second question — **NOT REACHED**

## Evidence: skip, interrupt, blocked

- [ ] `08-skip.png` skip path — **NOT REACHED**
- [ ] `09-interrupt-locked.png` interrupt locks the card — **NOT REACHED**
- [ ] `10-blocked-indicator.png` + AUTO_WAKE log excerpt — **NOT REACHED**

## Blocked by environment (documented, not worked around)

- [x] Runs 2, 3 and 4 after the Zed fix never got a `zed_thread_id`. Zed + `claude-agent-acp` were live each time; the sync WebSocket flapped every ~2 min (`close 1006 (abnormal closure)`, RevDial `i/o timeout`). Run 2 ended `error`: "Agent never connected after auto-wake cold-start retries (#2397)". Screenshots `01-acp-error-after-zed-rebuild.png`, `02-third-task-acp-error-desktop-paused.png`
- [x] Root-caused the host instability: `sandbox-nvidia` healthcheck is `docker info` every 30 s with a 5 s timeout that does not kill the hung process; once the inner dockerd wedges one leaks every 30 s (measured 51 stacked, load 30→150, iowait ~50 %, API dead on 8080). Only a container restart clears it; recurs on a ~25 min cycle, triggered by desktop cold-starts
- [~] Retry the un-reached evidence whenever the sandbox is stable

## Report and close out

- [x] Write `design/2026-08-12-agent-questions-live-verification.md` in the **helix** repo with the honest verdict (**NOT VERIFIED**), per-claim status, DB/log excerpts, screenshots, and recommendations
- [x] State explicitly that no elicitation was injected — there are **no** injected/simulated screenshots
- [x] Commit and push the report to `feature/002731-end-to-end-agent`
- [x] **Could not post the PR comment**: no `gh` CLI, no GitHub credentials in this sandbox (only the internal `api:8080` git proxy), `github` MCP server never connected. Comment text saved at `pr-3009-comment.md` for the user to paste
- [x] Do not merge, do not open new PRs — nothing merged, no PRs created
