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

### D7: Custom Goose agents are project-scoped recipes from the git repo

The user asked: "How can advanced users import customised agents into
Helix? Slot it into the project YAML? Read from a git repo?"

**Decision**: Recipes are YAML files committed to the project's primary
git repo (e.g. under `goose-recipes/`). The project YAML lists them by
path. The git repo is *already* cloned into the container at session
start (Helix's existing project-repos feature), so no extra clone step.

**Project YAML extension** — new optional `goose:` block under `agent:`:

```yaml
agent:
  runtime: goose_code
  provider: anthropic
  model: claude-opus-4-7
  goose:
    recipes:
      - name: "Security Reviewer"
        path: goose-recipes/security-reviewer.yaml
      - name: "Migration Bot"
        path: goose-recipes/migration-bot.yaml
    github_recipe_repo: my-org/shared-goose-recipes   # optional
```

**Why this shape**:
- Mirrors how `repositories:` already works in `ProjectSpec` (path-based
  references inside a known repo).
- Recipes versioned with the code they operate on — a recipe and its
  matching codebase evolve together.
- Zero new infrastructure: no recipe store, no upload UI, no separate
  sync mechanism. Git is the source of truth.
- `github_recipe_repo` (optional) maps directly to Goose's existing
  `GOOSE_RECIPE_GITHUB_REPO` env var — for orgs that want a single
  shared recipe library across projects.

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
  "env": { /* same provider/model/base-url env as plain goose */ }
}
```

`<absolute-path-in-container>` is `/home/retro/work/<repo>/<recipe.path>`.
The repo is already cloned by Helix's project-repo flow.

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
    Recipes          []ProjectAgentGooseRecipe `json:"recipes,omitempty" yaml:"recipes,omitempty"`
    GithubRecipeRepo string                    `json:"github_recipe_repo,omitempty" yaml:"github_recipe_repo,omitempty"`
}

type ProjectAgentGooseRecipe struct {
    Name string `json:"name" yaml:"name"`
    Path string `json:"path" yaml:"path"`  // relative to primary repo root
}
```

`CodeAgentConfig` (sent to settings-sync-daemon) gains:

```go
GooseRecipes          []CodeAgentGooseRecipe `json:"goose_recipes,omitempty"`
GooseGithubRecipeRepo string                 `json:"goose_github_recipe_repo,omitempty"`
```

with `CodeAgentGooseRecipe { Name, AbsolutePath string }` — the API
server resolves `Path` against the primary repo's checkout dir before
sending, so the daemon doesn't need repo knowledge.

## Files Touched

| File                                                        | Change                                                                  |
|-------------------------------------------------------------|-------------------------------------------------------------------------|
| `Dockerfile.ubuntu-helix`                                   | Install pinned Goose CLI; telemetry-off config                          |
| `api/pkg/types/task_management.go`                          | Add `CodeAgentRuntimeGooseCode = "goose_code"`                          |
| `api/pkg/types/project.go`                                  | Add `Goose *ProjectAgentGoose` + nested types                           |
| `api/pkg/types/types.go`                                    | Extend `CodeAgentConfig` with `GooseRecipes`, `GooseGithubRecipeRepo`   |
| `api/pkg/server/project_handlers.go` (`applyProject`)       | Persist new YAML fields to DB Project row                               |
| `api/pkg/external-agent/zed_config.go` (`buildCodeAgentConfig`) | Resolve recipe paths against primary repo checkout                    |
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
  the primary repo's checkout root.
- **Recipe slug collisions** (two recipes named "Reviewer" produce the
  same slug). Reject duplicate names at YAML parse time with a clear
  error.
- **Frontend type churn.** Multiple files hard-code the runtime union
  as a string literal — implementer must grep for all of them.
