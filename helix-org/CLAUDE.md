# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Helix Org — a Go module that adds an AI organisation layer (Roles, Workers, Positions, Streams, Grants, Subscriptions, Events) to Helix, driven entirely through MCP and prompt-shaped Role markdown.

It started as a standalone prototype and now lives inside the `helix` monorepo as a sub-tree: production deployments mount it from `api/pkg/server/helix_org.go` behind `HELIX_ORG_ENABLED` + the per-user `alpha_features:['helix-org']` flag (see [helix PR #2286](https://github.com/helixml/helix/pull/2286)). It still builds as its own Go module with its own Makefile and `bin/helix-org` binary; that binary is now a **dev affordance** (local demos, tests, agent sandboxes) — not the production entry point.

When developing here:
- The parent `helix/CLAUDE.md` rules apply to this sub-tree too — same module, same repo. The redundant standalone-project framing this file used to carry has been removed.
- The redesign roadmap is in [`design/2026-05-21-redesign/`](design/2026-05-21-redesign/) (eight analysis docs + an integration reframe). Decisions are pinned in [`design/adr/`](design/adr/); start with ADR-0001 for terminology.
- The end state is dissolution into helix — see [`design/2026-05-21-redesign/09-integration-reframe.md`](design/2026-05-21-redesign/09-integration-reframe.md).

**Current year: 2026** — include "2026" in web searches for documentation and browser APIs.

## Design Philosophy (read this before writing code)

The ultimate goal is that the system is configured and used **almost entirely via the prompts of specific Roles and Positions**. Behaviour lives in the profile/prompt, not in the codebase. The code is scaffolding that lets this prompt-driven ecosystem thrive.

Practical consequences when choosing between alternatives:

- **Prefer data and text over code.** If a feature can be expressed as a profile edit, a Role-prompt constraint, or a tool description, do that before adding Go logic.
- **Keep the core generic.** Tool definitions and enforcement decisions are owned by individual tools — not hard-coded in the registry, server, or domain layer. New tools (including MCP tools later) must be addable without editing the core. The only authorisation primitive is `(WorkerID, ToolName)`; there is no per-grant scope field (see ADR-0001 §3).
- **Keep the MCP surface small.** MCP tools are reserved for org-graph primitives — both reads and mutations of the structural state (Workers, Positions, Roles, Grants, Streams, Subscriptions). Anything else a Worker needs to do should go through the shell tools provisioned in their Environment (`bash`, `curl`, `git`, `gh`, `python`, etc.). If you're tempted to add an MCP wrapper like `publish_to_blog` or `fetch_url`, stop — the Role text describes how to use the shell directly, and if the workflow changes, only the Role changes. The test: does this operation read or mutate org-graph state? If yes, MCP. If no, shell.
- **No workflow in code.** Tools do exactly one thing. The code does not orchestrate multi-step sequences on behalf of a Worker — it does not subscribe Workers to Streams, grant tools implicitly, auto-create related records, or otherwise chain steps together. Orchestration lives in the *prompt* of the Worker invoking the tool. A Role's typed `Tools` and `Streams` fields (the manifests of MCP tool names the Role's prompt expects and the Stream IDs it operates on — see `api/pkg/org/role`) are **reference data the hiring Worker's prompt reads**, not triggers the code acts on. `hire_worker` does NOT auto-grant the Role's `Tools`, does NOT auto-subscribe the new Worker to its `Streams` — the hiring caller is responsible for both. When writing or reviewing a tool, ask: "is the code making a decision that the Worker should be making?" If yes, remove it.
- **Write the smallest thing that works.** No speculative abstractions, no optional plumbing that isn't exercised today. If two tools share code, extract it then — not in advance.
- **Social enforcement first.** The default is that a Worker reads the constraints in their Role prompt and complies. Only reach for hard enforcement when the cost of a violation is high.

When a design choice looks like it could go either way, pick the one that pushes more responsibility into prompts/configuration and less into Go code.

## Architecture at a Glance

- **Library only — no binary.** The standalone `helix-org` CLI was deleted in H7. There is no `./bin/helix-org` to build, no `helix-org serve`, no `helix-org chat`. Production runs inside `helix api`, which mounts helix-org via `api/pkg/server/helix_org.go`. To exercise helix-org locally, run the helix api binary with `HELIX_ORG_ENABLED=true`.
- **Storage**: SQLite, driven by GORM with `AutoMigrate`. The file lives at `$FILESTORE_LOCALFS_PATH/helix-org/helix-org.db`, so the feature requires `FILESTORE_TYPE=fs` (gcs/s3 deployments skip the mount). No raw SQL migration files. The redesign target is to move persistence onto helix's Postgres-backed `store` — see migration H4 in [`design/2026-05-21-redesign/09-integration-reframe.md`](design/2026-05-21-redesign/09-integration-reframe.md).
- **Interface**: Every mutation of the org graph runs through MCP at `/workers/{id}/mcp` (Streamable HTTP), registered as a backend of helix's MCP gateway and reachable at `/api/v1/mcp/helix-org/workers/{id}/mcp` behind helix auth. The worker ID in the URL identifies the caller; the server exposes only the tools that worker holds grants for. The owner-facing UI at `/ui/` also reads and writes the store directly server-side (for things like settings); this is a deliberate operator-trusted bypass — see ADR-0010 (TBD) and `09-integration-reframe.md §3.04`.
- **Owner seeding**: On first start against an empty database, `initHelixOrgHandler` (`api/pkg/server/helix_org.go`) calls `bootstrap.Run` which creates the initial owner Worker (`w-owner`), Position (`p-root`), Role (`r-owner`), Environment, activation Stream, and the root grant set. After seeding, all org-graph mutations go through MCP.
- **Production runtime**: the embedded Helix agent backend, now at canonical location `api/pkg/org/runtime/helix/` (lifted across H1.0–H1.3d). Each Worker is provisioned with a per-Worker `helix.Project` + Agent App + git repo, and runs in a Zed sandbox with Claude Code authenticated via the operator's OAuth (no API key at rest). Per-Worker transcript subscription is in-process via pubsub (`SubscribeSessionUpdates`), no loopback WebSocket. The Helix runtime is the only runtime — the dev-only `agent/claude` subprocess Spawner and the dev-only owner-chat claude bridge in `server/chat/chat.go` were deleted in B9 (originally the third invocation site, `cmd/helix-org/chat.go`, was deleted in H7).
- **helixclient package** (`helix-org/helix/helixclient/`) is transitional. After H1.3d every runtime call goes through the canonical ports (`ProjectService`, `SpawnerClient`, `ProjectGit`); helixclient survives only as the HTTP+WS transport adapter satisfying those ports. H1.4 — delete the package outright — is blocked on H1.3c (replace the helixclient adapter with a direct controller adapter); see the deferred-work note at the top of `helix-org/helix/helixclient/client.go`.
- **Auth**: helix's `requireUser + requireFeature` middleware gates every surface. Real per-Worker authentication / multi-tenancy (one Org Graph per `helix.Organization`) is the H5–H6 migration; today every gated user shares one `w-owner` (PR #2286 OOS).

## Setup

Install required development tools before doing anything else:

```bash
make tools
```

## Build, Test, and Check

**Always prefer `make` targets over raw shell commands.** The Makefile sets required build tags, CGO flags, and an opinionated test invocation that ad-hoc `go test` / `golangci-lint` runs miss. Running raw commands silently drifts from how the project actually builds and runs.

If you find yourself reaching for a multi-step shell incantation to test, format, lint, clean, or seed local state — **add a `make` target for it instead**, then call that target. Future-you and other agents will reuse it; one-off shell strings rot. Keep targets discoverable via `make help`.

```bash
make test                        # Run all tests (race + -count=1)
make test PKG=./domain/...       # Test a specific package
make test-cover                  # Run tests + write coverage.out / coverage.html
make check                       # Format, vet, lint, and test (modifies files)
make ci                          # CI-safe: fmt-check, vet, lint, test (no writes)
make clean                       # Remove coverage and envs artefacts; nuke local *.db
```

`make check` is for local use — it runs `goimports -w` and may modify files. `make ci` runs `fmt-check` instead, failing if anything is unformatted without touching files. CI must use `make ci`; contributors must pass `make check` locally before pushing.

To run helix-org end-to-end, build and run the **helix api** binary with `HELIX_ORG_ENABLED=true` and grant the `helix-org` alpha feature flag to a user — see [PR #2286](https://github.com/helixml/helix/pull/2286) for the enable steps.

## Shipping Code

Before committing:

1. Run `make check` and confirm it passes (or at minimum `make test` for affected packages). Do not commit code that has not been validated.
2. Fix any failures before committing — do not skip or work around them.

Commits and PRs use **Conventional Commits**:

- Prefix: `feat:`, `fix:`, `docs:`, `refactor:`, `chore:`, `test:`, etc.
- Example commit: `feat: add webhook retry logic`
- PR titles follow the same format: `feat: add webhook retry logic`

When pushing additional commits to an existing PR, update the PR title and description to reflect the full set of changes in the branch.

## Go

- Fail fast: `return fmt.Errorf("failed: %w", err)` — never log and continue
- Error on missing configuration — fail with an error, don't log a warning and continue
- Use structs, not `map[string]interface{}`
- GORM AutoMigrate only — no SQL migration files
- Use gomock, not testify/mock
- No fallbacks — one approach, no fallback code paths
- No type aliases — update all references when moving or renaming types
- **NEVER re-export.** Don't write `type X = otherpkg.X` to surface a foreign type under a local name, and don't write `func F(...) { return otherpkg.F(...) }` to surface a foreign function. If callers want the type or function, they import it from its canonical home. Re-exports lie about ownership (the local name implies authorship), double the surface area to audit during renames, and tempt the next refactor to keep the alias "for backwards compat" until the codebase has two names for everything. Touched a foreign type? Import its package and qualify the reference. Used often enough that the qualifier hurts? That's a hint to lift it to a shared package, not to alias it.
- No panics — return errors; rewrite methods to support error returns if needed
- Log errors once at the top level — domain code returns errors, only handlers/workers log them

## Testing

When the user says "tdd", follow red-green strictly:

1. **Red**: Write a failing test. Run it, confirm it fails.
2. **Green**: Minimal fix. Run test, confirm it passes.
3. Run the full test suite for regressions.

### Characterisation tests before heavy lifts

For any refactor that moves substantial code between packages, splits a
file behind a new port, or renames a load-bearing interface — write
**characterisation tests first**. These pin the *current* behaviour of
the code, not its intended behaviour. The point is to detect drift
during the move, not to validate the design.

1. **First commit on the refactor branch is the tests.** Cover the
   public surface you intend to preserve. Run them, confirm green
   against the *unmoved* code. Commit before touching anything else.
2. **Rerun on every meaningful step of the lift.** Any new failure is
   a real regression — investigate, don't paper over with test edits.
3. **After the lift, the tests still pass without modification.** If
   they need to change, the lift was not behaviour-preserving and
   needs justification (or splitting into two PRs: behaviour-change
   first with new tests, then pure restructure).
4. **100% coverage is not the goal.** Cover the public surface, the
   named guarantees in code comments, and any invariants prior tests
   already pin. Leave internal helpers uncovered if they have no
   observable behaviour of their own.

This applies to every migration in `design/2026-05-21-redesign/08-migration-plan.md`
that moves code between contexts (B1, B2, B3, B4, B5, B6, B9 and the
helix dissolution moves H1–H8). Pure renames (like B0/ADR-0001) don't
need characterisation tests — existing tests pin behaviour adequately.

When in doubt: if the diff in this PR could plausibly change runtime
behaviour for any caller, write the characterisation tests first.

### Refactored files land in `api/pkg/org/` (canonical), not back in helix-org/

The helix-org redesign dissolves this sub-tree into helix
(see [`design/2026-05-21-redesign/09-integration-reframe.md`](design/2026-05-21-redesign/09-integration-reframe.md)).
Rather than refactoring code in place under `helix-org/` and then moving
it later as a separate symbolic step, **every refactor PR lifts its
target file(s) into their canonical home under `api/pkg/org/` directly**.

Rules for any file landing under `api/pkg/org/`:

1. **The location is a stamp of approval.** Anything under `api/pkg/org/`
   is canonical, reviewed, and behaviour-locked. Anything still under
   `helix-org/` is legacy / vibe-coded / slated for dissolution.
2. **Every file has associated high-level TDD unit tests.** These are
   e2e-shaped — drive meaningful behaviour through the public surface,
   not stub-heavy single-method unit tests. They are the
   characterisation tests written *first* (see previous section) and
   must remain green throughout and after the lift.
3. **No type aliases, no shim files.** Parent `helix/CLAUDE.md` forbids
   them; every lift updates all callers in the same PR. The old
   `helix-org/<path>` file is deleted, not left as a re-export.
4. **Imports flow downhill.** `helix-org/` may import from
   `api/pkg/org/`; `api/pkg/org/` must never import from `helix-org/`.
   A lift that would invert this direction is a sign more code needs
   to move at the same time — split the PR or expand it, don't invert
   the arrow.
5. **The eventual layout mirrors the bounded contexts** from
   [`design/2026-05-21-redesign/04-bounded-contexts.md`](design/2026-05-21-redesign/04-bounded-contexts.md) §1:
   `api/pkg/org/{transport,stream,worker,role,position,grant,activation,...}/`.
   New sub-directories are added by lifts, not invented up-front.

This collapses Tracks A and B of the migration plan into a single
track: every B-numbered refactor performs the corresponding move-into-
helix step at the same time. H8 (symbolic move) goes away — the move
*is* the refactor.

Practices:

- **Tests live next to the code**: `foo.go` → `foo_test.go` in the same directory. Use package `foo` for whitebox tests and `foo_test` only when the test must exercise the public API in isolation (e.g. to avoid import cycles).
- **Table-driven tests** for anything with multiple input cases. Use `t.Run(name, ...)` per case so failures name the case.
- **`t.Parallel()`** in tests that don't share global state.
- **Race detector is always on** (`make test` adds `-race`). Treat race failures as hard bugs, never flakes.
- **`-count=1`** is set so tests never use the build cache — all runs are fresh.
- **Mocks**: use `gomock` generated via `mockgen`. Place generated mocks in `mocks/<pkg>/` and regenerate them as part of `make tools` updates, not by hand. Prefer hand-rolled fakes where the interface is small enough that a mock adds no value.
- **Fixtures**: put reusable test data under `testdata/` (Go ignores it during builds).
- **Coverage**: run `make test-cover` before large PRs. Treat coverage as a diagnostic, not a gate — low coverage on a file means "go look," not "write busywork tests."
- **No `testing.Short()` skips by default**. If a test must be slow or external, gate it on an explicit build tag (e.g. `//go:build integration`) and document how to run it.

## Linting

`golangci-lint` config lives in `.golangci.yml`. The enabled linters (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`, `gofmt`, `goimports`, `misspell`, `revive`, `gosec`, `bodyclose`, `errorlint`, `nolintlint`) catch a deliberate, narrow set. Do not disable linters to silence findings — fix the code.

- **Fix the finding, don't suppress it.** `//nolint:<linter>` is only acceptable with a trailing comment explaining *why* the rule is wrong for this site (`nolintlint` enforces this). Unexplained `//nolint` directives fail the lint run.
- **Formatting is non-negotiable**: `goimports` with `-local github.com/helixml/helix-org` groups local imports separately. Run `make fmt` before committing; CI runs `make fmt-check` and will fail on any drift.
- **Error wrapping**: `errorlint` enforces `%w` instead of `%v` for errors, and forbids type-assertion on errors — use `errors.As` / `errors.Is`.
- **`gosec`** flags raw SQL string concatenation, weak crypto, and command injection. If one of these fires, the fix is almost always to restructure the code, not to suppress the warning.
- **New linters** are added by editing `.golangci.yml` in a dedicated `chore: enable <linter>` PR that also fixes all findings it surfaces — never in the same PR as unrelated changes.

## Software Engineering

Object-oriented design principles:

- **Naming**: Classes by what they are, not what they do (avoid -er suffixes). Methods are builders (noun) or manipulators (verb), rarely both. Variables should be explainable as single/plural nouns; prefer simple names over compound ones. **Scope**: this rule applies to new code in `api/pkg/org/` and to renames during refactors. Pre-existing `-er` names elsewhere in `helix-org/` and the parent helix repo (including legitimate Go-stdlib precedents like `io.Reader`/`io.Writer`/`time.Ticker`) stay until the surrounding code is touched for other reasons — don't open drive-by renames just to enforce the suffix rule.
- **Constructors**: One primary constructor, secondaries delegate to it. Keep constructors light. Prefer `new` only in secondary constructors.
- **Methods**: Prefer fewer than five public methods per class. Avoid static methods. Avoid null arguments and return values. Prefer richer encapsulation over getters/setters.
- **Encapsulation**: Prefer four or fewer encapsulated objects per class. Favour composition over inheritance.
- **Interfaces**: Prefer interfaces. Keep them small.
- **Immutability**: Default to immutable classes. Avoid type introspection and reflection unless the language idiom demands it.
- **No globals**: Prefer classes over public constants or enums.
- **Testing**: Prefer fakes over mocks.
- **Design**: Think in objects, not algorithms. Tell objects what you want; don't ask for data.
- **Boolean parameters**: Don't use a boolean to switch between fundamentally different behaviours (split the method or use polymorphism). Booleans are fine for orthogonal modifiers like filters or formatting options.
- **No discriminator switches with branching logic.** When a function does `switch x.Kind { case A: ...; case B: ...; }` and each case is variant-specific behaviour (validation, parsing, side-effects, multi-line bodies), the Kind should be polymorphic. Each variant owns its own logic; dispatch through an interface or a `Kind → Strategy` lookup populated at package init. Adding a new variant must not require editing the dispatch site (Open/Closed). This is the **Boolean parameters** rule generalised from `bool` to any enum. `switch` is still fine for *flat lookups* (return a constant per case, single-line bodies) — the smell is variant-specific *behaviour* in each arm. If you find yourself drafting a multi-line `case`, stop and ask: should this `case` be a method on a Strategy instead?
- **One file per variant for polymorphic Kinds.** When a Kind has its own behaviour (per the previous bullet), the Config type, Strategy implementation, validation rules, and per-Kind helpers all live together in one file named after the Kind (e.g. `webhook.go`, `email.go`, `github.go`), not scattered across the umbrella file. The umbrella file (typically named after the package) owns the `Kind` enum, the interfaces, the strategies map, and the umbrella `Transport`-style struct that delegates through them.
- **Dependency injection**: Use constructor injection. When a constructor accumulates many parameters, group related ones into a parameter/options object.
- **Language idioms**: Where these principles conflict with strong language conventions (e.g. Go exported struct fields), follow the language idiom and note the deviation.
