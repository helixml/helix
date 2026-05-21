# Design: Integrate Goose AI Agent into Zed via ACP

## Summary

Add Goose as a fourth code-agent runtime alongside `zed_agent`,
`qwen_code`, `claude_code`. The integration has two layers:

1. **Base runtime** — install the `goose` CLI into `helix-ubuntu` and
   have `settings-sync-daemon` write an `agent_servers.goose` block into
   Zed's settings.json. Mirrors the Qwen pattern exactly.
2. **Custom agents** — extend the existing project YAML spec so users
   can declare named Goose recipes that live in their project's git
   repo. Each recipe becomes its own `agent_servers.<slug>` entry, so
   Zed shows it as a distinct thread option.

## Architecture

```
Helix project YAML (agent.goose.recipes: [...])
        │
        │  on session start
        ▼
helix-api ──► ProjectAgentSpec.Goose ──► CodeAgentConfig (extended)
        │
        │  WebSocket
        ▼
settings-sync-daemon (in helix-ubuntu container)
        │
        │  writes
        ▼
~/.config/zed/settings.json:
  agent_servers:
    goose:                 { command: "goose", args: ["acp"], env: {...} }
    security-reviewer:     { command: "goose", args: ["acp"], env: {GOOSE_RECIPE=...}}
    migration-bot:         { command: "goose", args: ["acp"], env: {GOOSE_RECIPE=...}}
        │
        ▼
Zed agent panel ──spawns──► goose acp ──ACP/stdio──► Goose
                                                       │
                                                       └──► LLM via GOOSE_PROVIDER/MODEL
                                                       └──► recipe extensions (MCPs)
                                                       └──► Zed context_servers (inherited MCPs)
```

## Key Decisions

### D1: Pre-install `goose` in `Dockerfile.ubuntu-helix`, not at runtime

Goose's recommended install is `curl … | bash` which fetches a binary.
Doing that at session start would add a download per cold start and break
sessions if GitHub is unreachable. Bake the binary in, pinned via
`GOOSE_VERSION` (upstream's documented CI/CD-safe install knob).

### D2: New runtime constant `goose_code`

`CodeAgentRuntime` is a string enum in `api/pkg/types/task_management.go`.
Adding `goose_code` keeps existing runtimes untouched and is symmetric
with the prior pattern.

### D3: Use Zed's `type: "custom"` agent_server — not `type: "registry"`

`claude-acp` uses `registry` because it's in Zed's extension registry.
Goose isn't. The Qwen entry uses `custom`, which is the right precedent
for a CLI-on-disk.

### D4: Forward LLM config via `GOOSE_PROVIDER` + `GOOSE_MODEL`

Per Goose docs, env vars override the user's Goose config file. That's
the right hook for Helix — `CodeAgentConfig` already has the user's
chosen provider/model.

| Helix `APIType` | `GOOSE_PROVIDER` | API-key env       | Base-URL env         |
|-----------------|------------------|-------------------|----------------------|
| `openai`        | `openai`         | `OPENAI_API_KEY`  | `OPENAI_BASE_URL`    |
| `anthropic`     | `anthropic`      | `ANTHROPIC_API_KEY` | `ANTHROPIC_BASE_URL` |
| `google`        | `google`         | `GOOGLE_API_KEY`  | — (provider-managed) |

`rewriteLocalhostURL` is applied to all base URLs (same as Qwen/Claude).

### D5: Don't touch `Dockerfile.sway-helix` in this PR

Sway is gated behind `HELIX_EXPERIMENTAL_DESKTOPS` (see CLAUDE.md). User
is leaning toward deleting it. Installing Goose there now is wasted work
if it gets deleted next.

### D6: Reuse Zed's `context_servers` for shared MCPs

Goose automatically picks up MCP servers declared in Zed's
`context_servers`. Helix already populates this (chrome-devtools, Kodit,
github, drone-ci). Recipe-declared `extensions:` layer on top — no
extra plumbing needed.

---

### D7: Custom Goose agents are recipes from a git repo (any host)

The user asked: "How can advanced users import customised agents into
Helix? Slot it into the project YAML? Read from a git repo?"

**Decision**: Recipes are YAML files committed to a git repo. The
project YAML lists them by path. By default that's the project's
primary repo (already cloned at session start). For teams that want a
shared recipe library separate from any one codebase, an optional
`recipe_repo:` field accepts **any git URL** — GitHub, GitLab,
Bitbucket, self-hosted Gitea, etc. — and Helix clones it the same way
it already clones primary project repos.

**Project YAML extension** — new optional `goose:` block under `agent:`:

```yaml
agent:
  runtime: goose_code
  provider: anthropic
  model: claude-opus-4-7
  goose:
    # Optional: pull recipes from a separate git repo. Any URL `git clone`
    # accepts works. Omit to load from the project's primary repo.
    recipe_repo: https://github.com/my-org/goose-recipes
    recipe_repo_branch: main    # optional
    recipes:
      - name: "Security Reviewer"
        path: security-reviewer.yaml      # relative to recipe_repo root
      - name: "Migration Bot"
        path: workflows/migration-bot.yaml
```

**Why this shape**:
- Mirrors how `repositories:` already works in `ProjectSpec` (path-based
  references inside a known repo).
- Recipes versioned with the code they operate on (when in the project
  repo), or shared across projects (when in a `recipe_repo`).
- Zero new infrastructure: no recipe store, no upload UI, no separate
  sync mechanism. Git is the source of truth.
- **Host-agnostic.** Earlier draft used Goose's
  `GOOSE_RECIPE_GITHUB_REPO` (which depends on the `gh` CLI and only
  speaks GitHub). Replacing that with a generic `recipe_repo:` URL
  means GitLab/Bitbucket/self-hosted users get the same DX. The clone
  goes through Helix's git-repo plumbing — same auth, same proxy
  handling, same TLS config — instead of through `gh`.
- Goose itself just sees a local directory; we point it at the
  checkout via `GOOSE_RECIPE_PATH` (one of Goose's standard recipe-load
  locations). No GitHub-specific code path remains.

**Why per-recipe `agent_servers` entries (vs one Goose with a recipe picker)**:
- Zed surfaces each `agent_servers.<name>` as a separate "New <name>"
  option in the agent panel. That's the cleanest UX — your custom
  agents look like first-class agents, not options buried in a
  sub-menu.
- The Goose ACP server doesn't expose a recipe-picker UI inside the
  ACP session, so embedding the choice in `agent_servers` is the only
  user-facing surface that actually works in Zed today.
- Per-entry `env` lets each recipe pin its own `GOOSE_RECIPE_PATH` or
  use Goose's existing `--recipe` argument-passing convention.

**How `settings-sync-daemon` builds the agent_servers map**:

For each recipe entry in `agent.goose.recipes`, emit:

```jsonc
"<slug>": {
  "name": "<recipe.name>",
  "type": "custom",
  "command": "goose",
  "args": ["run", "--recipe", "<absolute-path-in-container>", "--interactive"],
  "env": {
    /* same provider/model/base-url env as plain goose */
    "GOOSE_RECIPE_PATH": "<recipe-repo-checkout-dir>"
  }
}
```

`<absolute-path-in-container>` is the resolved recipe path. When
`recipe_repo` is unset, paths resolve against
`/home/retro/work/<primary-repo>/`. When `recipe_repo` is set, paths
resolve against the recipe-repo checkout (e.g.
`/home/retro/work/.goose-recipes/<repo-name>/`). Both directories are
populated by Helix's existing git-clone flow before the daemon writes
Zed settings.

`GOOSE_RECIPE_PATH` is set on every Goose agent_server entry so that
recipes can also reference sibling files (subrecipes, prompt
fragments) by short name relative to the checkout.

> **Open question for implementation**: confirm whether `goose acp`
> accepts a recipe via CLI flag, env var (`GOOSE_RECIPE`), or only
> through `goose run --recipe`. The upstream docs cover `goose run` for
> recipes and `goose acp` for ACP, but don't explicitly document
> combining them. First step in implementation is to test
> `goose acp --recipe <file>` in a dev container — if it works, use it;
> if not, fall back to `goose run --recipe <file> --acp` (or whichever
> flag upstream provides). If neither works, file an upstream issue and
> ship US-1/2/3 first; US-4/5 can wait on a Goose release.

The plain `goose` entry is always emitted, even when recipes are
defined, so users keep access to a vanilla Goose session.

### D8: Iteration DX — edit, validate, reload, commit

The iteration loop for a custom-agent author inside a Helix session:

1. **Edit** the recipe in Zed (it's a YAML file in the workspace).
2. **Validate** from a Zed terminal: `goose recipe validate <file>`.
3. **Reload** by closing and reopening the recipe's thread in Zed's
   agent panel. Each `initialize` call re-reads the recipe from disk —
   no full session restart needed.
4. **Commit** when happy. Git push propagates to all future sessions.

No new Helix infrastructure is required for this loop — it falls out
of the existing combination of (project git repo) + (Zed workspace open
on that repo) + (per-thread `goose acp` lifecycle).

### D9: ProjectAgentSpec extension shape

Add to `ProjectAgentSpec` in `api/pkg/types/project.go`:

```go
type ProjectAgentSpec struct {
    // ...existing fields...
    Goose *ProjectAgentGoose `json:"goose,omitempty" yaml:"goose,omitempty"`
}

type ProjectAgentGoose struct {
    RecipeRepo       string                    `json:"recipe_repo,omitempty" yaml:"recipe_repo,omitempty"`
    RecipeRepoBranch string                    `json:"recipe_repo_branch,omitempty" yaml:"recipe_repo_branch,omitempty"`
    Recipes          []ProjectAgentGooseRecipe `json:"recipes,omitempty" yaml:"recipes,omitempty"`
}

type ProjectAgentGooseRecipe struct {
    Name string `json:"name" yaml:"name"`
    Path string `json:"path" yaml:"path"`  // relative to recipe_repo root, or primary repo root if recipe_repo is unset
}
```

`CodeAgentConfig` (sent to settings-sync-daemon) gains:

```go
GooseRecipes        []CodeAgentGooseRecipe `json:"goose_recipes,omitempty"`
GooseRecipeRootDir  string                 `json:"goose_recipe_root_dir,omitempty"` // absolute path in container
```

with `CodeAgentGooseRecipe { Name, AbsolutePath string }` — the API
server clones `recipe_repo` (if set) via the existing
`GitRepository` infrastructure, resolves each `Path` to an absolute
path under the checkout, and sends both the per-recipe paths and the
root dir to the daemon. The daemon stays repo-agnostic.

## Files Touched

| File                                                        | Change                                                                  |
|-------------------------------------------------------------|-------------------------------------------------------------------------|
| `Dockerfile.ubuntu-helix`                                   | Install pinned Goose CLI; telemetry-off config                          |
| `api/pkg/types/task_management.go`                          | Add `CodeAgentRuntimeGooseCode = "goose_code"`                          |
| `api/pkg/types/project.go`                                  | Add `Goose *ProjectAgentGoose` + nested types                           |
| `api/pkg/types/types.go`                                    | Extend `CodeAgentConfig` with `GooseRecipes`, `GooseGithubRecipeRepo`   |
| `api/pkg/server/project_handlers.go` (`applyProject`)       | Persist new YAML fields; auto-register `recipe_repo` as a GitRepository |
| `api/pkg/external-agent/zed_config.go` (`buildCodeAgentConfig`) | Ensure recipe-repo clone is current; resolve recipe paths to absolute |
| `api/cmd/settings-sync-daemon/main.go`                      | New `case "goose_code":` emitting `agent_servers.goose` + per-recipe    |
| `frontend/src/types.ts`, `api/api.ts`, `contexts/apps.tsx`  | Add `'goose_code'` to runtime union + display name                      |
| `frontend/src/pages/Onboarding.tsx`, `ProjectSettings.tsx`  | Surface "Goose" as a selectable runtime                                 |
| `examples/project.yaml`                                     | Add commented example of `agent.goose.recipes:` block                   |

## Risks & Mitigations

- **Goose release URLs are version-pinned.** Use `GOOSE_VERSION` —
  upstream's documented CI/CD-safe install path.
- **`goose acp` + recipes interaction is undocumented upstream.** See
  D7 open question. Implementation ships US-1/2/3 first (no recipes),
  then probes the right flag for US-4. If upstream doesn't support
  recipe-aware ACP yet, US-4 falls back to per-recipe agent_servers
  entries that use `goose run --recipe <file> --interactive` if that
  works over stdio, otherwise file an upstream issue.
- **Recipe paths could escape the repo** (e.g.
  `path: ../../etc/passwd`). The server-side path resolver must
  reject any `path` that doesn't `filepath.Clean` to a subdirectory of
  the recipe-repo checkout root (or primary repo, when `recipe_repo`
  is unset).
- **Recipe-repo auth.** Private `recipe_repo` URLs need credentials.
  v1 supports public URLs only; private-repo auth follows whatever
  mechanism the existing GitRepository flow uses for primary repos —
  no new auth surface in this task.
- **Recipe slug collisions** (two recipes named "Reviewer" produce the
  same slug). Reject duplicate names at YAML parse time with a clear
  error.
- **Frontend type churn.** Multiple files hard-code the runtime union
  as a string literal — implementer must grep for all of them.
