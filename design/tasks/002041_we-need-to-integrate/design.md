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

**URL, not ID — by design.** `recipe_repo_url` is the upstream URL
(GitHub/GitLab/etc.), not Helix's internal `GitRepository.ID`.
Internal IDs are per-Helix-instance UUIDs and would not match across
deployments — using them would break YAML portability when a project
spec is moved between Helix installs. URL-keying mirrors how
`ProjectSpec.Repositories` already references repos in the existing
`repositories:` block; we're not inventing a new identifier scheme.

**Validation rule** in `applyProject`: if `recipe_repo_url` is set,
look it up via `GetGitRepositoryByExternalURL(orgID, url)`. If it's
not found, or it's not attached to the project / accessible to the
project's org, return a 400 with the exact attach instructions.
Normalise both sides (trim trailing `/` and `.git`) before comparing
to match how Helix already dedups attached repos.

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

**How `settings-sync-daemon` builds the agent_servers map** (the
config-baking approach — see D7c for why):

For each recipe attached to the spec task (or registered on the
project, for ad-hoc Zed sessions), emit:

```jsonc
"<slug>": {
  "name": "<recipe.name>",
  "type": "custom",
  "command": "goose",
  "args": ["acp"],
  "env": {
    /* same provider/model/base-url env as plain goose */
    "GOOSE_CONFIG_PATH": "/home/retro/.config/goose/<slug>.yaml",
    "GOOSE_RECIPE_PATH": "<recipe-repo-checkout-dir>"
  }
}
```

The recipe-specific `<slug>.yaml` is also written by the daemon. It
contains:
- The recipe's `instructions:` baked into goose's system prompt
  (parameter-substituted server-side — see D7c)
- The recipe's `extensions:` list registered as goose extensions
- The recipe's `settings:` overlaying the user's provider/model
- A single-entry `slash_commands:` map so the recipe can be re-invoked
  mid-session via `/recipe-name` (Goose's slash-command syntax also
  accepts inline overrides: `/recipe-name arg=value`)

`<absolute-path-in-container>` (used when the slash command resolves
the recipe) is `{recipe_repo_url → GitRepository → LocalPath} / recipes[i].path`,
resolved server-side by the Helix API. When `recipe_repo_url` is
unset, the resolver falls back to the project's primary repo's
checkout. Either way, by the time the daemon writes Zed settings the
directory exists locally — Helix's existing project-repo sync has
already placed it on disk.

`GOOSE_RECIPE_PATH` is set so subrecipes / prompt fragments referenced
by short name still resolve.

### D7c: Recipe parameters come from spec-task creation, not Zed UI

Zed inside Helix is the *execution surface* for spec tasks; spec-task
creation is the *configuration surface*. Recipe parameters belong at
configuration time, not execution time — prompting users mid-Zed-session
would defeat the point of an automated spec task.

**Flow**:

1. **Spec-task creation (API + UI)**:
   - User picks a recipe from the project's declared list. The
     creation page fetches the recipe YAML via a new endpoint
     `GET /api/v1/projects/{id}/goose/recipes/{name}/schema` that
     parses the recipe's `parameters:` block and returns the JSON
     schema needed to render a form.
   - Form dynamically renders one input per parameter (mapping in
     US-5). `file:` parameters expect a **repo-relative path** to a
     file inside the spec task's primary repo.
   - On submit, the spec task persists `recipe_name string` and
     `recipe_params map[string]string` on the task row.

2. **Spec-task start (Helix API)**:
   - Read the recipe YAML from the GitRepository mirror.
   - For each parameter:
     - `string`/`number`/`boolean`/`date`/`select` → substitute the
       value as-is.
     - `file` → join the value with the primary repo's checkout root,
       reject paths that escape via `filepath.Clean` containment,
       read the file, substitute the *contents* (matches Goose's CLI
       semantics).
   - Run Jinja-style `{{ var }}` substitution on `instructions:`,
     `prompt:`, and `activities:`. Use a simple regex substitution
     (Goose recipes only use `{{ name }}` — no Jinja control flow).
   - Pack the substituted recipe (system prompt + extensions +
     settings) into `CodeAgentConfig.GooseBakedRecipe`.

3. **Container start (settings-sync-daemon)**:
   - Receives the baked recipe via the existing WebSocket config
     push. No recipe YAML parsing in the daemon.
   - Writes `~/.config/goose/<slug>.yaml` with the baked content.
   - Emits the `agent_servers.<slug>` entry pointing at it.

**Why server-side substitution (vs daemon-side)**:
- The API container already has the GitRepository mirror; the desktop
  container would have to wait for the project-repo sync to finish
  before parsing.
- Keeps the daemon repo-agnostic.
- Parameter values are part of the spec task — substitution at the API
  layer keeps the task spec self-describing and reproducible.

**Ad-hoc Zed sessions (no spec task)**:
- The plain "Goose" agent_servers entry is still emitted with the
  project's recipes registered as slash commands (Phase 2a's
  original UX).
- Users type `/recipe-name arg=value` inline. Goose's CLI parameter
  syntax handles ad-hoc parameterisation natively.
- No Helix-side parameter form. This path is for exploratory use, not
  automated tasks.

**Upstream validation (2026-05-21)** — checked goose CLI source,
release tags, and merge metadata via GitHub API. Confirmed state:

| Capability | `goose acp` accepts? | Where it lives |
|---|---|---|
| `--with-builtin <name>` (built-in extensions) | ✅ Yes | Stable, all releases |
| `--recipe <file>` | ❌ No flag exists on `Acp` subcommand | — |
| Slash-command-driven recipe execution inside an ACP session | ✅ Yes — **but only on `main`** | PR [#8925](https://github.com/aaif-goose/goose/pull/8925), merged 2026-05-12 |
| First-class recipe at session creation (`NewSessionRequest.recipe`) | ❌ Not yet | Issue [#7596](https://github.com/aaif-goose/goose/issues/7596), assigned, snoozed to 2026-05-28 |

**Release state** (verified via GH API + raw file inspection):
- Latest stable: **v1.34.1** (2026-05-15)
- v1.34.0 (2026-05-13) was tagged ~22h after PR #8925 merged, but its
  release branch was cut from a commit upstream of the merge — v1.34.0
  and v1.34.1 are both **missing PR #8925**. Confirmed by inspecting
  `crates/goose/src/acp/server.rs` at `v1.34.1`: no
  `AvailableCommand`, no `available_commands_update`, no
  "Running recipe" message string. On `main`, all of those are
  present (lines ~1195, 2647, 3046, 3060 of the same file).

**This changes the gate**: PR #8925 is sitting in `main` today, ready
to use. We control the desktop image. The Helix codebase already
pins-and-builds upstream Rust projects from specific commits (see
`sandbox-versions.txt` with `ZED_COMMIT=<sha>`, `QWEN_COMMIT=<sha>`,
per `helix/CLAUDE.md`). Adding `GOOSE_COMMIT=<sha>` follows that
established pattern.

**Decision**: pin a `GOOSE_COMMIT` to a recent `main` SHA and build
goose into the image from source. Ship Phase 2 (slash-command UX)
without waiting. When #7596 lands and adds per-thread recipe support,
bump the pin and switch to per-recipe `agent_servers` entries.

**Two-phase Phase 2 plan**:

1. **Phase 2a (today, ships with `GOOSE_COMMIT` pinned to a `main`
   SHA that includes #8925)**:
   - One `agent_servers.goose` entry per project (not per recipe).
   - settings-sync-daemon writes a goose global config that registers
     each project recipe as a slash command (the
     `slash_commands` map in goose's config). Recipes still come from
     the Helix-mirrored `GitRepository` checkout — only the
     advertise-as-slash-command wiring is new.
   - In Zed, the user opens the single "Goose" thread, types `/`, and
     gets autocomplete of the project's recipes. Invoking
     `/security-reviewer` runs that recipe in the current session.
   - Limitations: not "first-class agent per recipe" in the agent
     panel; parameter prompts surface inline in chat rather than via
     a Zed dialog. Acceptable for v1 — the workflow is fully usable.

2. **Phase 2b (once #7596 ships in `main`)**:
   - Bump `GOOSE_COMMIT`.
   - Switch the daemon to emitting one `agent_servers.<slug>` per
     recipe with `args: ["acp", "--recipe", "<abs-path>"]` (or
     whatever flag upstream ends up shipping).
   - Project YAML schema and the UI picker built in Phase 2a do not
     change — only the daemon's emit-side does.

**Build path for `GOOSE_COMMIT`**: Goose's upstream CI produces
prebuilt Linux binaries on release tags, but probably not on every
`main` commit. Two install strategies — pick at implementation time:
- **A.** Use the `download_cli.sh` script with a pinned version pointer
  if Goose publishes nightly/canary artifacts (check
  `https://github.com/aaif-goose/goose/releases/tag/canary` first).
- **B.** Clone the goose repo at `$GOOSE_COMMIT` and `cargo build
  --release -p goose-cli` in a build stage of
  `Dockerfile.ubuntu-helix`. Same pattern as the Zed build stage
  already in that file.

Option B is more work but is the proven pattern; Option A only works
if a usable rolling artifact exists.

The plain `goose` entry (no recipe) is always emitted alongside the
slash-command-enabled entry, so users keep access to a vanilla Goose
session regardless of recipe state.

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

**Spec-task model** (`api/pkg/types/spec_task.go` or wherever spec
tasks live) gains:

```go
type SpecTask struct {
    // ...existing fields...
    GooseRecipeName   string            `json:"goose_recipe_name,omitempty"`
    GooseRecipeParams map[string]string `json:"goose_recipe_params,omitempty"`
}
```

`CodeAgentConfig` (sent to settings-sync-daemon) gains:

```go
GooseRecipes       []CodeAgentGooseRecipe `json:"goose_recipes,omitempty"`
GooseRecipeRootDir string                 `json:"goose_recipe_root_dir,omitempty"` // absolute path in container
GooseBakedRecipe   *CodeAgentBakedRecipe  `json:"goose_baked_recipe,omitempty"`    // populated for spec-task sessions
```

with:
- `CodeAgentGooseRecipe { Name, AbsolutePath string }` — used for
  slash-command registration on the plain "Goose" entry.
- `CodeAgentBakedRecipe { Name, Slug, SystemPrompt string,
  Extensions []GooseExtension, Settings GooseSettings }` — the
  parameter-substituted recipe content the daemon writes into
  `~/.config/goose/<slug>.yaml`.

The API server (a) reads `LocalPath` from the resolved
`GitRepository`, (b) for spec-task sessions, reads + substitutes the
chosen recipe, and (c) ships both the per-recipe absolute paths and
the baked content to the daemon. The daemon stays repo-agnostic.

## Files Touched

| File                                                        | Change                                                                  |
|-------------------------------------------------------------|-------------------------------------------------------------------------|
| `Dockerfile.ubuntu-helix`                                   | Build goose from `$GOOSE_COMMIT` in a dedicated build stage; copy binary into runtime image; telemetry-off config |
| `sandbox-versions.txt`                                      | Add `GOOSE_COMMIT=<sha>` pin (same pattern as `ZED_COMMIT`, `QWEN_COMMIT`) |
| `api/pkg/types/task_management.go`                          | Add `CodeAgentRuntimeGooseCode = "goose_code"`                          |
| `api/pkg/types/project.go`                                  | Add `Goose *ProjectAgentGoose` + nested types                           |
| `api/pkg/types/types.go`                                    | Extend `CodeAgentConfig` with `GooseRecipes`, `GooseRecipeRootDir`      |
| `api/pkg/server/project_handlers.go` (`applyProject`)       | Validate `recipe_repo_url` resolves to an attached `GitRepository`      |
| `api/pkg/external-agent/zed_config.go` (`buildCodeAgentConfig`) | Resolve `recipe_repo_url` → `GitRepository.LocalPath` → absolute paths |
| `api/cmd/settings-sync-daemon/main.go`                      | New `case "goose_code":` emitting `agent_servers.goose` + per-recipe    |
| `frontend/src/types.ts`, `api/api.ts`, `contexts/apps.tsx`  | Add `'goose_code'` to runtime union + display name                      |
| `frontend/src/pages/Onboarding.tsx`, `ProjectSettings.tsx`  | Surface "Goose" as a selectable runtime                                 |
| `frontend/src/pages/ProjectSettings.tsx` (new section)      | "Goose recipes" card: repo picker + recipe list + reuses `LinkExternalRepositoryDialog` |
| Spec-task model + creation handler                          | Persist `goose_recipe_name` + `goose_recipe_params`; on task start, parse recipe YAML + substitute params + populate `CodeAgentConfig.GooseBakedRecipe` |
| New API endpoint `GET /api/v1/projects/{id}/goose/recipes/{name}/schema` | Returns the recipe's `parameters:` block as a form schema for the spec-task creation page |
| Spec-task creation page (frontend)                          | Recipe picker (visible when project runtime is `goose_code`) + dynamic parameter form generated from the schema endpoint |
| `examples/project.yaml`                                     | Add commented example of `agent.goose.recipe_repo_url` + `recipes`      |

## Risks & Mitigations

- **Goose release URLs are version-pinned.** Use `GOOSE_VERSION` —
  upstream's documented CI/CD-safe install path.
- **PR #8925 (slash-command discovery in ACP) is in `main` but not
  v1.34.1 — validated by inspecting `acp/server.rs` at the tag.**
  Phase 2a requires building goose from a pinned `main` commit
  (`GOOSE_COMMIT=<sha>`, same pattern as `ZED_COMMIT` and
  `QWEN_COMMIT` in `sandbox-versions.txt`). When goose cuts the next
  stable release that includes #8925, switch to the released binary
  via `download_cli.sh` and drop the source build.
- **Phase 2b depends on upstream #7596** (per-thread recipe at
  session creation). If that slips past Q3 2026, Phase 2a is good
  enough — the slash-command UX is fully functional, just less
  discoverable than per-recipe agent entries in Zed's panel.
- **Source-built goose adds build time.** Mitigations: a dedicated
  build stage in `Dockerfile.ubuntu-helix` (same as Zed's stage) with
  cargo's incremental cache so only `goose-cli` and its direct
  dependencies rebuild on commit bump. Expect 5–15 min on cache miss,
  seconds on cache hit.
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

## Implementation Notes (Phase 1)

### Goose CLI build

- Pinned `GOOSE_COMMIT=ca26f01d3acd9871691fa8981f05d19aed9a3b82` (current `main` HEAD as of 2026-05-21). Verified the SHA contains PR #8925 by grepping `crates/goose/src/acp/server.rs` — 11 matches for `AvailableCommand|available_commands_update|Running recipe`.
- Goose's `rust-toolchain.toml` pins to **Rust 1.92**. Helix's existing `rust-build-env` stage uses 1.87, so the goose build needs its own stage. Bumping `rust-build-env` to 1.92 risks breaking the GStreamer plugin builds — not worth the blast radius.
- Build deps from upstream's CI (`.github/workflows/build-cli.yml`, standard variant): `build-essential pkg-config libssl-dev libdbus-1-dev libxcb1-dev`. No vulkan/musl-specific deps needed.
- Default features (`code-mode local-inference aws-providers telemetry otel rustls-tls system-keyring`) are kept — matches what users get from the upstream stable release. Telemetry is disabled at runtime, not via feature flag.
- `cargo build` uses BuildKit cache mounts (`/root/.cargo/registry`, `/root/.cargo/git`, `/build/target`) so re-builds at the same commit are seconds, and cache survives across commit bumps.

### Telemetry / auto-update

- Goose has **no `GOOSE_TELEMETRY_ENABLED` env var** (verified by searching the env-vars docs page). The `telemetry`/`otel` cargo features wire up OpenTelemetry exporters at compile time. The standard kill switch is the OTel SDK env var `OTEL_SDK_DISABLED=true`, added to the global `ENV` block alongside the existing telemetry kill switches.
- Goose has no auto-updater. `goose update` is a user-invoked command; the binary is pinned in the image, so even a user-triggered update would be overwritten on the next image build.
- Did NOT write a `~/.config/goose/profiles.yaml` or similar — the global config file is `~/.config/goose/config.yaml`, and Phase 2 will write per-session content there. Adding a baked-in stub now would just get overwritten by the settings-sync-daemon at session start.

### Implementation note: `./stack update_openapi` regen drift was kept

Running `./stack update_openapi` to pick up the new `CodeAgentRuntimeGooseCode` enum value also pulled in **unrelated** schema drift — upstream Go modules (notably `github.com/mark3labs/mcp-go`) had added new types and renamed some symbols since the last regen (`McpTool` → `GithubComMark3LabsMcpGoMcpTool`). This broke `frontend/src/components/app/AddMcpSkillDialog.tsx` with `Module '"../../api/api"' has no exported member 'McpTool'`.

Per project convention ("the OpenAPI spec gets updated by randomly regenerating it whenever we need to"), the drift is **kept** rather than reverted. `AddMcpSkillDialog.tsx` was updated to import the renamed type via an alias:

```ts
import { GithubComMark3LabsMcpGoMcpTool as McpTool, … } from '../../api/api';
```

Files regenerated: `frontend/src/api/api.ts`, `frontend/swagger/swagger.yaml`, `swagger.json`, `openapi.json`, `api/pkg/server/swagger.json|yaml|docs.go`. Committed separately as `chore(api): regen openapi (drift from upstream module updates)` so the goose change and the drift are easy to tell apart in review.
