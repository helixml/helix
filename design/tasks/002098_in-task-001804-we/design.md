# Design: Verify Qwen-Code Upgrade Landed and Diagnose Per-Tool Permission Prompts

## Phase 1 — Audit (no code changes)

### qwen-code PR status

Already established during planning. Repeat with up-to-date `git fetch`:

```bash
cd /home/retro/work/qwen-code && git fetch --all
git log origin/main --oneline | head -5                     # expect 14ebe78ca at tip
git log origin/feature/001804-we-havent-updated-qwen --oneline | head -5
git log origin/main..origin/feature/001804-we-havent-updated-qwen --oneline
```

If the feature branch still has commits not on `main`, the PR was never
merged. Use `gh pr list --repo helixml/qwen-code --state all --head
feature/001804-we-havent-updated-qwen` (or the equivalent against the
internal git server — `origin` is `http://api:8080/git/...`, not
GitHub, so `gh` won't work; use the Helix UI or `curl` the git server
API instead).

### helix PR status

```bash
cd /home/retro/work/helix && git show a532195d1 --stat
cat sandbox-versions.txt | grep QWEN_COMMIT
```

`a532195d1` is the merge commit of helix PR #2238. Confirm its diff is
the OpenAI reasoning-field mapper, not a qwen-commit bump. Confirm
`QWEN_COMMIT` is still `14ebe78ca…`.

### GLM-5.1 availability

The outer LLM proxy is reachable from the inner Helix (host) at
`http://host.docker.internal:8081`. From the planning environment
that host wasn't resolvable, but from the inner Helix containers it
should be. Steps:

```bash
# From inside inner Helix sandbox or browser (not from /home/retro/work shell):
curl -s -H "Authorization: Bearer $OPENAI_API_KEY" \
  http://host.docker.internal:8081/v1/models | jq '.data[].id' | grep -i glm
```

Codebase search shows references to `glm-4.6`, `glm-4.7-flash`, and
a note in `design/sample-profiles/8xMI300X-deepseek-v4-pro.yaml`
about `GLM-5.1-FP8 (754B)` as an alternative — but no first-class
support for `glm-5.1` in `frontend/src/constants/models.ts`. If
GLM-5.1 isn't proxied, fall back to the largest available
GLM/Qwen-coder model the proxy exposes. The user has offered to wire
up GLM-5.1 if it's missing.

## Phase 2 — Reproduce the permission-prompt bug

Use the inner Helix at `http://localhost:8080` (CLAUDE.md credentials:
`test@helix.ml` / `helixtest`). Steps:

1. Register + onboard (testorg → testproj).
2. Create a `qwen_code` agent in the project, pointed at the chosen
   model (GLM-5.1 or fallback). Storage location: agent config in
   `Project → Agents → New Agent`.
3. Start a spec task with `JustDoItMode = false` (so the agent has to
   plan + implement, exercising tool use).
4. Watch the Zed session via `helix spectask stream <ses_id>` and
   the API logs: `docker compose -f docker-compose.dev.yaml logs -f api
   sandbox-nvidia | grep -iE "requestPermission|approval"`.
5. Record: (a) does the agent stall waiting for permission? (b) is
   `requestPermission` actually being called over ACP? (c) is Zed's
   "Always allow tool actions" toggle visibly set?

Document with at least one screenshot of the Zed pane showing the
permission prompt (if it appears) — save under
`/home/retro/work/helix-specs/design/tasks/002098_in-task-001804-we/screenshots/`.

## Phase 3 — Decide where the fix belongs

Three places the auto-approve can be wired. We pick after Phase 2
shows the actual call path.

### Option A — Helix injects `--approval-mode yolo` into the agent launch command

**Pros:** Smallest change. Touches only Helix. Doesn't require Zed
changes or a qwen-code rebuild if the flag already exists.

**Cons:** Need to verify qwen-code's CLI even accepts a startup flag
for approval mode (it currently expects ACP `session/set_mode` to be
called). If not, this is a qwen-code patch.

**Where:** wherever Helix builds the qwen-code agent command line.
`grep -rn "qwen-code\|qwen.cmd" /home/retro/work/helix/api/` to find
it (was not located during planning — likely in
`api/pkg/external-agent/` or in a sandbox boot script).

### Option B — Zed auto-calls `session/set_mode = "yolo"` when its settings have `tool_permissions.default = "allow"`

**Pros:** Fixes all ACP agents (qwen-code, gemini-cli, codex-cli) in
one place. Matches the spirit of Zed's existing "always allow" setting.

**Cons:** Touches Zed. Needs its own PR + sandbox-versions.txt bump.
Could break Zed users who explicitly want the ACP agent to prompt
even when Zed's own tools are auto-approved.

**Where:** Zed's ACP client integration. `grep -rn "set_mode\|setMode"
/home/retro/work/zed/crates/` to find the call site.

### Option C — qwen-code's `acpAgent.ts` defaults to `AUTO_EDIT` for sessions started without explicit `setMode`

**Pros:** Single source of truth at the agent. Works regardless of
ACP client.

**Cons:** Diverges from upstream qwen-code behaviour. Worst for
maintenance — every upstream merge has to re-apply this patch.
Also wrong for interactive `qwen` CLI users.

**Recommendation:** Option A first. If qwen-code doesn't already
have a startup flag, add one (small patch, easy upstream PR). Fall
back to Option B if the user wants the behaviour shared across
all ACP agents.

## Phase 4 — Wire up the missing 001804 work

Independent of the permission fix. Two sub-tasks:

1. If qwen-code's `feature/001804-we-havent-updated-qwen` branch is
   still valid (build green, tests pass), open a fresh PR against
   `main`. If it's gone stale, rebase or re-do the merge.
2. Once merged, bump `helix/sandbox-versions.txt` `QWEN_COMMIT` to
   the new tip and open a Helix PR. Follow CLAUDE.md's strict
   "open Helix PR with bumped hash *before* pushing qwen-code branch"
   ordering rule (see `helix/CLAUDE.md` "Bumping sandbox-versions.txt
   after Zed or Qwen changes").

## Phase 1 Audit Results (2026-06-12)

Run after `git fetch --all` in both repos:

- **qwen-code `main` tip**: still `14ebe78ca feat: extend ACP schema to support HTTP/SSE MCP servers per ACP spec` — unchanged since planning.
- **qwen-code feature branch**: `origin/feature/001804-we-havent-updated-qwen` has exactly 3 commits ahead of `main` (above the upstream commits it pulled in):
  - `ca5f3a28c Disable all external telemetry and phone-home in Helix fork`
  - `f01cdc413 Complete implementation`
  - `420b2013a Merge upstream QwenLM/qwen-code v0.14.4 into fork`
- **Helix PR #2238**: merged as `a532195d1`. Diff is `api/pkg/openai/openai_client.go (+123)` and `api/pkg/openai/reasoning_field_mapper_test.go (+365)` — **completely unrelated** to a qwen-code commit bump. The branch name was reused for an OpenAI reasoning-field mapper PR.
- **`sandbox-versions.txt`**: still `QWEN_COMMIT=14ebe78ca83328323bbaa8cc714d8f3b4a6fce46` (pre-merge tip).
- **Net effect**: CI is still building the pre-v0.14.4 qwen-code. The upstream merge work is sitting on the feature branch, abandoned.
- **PR state check on internal git server**: I do not have a working endpoint to query PR state on the internal git server from this shell. Will need to confirm with the user, or check via the Helix UI. The branch existing on `origin` is evidence the PR was at least pushed.

### GLM-5.1 availability via outer LLM proxy

`/v1/models?provider=<name>` works against the outer Helix API at `outer-api:8080` (from this planning environment) using the `.env` `OPENAI_API_KEY` as Bearer:

- **nebius** (34 models): includes `zai-org/GLM-5` and `zai-org/GLM-5.1`.
- **togetherai** (265 models): includes `zai-org/GLM-5.1`, `zai-org/GLM-5`, `zai-org/GLM-5-FP4`, `zai-org/GLM-4.7`, `zai-org/GLM-4.7-FP8`, `zai-org/GLM-4.7-fp4`, `zai-org/GLM-4.6`, `zai-org/GLM-4.5-Air-FP8`, `zai-org/GLM-4.5V`, `zai-org/GLM-OCR`.
- **openai** (84 models): no GLM.

**Decision: use `nebius / zai-org/GLM-5.1`** for the reproduction test. Nebius has a smaller list (less ambiguity) and the model is current.

The aggregated `/v1/models` (no `?provider=`) returns empty for this token because the aggregation iterates *cached* model lists keyed by `(provider, owner)` (`openai_model_handlers.go:52`) — the per-provider call is the reliable path. The inner Helix's `.env` (`OPENAI_BASE_URL=http://host.docker.internal:8081/v1`) points at the same outer Helix, so the inner Helix sees the same provider catalog.

## Notes for future agents

- The inner Helix is the testbed: `http://localhost:8080`. Do not
  attempt to test against an external Helix.
- The outer LLM proxy at `host.docker.internal:8081` is the model
  source. The `.env` in `/home/retro/work/helix/` contains the
  bearer token.
- CLAUDE.md is gospel for this repo (build commands, git hygiene,
  forbidden actions). Read it before changing build pipeline files
  or sandbox config.
- The fact that helix PR #2238 was named after task 001804 but
  shipped a different change is a process problem worth flagging
  to the user — likely the spec-task system marked 001804 done on
  the helix-side merge without verifying the qwen-side ever landed.
- `QWEN_COMMIT=14ebe78ca…` is the *current* shipping qwen-code. Any
  qwen-side patch (e.g. for Option C above) needs to be made on
  whatever commit becomes the new tip after the v0.14.4 merge lands.
