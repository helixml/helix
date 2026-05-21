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
  env vars (`OPENAI_API_KEY` + `OPENAI_BASE_URL`, etc.), with
  `rewriteLocalhostURL` applied.

### US-4: As an advanced user, I want to bring my own custom Goose agents
So that my team's recipes (prompts, MCPs, model settings) appear as
first-class threads in Zed inside Helix.

**Acceptance Criteria**
- Project YAML supports a new field under `agent:`:
  ```yaml
  agent:
    runtime: goose_code
    goose:
      # Optional separate git repo for recipes — works with any git host
      # (GitHub, GitLab, Bitbucket, Gitea, self-hosted, etc.).
      # Omit to load recipes from the project's primary repo.
      recipe_repo: https://github.com/my-org/goose-recipes
      recipe_repo_branch: main   # optional, defaults to repo's default branch
      recipes:
        - name: "Security Reviewer"
          path: security-reviewer.yaml      # relative to recipe_repo root
        - name: "Migration Bot"
          path: workflows/migration-bot.yaml
  ```
- Each entry in `recipes` becomes its own `agent_servers.<slug>` entry
  in Zed settings.json. The Zed agent panel then shows "New Security
  Reviewer", "New Migration Bot", etc. as separate thread options.
- When `recipe_repo` is omitted, recipe `path` values resolve against
  the project's primary git repo (already cloned at session start).
- When `recipe_repo` is set, Helix clones that URL into the container
  at session start (reusing the same git-clone infrastructure that
  handles primary project repos) and resolves recipe `path` values
  against that checkout.
- `recipe_repo` accepts any URL `git clone` accepts — `https://…`,
  `git@…:…`, self-hosted hosts. There is no GitHub-specific code path.
- A project with `recipes:` defined and `runtime: goose_code` still
  shows the plain "Goose" thread alongside the named recipes — losing
  the default would be a regression.

### US-5: As a custom-agent author, I want to iterate on recipes inside Helix
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

### US-6: As an operator, I need a decision on Sway
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
