# Implementation Tasks: Live Verification of Agent Questions (ACP Elicitations) in the Helix UI

> Verification only. No feature work, no refactors, no new PRs, no merges.
> Commit and push after every chunk. Poll, never blind-`sleep`. Only `http://localhost:8080`.

## Bring the stack up

- [ ] Confirm `/home/retro/work/helix` is on `feature/002731-end-to-end-agent` and `ZED_COMMIT` is pinned to zed PR #83's head
- [ ] Start the inner dev stack: `docker compose -f docker-compose.dev.yaml up -d`
- [ ] Poll readiness in a short loop for several minutes: `docker compose -f docker-compose.dev.yaml ps` and `curl -s -o /dev/null -w '%{http_code}' http://localhost:8080` (treat `000` as "booting")
- [ ] Record ready state: `helix-api-1`, `helix-frontend-1`, `helix-postgres-1` all `Up` and `8080` returning `200`
- [ ] Create `screenshots/` in the task folder; commit and push the stack-up notes

## Get to a live spec task

- [ ] Register at `http://localhost:8080` as `test@helix.ml` / `helixtest` ("Test User"); sign in if already registered
- [ ] Complete onboarding: create org, then project
- [ ] Create a **spec task** (not a bare chat session) so a git repo is provisioned
- [ ] Verify liveness in postgres: `config->>'zed_thread_id'` is a non-empty UUID
- [ ] Confirm the task's agent uses the **Claude Code harness**; if no Claude-backed agent exists, stop and report it as a **blocker**
- [ ] Commit and push progress notes

## Evidence: the happy path

- [ ] Prompt: "Before you write any code, ask me which caching backend to use — Redis, in-memory, or none" (bounded retries if the model answers without calling `AskUserQuestion`)
- [ ] Screenshot `01-question-rendered.png`: every option visible with label **and** description, plus the "Other" free-text field
- [ ] Answer the question in the UI; capture the `respond` network call and its status
- [ ] Screenshot `02-answer-accepted.png` and `03-turn-resumed.png`: next agent message reflects the chosen option
- [ ] Cross-check DB: row is `accepted` with `resolution_reason = answered`
- [ ] Reload the page; screenshot `04-after-reload.png` showing the answered card still records the user's choice
- [ ] Commit and push screenshots + notes

## Evidence: the session keeps working

- [ ] Send a normal follow-up message in the same thread; screenshot `05-followup-reply.png` of the normal reply
- [ ] Provoke a **second** question in the same session; screenshot `06-second-question.png`
- [ ] Answer it; screenshot `07-second-answered.png` and confirm the first answer did not poison the second
- [ ] Commit and push

## Evidence: skip, interrupt, blocked

- [ ] Provoke a question and **skip/decline** it; confirm the turn settles cleanly and continues; screenshot `08-skip.png`
- [ ] Provoke a question, leave it unanswered, **interrupt** the turn from Helix; confirm the card stops being answerable; screenshot `09-interrupt-locked.png`
- [ ] Cross-check DB: `cancelled` / reason `interrupted` (or record what actually happened)
- [ ] With a question pending, screenshot `10-blocked-indicator.png` of the task list / detail header showing the task waits on a human
- [ ] Confirm the auto-wake worker leaves it alone: `docker compose -f docker-compose.dev.yaml logs api | grep AUTO_WAKE`; capture the log excerpt
- [ ] Commit and push

## Report and close out

- [ ] Write `design/2026-08-12-agent-questions-live-verification.md` in the **helix** repo: per-claim observed/expected/verdict, screenshot references, DB/log excerpts, defects described (not fixed), blunt overall verdict
- [ ] State explicitly if the model would not call `AskUserQuestion`; label any injected-event screenshot **"INJECTED"**
- [ ] Commit and push the report to `feature/002731-end-to-end-agent`
- [ ] Post a summary comment on https://github.com/helixml/helix/pull/3009 (`gh pr comment`) — **do not merge, do not open new PRs**
