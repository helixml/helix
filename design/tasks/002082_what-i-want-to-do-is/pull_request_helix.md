# Fork-and-pause: validate, harden, ship the full UX

## Summary

This branch started as a **validation task** for the fork-and-pause work from spec [002081](https://github.com/helixml/helix/tree/helix-specs/design/tasks/002081_kickoff-mid-session) (commits `d2612ed3a` → `562b1f38f`). During hands-on testing it grew into hardening: every issue uncovered by manual use was fixed, tested, and re-verified end-to-end against the inner Helix. The result is a fork flow that meets a normal user's "switch agent without losing anything" expectation — conversation history, file changes, agent context, workspace branch all preserved.

## What you can do now

Pick a different agent from the chat-panel dropdown on any spec-task session:

1. **Confirmation modal** — names both agents, explains what's about to happen
2. **Workspace check** — runs in the background. If you have uncommitted file changes, surfaces them with the destination branch ("Commit and push these changes to `feature/000XXX-…` before forking") and a default-checked checkbox. If the safety net has nowhere viable to commit (legacy session with no spec task, or task on a protected branch), shows a red blocking alert and hides the Fork button.
3. **Pre-fork commit + push** — when you click Fork, the backend reaches into the parent's desktop container, runs `git add -A && git commit -m "chore(fork): pre-fork checkpoint before switching to <runtime>" && git push origin HEAD` per dirty repo. Conventional-commit format so the commit-msg hook accepts it. If the container is on `main`, recovery switches to the spec task's branch first (creates from `origin/<branch>` if no local; creates from current HEAD if neither exists yet — handles the "fresh task before agent did its own checkout" case).
4. **Fork** — child session created, parent paused, spec task re-pointed at the child, parent's desktop stopped to free the `max_concurrent_desktops` slot.
5. **Inherited history** — every non-`fork_seed`/`fork_handoff` interaction is copied into the child as `Trigger=fork_inherited`. Fork-of-fork preserves full ancestry by inheriting the parent's inherited rows along with the parent's own.
6. **Auto-handoff** — synthetic Waiting interaction fires on the child. `maybePrependTranscript` prepends the parent transcript so the new agent's first response acknowledges full context before the user types anything.
7. **Child container** — boots with `WorkingBranch` / `BranchMode` env vars set; `helix-workspace-setup.sh` checks out the right branch with the just-pushed files visible.

## Backend

**New endpoints** (`api/pkg/server/`):
- `POST /api/v1/sessions/{id}/fork` — the fork itself. Body: `{ helix_app_id, code_agent_runtime?, auto_commit_uncommitted? }`. Returns `{ new_session_id }`. Per-error path documented inline.
- `GET /api/v1/sessions/{id}/workspace-status` — per-repo dirty/unpushed counts + `expected_branch` (resolved from spec task, fallback to generated `feature/NNNNNN-name`) + `can_save_changes` / `cannot_save_reason` so the UI can pre-block obviously-impossible forks.

**Desktop-side endpoints** (`api/pkg/desktop/workspace.go`, exposed via the `helix-workspace-setup.sh`-style image):
- `GET /workspace/status` — per-repo `{branch, uncommitted_files, unpushed_commits}`
- `POST /workspace/commit-and-push` — checks current branch against `expected_branch`, switches if needed (existing-local → existing-remote → create-from-HEAD fallback chain), commits + pushes

**Session metadata propagation**: 15 additional fields copied from parent to child (`ActiveTools`, `RagEnabled`, `RagSettings`, `TextFinetuneEnabled`, `LoraDir`, `AppQueryParams`, `Priority`, video settings, document/RAG attachments, `TitleHistory`, etc.) with the intentional-not-copied list documented inline so future audits don't re-derive it.

**Quota hygiene**: parent's desktop container is stopped after a successful fork to free the `max_concurrent_desktops` slot for the child — paused sessions don't need running containers.

**Spec-task re-point**: any `SpecTask` whose `PlanningSessionID` pointed at the parent is updated to point at the child + adopt the new `HelixAppID`. Without this, revisiting the spec task lands you on the paused parent.

## Frontend (`frontend/src/components/session/ForkAgentControl.tsx`)

- MUI dropdown with tooltip placement fix (was hiding the first option)
- Confirm modal with two dirty-state variants:
  - **Safe**: yellow alert with file list + checkbox `"Commit and push these changes to <branch> before forking"`
  - **Unsafe**: red alert with reason, no Fork button
- Workspace status polled every 3s while modal open so a startup-race "container not reachable" auto-corrects
- Spec-task page wires `onForked` to remount the chat panel on the child without leaving the task

## Tests

| File | Coverage |
|---|---|
| `api/pkg/server/session_fork_handlers_test.go` | Fork handler unit + HTTP integration: happy path, pause-rejection, same-runtime rejection, same-runtime-different-app allowed, non-zed-external rejection, not-found, non-owner forbidden, spec-task re-point, chain-depth-2 full ancestry preservation, snapshot+pause, parent-app override, empty parent, fork-of-fork strips parent's fork_seed |
| `api/pkg/desktop/workspace_test.go` | Workspace endpoints: status dirty / status clean / commit-push dirty (verifies origin really got the commit) / commit-push clean (no-op) / branch recovery (parent on main, expected on feature) / branch recovery fresh-task (no remote ref yet) / hook rejection surfaces failure clearly |
| `api/pkg/server/session_workspace_handlers.go` | Defensive nil-check on connman + 404 fallback for older desktop images |

All workspace tests use temp git repos + bare-remote setup so they actually exercise `git commit` and `git push` round-trips without touching the dev machine.

## Commits

| Commit | What |
|---|---|
| `d05100039` | feat(fork): allow same-runtime app switches + confirm dialog |
| `52d48769b` | fix(fork): re-point spec task to child so old session doesn't black-hole |
| `50a281c2f` | feat(frontend): replay parent transcript as proper bubbles in forked session |
| `e3e775ce8` | refactor(fork): copy interactions at fork time so child is self-contained |
| `212f310ec` | feat(fork): auto-fire a handoff turn so the new agent warms up on fork |
| `725a63fc2` | feat(fork): pre-fork git commit+push so the child sees in-progress files |
| `8db592e62` | fix(fork): replace /exec abuse with dedicated /workspace/* endpoints |
| `75e9a939c` | feat(fork): user checkbox + conventional-commit fix for pre-fork push |
| `8b9080e01` | test(desktop): isolate findAllWorkspaces in tests via WORKSPACE_DIR priority |
| `07059b863` | fix(fork): propagate parent's branch to child container so it checks out the right ref |
| `1c431d589` | fix(fork): copy stragglers + stop parent desktop to free quota |
| `5659d5b8b` | fix(fork): recover when parent container is on main, not feature branch |
| `75f151eae` | fix(fork): recover when expected branch doesn't exist locally OR on origin |
| `6d6723bd8` | feat(fork): block fork in UI when outstanding changes can't be saved |
| `52ff36693` | fix(fork): fall back to generated branch name when spec task hasn't materialised one |
| `93c98872d` | fix(frontend): suppress misleading "container isn't running" message during fork |

## Validation outcome

End-to-end verified against the inner Helix on multiple test sessions: dirty workspace fork (`marker.txt` + `marker2.txt` → committed via `chore(fork): pre-fork checkpoint…` → pushed to origin's feature branch → child container clones with files visible), cross-agent recall via auto-handoff ("Blue." remembered across Claude Code → Sonnet fork), chain-depth-2 (fork of fork preserves full ancestry), checkbox opt-out works, blocking modal fires correctly for legacy sessions.

## Spec doc

Full design + validation outcome + screenshots: https://github.com/helixml/helix/tree/helix-specs/design/tasks/002082_what-i-want-to-do-is
