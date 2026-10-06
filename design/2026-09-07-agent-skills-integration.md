# Agent skills in spec-task sandboxes: current state and integration plan

**Date:** 2026-09-07
**Status:** Phase 1 implemented (runtime refresh + plumbing); see "Decision" below

Goal: agents running inside Helix spec-task sandboxes should reliably *have*
the [helixml/skills](https://github.com/helixml/skills) skills, be able to
*run* what they describe, and be *nudged* to use them when a task matches.

## 1. What exists today (verified 2026-09-07)

### Install path

- `Dockerfile.ubuntu-helix:1448-1472` (and the same block in
  `Dockerfile.sway-helix:946-969`) shallow-fetches `helixml/skills` at
  `ARG SKILLS_COMMIT` and copies the dirs named in `ARG HELIX_SKILLS`
  (default `"helix-artifacts"`) into `/opt/helix/skills`.
- `desktop/shared/helix-workspace-setup.sh:645-675` symlinks each skill into
  `~/.claude/skills`, `~/.agents/skills`, `~/.qwen/skills` on every container
  start (after `~/.claude` is re-pointed at persistent storage).
- The upstream repo has 8 skills: `helix-cli`, `helix-board`,
  `helix-spec-tasks`, `helix-files`, `helix-artifacts`, `helix-agents`,
  `helix-deploy`, `helix-e2e`. The pin `657b131` is upstream HEAD.
  Only `helix-artifacts` ships, by design (commit `ee7852fb4`: the
  control-plane skills would point an agent at reconfiguring the Helix that is
  running it).

### How each harness discovers skills (all checked against source or vendor docs)

| Runtime | Global dirs scanned | Project dirs | Covered by our links? |
|---|---|---|---|
| `zed_agent` | `~/.agents/skills` (`agent_skills.rs`) | `<worktree>/.agents/skills`; Helix fork auto-trusts worktrees (`trusted_worktrees.rs:can_trust`, `cfg!(feature="external_websocket_sync")`) | yes |
| `claude_code` | `~/.claude/skills` | `.claude/skills` | yes |
| `codex_cli` | `$HOME/.agents/skills`, `/etc/codex/skills`; symlinks followed | `.agents/skills` at cwd/parent/repo root | yes |
| `gemini_cli` | `~/.gemini/skills` **or** `~/.agents/skills` | `.gemini/skills` or `.agents/skills` | yes |
| `goose_code` | `~/.agents/skills` (`crates/goose/src/sources.rs` @ `GOOSE_COMMIT`) | `.agents/`, `.goose/`, `.claude/` | yes |
| `opencode` | `~/.config/opencode/skills`, `~/.claude/skills`, `~/.agents/skills` | `.opencode/`, `.claude/`, `.agents/` | yes (logs one dup warning per skill) |
| `qwen_code` | `$QWEN_HOME/skills` **and** `~/.agents/skills` (`getSkillsBaseDirs` in the bundled `acpAgent` chunk) | `.qwen/skills`, `.agents/skills` | yes via `~/.agents`; **`~/.qwen/skills` is dead** because settings-sync-daemon sets `QWEN_HOME=/home/retro/work/.qwen-state` |
| `deepseek_harness` | unknown — not checked | | **unverified** |

Takeaway: `~/.agents/skills` is the universal directory; `~/.claude/skills` is
needed only for Claude Code. The `~/.qwen/skills` link does nothing.

Every harness also injects the skill catalog (name + description) into its own
system prompt natively — Zed renders `<available_skills>` from
`system_prompt.hbs:220-247`. So once a skill is on disk the *harness* already
advertises it; what is missing is Helix-side context on *when* it matters.

### What is broken

1. **The pin is dead config.** `SKILLS_COMMIT` in `sandbox-versions.txt` is
   read by nothing — `stack build-desktop` passes only `GO_VERSION`
   (`stack:868-869`), `.drone.yml:1724-1731` passes `CUDA_BASE_IMAGE` and
   `GO_VERSION`. The Dockerfile `ARG` default is what gets built; the two
   values match only by hand, in three files. `CLAUDE.md:704` ("a
   `SKILLS_COMMIT` edit plus `./stack build-ubuntu`") is wrong.
2. **The `helix` CLI cannot authenticate inside the sandbox.** The sandbox
   exports `HELIX_API_URL` + `USER_API_TOKEN` (+ `HELIX_PROJECT_ID`,
   `HELIX_SPEC_TASK_ID`, `HELIX_SESSION_ID`, `HELIX_USER_ID` —
   `hydra_executor.go:1370-1480`). The CLI reads `HELIX_URL` (default
   `https://app.helix.ml`) + `HELIX_API_KEY` (`config/cli_config.go`,
   `client.go:162-176`). Only `helix artifact` has a private fallback
   (`cli/artifact/cmd.go:279-280`). Verified with the built binary:

   ```
   $ HELIX_API_URL=http://localhost:8080 USER_API_TOKEN=hl-test helix api /projects/prj_x
   Error: apiKey is required, find yours in your helix account page and set HELIX_API_KEY and HELIX_URL
   ```

   The shipped `helix-artifacts` skill's own first step is
   `helix api "/projects/${HELIX_PROJECT_ID}"` — it fails in the very
   environment it is installed into. Every other skill would fail the same way.
3. **`HELIX_ORGANIZATION_ID` is not set for spec-task desktops** — only for
   Sandboxes-API containers (`sandbox/controller_provision.go:142`). The
   `helix-artifacts` skill tells the agent to use it.
4. **Nothing Helix-side mentions skills.** The planning and implementation
   prompts (`spec_task_prompts.go`, `agent_instruction_service.go`) describe
   MCP servers, screenshots, web search, startup scripts — not skills. The
   precedent for why that matters is `BuildAgentToolsSection`
   (`spec_task_prompts.go:349-352`): tools that are only in the tool list and
   never in the prompt don't get reached for.
5. **Naming collision.** Project Settings already has a "Skills" tab
   (`ProjectSettingsSidebar.tsx:82`) meaning Helix-app API/MCP tools
   (`TypesAssistantSkills`), unrelated to `SKILL.md` agent skills. Any UI or
   docs for agent skills must not reuse that label unqualified.

### What already works and should be reused

- desktop-bridge scans skill roots (`desktop/workspace_review.go:504-534`)
  and the composer offers `$skill` completion on connected sandboxes
  (`sandboxComposerSuggestions.logic.ts`, `RobustPromptInput.tsx:127`),
  inserting `$name` as plain text. Once skills are linked, users can already
  name them in a message.
- Project-local skills in the task repo (`<repo>/.agents/skills/`) are
  honoured by every runtime with zero Helix changes, because the Zed fork
  auto-trusts worktrees.

## Decision (2026-09-07)

Rather than pin-and-bake, tasks now **always get the latest skills**: the image
bakes a shallow clone as an offline seed, and `helix-workspace-setup.sh`
refreshes it from `helixml/skills` `main` at every container start, keeping the
last good checkout when the fetch fails. The default linked set is the
**task-facing** skills — `helix-cli`, `helix-artifacts`, `helix-spec-tasks`,
`helix-board`, `helix-files`. `helix-deploy`, `helix-e2e` and `helix-agents`
stay opt-in (`HELIX_SKILLS=all` or an explicit list): they teach control-plane
install/upgrade, DB access and org creation, and a task agent holds a
`USER_API_TOKEN`-authenticated CLI, so handing them out by default would give a
prompt-injected agent an on-ramp to the Helix running it (review on PR 3190).
The task-facing skills widen the earlier `helix-artifacts`-only set on
purpose: delegation and board/task/file operations are what an in-task agent
legitimately needs, and are project-scoped by the token. Shipped with it: the
`SKILLS_COMMIT` build-arg plumbing, the CLI credential fallback in
`config.LoadCliConfig`, `HELIX_ORGANIZATION_ID` for spec-task desktops, links
into `~/.agents/skills` + `~/.claude/skills` only, and a static "## Helix
skills" section in both task prompts. The `helix-sandbox` skill (phase 2) is
still to be written in the skills repo.

## 2. Plan (as originally proposed)

Four phases, ordered so each ships alone. Phases 1–3 are one Helix PR each;
phase 2 also needs a skills-repo PR.

### Phase 1 — make what is installed actually work (plumbing)

1. **Wire `SKILLS_COMMIT` through the build**, mirroring `QWEN_VERSION`:
   `stack build-desktop` reads it from `sandbox-versions.txt` and passes
   `--build-arg SKILLS_COMMIT=…`; `.drone.yml` does the same in the
   helix-ubuntu step. Drop the hard-coded default from both Dockerfiles'
   `ARG SKILLS_COMMIT` so a build without the arg fails loudly instead of
   drifting. `sandbox-versions.txt` becomes the single source.
2. **Fix CLI credentials at the root, not per-command.** `config.LoadCliConfig`
   resolves `HELIX_URL` → `HELIX_API_URL` and `HELIX_API_KEY` →
   `USER_API_TOKEN`, in that order. Remove the private fallback in
   `cli/artifact/cmd.go` (`newClient`) so there is one resolution path. Add a
   unit test for both env shapes. Do **not** solve this by exporting
   `HELIX_URL`/`HELIX_API_KEY` from the sandbox: `HELIX_URL` defaulting to
   `app.helix.ml` is the footgun, and the CLI is what users run outside the
   sandbox too.
3. **Export `HELIX_ORGANIZATION_ID`** in `buildEnvVars` for spec-task desktops
   (the project's org is known at provisioning). Keeps the skill text true.
4. **Simplify the link targets** in `helix-workspace-setup.sh` to
   `~/.agents/skills` + `~/.claude/skills`; drop `~/.qwen/skills` and fix the
   comment to say why (`QWEN_HOME` override). Add `deepseek_harness` to the
   verification matrix and check its discovery dir before claiming coverage.
5. **Fix `CLAUDE.md`** (`Agent skills` rows and the "Agent skills" section) to
   match.

Verification: inner Helix, one spec task per runtime; `helix spectask exec`
`ls -l ~/.agents/skills`, `helix api /projects/$HELIX_PROJECT_ID`, and
`helix artifact list` from inside the container.

### Phase 2 — ship the right skills, selected at runtime

**Bake all skills, select at link time.** Copy the whole `skills/` tree to
`/opt/helix/skills-available/` at build; `helix-workspace-setup.sh` links only
the names in `HELIX_SKILLS` (runtime env, default set by the image). Cost is a
few hundred KB; benefit is that operators and dev stacks can enable
`helix-board`/`helix-spec-tasks` for an org that wants delegating agents, or
turn a skill off, without a desktop rebuild. `hydra_executor.buildEnvVars`
forwards a per-deployment `HELIX_SKILLS` if set. Per-project selection is a
later UI change on top of the same env var — the plumbing is the same.

**Default set:** `helix-artifacts` plus a new **`helix-sandbox`** skill,
authored in `helixml/skills`, that is the thing an in-sandbox agent most lacks:
what this environment is. Contents:

- the env vars that exist (`HELIX_API_URL`, `USER_API_TOKEN`,
  `HELIX_PROJECT_ID`, `HELIX_SPEC_TASK_ID`, `HELIX_SESSION_ID`,
  `HELIX_ORGANIZATION_ID`, `HELIX_WORKING_BRANCH`, …) and that `helix` is on
  PATH and already authenticated;
- the two repos (`/home/retro/work/<repo>` vs `helix-specs`), the task's
  design-doc dir, attachments path, screenshots dir, `startup.sh`;
- what the `helix-desktop` and `chrome-devtools` MCP servers are for;
- how to publish results (`$helix-artifacts`), attach files to the task, and
  what *not* to do (no `gh`/ssh for pushes — the "How Pushing Works" section
  exists because agents keep doing this).

This is progressive disclosure for material the prompt currently repeats in
full on every turn. **Do not** move phase-critical instructions (where to work,
tasks.md format, push-after-every-task) out of the prompt; skills are
on-demand and an agent that never opens one must still behave.

Keep the control-plane skills (`helix-cli`, `helix-board`, `helix-spec-tasks`,
`helix-files`, `helix-agents`, `helix-deploy`, `helix-e2e`) out of the default
set. Delegation is already covered by the MCP tools in
`BuildAgentToolsSection`; the CLI equivalents would be a second, unscoped path
to the same thing.

The skill repo's README `Version note` says the `spectask board|…` commands are
post-2.12.3; the desktop image builds the CLI from the same commit as the API,
so in-sandbox they are always current — but a bumped `SKILLS_COMMIT` must be
checked against `helix <cmd> --help` in the image, per the repo's own rule.

### Phase 3 — tell the agent, briefly

Add `BuildSkillsSection()` next to `BuildAgentToolsSection` and include it in
both the planning and approval templates. It is short and static, because the
harness already prints the catalog:

> ## Helix skills
> This sandbox ships agent skills (`helix-sandbox`, `helix-artifacts`, …).
> Your harness lists them with their descriptions; open one when a task
> matches — e.g. read `helix-sandbox` before your first `helix` command, and
> use `helix-artifacts` whenever the user wants a page, dashboard, report, PDF
> or image they can open from the project.

Users can also type `$helix-artifacts` in the composer. Add a one-line mention
to the composer placeholder/tooltip so the existing `$` completion is
discoverable; do not build a second skills UI, and do not label anything
"Skills" in Project Settings without qualifying it (see collision above).

### Phase 4 — verify per runtime, then document

Matrix in the inner Helix: for each of `zed_agent`, `claude_code`,
`codex_cli`, `gemini_cli`, `goose_code`, `opencode`, `qwen_code`,
`deepseek_harness`:

1. start a spec task; confirm links in `~/.agents/skills`;
2. ask the agent "use $helix-sandbox to tell me which project you're in" —
   expect it to read the skill and run `helix api`/`helix artifact list`
   successfully;
3. ask for a one-file HTML report via `$helix-artifacts` — expect an artifact
   row for the project.

Record which harnesses opened the skill unprompted vs only when named; that
feedback goes back into the skill descriptions (descriptions are the retrieval
key). Then update `CLAUDE.md` and `docs`.

## 3. Deferred / explicitly not doing

- **Per-project skill selection UI** — after phase 2 proves the env-var
  plumbing; needs its own label to avoid the existing "Skills" tab.
- **Serving skills from `helix-specs/.helix/skills/`** as an org-managed
  channel (link into `~/.agents/skills` at setup). Cheap and attractive, but
  the task repo's `.agents/skills/` already gives teams a versioned channel
  today; add the helix-specs one only if a team asks for skills that must not
  live in the code repo.
- **Moving the "Visual Testing" / "Web Search" / push-recovery prose out of the
  prompts** into `helix-sandbox`. Worth measuring after phase 2 — compare turn
  counts and screenshot usage on the same tasks before cutting prompt text.
