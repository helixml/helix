# Design: Remove PR Review Coordinator Bootstrap Overhead

## Approach

This is a **durable-configuration change only — no Go code changes.** The PR Review Coordinator is an org bot whose behaviour lives entirely in its `content` prompt (helix-org design philosophy: behaviour in prompt/profile, not code). The change is one `PATCH` of the bot content plus a restart, then end-to-end validation.

- Bot: `b-pr-coordinator` ("PR Review Coordinator"), org `helix` (`org_01kcqz4cdbg9rq9r1t4be1w80b`), project `prj_01m145sbbc3jyvpcb5ngh6ezvq`, runtime `opencode`, model `qwen3.8-flash-next`.
- Update: `helix api -X PATCH /orgs/helix/bots/b-pr-coordinator --input @new-content.json` with `{"content": "<new prompt>"}`. Read current content first via `helix api /orgs/helix/bots/b-pr-coordinator` and edit **in place** (do not rewrite from scratch) so unrelated text stays untouched.
- Content writes mark the bot restart-required: after the PATCH, `helix org bots restart b-pr-coordinator --org helix`.

## Why the trigger is sufficient (no `read_events`)

The GitHub transport puts the verbatim webhook payload in `Message.Extra` (`api/pkg/org/infrastructure/transports/github/github.go:301-320`), and the `p-gh-helix` JS processor preserves `extra` untouched while rewriting only `subject`/`body`. `BuildPrompt` renders every populated Message field — including the full `extra` JSON — into the activation prompt (`api/pkg/org/domain/briefing/prompt.go:88-136`, "no separate read_events round-trip needed"). So at activation the coordinator already has, in the trigger text:

- `extra.event` (= `pull_request`), `extra.action` (`opened|reopened|synchronize` after p-gh-helix's filter)
- `extra.number`, `extra.pull_request.head.sha`, `extra.pull_request.draft`, `extra.pull_request.state`, `extra.pull_request.user.login`, `extra.pull_request.html_url`
- `extra.sender.login`, `extra.repository.full_name`

Multiple coalesced triggers arrive as a numbered list; process all of them, latest-first per PR.

## Prompt deltas (edit the existing content in place)

1. **GitHub authentication section** — keep `list_secrets` → `get_secret` → export `GH_TOKEN`; keep the ask_human fallback when no token is granted. Replace the `gh api` health check with `curl -sf -H "Authorization: Bearer $GH_TOKEN" https://api.github.com/repos/helixml/helix | jq -r .full_name`. Reword the installation-token note (`/user` 403s on installation tokens — test with a repo read). Add one line: **never install or invoke `gh`**.

2. **State** — keep `/home/retro/work/pr-review-state.json` with `reviewer_login` and the `dispatched` map of `"<pr>-<sha8>" -> {task_id, status}`. Drop `last_event_id` (no read_events ⇒ nothing to watermark). Keep the "sandbox may be recreated — rebuild from GitHub API + `list_spectasks`, never assume" rule.

3. **Per-activation procedure**:
   - Step 1 (`read_events`) is **deleted**. New step: "Process the GitHub event(s) already present in this activation trigger. Do **not** call `read_events` or `list_trigger_events` — the trigger's `extra` contains the full webhook payload."
   - Step 2 filters apply unchanged to the trigger payload (`extra.action`, `extra.sender.login`, `extra.pull_request.draft/state/user.login`). Keep the DROP-sender-==-REVIEWER_LOGIN self-loop rule and the `opened|reopened|synchronize` KEEP list verbatim (event filtering is out of scope).
   - Step 4 review-needed check: replace `gh api repos/helixml/helix/pulls/N/reviews --paginate` with `curl -sf -H "Authorization: Bearer $GH_TOKEN" "https://api.github.com/repos/helixml/helix/pulls/N/reviews?per_page=100"` (+ `&page=` loop only when pagination is needed), piped through `jq` to find the most recent review by `REVIEWER_LOGIN` and its `commit_id`. Same skip rule.
   - Step 5 dispatch guard unchanged: `list_spectasks`, skip if `pr-review-<N>-<short>` is non-terminal; keep the 4-in-flight concurrency cap.
   - Step 6 dispatch: `create_spectask` with the same `name`, `skip_planning: true`, `priority: high`, plus **`sandbox_runtime: "headless-ubuntu"`**. The `description` is the PR Review Brief **with the FINISH-STEP line already included** (below). **Delete** the `update_spectask` post-create bullet. Keep `start_spectask_planning` immediately after, and parallel dispatch of multiple PRs.
   - Step 7 state update unchanged (minus `last_event_id`).

4. **FINISH-STEP (now inside the brief, no post-create call)** — the review task sandbox exports `HELIX_SPEC_TASK_ID` (`api/pkg/external-agent/hydra_executor.go:1504`), so the description carries:

   > FINISH-STEP: After your review is posted and you have verified via the reviews API that it exists, move this task to done as your last action: `export HELIX_URL="$HELIX_API_BASE_URL"; helix spectask move "$HELIX_SPEC_TASK_ID" done`

5. **Task bookkeeping (column hygiene)** — unchanged rules; swap the `gh api .../reviews` verification to the curl+jq GET reviews call. Keep `export HELIX_URL="$HELIX_API_BASE_URL"` + `helix spectask move <spt_id> done`.

6. **PR Review Brief (task description)**:
   - Rule 1 auth: same `list_secrets`/`get_secret` recipe; add "never print or persist the token (no logs, no review body, no state file)".
   - Rule 2 fetch state: replace `gh pr view/diff` with REST — `GET /repos/helixml/helix/pulls/<N>` (metadata, current `head.sha`), `GET /repos/helixml/helix/pulls/<N>/files?per_page=100`, and the diff via `curl -sf -H "Authorization: Bearer $GH_TOKEN" -H "Accept: application/vnd.github.v3.diff" .../pulls/<N>`. Keep checkout of `<HEAD_SHA>` and the head-moved note.
   - Rules 3–4 (review rigor, verify-don't-guess) unchanged.
   - Rule 5 verdict: replace `gh pr review --approve/--request-changes/--comment` with one REST call:
     `curl -sf -X POST -H "Authorization: Bearer $GH_TOKEN" -H "Accept: application/vnd.github+json" https://api.github.com/repos/helixml/helix/pulls/<N>/reviews -d '{"commit_id":"<HEAD_SHA>","event":"APPROVE|REQUEST_CHANGES|COMMENT","body":<json-escaped body>}'` — `commit_id` pins the review to the intended commit. Keep "an APPROVE means ready to ship".
   - Rule 6 (i-have-adhd body style) unchanged.
   - Rule 7 (`curl -i` reproduction of GitHub write refusals) stays — curl is now the primary path.
   - Add: never install or invoke `gh`.

7. **Loop safety + Reporting** — unchanged; append "never call `read_events`/`update_spectask`" to loop safety.

## Key decisions

- **Prompt-only, in-place edit** (ponytail): the smallest diff that meets every acceptance criterion; tool grants and the processor/trigger topology stay untouched.
- **REST endpoints**: repository health = `GET /repos/helixml/helix`; metadata/diff = `GET /repos/helixml/helix/pulls/<N>` (diff via `Accept: application/vnd.github.v3.diff`); files = `GET .../pulls/<N>/files`; existing reviews = `GET .../pulls/<N>/reviews`; submission = `POST .../pulls/<N>/reviews` with `commit_id`. Auth header `Authorization: Bearer $GH_TOKEN` (installation token). All tokens from `get_secret`, called immediately before use, re-called after 401/403.
- **`headless-ubuntu` on the dispatched task only**: the create-time `sandbox_runtime` field is the requested scope (`types.SandboxRuntimeHeadlessUbuntu` is a valid spec-task runtime; headless containers run the same agent image minus display/streaming — `hydra/devcontainer.go:897`, `hydra_executor.go:695-697`). Coordinator's own runtime untouched (open question).
- **Credential hygiene**: `get_secret` mints a GitHub App installation token; prompt forbids printing it, putting it in argv-visible places beyond the immediate curl invocation, or persisting it in state files, task descriptions, or review bodies.

## Implementation constraints / gotchas

- The task agent's API key is **scoped to its own project** (`prj_01m0e1w5d39dy6j90ydthcnk1x`). Org-level endpoints (bot GET/PATCH, restart, bot log) work; project-scoped reads of the coordinator's project (`/spec-tasks?project_id=prj_01m145...`) return 403. Verify dispatched-task details via the coordinator's bot log/session, the os.helix.ml UI, or an org admin key.
- PATCH body must be valid JSON with the full new `content` string — build it with `jq -Rs` or a here-doc, never by hand-escaping.
- p-gh-helix already drops non-`opened/reopened/synchronize` actions; the coordinator prompt's filter is belt-and-braces and stays.
- Test PR must not be authored by the coordinator's reviewer login (self-loop guard) and must not be a draft.
