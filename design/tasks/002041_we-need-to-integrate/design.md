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
User attaches recipe repo  ──►  Helix GitRepository row + mirror clone
  (UI: LinkExternalRepositoryDialog,        at /filestore/git-repositories/{repoID}
   or YAML: repositories: [...])

Helix project YAML / ProjectSettings UI (agent.goose.recipe_repo_url + recipes)
        │
        │  on session start
        ▼
helix-api ──► ProjectAgentSpec.Goose ──► CodeAgentConfig (extended)
        │       └─► resolves recipe_repo_url → GitRepository → mirror path
        │  WebSocket
        ▼
settings-sync-daemon (in helix-ubuntu container)
        │
        │  writes
        ▼
~/.config/zed/settings.json:
  agent_servers:
    goose:                 { command: "goose", args: ["acp"], env: {...} }
    security-reviewer:     { command: "goose", args: ["run","--recipe",<abs>,"--interactive"], env: {...}}
    migration-bot:         { command: "goose", args: ["run","--recipe",<abs>,"--interactive"], env: {...}}
        │
        ▼
Zed agent panel ──spawns──► goose ──ACP/stdio──► Goose
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

### D7: Recipe repos are Helix-attached `GitRepository` rows, not raw URLs

The user clarified: "In Helix the way that repo auth works is that we
clone and mirror the repo from upstream into Helix. I'd rather point
the recipe repo at the internal Helix repo and tell users to attach
their goose repos to Helix and then select them somehow."

**Decision**: A goose-recipe repo is just another repository attached
via the existing `GitRepository` flow. Helix mirrors it into
`/filestore/git-repositories/{repoID}` once per org. Projects
reference it by its upstream URL (the same key Helix already uses to
de-dupe attached repos in `applyProject`). The project YAML grows two
hooks: a `recipe_repo_url:` selector on the `agent.goose` block, and
a constraint that the URL must already be attached.

**Why this is better than a raw `recipe_repo: <url>` field**:
- One repo model, one auth surface, one mirror. The previous draft's
  `recipe_repo:` would have introduced a parallel clone path that
  bypassed `GitRepository` — duplicating auth (private repos), proxy,
  TLS, and lifecycle logic.
- Private recipe repos "just work" — they reuse whatever
  `GitProviderConnection` / OAuth / PAT the user already set up to
  attach the repo. No new credential prompts.
- Recipe repos can be shared across projects in the same org: attach
  once, reference from many projects.
- Goose itself never sees the upstream URL — it just sees a local
  directory pointed to by `GOOSE_RECIPE_PATH`. Goose's
  `GOOSE_RECIPE_GITHUB_REPO` env var (and its `gh`-CLI dependency) is
  not used. Host-agnostic by construction.

**Project YAML extension** — new optional `goose:` block under `agent:`:

```yaml
repositories:
  - url: https://github.com/my-org/my-codebase
    primary: true
  - url: https://github.com/my-org/goose-recipes     # standard attach

agent:
  runtime: goose_code
  provider: anthropic
  model: claude-opus-4-7
  goose:
    # Optional. Must match the `url` of a repo in `repositories:` (or
    # an org-scoped attached repo). Omit to load recipes from the
    # primary repo.
    recipe_repo_url: https://github.com/my-org/goose-recipes
    recipes:
      - name: "Security Reviewer"
        path: security-reviewer.yaml      # relative to recipe-repo root
      - name: "Migration Bot"
        path: workflows/migration-bot.yaml
```

**Validation rule** in `applyProject`: if `recipe_repo_url` is set,
look it up via `GetGitRepositoryByExternalURL(orgID, url)`. If it's
not found, or it's not attached to the project / accessible to the
project's org, return a 400 with the exact attach instructions.

### D7b: UI for selecting / attaching a recipe repo

YAML is the source of truth, but most users will configure this from
the UI. Add a "Goose recipes" section to `frontend/src/pages/ProjectSettings.tsx`:

1. **Recipe-repo picker** — a dropdown of `GitRepository` rows
   currently attached to the project (and a separator listing
   org-scoped attached repos not yet attached to this project, with
   an "attach to this project" action). Backed by the existing
   `GET /api/v1/projects/{id}/repositories` and
   `GET /api/v1/git-repositories?organization_id=` endpoints — no
   new list endpoints needed.

2. **"Attach a recipe repo" button** — opens the existing
   `LinkExternalRepositoryDialog` (`frontend/src/components/project/LinkExternalRepositoryDialog.tsx`),
   the same dialog used for code repos. On save, the new repo is
   attached and pre-selected in the picker. No code duplication.

3. **Recipe list** — for each recipe entry, two fields (`name`,
   `path`) with add / remove buttons. Path autocomplete (querying the
   mirror's `git ls-tree` for `*.yaml`/`*.yml`) is nice-to-have, not
   required for v1.

4. **Save** — writes back to the same `agent.goose` config the YAML
   path writes to. Form ↔ YAML round-trip is lossless.

Visual placement: a new `<Card>` under the existing "Code Agent
Runtime" card, only shown when runtime is `goose_code`. Mirrors how
Claude-Code-specific settings (subscription mode, etc.) are already
conditionally rendered today.

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

`<absolute-path-in-container>` is the resolved recipe path inside the
recipe-repo checkout. The Helix API resolves it server-side from
`{recipe_repo_url → GitRepository → LocalPath} / recipes[i].path` and
sends both the per-recipe absolute path and the root dir to the
daemon. The daemon stays repo-agnostic.

When `recipe_repo_url` is unset, the resolver falls back to the
project's primary repo's checkout. Either way, by the time the daemon
writes Zed settings the directory exists locally — Helix's existing
project-repo sync has already placed it on disk.

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
    // Upstream URL of an attached GitRepository. Must match an entry
    // in ProjectSpec.Repositories (or an org-scoped attached repo).
    // Omit to load recipes from the primary repo.
    RecipeRepoURL string                    `json:"recipe_repo_url,omitempty" yaml:"recipe_repo_url,omitempty"`
    Recipes       []ProjectAgentGooseRecipe `json:"recipes,omitempty" yaml:"recipes,omitempty"`
}

type ProjectAgentGooseRecipe struct {
    Name string `json:"name" yaml:"name"`
    Path string `json:"path" yaml:"path"`  // relative to recipe-repo root (or primary repo root when RecipeRepoURL is empty)
}
```

`applyProject` resolves `RecipeRepoURL` via
`GetGitRepositoryByExternalURL(orgID, url)`. If the lookup fails, the
request is rejected with a 400 telling the user to attach the repo
first (via the Git Repositories page or `repositories:` block).

`CodeAgentConfig` (sent to settings-sync-daemon) gains:

```go
GooseRecipes       []CodeAgentGooseRecipe `json:"goose_recipes,omitempty"`
GooseRecipeRootDir string                 `json:"goose_recipe_root_dir,omitempty"` // absolute path in container
```

with `CodeAgentGooseRecipe { Name, AbsolutePath string }`. The API
server reads `LocalPath` from the resolved `GitRepository`, joins it
with each recipe `Path`, and ships both the per-recipe absolute paths
and the checkout root to the daemon. The daemon stays repo-agnostic
and Goose-side env vars (`GOOSE_RECIPE_PATH`) just point at a local
directory.

## Files Touched

| File                                                        | Change                                                                  |
|-------------------------------------------------------------|-------------------------------------------------------------------------|
| `Dockerfile.ubuntu-helix`                                   | Install pinned Goose CLI; telemetry-off config                          |
| `api/pkg/types/task_management.go`                          | Add `CodeAgentRuntimeGooseCode = "goose_code"`                          |
| `api/pkg/types/project.go`                                  | Add `Goose *ProjectAgentGoose` + nested types                           |
| `api/pkg/types/types.go`                                    | Extend `CodeAgentConfig` with `GooseRecipes`, `GooseRecipeRootDir`      |
| `api/pkg/server/project_handlers.go` (`applyProject`)       | Validate `recipe_repo_url` resolves to an attached `GitRepository`      |
| `api/pkg/external-agent/zed_config.go` (`buildCodeAgentConfig`) | Resolve `recipe_repo_url` → `GitRepository.LocalPath` → absolute paths |
| `api/cmd/settings-sync-daemon/main.go`                      | New `case "goose_code":` emitting `agent_servers.goose` + per-recipe    |
| `frontend/src/types.ts`, `api/api.ts`, `contexts/apps.tsx`  | Add `'goose_code'` to runtime union + display name                      |
| `frontend/src/pages/Onboarding.tsx`, `ProjectSettings.tsx`  | Surface "Goose" as a selectable runtime                                 |
| `frontend/src/pages/ProjectSettings.tsx` (new section)      | "Goose recipes" card: repo picker + recipe list + reuses `LinkExternalRepositoryDialog` |
| `examples/project.yaml`                                     | Add commented example of `agent.goose.recipe_repo_url` + `recipes`      |

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
  the resolved checkout root.
- **Unattached `recipe_repo_url`.** YAML referencing a URL that isn't
  attached must fail fast with an instructive error, not silently
  ignore the recipes block. Validation lives in `applyProject`.
- **Recipe-repo auth.** Authentication for private recipe repos is
  handled at attach time via the existing `GitProviderConnection` flow
  (the same one that handles private code repos). This task does not
  introduce a new credential surface.
- **Mirror freshness.** Helix's existing project-repo sync determines
  how often the recipe-repo mirror is updated. If users edit recipes
  via the upstream host (e.g. GitHub web UI) and expect Helix to pick
  them up, they're subject to the same sync cadence as any other
  repo — call this out in user docs.
- **Recipe slug collisions** (two recipes named "Reviewer" produce the
  same slug). Reject duplicate names at YAML parse time with a clear
  error.
- **Frontend type churn.** Multiple files hard-code the runtime union
  as a string literal — implementer must grep for all of them.
