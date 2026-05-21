# Requirements: Integrate Goose AI Agent into Zed via ACP

## Background

Goose (now hosted by AAIF, originally by Block) is an open-source AI agent
that natively supports the Agent Client Protocol (ACP) — the same protocol
Zed uses to talk to Claude Code and Qwen Code in Helix today. Adding Goose
gives Helix users another agent option without re-inventing any of the Zed
plumbing.

In Goose terminology, **a "custom agent" is a recipe** — a YAML file that
bundles `instructions`, `prompt`, `extensions` (MCP servers), `parameters`,
and `settings` (model/provider) into a reusable workflow. Recipes can be
loaded from the local filesystem (`GOOSE_RECIPE_PATH`) or from a GitHub
repo (`GOOSE_RECIPE_GITHUB_REPO`). Each recipe surfaces in an ACP client
(Zed) as its own selectable agent.

Helix already has a project YAML spec (`ProjectSpec` in
`api/pkg/types/project.go`) with an `agent:` block and a `repositories:`
list. Custom Goose agents slot in there naturally: recipes live as files
inside the project's primary git repo, and the project YAML points at
them.

## User Stories

### US-1: As a Helix user, I want to choose Goose as my code agent
So that I can use Goose's tools and extensions from inside Zed in my Helix
session, the same way I can already use Qwen Code or Claude Code today.

**Acceptance Criteria**
- Project Settings → Code Agent Runtime offers "Goose" alongside the
  existing options (Zed Agent, Qwen Code, Claude Code).
- Onboarding flow lets a new user pick Goose with no extra steps.
- When Goose is selected with no custom recipes, opening Zed shows a
  single "Goose" thread option in the agent panel and a fresh thread
  starts a working Goose session bound to the user's configured LLM.

### US-2: As an operator, I want Goose pre-installed in the desktop image
So that sessions start instantly with no per-session download.

**Acceptance Criteria**
- `goose --version` works inside a fresh `helix-ubuntu` container.
- Version is pinned (`GOOSE_VERSION`) so rebuilds are reproducible.
- Goose telemetry/auto-update are disabled (mirroring the
  `~/.qwen/settings.json` and `~/.gemini/settings.json` pattern in
  `Dockerfile.ubuntu-helix`).

### US-3: My LLM provider/model selection drives Goose
So that I don't have to configure Goose separately.

**Acceptance Criteria**
- `GOOSE_PROVIDER` and `GOOSE_MODEL` are derived from `CodeAgentConfig`
  and written into Zed `agent_servers.goose.env` by settings-sync-daemon.
- Helix-proxy API key + base URL forwarded via the provider's expected
  env vars, picked based on `CodeAgentConfig.APIType`:
  - `openai` → `OPENAI_API_KEY` + `OPENAI_BASE_URL`
  - `anthropic` → `ANTHROPIC_API_KEY` + `ANTHROPIC_BASE_URL`
  - `google` → `GOOGLE_API_KEY` (base URL is provider-managed)
- `rewriteLocalhostURL` applied to any base URL so the daemon can
  reach the Helix API proxy from inside the container (same handling
  Qwen and Claude Code already get).
- See the per-provider table in `design.md` (D4) for the authoritative
  mapping; this list must stay in sync with it.

### US-4: As an advanced user, I want to bring my own custom Goose agents
So that my team's recipes (prompts, MCPs, model settings) appear as
first-class threads in Zed inside Helix.

**Model**: a Goose recipe repo is just another repository attached to
Helix via the existing "Attach external repository" flow. Helix
mirrors it into its internal git store the same way it mirrors any
other project repo — auth, proxies, TLS, and the
`/filestore/git-repositories/{repoID}` mirror path all flow through
the existing `GitRepository` plumbing. Goose itself only ever sees a
local directory; no goose-specific auth surface is introduced.

**Acceptance Criteria — UI path**
- A new Project Settings section "Goose recipes" lets the user:
  1. Pick an already-attached repository from a dropdown (org-scoped
     list of `GitRepository` rows) as the recipe source.
  2. If the desired repo isn't attached yet, an "Attach a recipe repo"
     button opens the existing `LinkExternalRepositoryDialog` (the
     same dialog used for code repos) — on save, the repo is attached
     to the project and auto-selected as the recipe source.
  3. List recipes inside the selected repo with `name` + `path`
     (path within the repo). Add/edit/remove entries inline.
- Saving the form persists the same fields the YAML path writes (see
  below) — UI and YAML are two front-ends to one backing model.

**Acceptance Criteria — YAML path**
- Project YAML supports a new field under `agent:`:
  ```yaml
  repositories:
    - url: https://github.com/my-org/my-codebase
      primary: true
    - url: https://github.com/my-org/goose-recipes   # must be attached

  agent:
    runtime: goose_code
    goose:
      # Optional. Reference an attached repository by its upstream URL.
      # Omit to load recipes from the project's primary repo.
      recipe_repo_url: https://github.com/my-org/goose-recipes
      recipes:
        - name: "Security Reviewer"
          path: security-reviewer.yaml      # relative to recipe-repo root
        - name: "Migration Bot"
          path: workflows/migration-bot.yaml
  ```
- `recipe_repo_url` is the **upstream URL** (e.g. the GitHub URL the
  user entered when attaching the repo), **not** Helix's internal
  `GitRepository.ID`. This is deliberate: project YAML must be
  portable between Helix deployments, and internal IDs are
  per-instance UUIDs that wouldn't match across installs.
- Resolution happens server-side at YAML apply time and at task start:
  `GetGitRepositoryByExternalURL(orgID, recipe_repo_url)` finds the
  attached repo, then `LocalPath` gives the mirror checkout. The
  upstream URL goes over the wire; the mirror path stays internal.
- URL normalisation: trim trailing slashes and optional `.git` suffix,
  treat `https://github.com/x/y` and `https://github.com/x/y.git` as
  the same repo. (Helix's existing dedup behaviour in `applyProject`
  should already handle this; if not, normalise both sides before
  comparing.)
- If `recipe_repo_url` doesn't resolve to an attached repo,
  `applyProject` returns a clear error: "repository <url> not
  attached — attach it via the Git Repositories page or add it to
  `repositories:` first".
- Each entry in `recipes` becomes its own `agent_servers.<slug>` entry
  in Zed settings.json. The Zed agent panel then shows "New Security
  Reviewer", "New Migration Bot", etc. as separate thread options.
- When `recipe_repo_url` is omitted, recipe `path` values resolve
  against the project's primary git repo.
- A project with `recipes:` defined and `runtime: goose_code` still
  shows the plain "Goose" thread alongside the named recipes — losing
  the default would be a regression.

**Acceptance Criteria — shared by both paths**
- The recipe repo is mirrored once per org. Multiple projects in the
  same org that reference the same upstream URL share one mirror clone.
- Private upstream repos work as long as the user attached them with
  valid credentials (via the existing `LinkExternalRepositoryDialog`
  / `GitProviderConnection` flow). No new auth code in this task.

### US-5: As a spec-task creator, I want to pick a recipe and supply its parameters at task-creation time
So that I get a fully automated agent run with the right context, instead
of being prompted mid-session — which would defeat the point of a spec
task.

**Rationale**: Zed inside Helix is the *execution surface* for spec
tasks; spec-task creation is the *configuration surface*. Recipe
parameters belong at configuration time, not execution time.

**Acceptance Criteria**
- The spec-task creation page shows a "Goose recipe" selector when the
  project's runtime is `goose_code` and the project declares recipes.
  Selecting a recipe dynamically renders a form derived from that
  recipe's `parameters:` schema:
  - `string` → text input
  - `number` → number input
  - `boolean` → toggle
  - `select` → dropdown populated from the recipe's `options:`
  - `date` → date picker
  - `file` → text input expecting a **repo-relative path** inside the
    spec task's primary repo (validated to exist + not escape the
    repo root via `filepath.Clean` containment check)
- Required parameters are marked required; optional parameters
  pre-fill with the recipe's `default:` value. Goose's `user_prompt`
  requirement is treated as required at this layer (no runtime
  prompt surface in Helix).
- The spec task persists the chosen recipe (`recipe_name`) and the
  parameter values (`recipe_params: map[string]string`) on the task
  row.
- At task start, the Helix API reads the recipe YAML from the
  GitRepository mirror, performs Jinja-style `{{ var }}` substitution
  on `instructions:` and `prompt:` using `recipe_params`, and ships
  the *substituted* recipe content (system prompt + extensions +
  settings) to settings-sync-daemon as part of `CodeAgentConfig`.
- `file:` parameters resolve at substitution time: the API reads the
  file from the spec task's primary repo checkout and substitutes
  the **contents** (matching Goose's CLI semantics).
- A spec task with no recipe chosen continues to use the plain
  "Goose" agent_servers entry — recipes are opt-in per task.

### US-6: As a custom-agent author, I want to iterate on recipes inside Helix
So that I can edit a recipe in Zed, try it, fix it, and ship it without
leaving the Helix session.

**Acceptance Criteria**
- Recipe files in the project repo are immediately editable in Zed
  (they're just YAML files in the workspace).
- A terminal in the Helix session can run `goose recipe validate <file>`
  against the edited recipe — the `goose` binary is on `$PATH`.
- Closing and reopening a Goose thread in Zed picks up edits without
  needing to restart the whole session (each new ACP `initialize` call
  re-reads the recipe from disk).
- Committing the recipe via Git from inside the Helix session
  propagates to all future sessions (same git flow Helix already uses
  for code).

### US-7: As an operator, I need a decision on Sway
The user explicitly asked: "do we need to install goose in helix-ubuntu
(and sway, although increasingly I'm thinking we should just delete sway)".

**Acceptance Criteria**
- This task installs Goose in `helix-ubuntu` only.
- `helix-sway` is left untouched; recommendation in `design.md` is to
  delete it in a follow-up task (it's already gated behind
  `HELIX_EXPERIMENTAL_DESKTOPS` and is not on the production path).

## Out of Scope

- Deleting `helix-sway` (separate task).
- A Helix-hosted Goose recipe registry / marketplace.
- Authoring/editing recipes through a Helix UI form (vs editing the
  YAML directly in Zed). Form-based authoring can be a later UX layer
  on top of the same YAML schema.
- Goose Desktop (Electron app) — Helix users interact with Goose
  through Zed.
- Per-user recipe overrides — recipes are scoped to the project for v1.
  Per-user customisation can come later by layering a user recipe dir
  on top of the project's.

## Notes for Future Agents

- Goose docs live at `https://goose-docs.ai`. Authoritative refs:
  - ACP clients: `/docs/guides/acp-clients`
  - Recipe reference: `/docs/guides/recipes/recipe-reference`
  - Reusable recipes: `/docs/guides/recipes/session-recipes`
- Goose's source repo: `github.com/aaif-goose/goose`.
- Goose recipes use Jinja-style `{{ parameter }}` substitution; required
  vs optional vs `user_prompt` parameter modes affect the Desktop dialog
  (recipes used through ACP rely on `user_prompt` for interactivity).
- The `extensions:` block inside a recipe declares MCP servers per-recipe.
  This is separate from Helix-injected MCPs in Zed's `context_servers`,
  but both are visible to Goose at runtime — recipe-declared extensions
  layer on top of `context_servers`.
- Goose currently works best with Claude 4 models (per upstream docs)
  but any provider Goose supports can be wired through Helix's
  OpenAI-compatible proxy via `GOOSE_PROVIDER=openai` +
  `OPENAI_BASE_URL=<helix-proxy>`.
